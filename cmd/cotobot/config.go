package main

import (
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/knadh/koanf/parsers/yaml"
	"github.com/knadh/koanf/providers/env"
	"github.com/knadh/koanf/providers/file"
	"github.com/knadh/koanf/v2"
)

type AppConfig struct {
	k *koanf.Koanf
}

func NewAppConfig() *AppConfig {
	c := &AppConfig{k: koanf.New(".")}
	setDefaults(c.k)

	return c
}

func (c *AppConfig) Load(filename ...string) bool {
	loaded := false

	for _, name := range filename {
		if err := c.k.Load(file.Provider(name), yaml.Parser()); err != nil {
			slog.Info(fmt.Sprintf("error loading config: %s", err.Error()))
		} else {
			loaded = true
		}
	}

	return loaded
}

func (c *AppConfig) LoadEnv(prefix string) error {
	return c.k.Load(env.Provider(prefix, ".", func(s string) string {
		return strings.ToLower(strings.ReplaceAll(strings.TrimPrefix(s, prefix), "_", "."))
	}), nil)
}

func (c *AppConfig) Bool(key string) bool {
	return c.k.Bool(key)
}

func (c *AppConfig) String(key string) string {
	return c.k.String(key)
}

func (c *AppConfig) FirstString(key ...string) string {
	for _, k := range key {
		if s := c.k.String(k); s != "" {
			return s
		}
	}

	return ""
}

func (c *AppConfig) Float64(key string) float64 {
	return c.k.Float64(key)
}

func (c *AppConfig) Int(key string) int {
	return c.k.Int(key)
}

func (c *AppConfig) Duration(key string) time.Duration {
	return c.k.Duration(key)
}

func setDefaults(k *koanf.Koanf) {
	k.Set("database", "bot.sqlite")
	k.Set("cot.proto", "tcp")
	k.Set("cot.stale", time.Minute*10)
	k.Set("cot.stale", time.Minute*10)
}
