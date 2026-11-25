package main

import (
	_ "embed"
	"runtime"
	"syscall"
	"unsafe"

	"github.com/lxn/walk"
	. "github.com/lxn/walk/declarative"
	"github.com/lxn/win"
)

//go:embed fonts/Barlow-Regular.ttf
var regularFont []byte

//go:embed fonts/Barlow-Bold.ttf
var boldFont []byte

func loadStudioFonts() {
	register := syscall.NewLazyDLL("gdi32.dll").NewProc("AddFontMemResourceEx")
	for _, data := range [][]byte{regularFont, boldFont} {
		var count uint32
		register.Call(uintptr(unsafe.Pointer(&data[0])), uintptr(len(data)), 0, uintptr(unsafe.Pointer(&count)))
		runtime.KeepAlive(data)
	}
}

type palette struct{ base, panel, field, raised, ink, muted, edge, accent, onAccent walk.Color }

func rgb(v uint32) walk.Color { return walk.RGB(byte(v>>16), byte(v>>8), byte(v)) }
func (u *desk) colors() palette {
	switch u.Theme {
	case "light":
		return palette{rgb(0xf1efeb), rgb(0xfffdfa), rgb(0xf2efea), rgb(0xe7e3dc), rgb(0x242225), rgb(0x59575f), rgb(0xcac4be), rgb(0xb92c39), rgb(0xffffff)}
	case "dark":
		return palette{rgb(0x14171b), rgb(0x1c2026), rgb(0x242a31), rgb(0x303740), rgb(0xf1f3f5), rgb(0xb5bbc3), rgb(0x424851), rgb(0xfa6671), rgb(0x17191d)}
	default:
		return palette{rgb(0x171416), rgb(0x211e20), rgb(0x2c282b), rgb(0x3a3135), rgb(0xfaf5ee), rgb(0xc6babd), rgb(0x4e4045), rgb(0xff5965), rgb(0x231718)}
	}
}

type paintedWidget struct{ create func(*Builder) error }

func (w paintedWidget) Create(b *Builder) error { return w.create(b) }

var (
	commonControls  = syscall.NewLazyDLL("comctl32.dll")
	setSubclass     = commonControls.NewProc("SetWindowSubclass")
	nextSubclass    = commonControls.NewProc("DefSubclassProc")
	fillRectProc    = syscall.NewLazyDLL("user32.dll").NewProc("FillRect")
	buttonPaintProc = syscall.NewCallback(paintProc)
	painted         = map[win.HWND]buttonPaint{}
)

type buttonPaint struct {
	p                     palette
	primary, check, combo bool
	listBrush             win.HBRUSH
	background            walk.Color
	text                  func() string
	dpi                   func() int
}

