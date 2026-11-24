package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"unsafe"

	"github.com/lxn/walk"
	. "github.com/lxn/walk/declarative"
	"golang.org/x/sys/windows"
)

type options struct {
	transcribe, audio, visual, judge bool
	min, max, limit                  int
}

func (u *desk) analysisDialog(path string) {
	t := u.t
	var d *walk.Dialog
	var accept, cancel *walk.PushButton
	var chat, video *walk.LineEdit
	var audio, transcribe, visual, judge *walk.CheckBox
	var lo, hi, count *walk.NumberEdit
	num := func(ptr **walk.NumberEdit, value, limit int) NumberEdit {
		return NumberEdit{AssignTo: ptr, MinValue: 1, MaxValue: float64(limit), Value: float64(value), Decimals: 0}
	}
	ok, err := (Dialog{AssignTo: &d, Title: t("Analisar gravação", "Analyze recording"), DefaultButton: &accept, CancelButton: &cancel, MinSize: Size{Width: 470}, Layout: VBox{Margins: Margins{18, 14, 18, 14}, Spacing: 8}, Children: []Widget{
		Label{Text: t("Vídeo da live", "Stream recording"), Font: font(15, true)},
		Composite{Layout: HBox{MarginsZero: true}, Children: []Widget{LineEdit{AssignTo: &video, Text: path}, PushButton{Text: t("Procurar vídeo…", "Browse video…"), OnClicked: func() {
			f := walk.FileDialog{Title: t("Escolha a gravação", "Choose a recording"), Filter: "MP4, MKV, MOV|*.mp4;*.mkv;*.mov", FilePath: video.Text()}
			if ok, err := f.ShowOpen(d); err != nil {
				walk.MsgBox(d, title, err.Error(), walk.MsgBoxIconWarning)
			} else if ok {
				video.SetText(f.FilePath)
			}
		}}}},
		Label{Text: t("Quais sinais procurar?", "Which signals should be used?"), Font: font(10, true)},
		CheckBox{AssignTo: &audio, Checked: u.opts.audio, Text: t("Áudio · picos de volume, risadas e reações", "Audio · volume spikes, laughter and reactions")},
		CheckBox{AssignTo: &transcribe, Checked: u.opts.transcribe, Text: t("Transcrição · usa as falas para encontrar contexto", "Transcription · use speech to find context")},
		CheckBox{AssignTo: &visual, Checked: u.opts.visual, Text: t("Imagem · mudanças de cena e movimento", "Video · scene changes and motion")},
		CheckBox{AssignTo: &judge, Checked: u.opts.judge, Text: t("Avaliação extra com meu modelo de IA local", "Extra scoring with my local AI model")},
		Label{Text: t("Chat da live (opcional)", "Stream chat (optional)")},
		Composite{Layout: HBox{MarginsZero: true}, Children: []Widget{LineEdit{AssignTo: &chat, Text: u.chat}, PushButton{Text: t("Procurar…", "Browse…"), OnClicked: func() {
			dlg := walk.FileDialog{Title: t("Escolha o chat", "Choose chat file"), Filter: "Chat (*.json, *.csv)|*.json;*.csv"}
			if ok, e := dlg.ShowOpen(d); e != nil {
				u.error(e)
			} else if ok {
				chat.SetText(dlg.FilePath)
			}
		}}}},
		Composite{Layout: Grid{Columns: 2, MarginsZero: true}, Children: []Widget{
			Label{Text: t("Duração mínima do corte (s)", "Minimum clip length (s)")}, num(&lo, u.opts.min, 900),
			Label{Text: t("Duração máxima do corte (s)", "Maximum clip length (s)")}, num(&hi, u.opts.max, 900),
			Label{Text: t("Quantidade máxima de destaques", "Maximum highlights")}, num(&count, u.opts.limit, 250),
		}},
		Composite{Layout: HBox{MarginsZero: true}, Children: []Widget{HSpacer{}, PushButton{AssignTo: &cancel, Text: t("Cancelar", "Cancel"), OnClicked: func() { d.Cancel() }},
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
				if !audio.Checked() && !transcribe.Checked() && !visual.Checked() && strings.TrimSpace(chat.Text()) == "" {
					walk.MsgBox(d, title, t("Selecione um sinal ou adicione o chat.", "Select a signal or add a chat file."), walk.MsgBoxIconWarning)
					return
				}
				u.opts = options{transcribe.Checked(), audio.Checked(), visual.Checked(), judge.Checked(), int(lo.Value()), int(hi.Value()), int(count.Value())}
				u.source, u.chat = path, strings.TrimSpace(chat.Text())
				d.Accept()
			}}}},
	}}).Run(u.mw)
	if err != nil {
		u.error(err)
		return
	}
	if ok != walk.DlgCmdOK {
		return
	}
	o := u.opts
	u.operate(map[string]any{"action": "analyze", "options": map[string]any{"path": u.source, "chat": u.chat, "transcription": o.transcribe, "audio": o.audio, "visual": o.visual, "judge": o.judge, "min_seconds": o.min, "max_seconds": o.max, "top_n": o.limit}})
}

