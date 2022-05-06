package main

import (
	"encoding/xml"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api"
	"github.com/kdudkov/goatak/cotxml"
	"github.com/spf13/viper"
	"go.uber.org/zap"
)

var (
	gitRevision string
	gitBranch   string
)

type App struct {
	bot    *tgbotapi.BotAPI
	logger *zap.SugaredLogger
}

func NewApp(logger *zap.SugaredLogger) (app *App) {
	app = &App{
		logger: logger,
	}
	return
}

func (app *App) GetUpdatesChannel() tgbotapi.UpdatesChannel {
	if webhook := viper.GetString("webhook.ext"); webhook != "" {
		app.logger.Infof("starting webhook %s", webhook)

		info, err := app.bot.GetWebhookInfo()
		if err != nil {
			app.logger.Fatal(err)
		}
		if info.LastErrorDate != 0 {
			app.logger.Infof("Telegram callback failed: %s", info.LastErrorMessage)
		}

		app.logger.Infof("start listener on %s, path %s", viper.GetString("webhook.listen"), viper.GetString("webhook.path"))
		go func() {
			if err := http.ListenAndServe(viper.GetString("webhook.listen"), nil); err != nil {
				panic(err)
			}
		}()

		return app.bot.ListenForWebhook(viper.GetString("webhook.path"))
	}

	app.logger.Info("start polling")
	u := tgbotapi.NewUpdate(0)
	u.Timeout = 60

	ch, err := app.bot.GetUpdatesChan(u)
	if err != nil {
		panic("can't add webhook")
	}
	return ch
}

func (app *App) quit() {
	app.bot.StopReceivingUpdates()
}

func (app *App) Run() {
	var err error

	app.bot, err = tgbotapi.NewBotAPI(viper.GetString("token"))

	if err != nil {
		panic("can't start bot " + err.Error())
	}
	app.logger.Infof("registering %s", app.bot.Self.String())

	sigc := make(chan os.Signal, 1)
	signal.Notify(sigc, syscall.SIGINT, syscall.SIGTERM, syscall.SIGKILL)

	updates := app.GetUpdatesChannel()

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

func (app *App) Process(update tgbotapi.Update) {
	var message *tgbotapi.Message

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

	var msg tgbotapi.Chattable

	switch message.Text {
	case "/start":
		msg = tgbotapi.NewMessage(message.Chat.ID, "Ok, now you can share position with me and I will send it to ATAK server as a COT event")
	}
	//msg.ReplyToMessageID = update.Message.MessageID

	if msg != nil {
		if _, err := app.bot.Send(msg); err != nil {
			logger.Errorf("can't send message: %s", err.Error())
		}
	}
}

func makeEvent(id, name string, lat, lon float64) *cotxml.Event {
	evt := cotxml.BasicMsg(viper.GetString("cot.type"), id, viper.GetDuration("cot.stale"))
	evt.How = "a-g"
	evt.Detail = cotxml.Detail{
		Contact: &cotxml.Contact{Callsign: name},
	}
	evt.Point.Lon = lon
	evt.Point.Lat = lat

	return evt
}

func getLocation(update tgbotapi.Update) *tgbotapi.Location {
	if update.EditedMessage != nil {
		return update.EditedMessage.Location
	}
	if update.Message != nil {
		return update.Message.Location
	}
	return nil
}

func (app *App) sendCotMessage(evt *cotxml.Event) {
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

	conn.SetWriteDeadline(time.Now().Add(time.Second * 10))
	if _, err := conn.Write(msg); err != nil {
		app.logger.Errorf("write error: %v", err)
	}
	conn.Close()
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
