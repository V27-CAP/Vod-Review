package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"syscall"

	"github.com/lxn/walk"
	. "github.com/lxn/walk/declarative"
	"golang.org/x/sys/windows"
)

type options struct {
	transcribe, audio, visual, facecam, voiceIsolation, judge bool
	audioTrack                                                int
	facecamROI                                                *FacecamROI
	min, max, limit                                           int
}

func formatTrackLabel(tr AudioTrack, pt bool) string {
	ch := fmt.Sprintf("%dch", tr.Channels)
	if tr.Channels == 1 {
		if pt {
			ch = "mono"
		} else {
			ch = "mono"
		}
	} else if tr.Channels == 2 {
		if pt {
			ch = "estéreo"
		} else {
			ch = "stereo"
		}
	}
	parts := []string{fmt.Sprintf("Faixa %d", tr.AudioIndex+1)}
	if !pt {
		parts[0] = fmt.Sprintf("Track %d", tr.AudioIndex+1)
	}
	if strings.TrimSpace(tr.Title) != "" {
		parts = append(parts, strings.TrimSpace(tr.Title))
	}
	if tr.Codec != "" {
		parts = append(parts, fmt.Sprintf("%s · %s", ch, tr.Codec))
	} else {
		parts = append(parts, ch)
	}
	return strings.Join(parts, " · ")
}

