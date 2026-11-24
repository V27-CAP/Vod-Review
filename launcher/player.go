package main

import (
	"archive/zip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"syscall"

	"golang.org/x/sys/windows"
)

var playerSize, playerHash string

// The portable player comes from the same FFmpeg distribution as the analysis
// tools. Its separate archive keeps the existing analysis installation usable.
func playerPath() (string, error) {
	cache, err := os.UserCacheDir()
	if err != nil {
		return "", err
	}
	folder := filepath.Join(cache, "VODCutStudio", "players", playerHash)
	target := filepath.Join(folder, "ffplay.exe")
	if _, err = os.Stat(target); err == nil {
		return target, nil
	}
	size, err := strconv.ParseInt(playerSize, 10, 64)
	if err != nil {
		return "", err
	}
	runtimeSize, err := strconv.ParseInt(payloadSize, 10, 64)
	if err != nil {
		return "", err
	}
	self, err := os.Executable()
	if err != nil {
		return "", err
	}
	f, err := os.Open(self)
	if err != nil {
		return "", err
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil {
		return "", err
	}
	offset := info.Size() - runtimeSize - size
	if offset < 0 || size <= 0 {
		return "", errors.New("Reprodutor incompleto. Baixe o VODCut novamente.")
	}
	z, err := zip.NewReader(io.NewSectionReader(f, offset, size), size)
	if err != nil {
		return "", err
	}
	if len(z.File) != 1 || z.File[0].Name != "ffplay.exe" {
		return "", errors.New("Pacote do reprodutor inválido.")
	}
	in, err := z.File[0].Open()
	if err != nil {
		return "", err
	}
	defer in.Close()
	if err = os.MkdirAll(folder, 0755); err != nil {
		return "", err
	}
	out, err := os.CreateTemp(folder, ".player-")
	if err != nil {
		return "", err
	}
	defer os.Remove(out.Name())
	hash := sha256.New()
	_, err = io.Copy(io.MultiWriter(out, hash), in)
	if err = errors.Join(err, out.Close()); err != nil {
		return "", err
	}
	if hex.EncodeToString(hash.Sum(nil)) != playerHash {
		return "", errors.New("O reprodutor está incompleto. Baixe o VODCut novamente.")
	}
	if err = os.Rename(out.Name(), target); err != nil {
		return "", err
	}
	return target, nil
}

func (u *desk) portablePreview(path string) {
	if u.previewCancel != nil {
		u.previewCancel()
	}
	ctx, cancel := context.WithCancel(context.Background())
	u.previewCancel = cancel
	window := u.mw
	caption := title + u.t(" · Prévia   |   Espaço para pausar · Esc para fechar", " · Preview   |   Space to pause · Esc to close")
	u.status.SetText(u.t("Abrindo a prévia…", "Opening preview…"))
	go func() {
		player, err := playerPath()
		if err == nil {
			cmd := exec.CommandContext(ctx, player, "-loglevel", "error", "-autoexit", "-x", "800", "-window_title", caption, path)
			cmd.SysProcAttr = &syscall.SysProcAttr{CreationFlags: windows.CREATE_NO_WINDOW}
			var diagnostic []byte
			diagnostic, err = cmd.CombinedOutput()
			if err != nil && len(diagnostic) > 0 {
				err = fmt.Errorf("%w\n%s", err, diagnostic)
			}
		}
		if ctx.Err() != nil {
			return
		}
		window.Synchronize(func() {
			if err != nil {
				u.error(err)
			} else {
				u.status.SetText(u.t("Prévia concluída.", "Preview finished."))
			}
		})
	}()
}
