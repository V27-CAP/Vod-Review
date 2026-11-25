package main

import (
	"fmt"
	"github.com/lxn/walk"
	. "github.com/lxn/walk/declarative"
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
func (u *desk) welcomePanel() Composite {
	p, t := u.colors(), u.t
	return Composite{AssignTo: &u.welcome, Background: solid(p.panel), Layout: VBox{Margins: Margins{36, 24, 36, 24}, Spacing: 14}, Children: []Widget{
		VSpacer{}, Label{Text: t("Encontre os destaques da sua live", "Find highlights in your stream"), Font: font(25, true), TextColor: p.ink},
		TextLabel{Text: t("Arraste uma gravação para cá ou escolha um arquivo MP4, MKV ou MOV.", "Drop a recording here or choose an MP4, MKV or MOV file."), TextColor: p.muted},
		Composite{Layout: HBox{MarginsZero: true}, Children: []Widget{PushButton{Name: "primary", AssignTo: &u.welcomeLoad, Text: t("Escolher vídeo…", "Choose video…"), MinSize: Size{Width: 165, Height: 40}, OnClicked: u.pickVideo}, HSpacer{}}},
		VSpacer{},
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
