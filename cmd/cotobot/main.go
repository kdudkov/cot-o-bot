package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/kdudkov/goatak/pkg/util"
	"google.golang.org/protobuf/proto"

	tg "github.com/go-telegram-bot-api/telegram-bot-api/v5"
	"github.com/kdudkov/goatak/pkg/cot"
	"github.com/kdudkov/goatak/pkg/cotproto"
	"github.com/knadh/koanf/parsers/yaml"
	"github.com/knadh/koanf/providers/file"
	"github.com/knadh/koanf/v2"

	"cotobot/cmd/cotobot/database"
)

const magicByte = 0xbf
const NO_TEAM = "no team"

var (
	colors = []string{
		NO_TEAM,
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

type Cb func(cq *tg.CallbackQuery, user *database.UserInfo, data string) (tg.Chattable, error)

type Command struct {
	key  string
	desc string
	cb   func(update *tg.Update, user *database.UserInfo) (tg.Chattable, error)
}

type App struct {
	k            *koanf.Koanf
	bot          *tg.BotAPI
	logger       *slog.Logger
	defaultScope string
	users        *UserManager
	commands     map[string]*Command
	callbacks    map[string]Cb
}

func NewApp(k *koanf.Koanf) *App {
	db, err := database.GetDatabase("bot.sqlite", false)

	if err != nil {
		panic(err)
	}

	app := &App{
		k:            k,
		bot:          nil,
		logger:       slog.Default(),
		defaultScope: "test",
		users:        NewUserManager(db, "users.yml"),
		commands:     make(map[string]*Command),
	}

	app.callbacks = map[string]Cb{
		"team": app.callbackTeam,
		"role": app.callbackRole,
	}

	return app
}

func (app *App) GetUpdatesChannel() (tg.UpdatesChannel, error) {
	if webhook := app.k.String("webhook.ext"); webhook != "" {
		app.logger.Info(fmt.Sprintf("start listener on %s, path %s", app.k.String("webhook.listen"), app.k.String("webhook.path")))
		go func() {
			if err := http.ListenAndServe(app.k.String("webhook.listen"), nil); err != nil {
				panic(err)
			}
		}()

		app.logger.Info("starting webhook " + webhook)

		wh, _ := tg.NewWebhook(webhook)
		if _, err := app.bot.Request(wh); err != nil {
			return nil, err
		}

		info, err := app.bot.GetWebhookInfo()
		if err != nil {
			return nil, err
		}

		if info.LastErrorDate != 0 {
			app.logger.Info(fmt.Sprintf("error %d %s", info.LastErrorDate, info.LastErrorMessage))
		}

		//if info.LastErrorDate != 0 {
		//	app.logger.Errorf("Telegram callback failed: %s", info.LastErrorMessage)
		//	return nil, fmt.Errorf(info.LastErrorMessage)
		//}

		return app.bot.ListenForWebhook(app.k.String("webhook.path")), nil
	}

	app.logger.Info("start polling")
	app.removeWebhook()
	u := tg.NewUpdate(0)
	u.Timeout = 60

	return app.bot.GetUpdatesChan(u), nil
}

func (app *App) removeWebhook() {
	if _, err := app.bot.Request(tg.WebhookConfig{URL: nil}); err != nil {
		app.logger.Error("remove webhook error", "error", err)
	}
}

func (app *App) quit() {
	app.bot.StopReceivingUpdates()
}

func (app *App) initCommands() error {
	commands := []*Command{
		{
			key:  "start",
			desc: "start",
			cb:   app.start,
		},
		{
			key:  "callsign",
			desc: "Change callsign",
			cb:   app.callsign,
		},
		{
			key:  "team",
			desc: "Change team",
			cb:   app.team,
		},
		{
			key:  "role",
			desc: "Change role",
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

	app.bot, err = tg.NewBotAPI(app.k.String("token"))

	if err != nil || app.bot == nil {
		panic("can't start bot " + err.Error())
	}

	app.logger.Info("registering " + app.bot.Self.String())

	updates, err := app.GetUpdatesChannel()
	if err != nil {
		app.logger.Error("error getting channel", "error", err.Error())
		return
	}

	if err := app.initCommands(); err != nil {
		app.logger.Error("error on init commands", "error", err.Error())
		return
	}

	if err := app.users.Start(); err != nil {
		panic(err)
	}

	sigc := make(chan os.Signal, 1)
	signal.Notify(sigc, syscall.SIGINT, syscall.SIGTERM)

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
		user := app.users.Get(
			fmt.Sprintf("%d", cq.From.ID),
			getLogin(cq.From),
			fmt.Sprintf("tg-%s", getName(cq.From)),
		)

		txt := cq.Data
		app.logger.Info("callback with data " + txt)
		tokens := strings.SplitN(txt, "_", 2)

		if len(tokens) != 2 {
			app.logger.Warn("invalid callback data: " + txt)
			return
		}

		if cb, ok := app.callbacks[tokens[0]]; ok {
			msg, err := cb(cq, user, tokens[1])
			if err != nil {
				app.logger.Error("callback error", "error", err.Error())
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
		app.logger.Warn("no message")
		return
	}

	user := app.users.Get(
		fmt.Sprintf("%d", message.From.ID),
		getLogin(message.From),
		fmt.Sprintf("tg-%s", getName(message.From)),
	)
	logger := app.logger.With("id", message.From.ID, "name", message.From.UserName)

	var answer tg.Chattable
	switch {
	case message.IsCommand():
		command := message.Command()
		if cmd, ok := app.commands[command]; ok {
			var err error
			answer, err = cmd.cb(&update, user)
			if err != nil {
				logger.Error(fmt.Sprintf("error in command %s: %s", message.Text, err.Error()))
				return
			}
		}
	case message.Location != nil:
		loc := message.Location
		logger.Info(fmt.Sprintf("location: %f %f %f", loc.Latitude, loc.Longitude, loc.HorizontalAccuracy))
		app.users.UpdatePos(user.Id, getLogin(message.From))
		if app.k.String("cot.server") != "" {
			app.sendCotMessage(app.makeCot(
				user,
				app.k.Duration("cot.stale"),
				loc.Latitude,
				loc.Longitude,
				loc.HorizontalAccuracy,
				float64(loc.Heading)),
			)
		}
	default:
		logger.Info("message: " + message.Text)
	}

	if err := app.sendMsg(answer); err != nil {
		logger.Error("answer send error", "error", err)
	}
}

func (app *App) sendMsg(msg tg.Chattable) error {
	if msg == nil {
		return nil
	}

	if _, err := app.bot.Send(msg); err != nil {
		app.logger.Error("can't send message", "error", err.Error())
		return err
	}

	return nil
}

func (app *App) request(msg tg.Chattable) error {
	if msg == nil {
		return nil
	}

	if _, err := app.bot.Request(msg); err != nil {
		app.logger.Error("can't send request", "error", err.Error())
		return err
	}

	return nil
}

func (app *App) makeCot(user *database.UserInfo, d time.Duration, lat, lon, acc, heading float64) *cot.CotMessage {
	scope := util.FirstString(user.Scope, app.defaultScope)

	evt := cot.BasicMsg(user.CotType, "tg-"+user.Id, d)
	evt.CotEvent.How = "a-g"
	evt.CotEvent.Lon = lon
	evt.CotEvent.Lat = lat
	evt.CotEvent.Ce = acc
	evt.CotEvent.Access = scope

	evt.CotEvent.Detail = &cotproto.Detail{
		Contact:           &cotproto.Contact{Callsign: user.Callsign},
		PrecisionLocation: &cotproto.PrecisionLocation{Geopointsrc: "GPS"},
		Track:             &cotproto.Track{Course: heading},
	}

	if user.Team != "" {
		evt.CotEvent.Detail.Group = &cotproto.Group{Name: user.Team, Role: user.Role}
	}

	return &cot.CotMessage{TakMessage: evt, Scope: scope}
}

func (app *App) sendCotMessage(msg *cot.CotMessage) {
	if app.k.String("cot.server") == "" {
		return
	}

	if app.k.String("cot.proto") == "http" {
		if err := app.sendHttp(msg); err != nil {
			app.logger.Error("http send error", "error", err.Error())
		}
	} else {
		app.sendTcp(msg)
	}
}

func (app *App) sendTcp(msg *cot.CotMessage) {
	data, err := proto.Marshal(msg.TakMessage)
	if err != nil {
		app.logger.Error("marshal error", "error", err)
		return
	}

	fulldata := make([]byte, len(data)+3)
	copy(fulldata[3:], data)
	fulldata[0] = magicByte
	fulldata[1] = 1
	fulldata[2] = magicByte

	conn, err := net.Dial(app.k.String("cot.proto"), app.k.String("cot.server"))
	if err != nil {
		app.logger.Error("connection error", "error", err)
		return
	}

	defer conn.Close()

	_ = conn.SetWriteDeadline(time.Now().Add(time.Second * 10))
	if _, err := conn.Write(fulldata); err != nil {
		app.logger.Error("write error", "error", err)
	}
}

func (app *App) sendHttp(msg *cot.CotMessage) error {
	cl := http.Client{
		Timeout: time.Second * 5,
	}

	data, err := json.Marshal(msg)
	if err != nil {
		return err
	}

	_, err = cl.Post(app.k.String("cot.server"), "application/json", bytes.NewReader(data))
	return err
}

func getName(u *tg.User) string {
	if u == nil {
		return ""
	}

	switch {
	case u.UserName != "":
		return u.UserName
	case u.LastName != "" || u.FirstName != "":
		return strings.Trim(u.FirstName+" "+u.LastName, " \t\n\r")
	default:
		return fmt.Sprintf("%d", u.ID)
	}
}

func getLogin(u *tg.User) string {
	if u == nil {
		return ""
	}

	return u.UserName
}

func main() {
	k := koanf.New(".")

	k.Set("cot.proto", "tcp")
	k.Set("cot.stale", time.Minute*10)

	if err := k.Load(file.Provider("cotobot.yml"), yaml.Parser()); err != nil {
		fmt.Printf("error loading config: %s", err.Error())
		return
	}

	h := slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo})
	slog.SetDefault(slog.New(h))

	slog.Default().Info("starting app version " + getVersion())

	NewApp(k).Run()
}
