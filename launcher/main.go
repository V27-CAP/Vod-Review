// VODCut's Windows launcher expands its embedded distribution once per release.
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
	"runtime"
	"strconv"

	"syscall"
	"unsafe"
)

var payloadHash string
var payloadSize string
var kernel = syscall.NewLazyDLL("kernel32.dll")

func installed(dir string) bool {
	marker, err := os.ReadFile(filepath.Join(dir, ".installed"))
	return err == nil && string(marker) == payloadHash
}

func extractFile(entry *zip.File, dest string) error {
	name := filepath.FromSlash(entry.Name)
	if !filepath.IsLocal(name) || !entry.Mode().IsRegular() {
		return fmt.Errorf("arquivo inválido no pacote: %s", entry.Name)
	}
	path := filepath.Join(dest, name)
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return err
	}
	in, err := entry.Open()
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0644)
	if err != nil {
		return err
	}
	_, err = io.Copy(out, in)
	return errors.Join(err, out.Close())
}

func unpack(ctx context.Context, dest string, progress func(string, int)) error {
	fileName, err := os.Executable()
	if err != nil {
		return err
	}
	source, err := os.Open(fileName)
	if err != nil {
		return err
	}
	defer source.Close()
	stat, err := source.Stat()
	if err != nil {
		return err
	}
	size, err := strconv.ParseInt(payloadSize, 10, 64)
	if err != nil || size <= 0 || stat.Size() < size {
		return errors.New("pacote incompleto")
	}
	bytes := io.NewSectionReader(source, stat.Size()-size, size)
	hash := sha256.New()
	if _, err = io.Copy(hash, bytes); err != nil {
		return err
	}
	if hex.EncodeToString(hash.Sum(nil)) != payloadHash {
		return errors.New("arquivo incompleto. Baixe o executável novamente")
	}
	archive, err := zip.NewReader(bytes, size)
	if err != nil {
		return err
	}
	var total, done uint64
	for _, entry := range archive.File {
		total += entry.UncompressedSize64
	}
	next := 10
	for _, entry := range archive.File {
		if err := ctx.Err(); err != nil {
			return err
		}
		if err := extractFile(entry, dest); err != nil {
			return fmt.Errorf("%s: %w", entry.Name, err)
		}
		done += entry.UncompressedSize64
		if int(done*100/total) >= next {
			progress("install", next)
			next += 10
		}
	}
	return os.WriteFile(filepath.Join(dest, ".installed"), []byte(payloadHash), 0644)
}

func install(ctx context.Context, progress func(string, int)) (string, error) {
	if len(payloadHash) != 64 {
		return "", errors.New("Este executável precisa do pacote de ferramentas. Use a versão de downloads.")
	}
	base, err := os.UserCacheDir()
	if err != nil {
		return "", err
	}
	base = filepath.Join(base, "VODCutStudio")
	release := filepath.Join(base, "app-"+payloadHash[:12])
	if installed(release) {
		return release, nil
	}
	if err = os.MkdirAll(base, 0755); err != nil {
		return "", err
	}
	// A named Win32 mutex also recovers cleanly when the first launch is interrupted.
	name, _ := syscall.UTF16PtrFromString("Local\\VODCutStudio-Install-" + payloadHash[:12])
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	handle, _, callErr := kernel.NewProc("CreateMutexW").Call(0, 0, uintptr(unsafe.Pointer(name)))
	if handle == 0 {
		return "", callErr
	}
	defer syscall.CloseHandle(syscall.Handle(handle))
	wait, _, callErr := kernel.NewProc("WaitForSingleObject").Call(handle, 0xffffffff)
	if wait != 0 && wait != 0x80 {
		return "", callErr
	}
	defer kernel.NewProc("ReleaseMutex").Call(handle)
	if installed(release) {
		return release, nil
	}
	stage := release + ".preparing"
	if err = os.RemoveAll(stage); err != nil {
		return "", err
	}
	if err = os.Mkdir(stage, 0755); err != nil {
		return "", err
	}
	defer os.RemoveAll(stage)
	progress("install", 0)
	if err = unpack(ctx, stage, progress); err != nil {
		return "", err
	}
	if err = os.Rename(stage, release); err != nil {
		return "", err
	}
	return release, nil
}

func interpreter(dir string, args ...string) *exec.Cmd {
	python := filepath.Join(dir, "python", "python.exe")
	cmd := exec.Command(python, append([]string{"-X", "utf8", "-B"}, args...)...)
	cmd.Env = append(os.Environ(), "PATH="+filepath.Join(dir, "bin")+";"+filepath.Dir(python)+";"+os.Getenv("PATH"),
		"KMP_AFFINITY=disabled", "KMP_TOPOLOGY_METHOD=flat", "HF_HUB_OFFLINE=1", "HF_HUB_DISABLE_TELEMETRY=1", "PYTHONUTF8=1", "PYTHONDONTWRITEBYTECODE=1")
	return cmd
}

func main() {
	if len(os.Args) > 1 {
		dir, err := install(context.Background(), func(string, int) {})
		if err == nil {
			args, module := os.Args[1:], "vodcut"
			if args[0] == "evaluate" {
				module = "vodcut.evaluate"
				args = args[1:]
			}
			cmd := interpreter(dir, append([]string{"-m", module}, args...)...)
			cmd.Stdin, cmd.Stdout, cmd.Stderr = os.Stdin, os.Stdout, os.Stderr
			err = cmd.Run()
		}
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		return
	}
	if err := desktop(); err != nil {
		message(err.Error())
		os.Exit(1)
	}
}
