package main

import (
	"log/slog"
	"time"

	"gorm.io/gorm"

	"cotobot/cmd/cotobot/database"
)

type UserManager struct {
	logger      *slog.Logger
	db          *gorm.DB
	defaultType string
}

func NewUserManager(db *gorm.DB) *UserManager {
	um := &UserManager{
		logger:      slog.Default().With("logger", "UserManager"),
		db:          db,
		defaultType: "a-f-G",
	}

	return um
}

func (um *UserManager) Get(id, login, name string) *database.UserInfo {
	if u := database.NewUserQuery(um.db).ID(id).One(); u != nil {
		return u
	}

	return &database.UserInfo{
		Id:       id,
		Login:    login,
		Callsign: name,
		Role:     "Team Member",
		Scope:    "",
		CotType:  um.defaultType,
	}
}

func (um *UserManager) UpdatePos(id string, login string) error {
	return database.NewUserQuery(um.db).ID(id).Update(map[string]any{"login": login, "last_pos": time.Now()})
}

func (um *UserManager) Save(u *database.UserInfo) error {
	err := um.db.Save(u).Error

	if err != nil {
		um.logger.Error("save error", slog.Any("error", err))
	}

	return err
}

func (um *UserManager) Start() error {
	if err := um.db.AutoMigrate(&database.UserInfo{}); err != nil {
		return err
	}

	if database.NewUserQuery(um.db).Count() == 0 {
		um.logger.Info("db is empty - load users files")
	}

	return nil
}
