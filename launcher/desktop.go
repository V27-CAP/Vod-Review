package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"image/jpeg"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"

	"github.com/lxn/walk"
	. "github.com/lxn/walk/declarative"
	"github.com/lxn/win"
	"golang.org/x/sys/windows"
)

const title = "VODCut Studio"

type settings struct{ Theme, Lang string }
type desk struct {
	previewCancel       context.CancelFunc
	welcome, reviewArea *walk.Composite
	welcomeLoad         *walk.PushButton
	settings
	prefs                                              string
	project                                            Review
	mw                                                 *walk.MainWindow
	queue                                              *walk.ListBox
	file, status, reason, queueTitle                   *walk.Label
	image                                              *walk.ImageView
	bitmap                                             walk.Image
	name                                               *walk.LineEdit
	start, end                                         *walk.NumberEdit
	words                                              *walk.TextEdit
	load, preview, approve, reject, save, export, stop *walk.PushButton
	progress                                           *walk.ProgressBar
	source, chat                                       string
	index                                              int
	settingsChanged, busy, restoring                   bool
	cancelTask                                         context.CancelFunc
	cancelPath                                         string
	opts                                               options
	draftName                                          string
	draftStart, draftEnd                               float64
}

func message(s string) { walk.MsgBox(nil, title, s, walk.MsgBoxIconError) }
func (u *desk) t(pt, en string) string {
	if u.Lang == "en" {
		return en
	}
	return pt
}
func font(n int, b bool) Font                { return Font{Family: "Segoe UI", PointSize: n, Bold: b} }
func solid(color walk.Color) SolidColorBrush { return SolidColorBrush{Color: color} }
func (u *desk) error(err error)              { walk.MsgBox(u.mw, title, err.Error(), walk.MsgBoxIconError) }
func desktop() error {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	mutex, err := windows.CreateMutex(nil, false, syscall.StringToUTF16Ptr("Local\\VODCutStudio-Native"))
	if mutex != 0 {
		defer windows.CloseHandle(mutex)
	}
	if errors.Is(err, windows.ERROR_ALREADY_EXISTS) {
		if h := win.FindWindow(nil, syscall.StringToUTF16Ptr(title)); h != 0 {
			if win.IsIconic(h) {
				win.ShowWindow(h, win.SW_RESTORE)
			}
			win.SetForegroundWindow(h)
		}
		return nil
	}
	if err != nil {
		return err
	}
	base, err := os.UserCacheDir()
	if err != nil {
		return err
	}
	u := desk{prefs: filepath.Join(base, "VODCutStudio", "appearance.json"), settings: settings{"red", "pt"}, opts: options{transcribe: true, audio: true, min: 15, max: 90, limit: 20}}
	if b, err := os.ReadFile(u.prefs); err == nil {
		json.Unmarshal(b, &u.settings)
	}
	first := true
	for {
		u.settingsChanged = false
		if err := u.build(); err != nil {
			return err
		}
		u.populate()
		if u.draftName != "" {
			u.name.SetText(u.draftName)
			u.start.SetValue(u.draftStart)
			u.end.SetValue(u.draftEnd)
			u.draftName = ""
		}
		if first {
			first = false
			u.restoring = true
			u.controls()
			w := u.mw
			w.Starting().Attach(func() {
				go func() {
					p, err := lastReview()
					w.Synchronize(func() {
						u.restoring = false
						u.project = p
						u.source = p.Meta.Path
						u.populate()
						if err != nil {
							u.status.SetText(u.t("A revisão anterior não pôde ser recuperada.", "Could not restore the previous review."))
						}
					})
				}()
			})
		}
		u.mw.Closing().Attach(func(cancel *bool, _ walk.CloseReason) {
			if u.busy && !u.settingsChanged {
				if walk.MsgBox(u.mw, title, u.t("Interromper o processamento e fechar?", "Stop processing and close?"), walk.MsgBoxYesNo|walk.MsgBoxIconQuestion) != walk.DlgCmdYes {
					*cancel = true
					return
				}
				u.cancelTask()
			}
		})
		u.mw.Run()
		if u.previewCancel != nil {
			u.previewCancel()
			u.previewCancel = nil
		}
		if u.bitmap != nil {
			u.bitmap.Dispose()
			u.bitmap = nil
		}
		if !u.settingsChanged {
			return nil
		}
	}
}