// Win32 reports a small ideal width for some themed buttons. Measure the
// localized label with the actual control font before laying out the dialog.
func fitButton(w walk.Widget, text string, padding int) error {
	if text == "" {
		return nil
	}
	h := w.Handle()
	dc := win.GetDC(h)
	defer win.ReleaseDC(h, dc)
	old := win.SelectObject(dc, win.HGDIOBJ(win.SendMessage(h, win.WM_GETFONT, 0, 0)))
	defer win.SelectObject(dc, old)
	chars, _ := syscall.UTF16FromString(text)
	var size win.SIZE
	if !win.GetTextExtentPoint32(dc, &chars[0], int32(len(chars)-1), &size) {
		return nil
	}
	minSize := w.MinSize()
	minSize.Width = max(minSize.Width, walk.IntTo96DPI(int(size.CX), w.DPI())+padding)
	return w.SetMinMaxSize(minSize, w.MaxSize())
}
func installPaint(h win.HWND, s buttonPaint) {
	if s.combo {
		s.listBrush = win.CreateBrushIndirect(&win.LOGBRUSH{LbColor: win.COLORREF(s.p.field)})
	}
	if ok, _, _ := setSubclass.Call(uintptr(h), buttonPaintProc, 1, 0); ok != 0 {
		painted[h] = s
		win.InvalidateRect(h, nil, true)
	} else if s.listBrush != 0 {
		win.DeleteObject(win.HGDIOBJ(s.listBrush))
	}
}
func fillDC(dc win.HDC, r win.RECT, col walk.Color) {
	b := win.CreateBrushIndirect(&win.LOGBRUSH{LbColor: win.COLORREF(col)})
	defer win.DeleteObject(win.HGDIOBJ(b))
	fillRectProc.Call(uintptr(dc), uintptr(unsafe.Pointer(&r)), uintptr(b))
}
func paintProc(h win.HWND, msg uint32, wp, lp, id, data uintptr) uintptr {
	s, ok := painted[h]
	if ok && s.combo && msg == win.WM_CTLCOLORLISTBOX {
		win.SetTextColor(win.HDC(wp), win.COLORREF(s.p.ink))
		win.SetBkColor(win.HDC(wp), win.COLORREF(s.p.field))
		return uintptr(s.listBrush)
	}
	if ok && (msg == win.WM_PAINT || msg == win.WM_PRINTCLIENT) {
		var ps win.PAINTSTRUCT
		dc := win.HDC(wp)
		if msg == win.WM_PAINT {
			dc = win.BeginPaint(h, &ps)
			defer win.EndPaint(h, &ps)
		}
		var r win.RECT
		win.GetClientRect(h, &r)
		px := func(n int) int32 { return int32(walk.IntFrom96DPI(n, s.dpi())) }
		fg, bg := s.p.ink, s.p.raised
		if s.primary {
			fg, bg = s.p.onAccent, s.p.accent
		}
		enabled := win.IsWindowEnabled(h)
		if !enabled {
			fg, bg = s.p.muted, s.p.field
		}
		if s.combo {
			bg = s.p.field
		}
		if s.check {
			bg = s.background
		}
		state := win.SendMessage(h, win.BM_GETSTATE, 0, 0)
		fillDC(dc, r, bg)
		if s.check {
			size := px(17)
			top := (r.Bottom - size) / 2
			box := win.RECT{Left: px(1), Top: top, Right: px(1) + size, Bottom: top + size}
			fillDC(dc, box, s.p.muted)
			box.Left++
			box.Top++
			box.Right--
			box.Bottom--
			fillDC(dc, box, s.p.field)
			if state&1 != 0 {
				fillDC(dc, box, s.p.accent)
				pen := win.ExtCreatePen(win.PS_GEOMETRIC|win.PS_SOLID, uint32(px(2)), &win.LOGBRUSH{LbColor: win.COLORREF(s.p.onAccent)}, 0, nil)
				old := win.SelectObject(dc, win.HGDIOBJ(pen))
				win.MoveToEx(dc, int(px(4)), int(top+px(8)), nil)
				win.LineTo(dc, px(8), top+px(12))
				win.LineTo(dc, px(14), top+px(5))
				win.SelectObject(dc, old)
				win.DeleteObject(win.HGDIOBJ(pen))
			}
			r.Left += px(26)
		}
		if !s.check && !s.combo && state&4 != 0 {
			fillDC(dc, win.RECT{Left: r.Left, Top: r.Bottom - px(3), Right: r.Right, Bottom: r.Bottom}, s.p.ink)
		}
		win.SetBkMode(dc, win.TRANSPARENT)
		win.SetTextColor(dc, win.COLORREF(fg))
		old := win.SelectObject(dc, win.HGDIOBJ(win.SendMessage(h, win.WM_GETFONT, 0, 0)))
		flags := uint32(win.DT_CENTER | win.DT_VCENTER | win.DT_SINGLELINE | win.DT_END_ELLIPSIS | win.DT_NOPREFIX)
		if s.combo {
			arrow := r
			arrow.Left = arrow.Right - px(23)
			// A three-step chevron stays crisp on scaled screens.
			mid := arrow.Left + px(11)
			top := (arrow.Bottom - px(5)) / 2
			for i := 0; i < 3; i++ {
				d := px(i * 2)
				fillDC(dc, win.RECT{Left: mid - px(6) + d, Top: top + d, Right: mid - px(3) + d, Bottom: top + d + px(2)}, fg)
				fillDC(dc, win.RECT{Left: mid + px(3) - d, Top: top + d, Right: mid + px(6) - d, Bottom: top + d + px(2)}, fg)
			}
			r.Left += px(8)
			r.Right -= px(24)
		}
		if s.check || s.combo {
			flags &^= win.DT_CENTER
		}
		txt, _ := syscall.UTF16FromString(s.text())
		if len(txt) > 1 {
			win.DrawTextEx(dc, &txt[0], int32(len(txt)-1), &r, flags, nil)
		}
		win.SelectObject(dc, old)
		if win.GetFocus() == h {
			r.Left += px(3)
			r.Top += px(3)
			r.Right -= px(3)
			r.Bottom -= px(3)
			win.DrawFocusRect(dc, &r)
		}
		return 0
	}
	if msg == win.WM_NCDESTROY {
		if s.listBrush != 0 {
			win.DeleteObject(win.HGDIOBJ(s.listBrush))
		}
		delete(painted, h)
	}
	res, _, _ := nextSubclass.Call(uintptr(h), uintptr(msg), wp, lp)
	switch msg {
	case win.CB_SETCURSEL, win.WM_SETFOCUS, win.WM_KILLFOCUS, win.WM_ENABLE, win.BM_SETCHECK, win.BM_SETSTATE, win.WM_SETTEXT:
		if ok {
			win.InvalidateRect(h, nil, false)
		}
	}
	return res
}

