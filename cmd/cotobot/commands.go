package main

import (
	"fmt"

	tg "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

func (app *App) start(update tg.Update, user *UserInfo) (tg.Chattable, error) {
	name := getName(update.Message.From)

	text := fmt.Sprintf("Now, %s, you can share your location here and it will be visible on takserver.ru using ATAK client", name)
	text += "\nchange callsign - /callsign\nchange team - /team\nchange role - /role"

	msg := tg.NewMessage(update.SentFrom().ID, text)
	return msg, nil
}

func (app *App) callsign(update tg.Update, user *UserInfo) (tg.Chattable, error) {
	args := update.Message.CommandArguments()
	if args == "" {
		return tg.NewMessage(update.SentFrom().ID, "usage: /callsign <callsign>"), nil
	}

	if args != user.Callsign {
		app.logger.Info(fmt.Sprintf("%s callsign %s -> %s", user.Id, user.Callsign, args))
		user.Callsign = args
		app.users.AddUser(user)
	}

	msg := tg.NewMessage(update.SentFrom().ID, getMessage(user))
	msg.ReplyMarkup = tg.NewRemoveKeyboard(false)

	return msg, nil
}

func (app *App) team(update tg.Update, user *UserInfo) (tg.Chattable, error) {
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

func (app *App) role(update tg.Update, user *UserInfo) (tg.Chattable, error) {
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

func (app *App) callbackTeam(cq *tg.CallbackQuery, user *UserInfo, data string) (tg.Chattable, error) {
	if data != user.Team {
		app.logger.Info(fmt.Sprintf("%s team %s -> %s", user.Id, user.Team, data))

		user.Team = data
		if user.Team == NO_TEAM {
			user.Team = ""
		}
		app.users.AddUser(user)
	}

	msg1 := tg.NewCallback(cq.ID, "")
	app.request(msg1)

	msg := tg.NewMessage(cq.From.ID, getMessage(user))
	msg.ReplyMarkup = tg.NewRemoveKeyboard(false)

	return msg, nil
}

func (app *App) callbackRole(cq *tg.CallbackQuery, user *UserInfo, data string) (tg.Chattable, error) {
	if data != user.Role {
		app.logger.Info(fmt.Sprintf("%s role %s -> %s", user.Id, user.Role, data))
		user.Role = data
		app.users.AddUser(user)
	}

	msg1 := tg.NewCallback(cq.ID, "")
	app.request(msg1)

	msg := tg.NewMessage(cq.From.ID, getMessage(user))
	msg.ReplyMarkup = tg.NewRemoveKeyboard(false)

	return msg, nil
}

func getMessage(user *UserInfo) string {
	if user.Team != "" {
		return fmt.Sprintf("now you are %s %s, callsign %s", user.Team, user.Role, user.Callsign)
	}

	return fmt.Sprintf("your callsign is %s, type %s", user.Callsign, user.Typ)
}