func (u *desk) analysisDialog(path string) {
	t := u.t
	var d *walk.Dialog
	var accept, cancel, probeBtn, facecamBtn, aiBtn *walk.PushButton
	var chat, video *walk.LineEdit
	var audio, transcribe, visual, voiceIso, facecamBox, judge *walk.CheckBox
	var trackBox *walk.ComboBox
	var facecamLabel, aiLabel *walk.Label
	var lo, hi, count *walk.NumberEdit

	var probed ProbeResult
	var hasProbe bool
	var trackIDs = []int{-1}
	var currentROI *FacecamROI
	if u.opts.facecamROI != nil {
		cp := *u.opts.facecamROI
		currentROI = &cp
	}

	num := func(ptr **walk.NumberEdit, value, limit int) NumberEdit {
		return NumberEdit{AssignTo: ptr, MinValue: 1, MaxValue: float64(limit), Value: float64(value), Decimals: 0}
	}
	updateFacecamText := func() {
		if facecamLabel != nil {
			facecamLabel.SetText(formatROI(currentROI, probed.Meta.Width, probed.Meta.Height, u.Lang != "en"))
		}
	}
	updateAIText := func() {
		if aiLabel != nil {
			aiLabel.SetText(u.aiSummary())
		}
	}
	applyTracks := func(tracks []AudioTrack) {
		labels := []string{t("Padrão / Mix principal (Faixa 1)", "Default / Primary mix (Track 1)")}
		trackIDs = []int{-1}
		selIdx := 0
		for _, tr := range tracks {
			labels = append(labels, formatTrackLabel(tr, u.Lang != "en"))
			trackIDs = append(trackIDs, tr.AudioIndex)
			if tr.AudioIndex == u.opts.audioTrack {
				selIdx = len(trackIDs) - 1
			}
		}
		if trackBox != nil {
			_ = trackBox.SetModel(labels)
			_ = trackBox.SetCurrentIndex(selIdx)
		}
	}
	runProbe := func(videoPath string, openFacecam bool) {
		clean := strings.Trim(strings.TrimSpace(videoPath), "\"")
		info, err := os.Stat(clean)
		if err != nil || !info.Mode().IsRegular() {
			if openFacecam {
				walk.MsgBox(d, title, t("Escolha primeiro um arquivo de vídeo válido.", "Choose a valid video file first."), walk.MsgBoxIconWarning)
			}
			return
		}
		if probeBtn != nil {
			probeBtn.SetEnabled(false)
		}
		if facecamBtn != nil {
			facecamBtn.SetEnabled(false)
		}
		go func() {
			res, perr := probeVideoSync(clean, nil, true)
			d.Synchronize(func() {
				if probeBtn != nil {
					probeBtn.SetEnabled(true)
				}
				if facecamBtn != nil {
					facecamBtn.SetEnabled(true)
				}
				if perr != nil {
					if openFacecam {
						walk.MsgBox(d, title, perr.Error(), walk.MsgBoxIconWarning)
					}
					return
				}
				probed = res
				hasProbe = true
				applyTracks(res.Meta.AudioTracks)
				updateFacecamText()
				if openFacecam {
					if updated, ok := u.facecamDialog(d, clean, probed, currentROI); ok {
						currentROI = updated
						if currentROI != nil && facecamBox != nil {
							facecamBox.SetChecked(true)
						}
						updateFacecamText()
					}
				}
			})
		}()
	}

	err := (Dialog{
		AssignTo:      &d,
		Title:         t("Analisar gravação", "Analyze recording"),
		DefaultButton: &accept,
		CancelButton:  &cancel,
		MinSize:       Size{Width: 560},
		Layout:        VBox{Margins: Margins{18, 14, 18, 14}, Spacing: 7},
		Children: []Widget{
			Label{Text: t("Vídeo da live", "Stream recording"), Font: font(14, true)},
			Composite{Layout: HBox{MarginsZero: true}, Children: []Widget{
				LineEdit{AssignTo: &video, Text: path},
				PushButton{Text: t("Procurar vídeo…", "Browse video…"), OnClicked: func() {
					f := walk.FileDialog{Title: t("Escolha a gravação", "Choose a recording"), Filter: "MP4, MKV, MOV|*.mp4;*.mkv;*.mov", FilePath: video.Text()}
					if ok, err := f.ShowOpen(d); err != nil {
						walk.MsgBox(d, title, err.Error(), walk.MsgBoxIconWarning)
					} else if ok {
						video.SetText(f.FilePath)
						hasProbe = false
						runProbe(f.FilePath, false)
					}
				}},
				PushButton{AssignTo: &probeBtn, Text: t("Ler faixas", "Read tracks"), OnClicked: func() {
					runProbe(video.Text(), false)
				}},
			}},
			Label{Text: t("Captura de fala do(a) streamer (OBS / filtro leve)", "Streamer speech capture (OBS / lightweight filter)"), Font: font(10, true)},
			Composite{Layout: HBox{MarginsZero: true}, Children: []Widget{
				Label{Text: t("Faixa de áudio do microfone (OBS):", "OBS microphone audio track:")},
				ComboBox{
					AssignTo:     &trackBox,
					Model:        []string{t("Padrão / Mix principal (Faixa 1)", "Default / Primary mix (Track 1)")},
					CurrentIndex: 0,
				},
			}},
			CheckBox{
				AssignTo: &voiceIso,
				Checked:  u.opts.voiceIsolation,
				Text:     t("Isolar fala do áudio do jogo (atenuação estéreo + filtro de frequência + denoise + Silero VAD)", "Isolate speech from game audio (stereo attenuation + bandpass + denoise + Silero VAD)"),
			},
			Label{Text: t("Quais sinais procurar?", "Which signals should be used?"), Font: font(10, true)},
			CheckBox{AssignTo: &audio, Checked: u.opts.audio, Text: t("Áudio · picos de volume, risadas e reações", "Audio · volume spikes, laughter and reactions")},
			CheckBox{AssignTo: &transcribe, Checked: u.opts.transcribe, Text: t("Transcrição · usa as falas para encontrar contexto", "Transcription · use speech to find context")},
			CheckBox{AssignTo: &visual, Checked: u.opts.visual, Text: t("Imagem geral · mudanças de cena e movimento", "Full video · scene changes and motion")},
			Composite{Layout: HBox{MarginsZero: true}, Children: []Widget{
				CheckBox{AssignTo: &facecamBox, Checked: u.opts.facecam, Text: t("Facecam · expressões e movimento facial", "Facecam · facial expression and movement")},
				HSpacer{},
				PushButton{AssignTo: &facecamBtn, Text: t("Marcar Facecam no vídeo…", "Select Facecam region…"), OnClicked: func() {
					clean := strings.Trim(strings.TrimSpace(video.Text()), "\"")
					if hasProbe && probed.Meta.Path != "" && strings.EqualFold(filepath.Clean(probed.Meta.Path), filepath.Clean(clean)) && probed.Frame != "" {
						if updated, ok := u.facecamDialog(d, clean, probed, currentROI); ok {
							currentROI = updated
							if currentROI != nil {
								facecamBox.SetChecked(true)
							}
							updateFacecamText()
						}
						return
					}
					runProbe(clean, true)
				}},
			}},
			Label{AssignTo: &facecamLabel, Text: formatROI(currentROI, 0, 0, u.Lang != "en"), NoPrefix: true},
			Composite{Layout: HBox{MarginsZero: true}, Children: []Widget{
				CheckBox{AssignTo: &judge, Checked: u.opts.judge, Text: t("IA · refinar categorias, contexto, descrição e hashtags", "AI · refine categories, context, description and hashtags")},
				HSpacer{},
				PushButton{AssignTo: &aiBtn, Text: t("Configurar IA…", "Configure AI…"), OnClicked: func() {
					if u.aiConfigDialog(d) {
						updateAIText()
					}
				}},
			}},
			Label{AssignTo: &aiLabel, Text: u.aiSummary(), NoPrefix: true},
			Label{Text: t("Chat da live (opcional)", "Stream chat (optional)")},
			Composite{Layout: HBox{MarginsZero: true}, Children: []Widget{
				LineEdit{AssignTo: &chat, Text: u.chat},
				PushButton{Text: t("Procurar…", "Browse…"), OnClicked: func() {
					dlg := walk.FileDialog{Title: t("Escolha o chat", "Choose chat file"), Filter: "Chat (*.json, *.csv)|*.json;*.csv"}
					if ok, e := dlg.ShowOpen(d); e != nil {
						u.error(e)
					} else if ok {
						chat.SetText(dlg.FilePath)
					}
				}},
			}},
			Composite{Layout: Grid{Columns: 2, MarginsZero: true}, Children: []Widget{
				Label{Text: t("Duração mínima do corte (s)", "Minimum clip length (s)")}, num(&lo, u.opts.min, 900),
				Label{Text: t("Duração máxima do corte (s)", "Maximum clip length (s)")}, num(&hi, u.opts.max, 900),
				Label{Text: t("Quantidade máxima de destaques", "Maximum highlights")}, num(&count, u.opts.limit, 250),
			}},
			Composite{Layout: HBox{MarginsZero: true}, Children: []Widget{
				HSpacer{},
				PushButton{AssignTo: &cancel, Text: t("Cancelar", "Cancel"), OnClicked: func() { d.Cancel() }},
				PushButton{AssignTo: &accept, Text: t("Analisar vídeo", "Analyze video"), OnClicked: func() {
					path = strings.Trim(strings.TrimSpace(video.Text()), "\"")
					if info, err := os.Stat(path); err != nil || !info.Mode().IsRegular() {
						walk.MsgBox(d, title, t("Escolha um arquivo MP4, MKV ou MOV no computador.", "Choose an MP4, MKV or MOV on your computer."), walk.MsgBoxIconWarning)
						return
					}
					if lo.Value() > hi.Value() {
						walk.MsgBox(d, title, t("O mínimo precisa ser menor ou igual ao máximo.", "Minimum length must be at most the maximum."), walk.MsgBoxIconWarning)
						return
					}
					if facecamBox.Checked() && currentROI == nil {
						walk.MsgBox(d, title, t("Marque a região da Facecam no vídeo ou desmarque a opção Facecam.", "Mark the Facecam region on the video or uncheck Facecam."), walk.MsgBoxIconWarning)
						return
					}
					if !audio.Checked() && !transcribe.Checked() && !visual.Checked() && !facecamBox.Checked() && strings.TrimSpace(chat.Text()) == "" {
						walk.MsgBox(d, title, t("Selecione um sinal ou adicione o chat.", "Select a signal or add a chat file."), walk.MsgBoxIconWarning)
						return
					}
					selectedTrack := -1
					if idx := trackBox.CurrentIndex(); idx >= 0 && idx < len(trackIDs) {
						selectedTrack = trackIDs[idx]
					}
					var savedROI *FacecamROI
					if currentROI != nil {
						cp := *currentROI
						savedROI = &cp
					}
					u.opts = options{
						transcribe:     transcribe.Checked(),
						audio:          audio.Checked(),
						visual:         visual.Checked(),
						facecam:        facecamBox.Checked(),
						voiceIsolation: voiceIso.Checked(),
						judge:          judge.Checked(),
						audioTrack:     selectedTrack,
						facecamROI:     savedROI,
						min:            int(lo.Value()),
						max:            int(hi.Value()),
						limit:          int(count.Value()),
					}
					u.source, u.chat = path, strings.TrimSpace(chat.Text())
					d.Accept()
				}},
			}},
		},
	}).Create(u.mw)
	if err != nil {
		u.error(err)
		return
	}
	d.Starting().Attach(func() {
		if strings.TrimSpace(path) != "" {
			runProbe(path, false)
		}
	})
	if d.Run() != walk.DlgCmdOK {
		return
	}
	o := u.opts
	reqOpts := map[string]any{
		"path":            u.source,
		"chat":            u.chat,
		"transcription":   o.transcribe,
		"audio":           o.audio,
		"visual":          o.visual,
		"facecam":         o.facecam,
		"voice_isolation": o.voiceIsolation,
		"audio_track":     o.audioTrack,
		"judge":           o.judge,
		"min_seconds":     o.min,
		"max_seconds":     o.max,
		"top_n":           o.limit,
		"ai": map[string]any{
			"provider": u.ai.Provider,
			"endpoint": u.ai.Endpoint,
			"model":    u.ai.Model,
			"api_key":  u.ai.APIKey,
			"vision":   u.ai.Vision,
		},
	}
	if o.facecam && o.facecamROI != nil {
		reqOpts["facecam_roi"] = map[string]any{
			"x": o.facecamROI.X,
			"y": o.facecamROI.Y,
			"w": o.facecamROI.W,
			"h": o.facecamROI.H,
		}
	}
	u.operate(map[string]any{"action": "analyze", "options": reqOpts})
}