// Keep native HWNDs, tab order, accessibility and button messages while drawing
// controls with the same palette as their surrounding panel.
func (u *desk) skin(items []Widget, background walk.Color, primary **walk.PushButton) {
	p := u.colors()
	for i, item := range items {
		switch w := item.(type) {
		case Composite:
			bg := background
			if brush, ok := w.Background.(SolidColorBrush); ok {
				bg = brush.Color
			}
			u.skin(w.Children, bg, primary)
			items[i] = w
		case Label:
			if w.TextColor == 0 {
				w.TextColor = p.ink
			}
			items[i] = w
		case TextLabel:
			if w.TextColor == 0 {
				w.TextColor = p.ink
			}
			items[i] = w
		case LineEdit:
			w.Background = solid(p.field)
			w.TextColor = p.ink
			items[i] = w
		case TextEdit:
			w.Background = solid(p.field)
			w.TextColor = p.ink
			items[i] = w
		case NumberEdit:
			w.Background = solid(p.field)
			w.TextColor = p.ink
			items[i] = w
		case ComboBox:
			if w.Editable {
				items[i] = w
				continue
			}
			orig := w.AssignTo
			var local *walk.ComboBox
			if orig == nil {
				orig = &local
				w.AssignTo = orig
			}
			items[i] = paintedWidget{func(b *Builder) error {
				if err := w.Create(b); err != nil {
					return err
				}
				box := *orig
				installPaint(box.Handle(), buttonPaint{p: p, combo: true, background: background, text: box.Text, dpi: box.DPI})
				return nil
			}}
		case ListBox:
			w.Background = solid(p.field)
			items[i] = w
		case PushButton:
			isPrimary := w.Name == "primary" || (primary != nil && primary == w.AssignTo)
			orig := w.AssignTo
			var local *walk.PushButton
			if orig == nil {
				orig = &local
				w.AssignTo = orig
			}
			w.Font = font(10, true)
			if w.MinSize.Height == 0 {
				w.MinSize.Height = 30
			}
			items[i] = paintedWidget{func(b *Builder) error {
				if err := w.Create(b); err != nil {
					return err
				}
				btn := *orig
				b.Defer(func() error { return fitButton(btn, btn.Text(), 20) })
				installPaint(btn.Handle(), buttonPaint{p: p, primary: isPrimary, background: background, text: btn.Text, dpi: btn.DPI})
				return nil
			}}
		case CheckBox:
			w.Alignment = AlignHNearVCenter
			orig := w.AssignTo
			var local *walk.CheckBox
			if orig == nil {
				orig = &local
				w.AssignTo = orig
			}
			button := paintedWidget{func(b *Builder) error {
				if err := w.Create(b); err != nil {
					return err
				}
				btn := *orig
				b.Defer(func() error { return fitButton(btn, btn.Text(), 32) })
				installPaint(btn.Handle(), buttonPaint{p: p, check: true, background: background, text: btn.Text, dpi: btn.DPI})
				return nil
			}}
			items[i] = Composite{Layout: HBox{MarginsZero: true, SpacingZero: true, Alignment: AlignHNearVCenter}, Children: []Widget{button, HSpacer{}}}
		}
	}
}
func (u *desk) dialog(d Dialog) Dialog {
	p := u.colors()
	d.Background = solid(p.panel)
	d.Font = font(11, false)
	u.skin(d.Children, p.panel, d.DefaultButton)
	return d
}

func (u *desk) preferences() {
	t := u.t
	var dlg *walk.Dialog
	var theme, lang *walk.ComboBox
	var save, cancel *walk.PushButton
	ids := []string{"red", "dark", "light"}
	ti, li := 0, 0
	for i, id := range ids {
		if id == u.Theme {
			ti = i
		}
	}
	if u.Lang == "en" {
		li = 1
	}
	choice, err := u.dialog(Dialog{AssignTo: &dlg, Title: t("Preferências", "Preferences"), MinSize: Size{Width: 360}, Layout: VBox{Margins: Margins{20, 18, 20, 18}, Spacing: 10}, DefaultButton: &save, CancelButton: &cancel, Children: []Widget{
		Label{Text: t("Aparência", "Appearance"), Font: font(18, true)},
		Label{Text: t("Tema do aplicativo", "App theme")}, ComboBox{AssignTo: &theme, Model: []string{t("Vermelho", "Red"), t("Escuro", "Dark"), t("Branco", "Light")}, CurrentIndex: ti},
		Label{Text: t("Idioma", "Language")}, ComboBox{AssignTo: &lang, Model: []string{"Português", "English"}, CurrentIndex: li},

		Composite{Layout: HBox{MarginsZero: true}, Children: []Widget{HSpacer{}, PushButton{AssignTo: &cancel, Text: t("Cancelar", "Cancel"), OnClicked: func() { dlg.Cancel() }}, PushButton{AssignTo: &save, Text: t("Aplicar", "Apply"), OnClicked: func() { ti, li = theme.CurrentIndex(), lang.CurrentIndex(); dlg.Accept() }}}},
	}}).Run(u.mw)
	if err != nil {
		u.error(err)
		return
	}
	if choice == walk.DlgCmdOK && ti >= 0 && ti < len(ids) {
		language := "pt"
		if li == 1 {
			language = "en"
		}
		u.appearance(ids[ti], language)
	}
}
