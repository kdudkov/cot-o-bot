package main

import (
	"encoding/xml"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/kdudkov/goatak/cotproto"

	tg "github.com/go-telegram-bot-api/telegram-bot-api/v5"
	"github.com/kdudkov/goatak/cot"
	"github.com/spf13/viper"
	"go.uber.org/zap"
)

var (
	gitRevision string
	gitBranch   string
)

var (
	colors = []string{
		"White",
		"Yellow",
		"Orange",
		"Magenta",
		"Red",
		"Maroon",
		"Purple",
		"Dark Blue",
		"Blue",
		"Cyan",
		"Teal",
		"Green",
		"Dark Green",
		"Brown",
	}

	roles = []string{"Team Member", "HQ", "Team Lead", "K9", "Forward Observer", "Sniper", "Medic", "RTO"}
)

type Cb func(cq *tg.CallbackQuery, user *UserInfo, data string) (tg.Chattable, error)

type Command struct {
	key  string
	desc string
	cb   func(update tg.Update, user *UserInfo) (tg.Chattable, error)
}

type App struct {
	bot       *tg.BotAPI
	logger    *zap.SugaredLogger
	users     *UserManager
	commands  map[string]*Command
	callbacks map[string]Cb
}

func NewApp(logger *zap.SugaredLogger) (app *App) {
	app = new(App)
	app.commands = make(map[string]*Command)
	app.logger = logger
	app.users = NewUserManager(logger, "users.yml")
	app.callbacks = map[string]Cb{
		"team": app.callbackTeam,
		"role": app.callbackRole,
	}
	return
}

func (app *App) GetUpdatesChannel() (tg.UpdatesChannel, error) {
	if webhook := viper.GetString("webhook.ext"); webhook != "" {
		app.logger.Infof("start listener on %s, path %s", viper.GetString("webhook.listen"), viper.GetString("webhook.path"))
		go func() {
			if err := http.ListenAndServe(viper.GetString("webhook.listen"), nil); err != nil {
				panic(err)
			}
		}()

		app.logger.Infof("starting webhook %s", webhook)

		wh, _ := tg.NewWebhook(webhook)
		if _, err := app.bot.Request(wh); err != nil {
			return nil, err
		}

		info, err := app.bot.GetWebhookInfo()
		if err != nil {
			return nil, err
		}

		app.logger.Infof("%s %s", info.LastErrorDate, info.LastErrorMessage)

		//if info.LastErrorDate != 0 {
		//	app.logger.Errorf("Telegram callback failed: %s", info.LastErrorMessage)
		//	return nil, fmt.Errorf(info.LastErrorMessage)
		//}

		return app.bot.ListenForWebhook(viper.GetString("webhook.path")), nil
	}

	app.logger.Info("start polling")
	app.removeWebhook()
	u := tg.NewUpdate(0)
	u.Timeout = 60

	return app.bot.GetUpdatesChan(u), nil
}

func (app *App) removeWebhook() {
	if _, err := app.bot.Request(tg.WebhookConfig{URL: nil}); err != nil {
		app.logger.Errorf("remove webhook error: %v", err)
	}
}

func (app *App) quit() {
	app.bot.StopReceivingUpdates()
	app.users.Stop()
}

func (app *App) initCommands() error {
	commands := []*Command{
		{
			key:  "start",
			desc: "Запустить бота",
			cb:   app.start,
		},
		{
			key:  "callsign",
			desc: "Установить позывной",
			cb:   app.callsign,
		},
		{
			key:  "team",
			desc: "Указать команду",
			cb:   app.team,
		},
		{
			key:  "role",
			desc: "Указать роль",
			cb:   app.role,
		},
	}

	tgCommands := make([]tg.BotCommand, 0, len(commands))
	for _, cmd := range commands {
		app.commands[cmd.key] = cmd
		tgCommands = append(tgCommands, tg.BotCommand{
			Command:     "/" + cmd.key,
			Description: cmd.desc,
		})
	}

	config := tg.NewSetMyCommands(tgCommands...)
	_, err := app.bot.Request(config)
	return err
}

func (app *App) Run() {
	var err error

	app.bot, err = tg.NewBotAPI(viper.GetString("token"))

	if err != nil {
		panic("can't start bot " + err.Error())
	}

	app.logger.Infof("registering %s", app.bot.Self.String())

	sigc := make(chan os.Signal, 1)

	signal.Notify(sigc, syscall.SIGINT, syscall.SIGTERM)

	updates, err := app.GetUpdatesChannel()
	if err != nil {
		app.logger.Error(err.Error())
		return
	}

	err = app.initCommands()

	if err != nil {
		app.logger.Error(err.Error())
		return
	}

	app.users.Start()

	for {
		select {
		case update := <-updates:
			go app.Process(update)
		case <-sigc:
			app.logger.Info("quit")
			app.quit()
			return
		}
	}
}

