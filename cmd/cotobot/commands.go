package main

import (
	"fmt"
	"strings"

	tg "github.com/go-telegram-bot-api/telegram-bot-api/v5"

	"cotobot/cmd/cotobot/database"
)

func (app *App) start(update *tg.Update, user *database.UserInfo) (tg.Chattable, error) {
	name := getName(update.Message.From)

	text := fmt.Sprintf("Now, %s, you can share your location here and it will be visible on takserver.ru using ATAK client", name)
	text += "\nchange callsign - /callsign\nchange team - /team\nchange role - /role"

	msg := tg.NewMessage(update.SentFrom().ID, text)
	return msg, nil
}

func (app *App) callsign(update *tg.Update, user *database.UserInfo) (tg.Chattable, error) {
	var message *tg.Message

	if update.EditedMessage != nil {
		message = update.EditedMessage
	} else {
		message = update.Message
	}

	if message == nil {
		app.logger.Error("empty message")
		return nil, nil
	}

	args := message.CommandArguments()
	if args == "" {
		return tg.NewMessage(update.SentFrom().ID, "usage: /callsign <callsign>"), nil
	}

	newCs := strings.Fields(args)[0]

	if newCs != user.Callsign {
		app.logger.Info(fmt.Sprintf("%s callsign %s -> %s", user.Id, user.Callsign, newCs))
		user.Callsign = newCs
		app.users.Save(user)
	}

	msg := tg.NewMessage(update.SentFrom().ID, getMessage(user))
	msg.ReplyMarkup = tg.NewRemoveKeyboard(false)

	return msg, nil
}

func (app *App) team(update *tg.Update, user *database.UserInfo) (tg.Chattable, error) {
	msg := tg.NewMessage(update.SentFrom().ID, "select team")

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

func (app *App) role(update *tg.Update, user *database.UserInfo) (tg.Chattable, error) {
	msg := tg.NewMessage(update.SentFrom().ID, "select role")

	var keyboard [][]tg.InlineKeyboardButton
	row := make([]tg.InlineKeyboardButton, 0)
	for i, c := range roles {
		row = append(row, tg.NewInlineKeyboardButtonData(c, "role_"+c))
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

func (app *App) callbackTeam(cq *tg.CallbackQuery, user *database.UserInfo, data string) (tg.Chattable, error) {
	if data != user.Team {
		app.logger.Info(fmt.Sprintf("%s team %s -> %s", user.Id, user.Team, data))

		user.Team = data
		if user.Team == NO_TEAM {
			user.Team = ""
		}
		app.users.Save(user)
	}

	app.request(tg.NewCallback(cq.ID, ""))

	msg := tg.NewMessage(cq.From.ID, getMessage(user))
	msg.ReplyMarkup = tg.NewRemoveKeyboard(false)

	return msg, nil
}

func (app *App) callbackRole(cq *tg.CallbackQuery, user *database.UserInfo, data string) (tg.Chattable, error) {
	if user == nil {
		app.logger.Error("user is nil")
		return nil, nil
	}

	if data != user.Role {
		app.logger.Info(fmt.Sprintf("%s role %s -> %s", user.Id, user.Role, data))
		user.Role = data
		app.users.Save(user)
	}

	msg1 := tg.NewCallback(cq.ID, "")
	app.request(msg1)

	msg := tg.NewMessage(cq.From.ID, getMessage(user))
	msg.ReplyMarkup = tg.NewRemoveKeyboard(false)

	return msg, nil
}

func getMessage(user *database.UserInfo) string {
	if user.Team != "" {
		return fmt.Sprintf("now you are %s %s, callsign %s", user.Team, user.Role, user.Callsign)
	}

	return fmt.Sprintf("your callsign is %s, type %s", user.Callsign, user.CotType)
}
