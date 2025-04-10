package main

import (
	"log/slog"
	"os"
	"time"

	"gopkg.in/yaml.v3"
	"gorm.io/gorm"

	"cotobot/cmd/cotobot/database"
)

type UserManager struct {
	logger       *slog.Logger
	db           *gorm.DB
	userFile     string
	defaultScope string
}

func NewUserManager(db *gorm.DB, userFile string) *UserManager {
	um := &UserManager{
		logger:       slog.Default().With("logger", "UserManager"),
		db:           db,
		userFile:     userFile,
		defaultScope: "test",
	}

	return um
}

func (um *UserManager) Get(id, login, name string) *database.UserInfo {
	var u *database.UserInfo

	if u = database.NewUserQuery(um.db).ID(id).One(); u == nil {
		u = &database.UserInfo{
			Id:       id,
			Login:    login,
			Callsign: name,
			Role:     "Team Member",
			Scope:    um.defaultScope,
			CotType:  "a-f-G",
		}
	}

	return u
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

func (um *UserManager) loadUsersFile() error {
	dat, err := os.ReadFile(um.userFile)

	if err != nil {
		return err
	}

	users := make([]*database.UserInfo, 0)

	if err := yaml.Unmarshal(dat, &users); err != nil {
		return err
	}

	for _, user := range users {
		um.Save(user)
	}

	return nil
}

func (um *UserManager) Start() error {
	if err := um.db.AutoMigrate(&database.UserInfo{}); err != nil {
		return err
	}

	if database.NewUserQuery(um.db).Count() == 0 {
		um.logger.Info("db is empty - load users files")
		return um.loadUsersFile()
	}

	return nil
}
