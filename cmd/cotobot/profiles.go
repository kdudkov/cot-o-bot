package main

import (
	"log/slog"
	"os"
	"sync"

	"github.com/fsnotify/fsnotify"
	"gopkg.in/yaml.v3"
)

type UserInfo struct {
	Id       string `yaml:"id"`
	Callsign string `yaml:"callsign"`
	Team     string `yaml:"team,omitempty"`
	Role     string `yaml:"role"`
	Typ      string `yaml:"type"`
	Scope    string `yaml:"scope"`
}

type UserManager struct {
	userFile string
	logger   *slog.Logger
	users    map[string]*UserInfo

	watcher  *fsnotify.Watcher
	savechan chan bool

	mx sync.RWMutex
}

func (u *UserInfo) Copy() *UserInfo {
	return &UserInfo{
		Id:       u.Id,
		Callsign: u.Callsign,
		Team:     u.Team,
		Role:     u.Role,
		Typ:      u.Typ,
		Scope:    u.Scope,
	}
}

func NewUserManager(userFile string) *UserManager {
	um := &UserManager{
		logger:   slog.Default().With("logger", "UserManager"),
		userFile: userFile,
		users:    make(map[string]*UserInfo),
		mx:       sync.RWMutex{},
		savechan: make(chan bool, 10),
	}

	um.loadUsersFile()

	return um
}

func (um *UserManager) Get(id, name string) *UserInfo {
	if u := um.getCopy(id); u != nil {
		return u
	}

	u := &UserInfo{
		Id:       id,
		Callsign: name,
		Role:     "Team Member",
		Scope:    "test",
		Typ:      "a-f-G",
	}
	um.AddUser(u)
	return u
}

func (um *UserManager) getCopy(id string) *UserInfo {
	um.mx.RLock()
	defer um.mx.RUnlock()

	if u, ok := um.users[id]; ok {
		return u.Copy()
	}
	return nil
}

func (um *UserManager) AddUser(u *UserInfo) {
	if u == nil {
		return
	}

	um.mx.Lock()
	defer um.mx.Unlock()

	um.users[u.Id] = u.Copy()
	um.savechan <- true
}

func (um *UserManager) loadUsersFile() error {
	um.mx.Lock()
	defer um.mx.Unlock()

	dat, err := os.ReadFile(um.userFile)

	if err != nil {
		return err
	}

	users := make([]*UserInfo, 0)

	if err := yaml.Unmarshal(dat, &users); err != nil {
		return err
	}

	um.users = make(map[string]*UserInfo)
	for _, user := range users {
		if user.Id != "" {
			um.users[user.Id] = user
		}
	}

	return nil
}

func (um *UserManager) saver() {
	for range um.savechan {
		if err := um.save(); err != nil {
			um.logger.Error("error save file", "error", err.Error())
		}
	}
}

func (um *UserManager) save() error {
	um.mx.Lock()
	defer um.mx.Unlock()
	tmpFile := um.userFile + ".tmp"

	f, err := os.Create(tmpFile)
	if err != nil {
		return err
	}

	u1 := make([]*UserInfo, 0, len(um.users))

	for _, uu := range um.users {
		u1 = append(u1, uu)
	}

	enc := yaml.NewEncoder(f)
	if err := enc.Encode(u1); err != nil {
		return err
	}

	return os.Rename(tmpFile, um.userFile)
}

func (um *UserManager) Start() error {
	var err error
	um.watcher, err = fsnotify.NewWatcher()
	if err != nil {
		return err
	}

	um.watcher.Add(um.userFile)
	go func() {
		for {
			select {
			case event, ok := <-um.watcher.Events:
				if !ok {
					return
				}
				um.logger.Debug("event: " + event.String())
				if event.Has(fsnotify.Write) && event.Name == um.userFile {
					um.logger.Info("users file is modified, reloading")
					if err := um.loadUsersFile(); err != nil {
						um.logger.Error("error", "error", err.Error())
					}
				}
			case err, ok := <-um.watcher.Errors:
				if !ok {
					return
				}
				um.logger.Error("error", "error", err.Error())
			}
		}
	}()

	go um.saver()

	return nil
}

func (um *UserManager) Stop() {
	if um.watcher != nil {
		_ = um.watcher.Close()
	}
	close(um.savechan)
}
