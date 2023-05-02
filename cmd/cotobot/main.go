package main

import (
	"encoding/xml"
	"fmt"
	"github.com/kdudkov/goatak/cotproto"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	tg "github.com/go-telegram-bot-api/telegram-bot-api/v5"
	"github.com/kdudkov/goatak/cot"
	"github.com/spf13/viper"
	"go.uber.org/zap"
)

var (
	gitRevision string
	gitBranch   string
)

type App struct {
	bot    *tg.BotAPI
	logger *zap.SugaredLogger
}

func NewApp(logger *zap.SugaredLogger) (app *App) {
	app = &App{
		logger: logger,
	}
	return
}

func (app *App) GetUpdatesChannel() (tg.UpdatesChannel, error) {
	if webhook := viper.GetString("webhook.ext"); webhook != "" {
		app.logger.Infof("starting webhook %s", webhook)

		wh, _ := tg.NewWebhook(webhook)
		if _, err := app.bot.Request(wh); err != nil {
			return nil, err
		}

		info, err := app.bot.GetWebhookInfo()
		if err != nil {
			return nil, err
		}

		if info.LastErrorDate != 0 {
			app.logger.Errorf("Telegram callback failed: %s", info.LastErrorMessage)
			return nil, fmt.Errorf(info.LastErrorMessage)
		}

		app.logger.Infof("start listener on %s, path %s", viper.GetString("webhook.listen"), viper.GetString("webhook.path"))
		go func() {
			if err := http.ListenAndServe(viper.GetString("webhook.listen"), nil); err != nil {
				panic(err)
			}
		}()

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
}

func (app *App) Run() {
	var err error

	app.bot, err = tg.NewBotAPI(viper.GetString("token"))

	if err != nil {
		panic("can't start bot " + err.Error())
	}
	app.logger.Infof("registering %s", app.bot.Self.String())

	sigc := make(chan os.Signal, 1)
	signal.Notify(sigc, syscall.SIGINT, syscall.SIGTERM, syscall.SIGKILL)

	updates, err := app.GetUpdatesChannel()

	if err != nil {
		app.logger.Error(err.Error())
		return
	}

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
	var message *tg.Message

	if update.EditedMessage != nil {
		message = update.EditedMessage
	} else {
		message = update.Message
	}

	if message == nil {
		app.logger.Warnf("no message: %v", update)
		return
	}

	if message.From == nil {
		app.logger.Warnf("message without from: %v", update)
		return
	}

	logger := app.logger.With("from", message.From.UserName, "id", message.From.ID)

	// location
	if loc := getLocation(update); loc != nil {
		logger.Infof("location: %f %f", loc.Latitude, loc.Longitude)
		if viper.GetString("cot.server") != "" {
			evt := makeEvent(
				fmt.Sprintf("tg-%d", message.From.ID),
				fmt.Sprintf("tg-%s", message.From.UserName),
				loc.Latitude,
				loc.Longitude)
			app.sendCotMessage(evt)
			return
		}
	}

	if message.Text == "" {
		logger.Infof("empty message")
		return
	}

	if update.Message == nil {
		// edited message
		return
	}

	logger.Infof("message: %s", message.Text)

	var msg tg.Chattable

	switch message.Text {
	case "/start":
		msg = tg.NewMessage(message.Chat.ID, "Ok, now you can share position with me and I will send it to ATAK server as a COT event")
	}
	//msg.ReplyToMessageID = update.Message.MessageID

	if msg != nil {
		if _, err := app.bot.Send(msg); err != nil {
			logger.Errorf("can't send message: %s", err.Error())
		}
	}
}

func makeEvent(id, name string, lat, lon float64) *cot.Event {
	evt := cot.BasicMsg(viper.GetString("cot.type"), id, viper.GetDuration("cot.stale"))
	evt.CotEvent.How = "a-g"
	evt.CotEvent.Detail = &cotproto.Detail{
		Contact: &cotproto.Contact{Callsign: name},
	}
	evt.CotEvent.Lon = lon
	evt.CotEvent.Lat = lat

	return cot.ProtoToEvent(evt)
}

func getLocation(update tg.Update) *tg.Location {
	if update.EditedMessage != nil {
		return update.EditedMessage.Location
	}
	if update.Message != nil {
		return update.Message.Location
	}
	return nil
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

	_ = conn.SetWriteDeadline(time.Now().Add(time.Second * 10))
	if _, err := conn.Write(msg); err != nil {
		app.logger.Errorf("write error: %v", err)
	}
	_ = conn.Close()
}

func main() {
	viper.SetConfigName("cotobot")
	viper.AddConfigPath(".")

	viper.SetDefault("cot.proto", "tcp")
	viper.SetDefault("cot.type", "a-n-G")
	viper.SetDefault("cot.stale", time.Minute*10)

	if err := viper.ReadInConfig(); err != nil {
		panic(fmt.Errorf("Fatal error config file: %s \n", err))
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
