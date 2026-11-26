package main

import (
	"context"
	"fmt"
	"math"
	"strconv"
	"strings"

	"syscall"
	"time"
	"unsafe"

	"github.com/lxn/walk"
	. "github.com/lxn/walk/declarative"
)

var mciSend = syscall.NewLazyDLL("winmm.dll").NewProc("mciSendStringW")

func mci(command string) bool {
	text, err := syscall.UTF16PtrFromString(command)
	if err != nil {
		return false
	}
	code, _, _ := mciSend.Call(uintptr(unsafe.Pointer(text)), 0, 0, 0)
	return code == 0
}

func mciQuery(command string) (string, bool) {
	text, err := syscall.UTF16PtrFromString(command)
	if err != nil {
		return "", false
	}
	buf := make([]uint16, 256)
	code, _, _ := mciSend.Call(uintptr(unsafe.Pointer(text)), uintptr(unsafe.Pointer(&buf[0])), uintptr(len(buf)), 0)
	if code != 0 {
		return "", false
	}
	return syscall.UTF16ToString(buf), true
}

func clockPrecise(sec float64) string {
	if sec < 0 {
		sec = 0
	}
	totalTenths := int(math.Round(sec * 10))
	tenths := totalTenths % 10
	s := totalTenths / 10
	return fmt.Sprintf("%02d:%02d:%02d.%d", s/3600, (s/60)%60, s%60, tenths)
}

func round3(v float64) float64 {
	return math.Round(v*1000) / 1000
}

func (u *desk) stopPreview() {
	if u.previewCancel == nil {
		return
	}
	cancel := u.previewCancel
	u.previewCancel = nil
	cancel()
	u.previewHost.SetVisible(false)
	u.image.SetVisible(true)
	u.preview.SetText(u.t("Reproduzir", "Play"))
}