func (u *desk) exportDialog() {
	var d *walk.Dialog
	var ok, cancel *walk.PushButton
	var vertical *walk.CheckBox
	var checks [5]*walk.CheckBox
	formats := []string{"xml", "posts", "mp4", "csv", "json"}
	labels := []string{
		"Adobe Premiere Pro  ·  XML",
		u.t("Posts prontos (descrição + contexto + hashtags)  ·  posts.txt", "Ready-to-share posts (description + context + hashtags)  ·  posts.txt"),
		u.t("Vídeos individuais  ·  MP4", "Individual videos  ·  MP4"),
		u.t("Planilha de cortes (com categorias e posts)  ·  CSV", "Clip spreadsheet (with categories & posts)  ·  CSV"),
		u.t("Dados dos cortes (com categorias e posts)  ·  JSON", "Clip data (with categories & posts)  ·  JSON"),
	}
	children := []Widget{Label{Text: u.t("Exportar cortes aprovados", "Export approved clips"), Font: font(14, true)}}
	for i, label := range labels {
		children = append(children, CheckBox{AssignTo: &checks[i], Text: label, Checked: i == 0 || i == 1})
	}
	var selected []string
	portrait := false
	children = append(children, CheckBox{AssignTo: &vertical, Text: u.t("Sequência do Premiere em 1080 × 1920", "1080 × 1920 Premiere sequence")},
		Composite{Layout: HBox{MarginsZero: true}, Children: []Widget{HSpacer{}, PushButton{AssignTo: &cancel, Text: u.t("Cancelar", "Cancel"), OnClicked: func() { d.Cancel() }},
			PushButton{AssignTo: &ok, Text: u.t("Exportar", "Export"), OnClicked: func() {
				for i, c := range checks {
					if c.Checked() {
						selected = append(selected, formats[i])
					}
				}
				if len(selected) == 0 {
					walk.MsgBox(d, title, u.t("Escolha um formato.", "Choose a format."), walk.MsgBoxIconWarning)
					return
				}
				portrait = vertical.Checked()
				d.Accept()
			}}}})
	choice, err := (Dialog{AssignTo: &d, Title: u.t("Exportação", "Export"), MinSize: Size{Width: 440}, Layout: VBox{}, DefaultButton: &ok, CancelButton: &cancel, Children: children}).Run(u.mw)
	if err != nil {
		u.error(err)
	} else if choice == walk.DlgCmdOK {
		u.operate(map[string]any{"action": "export", "formats": selected, "vertical": portrait})
	}
}