func (u *desk) appearance(theme, lang string) {
	if u.busy || u.restoring {
		return
	}
	if theme == u.Theme && lang == u.Lang {
		return
	}
	u.draftName = u.name.Text()
	u.draftStart = u.start.Value()
	u.draftEnd = u.end.Value()
	u.Theme, u.Lang = theme, lang
	b, _ := json.Marshal(u.settings)
	if err := os.MkdirAll(filepath.Dir(u.prefs), 0755); err == nil {
		os.WriteFile(u.prefs, b, 0644)
	}
	u.settingsChanged = true
	u.mw.Close()
}

func (u *desk) build() error {
	t := u.t
	body, panel, ink, muted, brand := walk.RGB(40, 24, 29), walk.RGB(59, 36, 42), walk.RGB(255, 244, 245), walk.RGB(228, 200, 204), walk.RGB(157, 29, 53)
	if u.Theme == "dark" {
		body, panel, brand = walk.RGB(23, 25, 29), walk.RGB(35, 38, 44), walk.RGB(45, 47, 53)
	}
	if u.Theme == "light" {
		body, panel, ink, muted, brand = walk.RGB(241, 242, 245), walk.RGB(255, 255, 255), walk.RGB(37, 37, 42), walk.RGB(81, 78, 84), walk.RGB(157, 29, 53)
	}
	themeMenu := Menu{Text: t("Aparência", "Appearance")}
	for _, v := range []struct{ id, pt, en string }{{"red", "Vermelho", "Red"}, {"dark", "Escuro", "Dark"}, {"light", "Branco", "Light"}} {
		themeMenu.Items = append(themeMenu.Items, Action{Text: t(v.pt, v.en), Checked: u.Theme == v.id, OnTriggered: func() { u.appearance(v.id, u.Lang) }})
	}
	caption := func(s string) Label { return Label{Text: s, TextColor: muted, NoPrefix: true, Font: font(9, true)} }
	return (MainWindow{
		AssignTo: &u.mw, Title: title, Font: font(10, false), Size: Size{Width: 1070, Height: 690}, MinSize: Size{Width: 770, Height: 540},
		Background: solid(body), Layout: VBox{MarginsZero: true, SpacingZero: true},
		MenuItems: []MenuItem{themeMenu, Menu{Text: "Idioma / Language", Items: []MenuItem{
			Action{Text: "Português", Checked: u.Lang != "en", OnTriggered: func() { u.appearance(u.Theme, "pt") }},
			Action{Text: "English", Checked: u.Lang == "en", OnTriggered: func() { u.appearance(u.Theme, "en") }},
		}}},
		OnDropFiles: func(paths []string) {
			if len(paths) > 0 && !u.busy && !u.restoring {
				u.analysisDialog(paths[0])
			}
		},
		Children: []Widget{
			Composite{Background: solid(brand), Layout: HBox{Margins: Margins{20, 10, 20, 10}}, Children: []Widget{
				Label{Text: title, TextColor: walk.RGB(255, 255, 255), Font: font(21, true)}, HSpacer{},
				PushButton{AssignTo: &u.load, Text: t("Escolher vídeo…", "Choose video…"), OnClicked: u.pickVideo},
			}},
			Composite{Background: solid(body), Layout: VBox{Margins: Margins{18, 8, 18, 12}, Spacing: 8}, Children: []Widget{
				Label{AssignTo: &u.file, TextColor: muted, EllipsisMode: EllipsisEnd, NoPrefix: true},
				Composite{AssignTo: &u.welcome, Background: solid(panel), Layout: VBox{Margins: Margins{32, 26, 32, 26}, Spacing: 12}, Children: []Widget{
					VSpacer{},
					Label{Text: t("Encontre os destaques da sua live", "Find highlights in your stream"), Font: font(24, true), TextColor: ink},
					TextLabel{Text: t("Escolha uma gravação para encontrar reações e momentos engraçados. Você decide quais cortes exportar.", "Choose a recording to find reactions and funny moments. You decide which clips to export."), MinSize: Size{Width: 300}, TextColor: muted},
					Composite{Layout: HBox{MarginsZero: true}, Children: []Widget{PushButton{AssignTo: &u.welcomeLoad, Text: t("Escolher vídeo…", "Choose video…"), MinSize: Size{Width: 150, Height: 36}, OnClicked: u.pickVideo}, HSpacer{}}},
					VSpacer{},
				}},
				Composite{AssignTo: &u.reviewArea, Visible: false, Layout: HBox{MarginsZero: true, Spacing: 12}, Children: []Widget{
					Composite{StretchFactor: 4, Background: solid(panel), Layout: VBox{Margins: Margins{14, 10, 14, 12}, Spacing: 7}, Children: []Widget{
						Label{AssignTo: &u.queueTitle, Font: font(15, true), TextColor: ink},
						ListBox{AssignTo: &u.queue, MinSize: Size{Width: 206, Height: 110}, OnCurrentIndexChanged: u.selectClip},
						PushButton{AssignTo: &u.export, Text: t("Exportar aprovados…", "Export approved clips…"), OnClicked: u.exportDialog},
					}},
					Composite{StretchFactor: 7, Background: solid(panel), Layout: VBox{Margins: Margins{14, 10, 14, 12}, Spacing: 5}, Children: []Widget{
						Label{Text: t("Revisão do corte", "Clip review"), Font: font(15, true), TextColor: ink},
						ImageView{AssignTo: &u.image, Background: solid(walk.RGB(28, 28, 32)), Mode: ImageViewModeZoom, MinSize: Size{Height: 50}, StretchFactor: 3},
						Label{AssignTo: &u.reason, TextColor: muted, EllipsisMode: EllipsisEnd, NoPrefix: true},
						Composite{Layout: HBox{MarginsZero: true}, Children: []Widget{
							caption(t("Nome", "Name")), LineEdit{AssignTo: &u.name, MaxLength: 160},
							PushButton{AssignTo: &u.preview, Text: t("Ver prévia", "Preview"), OnClicked: func() { u.edit("preview", "") }},
						}},
						Composite{Layout: HBox{MarginsZero: true}, Children: []Widget{
							caption(t("Início (s)", "Start (s)")), NumberEdit{AssignTo: &u.start, Decimals: 3, MinValue: 0, MaxValue: 9999999, Increment: 0.1},
							caption(t("Fim (s)", "End (s)")), NumberEdit{AssignTo: &u.end, Decimals: 3, MinValue: 0, MaxValue: 9999999, Increment: 0.1},
						}},
						caption(t("Transcrição", "Transcript")),
						TextEdit{AssignTo: &u.words, ReadOnly: true, VScroll: true, MinSize: Size{Height: 32}, MaxSize: Size{Height: 72}},
						Composite{Layout: HBox{MarginsZero: true}, Children: []Widget{
							PushButton{AssignTo: &u.save, Text: t("Salvar ajustes", "Save edits"), OnClicked: func() { u.edit("edit", "") }},
							PushButton{AssignTo: &u.reject, Text: t("Rejeitar", "Reject"), OnClicked: func() { u.edit("edit", "rejected") }},
							PushButton{AssignTo: &u.approve, Text: t("Aprovar corte", "Approve clip"), OnClicked: func() { u.edit("edit", "approved") }},
						}},
					}},
				}},
				Composite{Layout: HBox{MarginsZero: true}, Children: []Widget{
					Label{AssignTo: &u.status, TextColor: ink, StretchFactor: 3, EllipsisMode: EllipsisEnd, NoPrefix: true, Text: t("Escolha um vídeo ou arraste o arquivo para cá.", "Choose a video or drop the file here.")},
					ProgressBar{AssignTo: &u.progress, MaxValue: 100, MaxSize: Size{Width: 165}, Visible: false},
					PushButton{AssignTo: &u.stop, Text: t("Parar", "Stop"), Visible: false, OnClicked: u.stopOperation},
				}},
			}},
		},
	}).Create()
}

