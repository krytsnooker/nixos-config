//go:build linux

package main

import "fmt"

type SteamAccount struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

func FindSteamAccounts(steamPath string) ([]SteamAccount, error) {
	return nil, fmt.Errorf("Steam shortcut management is not supported on Linux")
}

func AddSteamShortcut(steamPath, userID, name, exe, startDir string) error {
	return fmt.Errorf("Steam shortcut management is not supported on Linux")
}

func RemoveSteamShortcut(steamPath, userID, name string) error {
	return fmt.Errorf("Steam shortcut management is not supported on Linux")
}