func (u *desk) stage(s string) string {
	labels := map[string][2]string{
		"probe":      {"Lendo vídeo", "Reading recording"},
		"install":    {"Preparando ferramentas pela primeira vez", "Preparing tools for first use"},
		"extract":    {"Extraindo áudio", "Extracting audio"},
		"transcribe": {"Transcrevendo", "Transcribing"},
		"audio":      {"Analisando áudio", "Analyzing audio"},
		"visual":     {"Analisando imagem", "Analyzing video"},
		"facecam":    {"Analisando facecam", "Analyzing facecam"},
		"chat":       {"Lendo chat", "Reading chat"},
		"rank":       {"Selecionando destaques", "Ranking highlights"},
		"judge":      {"Avaliando com IA", "Scoring with AI"},
		"export":     {"Exportando", "Exporting"},
		"render":     {"Gerando vídeos", "Rendering videos"},
		"preview":    {"Preparando prévia", "Preparing preview"},
	}
	if label, ok := labels[s]; ok {
		return u.t(label[0], label[1])
	}
	return s
}

func (u *desk) stopOperation() {
	if err := os.WriteFile(u.cancelPath, nil, 0600); err != nil {
		u.error(err)
		return
	}
	u.stop.SetEnabled(false)
	u.status.SetText(u.t("Parando…", "Stopping…"))
}

