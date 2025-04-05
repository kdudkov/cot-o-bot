package database

import "time"

type UserInfo struct {
	Id       string `gorm:"primaryKey" yaml:"id"`
	Login    string `gorm:"not null;default:''" yaml:"login"`
	Callsign string `gorm:"not null;default:''" yaml:"callsign"`
	Team     string `gorm:"not null;default:''" yaml:"team,omitempty"`
	Role     string `gorm:"not null;default:''" yaml:"role"`
	CotType  string `gorm:"not null;default:''" yaml:"type"`
	Scope    string `gorm:"not null;default:''" yaml:"scope"`
	LastPos  *time.Time
}
