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

type AudioTrack struct {
	StreamIndex int    `json:"stream_index"`
	AudioIndex  int    `json:"audio_index"`
	Channels    int    `json:"channels"`
	SampleRate  int    `json:"sample_rate"`
	Codec       string `json:"codec"`
	Title       string `json:"title"`
	Language    string `json:"language"`
}

type FacecamROI struct {
	X float64 `json:"x"`
	Y float64 `json:"y"`
	W float64 `json:"w"`
	H float64 `json:"h"`
}

type VideoMeta struct {
	Path        string       `json:"path"`
	Duration    float64      `json:"duration"`
	Width       int          `json:"width"`
	Height      int          `json:"height"`
	Channels    int          `json:"channels"`
	AudioTracks []AudioTrack `json:"audio_tracks"`
	Facecam     *FacecamROI  `json:"facecam,omitempty"`
}

type ProbeResult struct {
	Meta      VideoMeta `json:"meta"`
	Frame     string    `json:"frame"`
	Timestamp float64   `json:"timestamp"`
}

type PreviewResult struct {
	Path         string  `json:"path"`
	PreviewStart float64 `json:"preview_start"`
	PreviewEnd   float64 `json:"preview_end"`
}

type Clip struct {
	ID          string            `json:"id"`
	Name        string            `json:"name"`
	Status      string            `json:"status"`
	Transcript  string            `json:"transcript"`
	Thumbnail   string            `json:"thumbnail"`
	Start       float64           `json:"start"`
	End         float64           `json:"end"`
	Peak        float64           `json:"peak"`
	Score       float64           `json:"score"`
	Reason      map[string]string `json:"reason"`
	Category    string            `json:"category"`
	Categories  []string          `json:"categories"`
	ContextNote string            `json:"context_note"`
	Description string            `json:"description"`
	Hashtags    []string          `json:"hashtags"`
}

type Review struct {
	Project string    `json:"project"`
	Run     string    `json:"run"`
	Meta    VideoMeta `json:"meta"`
	Clips   []Clip    `json:"clips"`
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
	scan.Buffer(make([]byte, 64*1024), 4*1024*1024)
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