func (u *desk) operate(req map[string]any) {
	if u.busy {
		return
	}
	tmp, err := os.MkdirTemp("", "vodcut-task-")
	if err != nil {
		u.error(err)
		return
	}
	u.cancelPath = filepath.Join(tmp, "cancel")
	req["project"], req["run"], req["cancel"] = u.project.Project, u.project.Run, u.cancelPath
	var activeClip Clip
	if c := u.clip(); c != nil {
		activeClip = *c
	}
	ctx, cancel := context.WithCancel(context.Background())
	u.cancelTask = cancel
	u.busy = true
	u.controls()
	u.stop.SetEnabled(true)
	u.progress.SetValue(0)
	u.status.SetText(u.t("Processando…", "Processing…"))
	go func() {
		defer cancel()
		defer os.RemoveAll(tmp)
		result, err := runWorker(ctx, req, func(stage string, p int) {
			u.mw.Synchronize(func() {
				if !u.stop.Enabled() {
					if stage == "install" {
						cancel()
					}
					return
				}
				u.status.SetText(fmt.Sprintf("%s   %d%%", u.stage(stage), p))
				u.progress.SetValue(p)
			})
		})
		var project Review
		reload := err == nil && (req["action"] == "edit" || req["action"] == "analyze")
		if reload {
			project, err = lastReview()
		}
		u.mw.Synchronize(func() {
			u.busy = false
			u.controls()
			if err != nil {
				if errors.Is(err, context.Canceled) || strings.Contains(err.Error(), "Cancelado") {
					u.status.SetText(u.t("Processamento cancelado.", "Processing cancelled."))
					return
				}
				u.status.SetText(u.t("O processamento foi interrompido.", "Processing stopped."))
				u.error(err)
				return
			}
			if reload {
				if req["action"] == "analyze" {
					u.index = 0
					u.filterCat = "all"
					if u.catFilter != nil {
						_ = u.catFilter.SetCurrentIndex(0)
					}
				}
				u.project = project
				u.populate()
				return
			}
			u.status.SetText(u.t("Concluído.", "Completed."))
			if req["action"] == "preview" {
				var prevRes PreviewResult
				if jerr := json.Unmarshal([]byte(result), &prevRes); jerr != nil || prevRes.Path == "" {
					prevRes = PreviewResult{Path: result, PreviewStart: activeClip.Start, PreviewEnd: activeClip.End}
				}
				u.playPreview(prevRes, activeClip)
				return
			}
			if walk.MsgBox(u.mw, title, u.t("Exportação concluída. Abrir a pasta?", "Export complete. Open the folder?")+"\n\n"+result, walk.MsgBoxYesNo|walk.MsgBoxIconInformation) == walk.DlgCmdYes {
				u.openFile(result)
			}
		})
	}()
}

func (u *desk) openFile(path string) {
	if err := windows.ShellExecute(windows.Handle(u.mw.Handle()), nil, syscall.StringToUTF16Ptr(path), nil, nil, 1); err != nil {
		u.error(err)
	}
}
