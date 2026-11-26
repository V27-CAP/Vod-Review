package main

import (
	"archive/zip"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"github.com/lxn/walk"
	. "github.com/lxn/walk/declarative"
	"github.com/lxn/win"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"syscall"
	"time"

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

func (u *desk) videoBounds(frame *walk.Composite) walk.Rectangle {
	r := frame.ClientBoundsPixels()
	w, h := u.project.Meta.Width, u.project.Meta.Height
	if r.Width <= 0 || r.Height <= 0 || w <= 0 || h <= 0 {
		return r
	}
	width := min(r.Width, r.Height*w/h)
	height := max(1, width*h/w)
	return walk.Rectangle{X: (r.Width - width) / 2, Y: (r.Height - height) / 2, Width: max(1, width), Height: height}
}

func (u *desk) portablePreview(ctx context.Context, frame, panel *walk.Composite, path string, offset float64) {
	window := u.mw
	var child win.HWND
	fit := func() {
		if child != 0 {
			r := u.videoBounds(frame)
			win.MoveWindow(child, int32(r.X), int32(r.Y), int32(r.Width), int32(r.Height), true)
		}
	}
	frame.SizeChanged().Attach(fit)
	key := func(v uintptr) {
		if child != 0 {
			win.PostMessage(child, win.WM_KEYDOWN, v, 0)
			win.PostMessage(child, win.WM_KEYUP, v, 0)
		}
	}
	var controls *walk.Composite
	var play *walk.PushButton
	paused := false
	row := []Widget{Composite{AssignTo: &controls, Enabled: false, Layout: HBox{MarginsZero: true}, Children: []Widget{
		PushButton{AssignTo: &play, Text: u.t("Pausar", "Pause"), OnClicked: func() {
			key(win.VK_SPACE)
			paused = !paused
			if paused {
				play.SetText(u.t("Continuar", "Resume"))
			} else {
				play.SetText(u.t("Pausar", "Pause"))
			}
		}},
		PushButton{Text: "−10 s", OnClicked: func() { key(win.VK_LEFT) }},
		PushButton{Text: "+10 s", OnClicked: func() { key(win.VK_RIGHT) }},
		PushButton{Text: u.t("Ativar/desativar som", "Toggle sound"), OnClicked: func() { key('M') }},
		HSpacer{},
	}}}
	u.skin(row, u.colors().panel, nil)
	if err := row[0].Create(NewBuilder(panel)); err != nil {
		u.stopPreview()
		u.error(err)
		return
	}

	caption := fmt.Sprintf("VODCut-%d-%d", os.Getpid(), time.Now().UnixNano())
	go func() {
		player, err := playerPath()
		if err == nil && ctx.Err() == nil {
			args := []string{"-loglevel", "error", "-autoexit", "-noborder", "-left", "-32000", "-top", "-32000", "-window_title", caption, "-ss", fmt.Sprintf("%.3f", offset), path}
			cmd := exec.CommandContext(ctx, player, args...)
			cmd.SysProcAttr = &syscall.SysProcAttr{CreationFlags: windows.CREATE_NO_WINDOW}
			var diagnostic bytes.Buffer
			cmd.Stderr = &diagnostic
			if err = cmd.Start(); err == nil {
				done := make(chan error, 1)
				go func() { done <- cmd.Wait() }()
				err = attachPlayer(ctx, cmd, done, caption, func(hwnd win.HWND) {
					window.Synchronize(func() {
						if ctx.Err() != nil {
							return
						}
						win.SetWindowLongPtr(hwnd, win.GWL_STYLE, (win.GetWindowLongPtr(hwnd, win.GWL_STYLE)&^uintptr(win.WS_POPUP))|win.WS_CHILD)
						win.SetParent(hwnd, frame.Handle())
						if win.GetParent(hwnd) != frame.Handle() {
							cmd.Process.Kill()
							return
						}
						child = hwnd
						fit()
						controls.SetEnabled(true)
					})
				})
			}
			if err != nil && diagnostic.Len() > 0 {
				err = fmt.Errorf("%w\n%s", err, diagnostic.String())
			}
		}
		if ctx.Err() != nil {
			return
		}
		window.Synchronize(func() {
			if ctx.Err() != nil {
				return
			}
			u.stopPreview()
			if err != nil {
				u.error(err)
			} else {
				u.status.SetText(u.t("Prévia concluída.", "Preview finished."))
			}
		})
	}()
}

// FFplay creates its SDL window after decoding starts. Keep it offscreen until
// it is attached and verify the process before handing its handle to the UI.
func attachPlayer(ctx context.Context, cmd *exec.Cmd, done <-chan error, caption string, attach func(win.HWND)) error {
	ticker := time.NewTicker(25 * time.Millisecond)
	defer ticker.Stop()
	timer := time.NewTimer(15 * time.Second)
	defer timer.Stop()
	name := syscall.StringToUTF16Ptr(caption)
	for {
		select {
		case err := <-done:
			return err
		case <-ctx.Done():
			return <-done
		case <-timer.C:
			cmd.Process.Kill()
			<-done
			return errors.New("Não foi possível iniciar o reprodutor embutido.")
		case <-ticker.C:
			h := win.FindWindow(nil, name)
			var pid uint32
			if h != 0 {
				win.GetWindowThreadProcessId(h, &pid)
			}
			if h != 0 && int(pid) == cmd.Process.Pid {
				attach(h)
				return <-done
			}
		}
	}
}