func (u *desk) playPreview(res PreviewResult, clip Clip) {
	t := u.t
	previewStart := res.PreviewStart
	previewEnd := res.PreviewEnd
	if previewEnd <= previewStart {
		previewStart = clip.Start
		previewEnd = clip.End
	}
	clipStart := u.start.Value()
	clipEnd := u.end.Value()
	clipPeak := clip.Peak
	totalDuration := u.project.Meta.Duration

	var panel, nativeControls *walk.Composite
	var frame *walk.Composite
	var timeLabel, markLabel *walk.Label
	var seekSlider, volSlider *walk.Slider
	var playPauseBtn, muteBtn *walk.PushButton
	var speedBox *walk.ComboBox

	speedLabels := []string{"0.5x", "0.75x", "1.0x", "1.25x", "1.5x", "2.0x"}
	speedValues := []int{500, 750, 1000, 1250, 1500, 2000}

	lengthMs := int(math.Round((previewEnd - previewStart) * 1000))
	if lengthMs < 100 {
		lengthMs = 100
	}
	sliderMax := maxInt(1, lengthMs/100) // deciseconds (0.1s resolution)

	playing := true
	muted := false
	currentSpeed := 1000
	currentVolume := 100
	syncing := false
	currentPosMs := int(math.Round(math.Max(0, clipStart-previewStart) * 1000))
	if currentPosMs > lengthMs {
		currentPosMs = 0
	}

	fit := func() {
		if frame != nil {
			r := u.videoBounds(frame)
			mci(fmt.Sprintf("put vodcutpreview window at %d %d %d %d", r.X, r.Y, r.Width, r.Height))
		}
	}
	applyAudio := func() {
		mci(fmt.Sprintf("setaudio vodcutpreview volume to %d", currentVolume*10))
		if muted {
			mci("setaudio vodcutpreview off")
		} else {
			mci("setaudio vodcutpreview on")
		}
	}
	applySpeed := func() {
		mci(fmt.Sprintf("set vodcutpreview speed %d", currentSpeed))
	}
	updateReadout := func(posMs int) {
		if timeLabel == nil {
			return
		}
		vodPos := previewStart + float64(posMs)/1000.0
		timeLabel.SetText(fmt.Sprintf(
			t("VOD: %s / %s   ·   Corte: %s – %s   ·   Pico: %s",
				"VOD: %s / %s   ·   Clip: %s – %s   ·   Peak: %s"),
			clockPrecise(vodPos),
			clockPrecise(previewEnd),
			clock(clipStart),
			clock(clipEnd),
			clock(clipPeak),
		))
	}
	updatePlayButton := func() {
		if playPauseBtn == nil {
			return
		}
		if playing {
			playPauseBtn.SetText(t("Pausar (Espaço)", "Pause (Space)"))
		} else {
			playPauseBtn.SetText(t("Reproduzir (Espaço)", "Play (Space)"))
		}
	}
	refocusSeek := func() {
		if seekSlider != nil {
			_ = seekSlider.SetFocus()
		}
	}
	seekToMs := func(targetMs int) {
		if targetMs < 0 {
			targetMs = 0
		}
		if targetMs > lengthMs {
			targetMs = lengthMs
		}
		currentPosMs = targetMs
		if playing {
			mci(fmt.Sprintf("play vodcutpreview from %d", targetMs))
			applySpeed()
		} else {
			mci(fmt.Sprintf("seek vodcutpreview to %d", targetMs))
		}
		syncing = true
		if seekSlider != nil {
			seekSlider.SetValue(targetMs / 100)
		}
		syncing = false
		updateReadout(targetMs)
	}
	togglePlay := func() {
		mode, ok := mciQuery("status vodcutpreview mode")
		isPlayingNow := playing
		if ok {
			isPlayingNow = strings.EqualFold(strings.TrimSpace(mode), "playing")
		}
		if isPlayingNow {
			mci("pause vodcutpreview")
			playing = false
		} else {
			if currentPosMs >= lengthMs-150 {
				currentPosMs = 0
				mci("play vodcutpreview from 0")
			} else {
				mci("play vodcutpreview")
			}
			applySpeed()
			playing = true
		}
		updatePlayButton()
	}
	jumpRelative := func(deltaMs int) {
		if posStr, ok := mciQuery("status vodcutpreview position"); ok {
			if ms, err := strconv.Atoi(strings.TrimSpace(posStr)); err == nil {
				currentPosMs = ms
			}
		}
		seekToMs(currentPosMs + deltaMs)
		refocusSeek()
	}
	handleKey := func(key walk.Key) {
		switch key {
		case walk.KeySpace:
			togglePlay()
		case walk.KeyLeft:
			jumpRelative(-5000)
		case walk.KeyRight:
			jumpRelative(5000)
		}
	}
	currentVodTime := func() float64 {
		if posStr, ok := mciQuery("status vodcutpreview position"); ok {
			if ms, err := strconv.Atoi(strings.TrimSpace(posStr)); err == nil {
				currentPosMs = ms
			}
		}
		return round3(previewStart + float64(currentPosMs)/1000.0)
	}

	u.stopPreview()
	view := Composite{AssignTo: &panel, Layout: VBox{MarginsZero: true, Spacing: 3},
		Children: []Widget{
			Composite{
				AssignTo:      &frame,
				Background:    solid(walk.RGB(16, 16, 18)),
				MinSize:       Size{Height: 170},
				StretchFactor: 4,
				Layout:        VBox{},
				OnSizeChanged: fit,
				OnKeyDown:     handleKey,
			},
			Composite{AssignTo: &nativeControls, Layout: VBox{MarginsZero: true, Spacing: 2}, Children: []Widget{Label{
				AssignTo: &timeLabel,
				Font:     font(9, true),
				NoPrefix: true,
				Text:     "",
			},
				Slider{
					AssignTo:  &seekSlider,
					MinValue:  0,
					MaxValue:  sliderMax,
					Value:     currentPosMs / 100,
					OnKeyDown: handleKey,
					OnValueChanged: func() {
						if syncing {
							return
						}
						seekToMs(seekSlider.Value() * 100)
					},
				},
				Composite{Layout: HBox{MarginsZero: true, Spacing: 6}, Children: []Widget{
					PushButton{AssignTo: &playPauseBtn, Text: t("Pausar (Espaço)", "Pause (Space)"), OnClicked: func() {
						togglePlay()
						refocusSeek()
					}},
					PushButton{Text: "-5s", OnClicked: func() { jumpRelative(-5000) }},
					PushButton{Text: "+5s", OnClicked: func() { jumpRelative(5000) }},
					PushButton{Text: t("Ir ao início", "Jump to start"), OnClicked: func() {
						seekToMs(int(math.Round(math.Max(0, clipStart-previewStart) * 1000)))
						refocusSeek()
					}},
					PushButton{Text: t("Ir ao pico", "Jump to peak"), OnClicked: func() {
						seekToMs(int(math.Round(math.Max(0, clipPeak-previewStart) * 1000)))
						refocusSeek()
					}},
					HSpacer{},
				}},
				Composite{Layout: HBox{MarginsZero: true, Spacing: 8}, Children: []Widget{
					Label{Text: t("Velocidade:", "Speed:")},
					ComboBox{
						AssignTo:     &speedBox,
						Model:        speedLabels,
						CurrentIndex: 2, // 1.0x
						MaxSize:      Size{Width: 78},
						OnCurrentIndexChanged: func() {
							idx := speedBox.CurrentIndex()
							if idx >= 0 && idx < len(speedValues) {
								currentSpeed = speedValues[idx]
								applySpeed()
							}
							refocusSeek()
						},
					},
					PushButton{AssignTo: &muteBtn, Text: t("Mudo", "Mute"), OnClicked: func() {
						muted = !muted
						if muted {
							muteBtn.SetText(t("Som", "Unmute"))
						} else {
							muteBtn.SetText(t("Mudo", "Mute"))
						}
						applyAudio()
						refocusSeek()
					}},
					Label{Text: t("Volume:", "Volume:")},
					Slider{
						AssignTo:  &volSlider,
						MinValue:  0,
						MaxValue:  100,
						Value:     100,
						MaxSize:   Size{Width: 140},
						OnKeyDown: handleKey,
						OnValueChanged: func() {
							currentVolume = volSlider.Value()
							if currentVolume > 0 && muted {
								muted = false
								muteBtn.SetText(t("Mudo", "Mute"))
							}
							applyAudio()
						},
					},
					HSpacer{},
				}},
				Composite{Layout: HBox{MarginsZero: true, Spacing: 6}, Children: []Widget{
					PushButton{Text: t("Marcar início aqui", "Mark start here"), OnClicked: func() {
						vodTime := currentVodTime()
						if vodTime > clipPeak {
							walk.MsgBox(u.mw, title, t("O início precisa ficar antes ou no pico do corte.", "Start must be at or before the clip peak."), walk.MsgBoxIconWarning)
							refocusSeek()
							return
						}
						clipStart = vodTime
						_ = u.start.SetValue(clipStart)
						updateReadout(currentPosMs)
						markLabel.SetText(fmt.Sprintf(t("Início marcado em %s · Salve para aplicar.", "Start marked at %s · Save to apply."), clockPrecise(clipStart)))
						refocusSeek()
					}},
					PushButton{Text: t("Marcar fim aqui", "Mark end here"), OnClicked: func() {
						vodTime := currentVodTime()
						if vodTime <= clipPeak || vodTime > totalDuration+0.0001 {
							walk.MsgBox(u.mw, title, t("O fim precisa ficar após o pico e dentro da duração do vídeo.", "End must be after the clip peak and inside the video duration."), walk.MsgBoxIconWarning)
							refocusSeek()
							return
						}
						clipEnd = vodTime
						_ = u.end.SetValue(clipEnd)
						updateReadout(currentPosMs)
						markLabel.SetText(fmt.Sprintf(t("Fim marcado em %s · Salve para aplicar.", "End marked at %s · Save to apply."), clockPrecise(clipEnd)))
						refocusSeek()
					}},
					HSpacer{},
				}},
				Label{
					AssignTo: &markLabel,
					NoPrefix: true,
					Text:     t("Dica: Espaço pausa/reproduz · Setas ←/→ pulam -5s/+5s.", "Tip: Space plays/pauses · Left/Right arrows seek -5s/+5s."),
				},
			},
			}}}
	widgets := []Widget{view}
	u.skin(widgets, u.colors().panel, nil)
	err := widgets[0].Create(NewBuilder(u.previewHost))
	if err != nil {
		u.error(err)
		return
	}
	ctx, cancel := context.WithCancel(context.Background())
	opened := false
	u.previewCancel = func() {
		cancel()
		if opened {
			mci("close vodcutpreview")
		}
		panel.Dispose()
	}
	u.image.SetVisible(false)
	u.previewHost.SetVisible(true)
	u.preview.SetText(t("Parar prévia", "Stop preview"))
	if !mci(fmt.Sprintf("open \"%s\" type mpegvideo alias vodcutpreview parent %d style child", res.Path, frame.Handle())) {
		nativeControls.SetVisible(false)
		u.portablePreview(ctx, frame, panel, res.Path, math.Max(0, clipStart-previewStart))
		return
	}
	opened = true

	mci("set vodcutpreview time format milliseconds")
	if lenStr, ok := mciQuery("status vodcutpreview length"); ok {
		if ms, err := strconv.Atoi(strings.TrimSpace(lenStr)); err == nil && ms > 100 {
			lengthMs = ms
			sliderMax = maxInt(1, lengthMs/100)
			seekSlider.SetRange(0, sliderMax)
		}
	}

	fit()
	mci("window vodcutpreview state show")
	applyAudio()
	applySpeed()
	if currentPosMs > 0 && currentPosMs < lengthMs {
		mci(fmt.Sprintf("play vodcutpreview from %d", currentPosMs))
	} else {
		currentPosMs = 0
		mci("play vodcutpreview from 0")
	}
	updateReadout(currentPosMs)
	updatePlayButton()
	refocusSeek()

	window := u.mw
	go func() {
		ticker := time.NewTicker(150 * time.Millisecond)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				if ctx.Err() != nil {
					return
				}
				window.Synchronize(func() {
					if ctx.Err() != nil {
						return
					}
					posStr, okPos := mciQuery("status vodcutpreview position")
					modeStr, okMode := mciQuery("status vodcutpreview mode")
					if okMode {
						isP := strings.EqualFold(strings.TrimSpace(modeStr), "playing")
						if isP != playing {
							playing = isP
							updatePlayButton()
						}
					}
					if okPos {
						if ms, err := strconv.Atoi(strings.TrimSpace(posStr)); err == nil {
							currentPosMs = ms
							syncing = true
							val := ms / 100
							if val < 0 {
								val = 0
							}
							if val > sliderMax {
								val = sliderMax
							}
							seekSlider.SetValue(val)
							syncing = false
							updateReadout(ms)
						}
					}
				})
			}
		}
	}()
}
