package app

import (
	"encoding/base64"
	"errors"
	"os"
	"path/filepath"
	"strings"
)

// ChooseFolder asks for a folder that WriteFile then writes into without
// asking again, as a capture of rare events needs. It returns "" when the
// user cancelled.
func (a *App) ChooseFolder() (string, error) {
	if a.FolderDialog == nil {
		return "", errors.New("выбор папки недоступен в этом режиме")
	}
	dir, err := a.FolderDialog()
	if err != nil || dir == "" {
		return "", err
	}
	a.mu.Lock()
	a.saveDir = dir
	a.mu.Unlock()
	return dir, nil
}

// WriteFile writes a file into the folder chosen with ChooseFolder. Only a
// plain file name is taken, so that the page cannot write anywhere else.
func (a *App) WriteFile(name, b64 string) (string, error) {
	a.mu.Lock()
	dir := a.saveDir
	a.mu.Unlock()
	if dir == "" {
		return "", errors.New("папка не выбрана")
	}
	if name == "" || name != filepath.Base(name) || strings.ContainsAny(name, `/\:`) || name == "." || name == ".." {
		return "", errors.New("недопустимое имя файла: " + name)
	}
	data, err := base64.StdEncoding.DecodeString(b64)
	if err != nil {
		return "", err
	}
	path := filepath.Join(dir, name)
	return path, os.WriteFile(path, data, 0o644)
}