func (u *desk) pickVideo() { u.analysisDialog(u.source) }
func clock(s float64) string {
	n := int(s)
	return fmt.Sprintf("%02d:%02d:%02d", n/3600, (n/60)%60, n%60)
}
func (u *desk) clip() *Clip {
	if u.index < 0 || u.index >= len(u.project.Clips) {
		return nil
	}
	return &u.project.Clips[u.index]
}
func (u *desk) populate() {
	rows := make([]string, 0, len(u.project.Clips))
	approved := 0
	for _, c := range u.project.Clips {
		state := u.t("Pendente", "Pending")
		if c.Status == "approved" {
			state = u.t("Aprovado", "Approved")
			approved++
		} else if c.Status == "rejected" {
			state = u.t("Rejeitado", "Rejected")
		}
		rows = append(rows, fmt.Sprintf("%s   ·   %s   ·   %s", clock(c.Start), c.Name, state))
	}
	u.file.SetText(u.project.Meta.Path)
	if u.project.Meta.Path == "" {
		u.file.SetText(u.t("Vídeos MP4, MKV ou MOV", "MP4, MKV or MOV recordings"))
	}
	u.queueTitle.SetText(fmt.Sprintf(u.t("Destaques  ·  %d", "Highlights  ·  %d"), len(rows)))
	idx := u.index
	u.queue.SetModel(rows)
	if len(rows) > 0 {
		if idx < 0 || idx >= len(rows) {
			idx = 0
		}
		u.queue.SetCurrentIndex(idx)
	}
	u.selectClip()
	if u.project.Project != "" {
		u.status.SetText(fmt.Sprintf(u.t("%d destaques encontrados  ·  %d aprovados", "%d highlights found  ·  %d approved"), len(rows), approved))
	}
}
func (u *desk) selectClip() {
	if u.queue == nil || u.words == nil {
		return
	}
	u.index = u.queue.CurrentIndex()
	c := u.clip()
	old := u.bitmap
	u.bitmap = nil
	if c != nil {
		u.name.SetText(c.Name)
		u.start.SetValue(c.Start)
		u.end.SetValue(c.End)
		u.words.SetText(c.Transcript)
		reason := fmt.Sprintf(u.t("Pico em %s   ·   Pontuação %.0f%%", "Peak at %s   ·   Score %.0f%%"), clock(c.Peak), 100*c.Score)
		if s := c.Reason[u.Lang]; s != "" {
			reason += "   ·   " + s
		}
		u.reason.SetText(reason)
		u.reason.SetToolTipText(reason)
		if c.Thumbnail != "" {
			f, err := os.Open(filepath.Join(cacheDir(), u.project.Project, "thumbs", c.Thumbnail))
			if err == nil {
				photo, err := jpeg.Decode(f)
				f.Close()
				if err == nil {
					if bmp, err := walk.NewBitmapFromImageForDPI(photo, 96); err == nil {
						u.bitmap = bmp
					}
				}
			}
		}
	} else {
		u.name.SetText("")
		u.start.SetValue(0)
		u.end.SetValue(0)
		u.words.SetText("")
		u.reason.SetText(u.t("A prévia aparece ao selecionar um destaque.", "Select a highlight to preview it."))
	}
	u.image.SetImage(u.bitmap)
	if old != nil {
		old.Dispose()
	}
	u.controls()
}
func (u *desk) controls() {
	idle := !u.busy && !u.restoring
	has := u.clip() != nil && idle
	review := u.project.Project != ""
	u.welcome.SetVisible(!review)
	u.reviewArea.SetVisible(review)
	u.load.SetVisible(review)
	u.welcomeLoad.SetEnabled(idle)
	u.load.SetEnabled(idle)
	u.queue.SetEnabled(idle)
	for _, w := range []walk.Widget{u.name, u.start, u.end, u.preview, u.approve, u.reject, u.save} {
		w.SetEnabled(has)
	}
	approved := false
	for _, c := range u.project.Clips {
		if c.Status == "approved" {
			approved = true
			break
		}
	}
	u.export.SetEnabled(idle && approved)
	u.stop.SetVisible(u.busy)
	u.progress.SetVisible(u.busy)
}
func (u *desk) edit(action, status string) {
	c := u.clip()
	if c == nil {
		return
	}
	a, b := u.start.Value(), u.end.Value()
	if a > c.Peak || b <= c.Peak || b > u.project.Meta.Duration+.0001 || strings.TrimSpace(u.name.Text()) == "" {
		u.error(errors.New(u.t("Preencha o nome e mantenha o pico entre o início e o fim do vídeo.", "Enter a name and keep the peak inside the clip and the clip inside the video.")))
		return
	}
	if status == "" {
		status = c.Status
	}
	u.operate(map[string]any{"action": action, "id": c.ID, "start": a, "end": b, "name": u.name.Text(), "status": status})
}
