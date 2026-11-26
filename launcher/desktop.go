package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"image/jpeg"
	"math"
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

type categoryDef struct {
	id, pt, en string
}

var clipCategories = []categoryDef{
	{id: "funny", pt: "Engraçado", en: "Funny"},
	{id: "intense", pt: "Intenso", en: "Intense"},
	{id: "cute", pt: "Fofo", en: "Cute"},
	{id: "story", pt: "História marcante", en: "Memorable story"},
}

type settings struct{ Theme, Lang string }

type desk struct {
	building              bool
	cutFields, postFields *walk.Composite
	switchPage            *walk.PushButton
	editorTitle           *walk.Label
	postOpen              bool

	previewCancel       context.CancelFunc
	welcome, reviewArea *walk.Composite
	welcomeLoad         *walk.PushButton
	settings
	prefs, aiPrefs                         string
	ai                                     AISettings
	project                                Review
	mw                                     *walk.MainWindow
	catFilter, clipCategory                *walk.ComboBox
	filterCat                              string
	visible                                []int
	queue                                  *walk.ListBox
	file, status, reason, queueTitle       *walk.Label
	image                                  *walk.ImageView
	bitmap                                 walk.Image
	name, contextNote, hashtags            *walk.LineEdit
	start, end                             *walk.NumberEdit
	description, words                     *walk.TextEdit
	load, preview, copyPostBtn             *walk.PushButton
	approve, reject, save, export, stop    *walk.PushButton
	progress                               *walk.ProgressBar
	source, chat                           string
	index                                  int
	settingsChanged, busy, restoring       bool
	cancelTask                             context.CancelFunc
	cancelPath                             string
	opts                                   options
	draftName, draftCategory, draftContext string
	draftDescription, draftHashtags        string
	draftStart, draftEnd                   float64
}

func message(s string) { walk.MsgBox(nil, title, s, walk.MsgBoxIconError) }
func (u *desk) t(pt, en string) string {
	if u.Lang == "en" {
		return en
	}
	return pt
}
func font(n int, b bool) Font                { return Font{Family: "Open Sans", PointSize: n, Bold: b} }
func solid(color walk.Color) SolidColorBrush { return SolidColorBrush{Color: color} }
func (u *desk) error(err error)              { walk.MsgBox(u.mw, title, err.Error(), walk.MsgBoxIconError) }

func (u *desk) categoryLabel(id string) string {
	for _, cat := range clipCategories {
		if cat.id == id {
			return u.t(cat.pt, cat.en)
		}
	}
	return u.t(clipCategories[0].pt, clipCategories[0].en)
}

func categoryIndex(id string) int {
	for i, cat := range clipCategories {
		if cat.id == id {
			return i
		}
	}
	return 0
}

