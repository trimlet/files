package main

import (
	"encoding/json"
	"os"
	"path/filepath"
)

// The folders of the two panels are kept in a small file, so the app reopens where it was left
// (author, 2026-11-19). A folder that no longer exists is replaced by the home folder.

type state struct {
	Left  string `json:"left"`
	Right string `json:"right"`
}

func statePath() (string, error) {
	dir, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "trimletfiles", "state.json"), nil
}

// loadDirs gives the folders to open: the saved ones, or the home folder.
func loadDirs() (left, right string) {
	left, right = home, home
	if home == "" {
		left, right = "/", "/"
	}
	path, err := statePath()
	if err != nil {
		return
	}
	b, err := os.ReadFile(path)
	if err != nil {
		return
	}
	var s state
	if json.Unmarshal(b, &s) != nil {
		return
	}
	isDir := func(p string) bool {
		fi, err := os.Stat(p)
		return err == nil && fi.IsDir()
	}
	if s.Left != "" && isDir(s.Left) {
		left = s.Left
	}
	if s.Right != "" && isDir(s.Right) {
		right = s.Right
	}
	return
}

// saveDirsFile writes the folders of the two panels; errors are ignored (it is only a convenience).
func saveDirsFile(left, right string) {
	path, err := statePath()
	if err != nil {
		return
	}
	b, _ := json.Marshal(state{Left: left, Right: right})
	if os.MkdirAll(filepath.Dir(path), 0o755) == nil {
		os.WriteFile(path, b, 0o644)
	}
}