func (u *desk) exportDialog() {
	var d *walk.Dialog
	var ok, cancel *walk.PushButton
	var vertical *walk.CheckBox
	var checks [4]*walk.CheckBox
	formats := []string{"xml", "mp4", "csv", "json"}
	labels := []string{"Adobe Premiere Pro  ·  XML", u.t("Vídeos individuais  ·  MP4", "Individual videos  ·  MP4"), u.t("Planilha de cortes  ·  CSV", "Clip spreadsheet  ·  CSV"), u.t("Dados dos cortes  ·  JSON", "Clip data  ·  JSON")}
	children := []Widget{Label{Text: u.t("Exportar cortes aprovados", "Export approved clips"), Font: font(14, true)}}
	for i, label := range labels {
		children = append(children, CheckBox{AssignTo: &checks[i], Text: label, Checked: i == 0})
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
	choice, err := (Dialog{AssignTo: &d, Title: u.t("Exportação", "Export"), MinSize: Size{Width: 360}, Layout: VBox{}, DefaultButton: &ok, CancelButton: &cancel, Children: children}).Run(u.mw)
	if err != nil {
		u.error(err)
	} else if choice == walk.DlgCmdOK {
		u.operate(map[string]any{"action": "export", "formats": selected, "vertical": portrait})
	}
}

func (u *desk) stage(s string) string {
	labels := map[string][2]string{"probe": {"Lendo vídeo", "Reading recording"}, "install": {"Preparando ferramentas pela primeira vez", "Preparing tools for first use"}, "extract": {"Extraindo áudio", "Extracting audio"}, "transcribe": {"Transcrevendo", "Transcribing"}, "audio": {"Analisando áudio", "Analyzing audio"}, "visual": {"Analisando imagem", "Analyzing video"}, "chat": {"Lendo chat", "Reading chat"}, "rank": {"Selecionando destaques", "Ranking highlights"}, "judge": {"Avaliando destaques", "Scoring highlights"}, "export": {"Exportando", "Exporting"}, "render": {"Gerando vídeos", "Rendering videos"}, "preview": {"Preparando prévia", "Preparing preview"}}
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
				}
				u.project = project
				u.populate()
				return
			}
			u.status.SetText(u.t("Concluído.", "Completed."))
			if req["action"] == "preview" {
				u.play(result)
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

var mciSend = syscall.NewLazyDLL("winmm.dll").NewProc("mciSendStringW")

func mci(command string) bool {
	text, err := syscall.UTF16PtrFromString(command)
	if err != nil {
		return false
	}
	code, _, _ := mciSend.Call(uintptr(unsafe.Pointer(text)), 0, 0, 0)
	return code == 0
}
func (u *desk) play(path string) {
	var dlg *walk.Dialog
	var frame *walk.Composite
	var closeButton *walk.PushButton
	fit := func() {
		if frame != nil {
			s := frame.ClientBoundsPixels()
			mci(fmt.Sprintf("put vodcutpreview window at 0 0 %d %d", s.Width, s.Height))
		}
	}
	err := (Dialog{AssignTo: &dlg, Title: u.t("Prévia do corte", "Clip preview"), Size: Size{Width: 700, Height: 460}, Layout: VBox{}, CancelButton: &closeButton, Children: []Widget{
		Composite{AssignTo: &frame, Background: solid(walk.RGB(16, 16, 18)), MinSize: Size{Width: 320, Height: 180}, Layout: VBox{}, OnSizeChanged: fit},
		Composite{Layout: HBox{MarginsZero: true}, Children: []Widget{
			PushButton{Text: u.t("Reproduzir", "Play"), OnClicked: func() { mci("play vodcutpreview") }},
			PushButton{Text: u.t("Pausar", "Pause"), OnClicked: func() { mci("pause vodcutpreview") }},
			PushButton{Text: u.t("Reiniciar", "Replay"), OnClicked: func() { mci("play vodcutpreview from 0") }}, HSpacer{},
			PushButton{AssignTo: &closeButton, Text: u.t("Fechar", "Close"), OnClicked: func() { dlg.Cancel() }},
		}},
	}}).Create(u.mw)
	if err != nil {
		u.error(err)
		return
	}
	if !mci(fmt.Sprintf("open \"%s\" type mpegvideo alias vodcutpreview parent %d style child", path, frame.Handle())) {
		dlg.Dispose()
		u.portablePreview(path)
		return
	}
	defer mci("close vodcutpreview")
	dlg.Starting().Attach(func() { fit(); mci("window vodcutpreview state show"); mci("play vodcutpreview from 0") })
	dlg.Run()
}