func (app *App) Process(update tg.Update) {
	if cq := update.CallbackQuery; cq != nil {
		user := app.users.Get(fmt.Sprintf("%d", cq.From.ID), fmt.Sprintf("tg-%s", getName(cq.From)))
		txt := cq.Data
		app.logger.Infof("callback with data %s", txt)
		tokens := strings.SplitN(txt, "_", 2)

		if len(tokens) != 2 {
			app.logger.Warnf("invalid callback data: %s", txt)
			return
		}

		if cb, ok := app.callbacks[tokens[0]]; ok {
			msg, err := cb(cq, user, tokens[1])
			if err != nil {
				app.logger.Errorf("callback error: %s", err.Error())
				return
			}
			app.sendMsg(msg)
		}

		return
	}

	var message *tg.Message

	if update.EditedMessage != nil {
		message = update.EditedMessage
	} else {
		message = update.Message
	}

	if message == nil {
		app.logger.Warnf("no message")
		return
	}

	user := app.users.Get(fmt.Sprintf("%d", message.From.ID), fmt.Sprintf("tg-%s", getName(message.From)))
	logger := app.logger.With("id", message.From.ID, "name", message.From.UserName)

	var answer tg.Chattable
	switch {
	case message.IsCommand():
		command := message.Command()
		if cmd, ok := app.commands[command]; ok {
			var err error
			answer, err = cmd.cb(update, user)
			if err != nil {
				logger.Errorf("error in command %s: %s", message.Text, err.Error())
				return
			}
		}
	case message.Location != nil:
		loc := message.Location
		logger.Infof("location: %f %f %f", loc.Latitude, loc.Longitude, loc.HorizontalAccuracy)
		if viper.GetString("cot.server") != "" {
			evt := makeEvent(
				user,
				loc.Latitude,
				loc.Longitude,
				loc.HorizontalAccuracy,
				float64(loc.Heading))

			app.sendCotMessage(evt)
		}
	default:
		logger.Infof("message: %s", message.Text)
	}

	app.sendMsg(answer)
}

func (app *App) sendMsg(msg tg.Chattable) error {
	if msg == nil {
		return nil
	}

	if _, err := app.bot.Send(msg); err != nil {
		app.logger.Errorf("can't send message: %s", err.Error())
		return err
	}

	return nil
}

func (app *App) request(msg tg.Chattable) error {
	if msg == nil {
		return nil
	}

	if _, err := app.bot.Request(msg); err != nil {
		app.logger.Errorf("can't send request: %s", err.Error())
		return err
	}

	return nil
}

func makeEvent(user *UserInfo, lat, lon, acc, heading float64) *cot.Event {
	evt := cot.BasicMsg(user.Typ, "tg-"+user.Id, viper.GetDuration("cot.stale"))
	evt.CotEvent.How = "a-g"
	evt.CotEvent.Lon = lon
	evt.CotEvent.Lat = lat
	evt.CotEvent.Ce = acc
	evt.CotEvent.Access = user.Scope

	evt.CotEvent.Detail = &cotproto.Detail{
		Contact:           &cotproto.Contact{Callsign: user.Callsign},
		PrecisionLocation: &cotproto.PrecisionLocation{Geopointsrc: "GPS"},
		Track:             &cotproto.Track{Course: heading},
		Group:             &cotproto.Group{Name: user.Team, Role: user.Role},
	}

	return cot.ProtoToEvent(evt)
}

func (app *App) sendCotMessage(evt *cot.Event) {
	if viper.GetString("cot.server") == "" {
		return
	}

	msg, err := xml.Marshal(evt)
	if err != nil {
		app.logger.Errorf("marshal error: %v", err)
		return
	}

	conn, err := net.Dial(viper.GetString("cot.proto"), viper.GetString("cot.server"))
	if err != nil {
		app.logger.Errorf("connection error: %v", err)
		return
	}
	defer conn.Close()

	_ = conn.SetWriteDeadline(time.Now().Add(time.Second * 10))
	if _, err := conn.Write(msg); err != nil {
		app.logger.Errorf("write error: %v", err)
	}
}

func getName(u *tg.User) string {
	switch {
	case u.UserName != "":
		return u.UserName
	case u.LastName != "" || u.FirstName != "":
		return strings.Trim(u.FirstName+" "+u.LastName, " \t\n\r")
	default:
		return fmt.Sprintf("%d", u.ID)
	}
}

func main() {
	viper.SetConfigName("cotobot")
	viper.AddConfigPath(".")

	viper.SetDefault("cot.proto", "tcp")
	viper.SetDefault("cot.stale", time.Minute*10)

	if err := viper.ReadInConfig(); err != nil {
		panic(fmt.Errorf("fatal error config file: %s", err))
	}

	config := zap.NewProductionConfig()
	config.Encoding = "console"

	logger, err := config.Build()
	defer logger.Sync()

	if err != nil {
		panic(err.Error())
	}

	sl := logger.Sugar()
	sl.Infof("starting app branch %s, rev %s", gitBranch, gitRevision)

	app := NewApp(sl)
	app.Run()
}
