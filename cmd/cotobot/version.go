package main

import "fmt"

var (
	version = "unknown"
	commit  = ""
	date    = ""
)

func getVersion() string {
	return version
}

func getVersionFull() string {
	return fmt.Sprintf("version: %s, commit: %s, built at: %s", version, commit, date)
}
