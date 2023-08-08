package main

import (
	"os"
	"sync"

	"github.com/fsnotify/fsnotify"
	"go.uber.org/zap"
	"gopkg.in/yaml.v3"
)

type UserInfo struct {
	Id       string `yaml:"id"`
	Callsign string `yaml:"callsign"`
	Team     string `yaml:"team"`
	Role     string `yaml:"role"`
	Scope    string `yaml:"scope"`
}

type UserManager struct {
	userFile string
	logger   *zap.SugaredLogger
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
		Scope:    u.Scope,
	}
}

func NewUserManager(logger *zap.SugaredLogger, userFile string) *UserManager {
	um := &UserManager{
		logger:   logger.Named("UserManager"),
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
		Team:     "White",
		Scope:    "test",
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
			um.logger.Errorf("error save file: %s", err.Error())
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
				um.logger.Debugf("event: %v", event)
				if event.Has(fsnotify.Write) && event.Name == um.userFile {
					um.logger.Infof("users file is modified, reloading")
					if err := um.loadUsersFile(); err != nil {
						um.logger.Errorf("error: %s", err.Error())
					}
				}
			case err, ok := <-um.watcher.Errors:
				if !ok {
					return
				}
				um.logger.Errorf("error: %s", err.Error())
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
