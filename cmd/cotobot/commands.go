package main

import (
	"fmt"

	tg "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

func (app *App) start(update tg.Update, user *UserInfo) (tg.Chattable, error) {
	name := update.Message.From.UserName
	if name == "" {
		name = update.Message.From.FirstName
	}

	msg := tg.NewMessage(update.SentFrom().ID, fmt.Sprintf("Теперь, %s, ты можешь расшарить своё местоположение и оно отобразится на сервере ATAK", name))
	return msg, nil
}

func (app *App) callsign(update tg.Update, user *UserInfo) (tg.Chattable, error) {
	args := update.Message.CommandArguments()
	if args == "" {
		return tg.NewMessage(update.SentFrom().ID, "usage: /callsign <callsign>"), nil
	}

	user.Callsign = args
	app.users.AddUser(user)

	msg := tg.NewMessage(update.SentFrom().ID, fmt.Sprintf("ваша команда теперь %s, позывной %s", user.Team, user.Callsign))
	msg.ReplyMarkup = tg.NewRemoveKeyboard(false)

	return msg, nil
}

func (app *App) team(update tg.Update, user *UserInfo) (tg.Chattable, error) {
	msg := tg.NewMessage(update.SentFrom().ID, "выберите команду")

	var keyboard [][]tg.InlineKeyboardButton
	row := make([]tg.InlineKeyboardButton, 0)
	for i, c := range colors {
		row = append(row, tg.NewInlineKeyboardButtonData(c, "team_"+c))
		if (i+1)%3 == 0 {
			keyboard = append(keyboard, row)
			row = make([]tg.InlineKeyboardButton, 0)
		}
	}
	keyboard = append(keyboard, row)

	msg.ReplyMarkup = tg.InlineKeyboardMarkup{
		InlineKeyboard: keyboard,
	}

	return msg, nil
}

func (app *App) callbackTeam(cq *tg.CallbackQuery, user *UserInfo, data string) (tg.Chattable, error) {
	user.Team = data
	app.users.AddUser(user)

	msg1 := tg.NewCallback(cq.ID, "")
	app.request(msg1)

	msg := tg.NewMessage(cq.From.ID, fmt.Sprintf("ваша команда теперь %s, позывной %s", user.Team, user.Callsign))
	msg.ReplyMarkup = tg.NewRemoveKeyboard(false)

	return msg, nil
}
