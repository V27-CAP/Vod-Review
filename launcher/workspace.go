package main

import (
	"fmt"
	"github.com/lxn/walk"
	. "github.com/lxn/walk/declarative"
	"math"
)

func fill(c *walk.Canvas, color walk.Color, r walk.Rectangle) {
	if r.Width <= 0 || r.Height <= 0 {
		return
	}
	b, err := walk.NewSolidColorBrush(color)
	if err != nil {
		return
	}
	defer b.Dispose()
	c.FillRectangle(b, r)
}
func brandMark(p palette) Widget {
	var mark *walk.CustomWidget
	return CustomWidget{AssignTo: &mark, Alignment: AlignHNearVCenter, MinSize: Size{Width: 34, Height: 34}, MaxSize: Size{Width: 34, Height: 34}, PaintMode: PaintBuffered, Paint: func(c *walk.Canvas, r walk.Rectangle) error {
		fill(c, p.panel, r)
		bounds := mark.ClientBounds()
		y := (bounds.Height - 34) / 2
		fill(c, p.accent, walk.Rectangle{X: 0, Y: y, Width: 34, Height: 34})
		for i, h := range []int{9, 18, 12, 21} {
			fill(c, p.onAccent, walk.Rectangle{X: 7 + i*6, Y: y + (34-h)/2, Width: 3, Height: h})
		}
		return nil
	}}
}

// Diagram of a selected audio interval. It contains no sample project data.
func intervalDiagram(p palette) Widget {
	var canvas *walk.CustomWidget
	return CustomWidget{AssignTo: &canvas, MinSize: Size{Width: 200, Height: 104}, MaxSize: Size{Height: 154}, PaintMode: PaintBuffered, InvalidatesOnResize: true, Paint: func(c *walk.Canvas, dirty walk.Rectangle) error {
		r := canvas.ClientBounds()
		fill(c, p.field, dirty)
		width, height := r.Width, r.Height
		left, right := width*38/100, width*73/100
		fill(c, p.raised, walk.Rectangle{X: left, Y: 12, Width: right - left, Height: height - 24})
		for x := 12; x < width-8; x += 6 {
			v := float64(x) / float64(width)
			envelope := .17 + .52*math.Exp(-math.Pow((v-.54)*8, 2)) + .21*math.Exp(-math.Pow((v-.2)*10, 2))
			n := int(float64(height) * envelope * (.35 + .65*math.Abs(math.Sin(float64(x)*1.37)*math.Cos(v*18))))
			color := p.edge
			if x >= left && x <= right {
				color = p.accent
			}
			fill(c, color, walk.Rectangle{X: x, Y: (height - n) / 2, Width: 3, Height: n + 2})
		}
		for _, x := range []int{left, right} {
			fill(c, p.accent, walk.Rectangle{X: x, Y: 12, Width: 2, Height: height - 24})
		}
		fill(c, p.accent, walk.Rectangle{X: left, Y: 12, Width: 9, Height: 4})
		fill(c, p.accent, walk.Rectangle{X: right - 7, Y: height - 16, Width: 9, Height: 4})
		return nil
	}}
}

func (u *desk) welcomePanel() Composite {
	p, t := u.colors(), u.t
	info := func(label, detail string) Widget {
		return Composite{Layout: VBox{MarginsZero: true, Spacing: 4}, Children: []Widget{
			Label{Text: label, Font: font(14, true), TextColor: p.ink}, TextLabel{Text: detail, TextColor: p.muted, MinSize: Size{Width: 165}},
		}}
	}
	return Composite{AssignTo: &u.welcome, Layout: HBox{MarginsZero: true, Spacing: 16}, Children: []Widget{
		Composite{StretchFactor: 7, Background: solid(p.panel), Layout: VBox{Margins: Margins{28, 22, 28, 22}, Spacing: 14}, Children: []Widget{
			VSpacer{}, intervalDiagram(p), Label{Text: t("Importar gravação", "Import recording"), Font: font(29, true), TextColor: p.ink},
			TextLabel{Text: t("Arraste sua live para esta janela ou escolha um vídeo no computador.", "Drop your stream recording here or choose a video from your computer."), MinSize: Size{Width: 220}, TextColor: p.muted},
			Composite{Layout: HBox{MarginsZero: true, Spacing: 12}, Children: []Widget{PushButton{Name: "primary", AssignTo: &u.welcomeLoad, Text: t("Escolher vídeo…", "Choose video…"), MinSize: Size{Width: 155, Height: 40}, OnClicked: u.pickVideo}, Label{Text: "MP4  /  MKV  /  MOV", TextColor: p.muted, Font: font(9, true)}, HSpacer{}}}, VSpacer{},
		}},
		Composite{StretchFactor: 4, Background: solid(p.panel), Layout: VBox{Margins: Margins{24, 22, 24, 22}, Spacing: 20}, Children: []Widget{VSpacer{},
			info(t("Detectar momentos", "Find moments"), t("Use áudio, fala ou movimento para localizar reações na gravação.", "Use sound, speech or motion to find reactions in your recording.")),
			info(t("Revisar os cortes", "Review clips"), t("Assista à prévia, ajuste o início e o fim e aprove seus favoritos.", "Watch a preview, trim the interval and approve your favorites.")),
			info(t("Exportar os aprovados", "Export your picks"), t("Salve clipes em MP4 ou leve a sequência de cortes para o Premiere.", "Save individual MP4 clips or send a sequence to Premiere.")), VSpacer{},
		}},
	}}
}

type clipRows struct{ u *desk }

func (s clipRows) ItemHeightDependsOnWidth() bool  { return false }
func (s clipRows) DefaultItemHeight() int          { return walk.IntFrom96DPI(60, s.u.queue.DPI()) }
func (s clipRows) ItemHeight(index, width int) int { return s.DefaultItemHeight() }
func (s clipRows) StyleItem(st *walk.ListItemStyle) {
	u := s.u
	i := st.Index()
	c := st.Canvas()
	if i < 0 || i >= len(u.visible) || c == nil {
		return
	}
	clip := u.project.Clips[u.visible[i]]
	p := u.colors()
	r := st.Bounds()
	selected := u.queue.CurrentIndex() == i
	bg := p.field
	if selected {
		bg = p.raised
	}
	fill(c, bg, r)
	fill(c, p.edge, walk.Rectangle{X: r.X + 12, Y: r.Y + r.Height - 1, Width: r.Width - 24, Height: 1})
	if selected {
		fill(c, p.accent, walk.Rectangle{X: r.X, Y: r.Y + 8, Width: 3, Height: r.Height - 16})
	}
	flags := walk.TextSingleLine | walk.TextEndEllipsis | walk.TextNoPrefix
	r.X += 12
	r.Width -= 23
	r.Y += 6
	r.Height = 24
	c.DrawText(clip.Name, u.queue.Font(), p.ink, r, flags)
	state := u.t("Pendente", "Pending")
	if clip.Status == "approved" {
		state = u.t("Aprovado", "Approved")
	} else if clip.Status == "rejected" {
		state = u.t("Rejeitado", "Rejected")
	}
	r.Y += 25
	c.DrawText(fmt.Sprintf("%s   ·   %s   ·   %s", clock(clip.Start), u.categoryLabel(clip.Category), state), u.queue.Font(), p.muted, r, flags)
}