func clipHasCategory(c Clip, cat string) bool {
	if cat == "" || cat == "all" {
		return true
	}
	if c.Category == cat {
		return true
	}
	for _, item := range c.Categories {
		if item == cat {
			return true
		}
	}
	return false
}

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
	loadStudioFonts()
	aiPrefsPath := filepath.Join(base, "VODCutStudio", "ai_settings.json")
	u := desk{
		prefs:     filepath.Join(base, "VODCutStudio", "appearance.json"),
		aiPrefs:   aiPrefsPath,
		ai:        loadAISettings(aiPrefsPath),
		settings:  settings{"red", "en"},
		filterCat: "all",
		opts: options{
			preset:         "funny_moments",
			outputLang:     "en",
			transcribe:     true,
			audio:          true,
			voiceIsolation: true,
			audioTrack:     -1,
			min:            5,
			max:            55,
			limit:          60,
		},
	}
	if b, err := os.ReadFile(u.prefs); err == nil {
		_ = json.Unmarshal(b, &u.settings)
	}
	first := true
	for {
		u.settingsChanged = false
		if err := u.build(); err != nil {
			return err
		}
		u.populate()
		u.showEditor(u.postOpen)
		if u.draftName != "" {
			u.name.SetText(u.draftName)
			_ = u.start.SetValue(u.draftStart)
			_ = u.end.SetValue(u.draftEnd)
			_ = u.clipCategory.SetCurrentIndex(categoryIndex(u.draftCategory))
			u.contextNote.SetText(u.draftContext)
			u.description.SetText(u.draftDescription)
			u.hashtags.SetText(u.draftHashtags)
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
						if p.Meta.Facecam != nil {
							cp := *p.Meta.Facecam
							u.opts.facecamROI = &cp
						}
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
	if idx := u.clipCategory.CurrentIndex(); idx >= 0 && idx < len(clipCategories) {
		u.draftCategory = clipCategories[idx].id
	}
	u.draftContext = u.contextNote.Text()
	u.draftDescription = u.description.Text()
	u.draftHashtags = u.hashtags.Text()
	u.Theme, u.Lang = theme, lang
	b, _ := json.Marshal(u.settings)
	if err := os.MkdirAll(filepath.Dir(u.prefs), 0755); err == nil {
		_ = os.WriteFile(u.prefs, b, 0644)
	}
	u.settingsChanged = true
	u.mw.Close()
}

func (u *desk) build() error {
	u.building = true
	defer func() { u.building = false }()
	t := u.t
	p := u.colors()
	body, panel, ink, muted := p.base, p.panel, p.ink, p.muted
	filterLabels := []string{t("Todas as categorias", "All categories")}
	catLabels := make([]string, len(clipCategories))
	filterIdx := 0
	for i, cat := range clipCategories {
		lbl := t(cat.pt, cat.en)
		catLabels[i] = lbl
		filterLabels = append(filterLabels, lbl)
		if u.filterCat == cat.id {
			filterIdx = i + 1
		}
	}
	caption := func(s string) Label { return Label{Text: s, TextColor: muted, NoPrefix: true, Font: font(10, true)} }
	field := func(label string, input Widget, weight int) Composite {
		return Composite{StretchFactor: weight, Layout: VBox{MarginsZero: true, Spacing: 3}, Children: []Widget{caption(label), input}}
	}
	view := MainWindow{
		AssignTo: &u.mw, Title: title, Font: font(11, false), Size: Size{Width: 1140, Height: 760}, MinSize: Size{Width: 840, Height: 600},
		Background: solid(body), Layout: VBox{Alignment: AlignHNearVNear, MarginsZero: true, SpacingZero: true},
		OnDropFiles: func(paths []string) {
			if len(paths) > 0 && !u.busy && !u.restoring {
				u.analysisDialog(paths[0])
			}
		},
		Children: []Widget{
			Composite{MaxSize: Size{Height: 64}, Background: solid(panel), Layout: HBox{Margins: Margins{20, 12, 20, 12}, Spacing: 10}, Children: []Widget{
				brandMark(p), Label{Text: title, Font: font(20, true), TextColor: ink}, HSpacer{},
				PushButton{AssignTo: &u.load, Text: t("Trocar vídeo…", "Change video…"), OnClicked: u.pickVideo},
				PushButton{Text: t("Preferências", "Preferences"), OnClicked: u.preferences},
			}},
			Composite{StretchFactor: 50, Background: solid(body), Layout: VBox{Alignment: AlignHNearVNear, Margins: Margins{18, 12, 18, 12}, Spacing: 10}, Children: []Widget{
				Label{AssignTo: &u.file, Font: font(12, true), TextColor: muted, EllipsisMode: EllipsisEnd, NoPrefix: true},
				u.welcomePanel(),
				Composite{AssignTo: &u.reviewArea, StretchFactor: 30, Visible: false, Layout: HBox{MarginsZero: true, Spacing: 12}, Children: []Widget{
					Composite{StretchFactor: 4, Background: solid(panel), Layout: VBox{Margins: Margins{14, 10, 14, 12}, Spacing: 7}, Children: []Widget{
						Label{AssignTo: &u.queueTitle, Font: font(15, true), TextColor: ink},
						Composite{Layout: HBox{MarginsZero: true}, Children: []Widget{
							caption(t("Categoria", "Category")),
							ComboBox{
								AssignTo:     &u.catFilter,
								Model:        filterLabels,
								CurrentIndex: filterIdx,
								OnCurrentIndexChanged: func() {
									if u.building {
										return
									}
									idx := u.catFilter.CurrentIndex()
									if idx <= 0 || idx > len(clipCategories) {
										u.filterCat = "all"
									} else {
										u.filterCat = clipCategories[idx-1].id
									}
									u.index = 0
									u.populate()
								},
							},
						}},
						ListBox{AssignTo: &u.queue, ItemStyler: clipRows{u}, Model: []string{}, Font: font(11, false), MinSize: Size{Width: 230, Height: 120}, OnCurrentIndexChanged: u.selectClip},
						PushButton{AssignTo: &u.export, Text: t("Exportar aprovados…", "Export approved clips…"), OnClicked: u.exportDialog},
					}},
					Composite{StretchFactor: 7, Background: solid(panel), Layout: VBox{Margins: Margins{14, 10, 14, 12}, Spacing: 5}, Children: []Widget{
						Composite{Layout: HBox{MarginsZero: true}, Children: []Widget{Label{AssignTo: &u.editorTitle, Font: font(17, true), TextColor: ink}, HSpacer{}, PushButton{AssignTo: &u.preview, Text: t("Ver prévia", "Preview"), OnClicked: func() { u.edit("preview", "") }}, PushButton{AssignTo: &u.switchPage, OnClicked: func() { u.showEditor(!u.postOpen) }}}},
						ImageView{AssignTo: &u.image, Background: solid(walk.RGB(28, 28, 32)), Mode: ImageViewModeZoom, MinSize: Size{Height: 64}, StretchFactor: 2},
						Label{AssignTo: &u.reason, TextColor: muted, EllipsisMode: EllipsisEnd, NoPrefix: true},
						Composite{AssignTo: &u.cutFields, Layout: VBox{MarginsZero: true, Spacing: 6}, Children: []Widget{
							Composite{Layout: HBox{MarginsZero: true}, Children: []Widget{
								field(t("Título do corte", "Clip title"), LineEdit{AssignTo: &u.name, MaxLength: 160}, 3),
								field(t("Categoria", "Category"), ComboBox{AssignTo: &u.clipCategory, Model: catLabels, CurrentIndex: 0}, 2),
							}},
							Composite{Layout: HBox{MarginsZero: true, Spacing: 12}, Children: []Widget{
								field(t("Início em segundos", "Start in seconds"), NumberEdit{AssignTo: &u.start, Decimals: 3, MinValue: 0, MaxValue: 9999999, Increment: 0.1}, 1),
								field(t("Fim em segundos", "End in seconds"), NumberEdit{AssignTo: &u.end, Decimals: 3, MinValue: 0, MaxValue: 9999999, Increment: 0.1}, 1),
							}},
							caption(t("Transcrição", "Transcript")),
							TextEdit{AssignTo: &u.words, ReadOnly: true, VScroll: true, MinSize: Size{Height: 46}, MaxSize: Size{Height: 78}},
						}},
						Composite{AssignTo: &u.postFields, Visible: false, Layout: VBox{MarginsZero: true, Spacing: 6}, Children: []Widget{
							Composite{Layout: HBox{MarginsZero: true}, Children: []Widget{
								caption(t("Contexto", "Context")), LineEdit{AssignTo: &u.contextNote},
							}},
							caption(t("Descrição para post", "Post description")),
							TextEdit{AssignTo: &u.description, VScroll: true, MinSize: Size{Height: 38}, MaxSize: Size{Height: 64}},
							Composite{Layout: HBox{MarginsZero: true}, Children: []Widget{
								caption(t("Hashtags", "Hashtags")), LineEdit{AssignTo: &u.hashtags},
							}},
							PushButton{AssignTo: &u.copyPostBtn, Text: t("Copiar post", "Copy post"), OnClicked: u.copyPost},
						}},
						Composite{Layout: HBox{MarginsZero: true}, Children: []Widget{
							PushButton{AssignTo: &u.save, Text: t("Salvar ajustes", "Save edits"), OnClicked: func() { u.edit("edit", "") }},
							PushButton{AssignTo: &u.reject, Text: t("Rejeitar", "Reject"), OnClicked: func() { u.edit("edit", "rejected") }},
							PushButton{Name: "primary", AssignTo: &u.approve, Text: t("Aprovar corte", "Approve clip"), OnClicked: func() { u.edit("edit", "approved") }},
						}},
					}},
				}},
				Composite{Layout: HBox{MarginsZero: true}, Children: []Widget{
					Label{AssignTo: &u.status, TextColor: ink, StretchFactor: 3, EllipsisMode: EllipsisEnd, NoPrefix: true, Text: ""},
					ProgressBar{AssignTo: &u.progress, MaxValue: 100, MaxSize: Size{Width: 165}, Visible: false},
					PushButton{AssignTo: &u.stop, Text: t("Parar", "Stop"), Visible: false, OnClicked: u.stopOperation},
				}},
			}},
		},
	}
	u.skin(view.Children, body, nil)
	return view.Create()
}
func (u *desk) showEditor(post bool) {
	u.postOpen = post
	u.switchPage.SetVisible(post)
	u.cutFields.SetVisible(!post)
	u.postFields.SetVisible(post)
	if post {
		u.editorTitle.SetText(u.t("Texto do post", "Post text"))
		u.switchPage.SetText(u.t("Voltar ao corte", "Back to clip"))
	} else {
		u.editorTitle.SetText(u.t("Revisão do corte", "Clip review"))
		u.switchPage.SetText(u.t("Editar post", "Edit post"))
	}
}

func (u *desk) pickVideo() { u.analysisDialog(u.source) }

func clock(s float64) string {
	n := int(s)
	if n < 0 {
		n = 0
	}
	return fmt.Sprintf("%02d:%02d:%02d", n/3600, (n/60)%60, n%60)
}

func (u *desk) clip() *Clip {
	if u.index < 0 || u.index >= len(u.visible) {
		return nil
	}
	realIdx := u.visible[u.index]
	if realIdx < 0 || realIdx >= len(u.project.Clips) {
		return nil
	}
	return &u.project.Clips[realIdx]
}

func (u *desk) populate() {
	if u.building || u.queue == nil || u.status == nil {
		return
	}
	rows := make([]string, 0, len(u.project.Clips))
	u.visible = u.visible[:0]
	approved := 0
	for i, c := range u.project.Clips {
		if c.Status == "approved" {
			approved++
		}
		if !clipHasCategory(c, u.filterCat) {
			continue
		}
		state := u.t("Pendente", "Pending")
		if c.Status == "approved" {
			state = u.t("Aprovado", "Approved")
		} else if c.Status == "rejected" {
			state = u.t("Rejeitado", "Rejected")
		}
		catTag := u.categoryLabel(c.Category)
		u.visible = append(u.visible, i)
		rows = append(rows, fmt.Sprintf("%s · [%s] %s · %s", clock(c.Start), catTag, c.Name, state))
	}
	u.file.SetText(filepath.Base(u.project.Meta.Path))
	u.file.SetToolTipText(u.project.Meta.Path)
	if u.project.Meta.Path == "" {
		u.file.SetText(u.t("Área de importação", "Import workspace"))
	}
	u.queueTitle.SetText(fmt.Sprintf(u.t("Destaques  ·  %d de %d", "Highlights  ·  %d of %d"), len(rows), len(u.project.Clips)))
	idx := u.index
	_ = u.queue.SetModel(rows)
	if len(rows) > 0 {
		if idx < 0 || idx >= len(rows) {
			idx = 0
		}
		_ = u.queue.SetCurrentIndex(idx)
	}
	u.selectClip()
	if u.project.Project != "" {
		u.status.SetText(fmt.Sprintf(u.t("Destaques: %d     Aprovados: %d", "Highlights: %d     Approved: %d"), len(u.project.Clips), approved))
	}
}

func (u *desk) selectClip() {
	if u.building || u.queue == nil || u.words == nil {
		return
	}
	u.index = u.queue.CurrentIndex()
	c := u.clip()
	old := u.bitmap
	u.bitmap = nil
	if c != nil {
		u.name.SetText(c.Name)
		_ = u.start.SetValue(c.Start)
		_ = u.end.SetValue(c.End)
		_ = u.clipCategory.SetCurrentIndex(categoryIndex(c.Category))
		u.contextNote.SetText(c.ContextNote)
		u.description.SetText(c.Description)
		u.hashtags.SetText(strings.Join(c.Hashtags, " "))
		u.words.SetText(c.Transcript)
		reason := fmt.Sprintf(u.t("Pico em %s   ·   Pontuação %.0f%%   ·   %s", "Peak at %s   ·   Score %.0f%%   ·   %s"), clock(c.Peak), 100*c.Score, u.categoryLabel(c.Category))
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
		_ = u.start.SetValue(0)
		_ = u.end.SetValue(0)
		_ = u.clipCategory.SetCurrentIndex(0)
		u.contextNote.SetText("")
		u.description.SetText("")
		u.hashtags.SetText("")
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
	u.file.SetVisible(review)
	u.load.SetVisible(review)
	u.welcomeLoad.SetEnabled(idle)
	u.switchPage.SetEnabled(has)
	u.load.SetEnabled(idle)
	u.queue.SetEnabled(idle)
	u.catFilter.SetEnabled(idle && review)
	for _, w := range []walk.Widget{u.name, u.clipCategory, u.start, u.end, u.contextNote, u.description, u.hashtags, u.preview, u.copyPostBtn, u.approve, u.reject, u.save} {
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

func (u *desk) copyPost() {
	c := u.clip()
	if c == nil {
		return
	}
	blocks := make([]string, 0, 4)
	if t := strings.TrimSpace(u.name.Text()); t != "" {
		blocks = append(blocks, t)
	}
	if d := strings.TrimSpace(u.description.Text()); d != "" {
		blocks = append(blocks, d)
	}
	if ctx := strings.TrimSpace(u.contextNote.Text()); ctx != "" {
		blocks = append(blocks, u.t("Contexto: ", "Context: ")+ctx)
	}
	if tags := strings.TrimSpace(u.hashtags.Text()); tags != "" {
		blocks = append(blocks, tags)
	}
	if len(blocks) == 0 {
		return
	}
	post := strings.Join(blocks, "\n\n")
	if err := walk.Clipboard().SetText(post); err != nil {
		u.error(err)
		return
	}
	u.status.SetText(u.t("Post copiado para a área de transferência!", "Post copied to clipboard!"))
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
	catID := "funny"
	if idx := u.clipCategory.CurrentIndex(); idx >= 0 && idx < len(clipCategories) {
		catID = clipCategories[idx].id
	}
	req := map[string]any{
		"action":       action,
		"id":           c.ID,
		"start":        a,
		"end":          b,
		"name":         u.name.Text(),
		"status":       status,
		"category":     catID,
		"context_note": u.contextNote.Text(),
		"description":  u.description.Text(),
		"hashtags":     u.hashtags.Text(),
	}
	if action == "preview" {
		// Provide 5 seconds of padding around the clip so the preview player can seek -5s/+5s and mark start/end.
		req["preview_start"] = math.Max(0, a-5)
		req["preview_end"] = math.Min(u.project.Meta.Duration, b+5)
	}
	u.operate(req)
}
