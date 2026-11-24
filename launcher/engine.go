package main

import (
	"bufio"
	"bytes"
	"context"
	_ "embed"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows"
)

//go:embed worker.py
var worker string

type Clip struct {
	ID, Name, Status, Transcript, Thumbnail string
	Start, End, Peak, Score                 float64
	Reason                                  map[string]string
}
type Review struct {
	Project, Run string
	Meta         struct {
		Path     string
		Duration float64
	}
	Clips []Clip
}

func cacheDir() string {
	if path := os.Getenv("VODCUT_CACHE"); path != "" {
		return path
	}
	home, _ := os.UserHomeDir()
	return filepath.Join(home, "VODCut-data")
}

func lastReview() (Review, error) {
	var ref, project Review
	b, err := os.ReadFile(filepath.Join(cacheDir(), "last.json"))
	if errors.Is(err, os.ErrNotExist) {
		return project, nil
	}
	if err != nil {
		return project, err
	}
	if err = json.Unmarshal(b, &ref); err != nil {
		return project, err
	}
	if !filepath.IsLocal(ref.Project) || !filepath.IsLocal(ref.Run) {
		return project, errors.New("Revisão inválida")
	}
	f, err := os.Open(filepath.Join(cacheDir(), ref.Project, "review-"+ref.Run+".json"))
	if err != nil {
		return project, err
	}
	defer f.Close()
	err = json.NewDecoder(f).Decode(&project)
	return project, err
}

// Each operation owns its worker and all FFmpeg descendants, even if the window closes.
func runWorker(ctx context.Context, request map[string]any, progress func(string, int)) (string, error) {
	dir, err := install(ctx, progress)
	if err != nil {
		return "", err
	}
	if err = ctx.Err(); err != nil {
		return "", err
	}
	input, err := json.Marshal(request)
	if err != nil {
		return "", err
	}
	job, err := windows.CreateJobObject(nil, nil)
	if err != nil {
		return "", err
	}
	defer windows.CloseHandle(job)
	limits := windows.JOBOBJECT_EXTENDED_LIMIT_INFORMATION{}
	limits.BasicLimitInformation.LimitFlags = windows.JOB_OBJECT_LIMIT_KILL_ON_JOB_CLOSE
	if _, err = windows.SetInformationJobObject(job, windows.JobObjectExtendedLimitInformation, uintptr(unsafe.Pointer(&limits)), uint32(unsafe.Sizeof(limits))); err != nil {
		return "", err
	}

	cmd := interpreter(dir, "-u", "-c", worker)
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: windows.CREATE_NEW_CONSOLE}
	cmd.Stdin = bytes.NewReader(input)
	var diagnostic bytes.Buffer
	cmd.Stderr = &diagnostic
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return "", err
	}
	if err = cmd.Start(); err != nil {
		return "", err
	}
	handle, err := windows.OpenProcess(windows.PROCESS_TERMINATE|windows.PROCESS_SET_QUOTA, false, uint32(cmd.Process.Pid))
	if err == nil {
		err = windows.AssignProcessToJobObject(job, handle)
		windows.CloseHandle(handle)
	}
	if err != nil {
		cmd.Process.Kill()
		cmd.Wait()
		return "", err
	}
	exited := make(chan struct{})
	stopped := make(chan struct{})
	go func() {
		defer close(stopped)
		select {
		case <-ctx.Done():
			windows.TerminateJobObject(job, 1)
		case <-exited:
		}
	}()
	scan := bufio.NewScanner(stdout)
	var result, problem string
	var complete bool
	for scan.Scan() {
		var msg struct {
			Stage, Error string
			Result       *string
			Percent      int
		}
		if err = json.Unmarshal(scan.Bytes(), &msg); err != nil {
			problem = err.Error()
			continue
		}
		if msg.Error != "" {
			problem = msg.Error
		} else if msg.Stage != "" {
			progress(msg.Stage, msg.Percent)
		} else if msg.Result != nil {
			complete = true
			result = *msg.Result
		}
	}
	waitErr := cmd.Wait()
	close(exited)
	<-stopped
	if err = ctx.Err(); err != nil {
		return "", err
	}
	if problem != "" {
		return "", errors.New(problem)
	}
	if err = scan.Err(); err != nil {
		return "", err
	}
	if waitErr != nil {
		return "", fmt.Errorf("%v\n%s", waitErr, strings.TrimSpace(diagnostic.String()))
	}
	if !complete {
		return "", errors.New("O processamento terminou sem resposta.")
	}
	return result, nil
}
