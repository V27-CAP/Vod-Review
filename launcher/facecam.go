package main

import (
	"context"
	"encoding/json"
	"fmt"
	"image/jpeg"
	"math"
	"os"
	"path/filepath"

	"github.com/lxn/walk"
	. "github.com/lxn/walk/declarative"
)

func loadJPEG(path string) (walk.Image, int, int, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, 0, 0, err
	}
	defer f.Close()
	img, err := jpeg.Decode(f)
	if err != nil {
		return nil, 0, 0, err
	}
	b := img.Bounds()
	bmp, err := walk.NewBitmapFromImageForDPI(img, 96)
	if err != nil {
		return nil, 0, 0, err
	}
	return bmp, b.Dx(), b.Dy(), nil
}

func probeVideoSync(videoPath string, timestamp *float64, extractFrame bool) (ProbeResult, error) {
	var out ProbeResult
	tmp, err := os.MkdirTemp("", "vodcut-probe-")
	if err != nil {
		return out, err
	}
	defer os.RemoveAll(tmp)
	req := map[string]any{
		"action":        "probe",
		"path":          videoPath,
		"extract_frame": extractFrame,
		"cancel":        filepath.Join(tmp, "cancel"),
	}
	if timestamp != nil {
		req["timestamp"] = *timestamp
	}
	raw, err := runWorker(context.Background(), req, func(string, int) {})
	if err != nil {
		return out, err
	}
	err = json.Unmarshal([]byte(raw), &out)
	return out, err
}

func formatROI(roi *FacecamROI, width, height int, pt bool) string {
	if roi == nil || roi.W < 0.02 || roi.H < 0.02 {
		if pt {
			return "Nenhuma região marcada"
		}
		return "No region selected"
	}
	if width > 0 && height > 0 {
		pxW := int(math.Round(roi.W * float64(width)))
		pxH := int(math.Round(roi.H * float64(height)))
		if pt {
			return fmt.Sprintf("Facecam: X=%.0f%% Y=%.0f%% · L=%.0f%% A=%.0f%% (%d×%d px)", roi.X*100, roi.Y*100, roi.W*100, roi.H*100, pxW, pxH)
		}
		return fmt.Sprintf("Facecam: X=%.0f%% Y=%.0f%% · W=%.0f%% H=%.0f%% (%d×%d px)", roi.X*100, roi.Y*100, roi.W*100, roi.H*100, pxW, pxH)
	}
	if pt {
		return fmt.Sprintf("Facecam: X=%.0f%% Y=%.0f%% · L=%.0f%% A=%.0f%%", roi.X*100, roi.Y*100, roi.W*100, roi.H*100)
	}
	return fmt.Sprintf("Facecam: X=%.0f%% Y=%.0f%% · W=%.0f%% H=%.0f%%", roi.X*100, roi.Y*100, roi.W*100, roi.H*100)
}

func (u *desk) facecamDialog(owner walk.Form, videoPath string, probed ProbeResult, current *FacecamROI) (*FacecamROI, bool) {
	t := u.t
	if owner == nil {
		owner = u.mw
	}
	bmp, imgW, imgH, err := loadJPEG(probed.Frame)
	if err != nil {
		walk.MsgBox(owner, title, t("Não foi possível carregar o quadro do vídeo: ", "Could not load video frame: ")+err.Error(), walk.MsgBoxIconWarning)
		return current, false
	}
	defer func() {
		if bmp != nil {
			bmp.Dispose()
		}
	}()

	var roi *FacecamROI
	if current != nil {
		cp := *current
		roi = &cp
	}

	var dlg *walk.Dialog
	var canvas *walk.CustomWidget
	var infoLabel, frameLabel *walk.Label
	var timeSlider *walk.Slider
	var loadBtn, saveBtn, cancelBtn *walk.PushButton
	var imgRect walk.Rectangle
	var dragging bool
	var anchorX, anchorY float64

	clamp01 := func(v float64) float64 {
		if v < 0 {
			return 0
		}
		if v > 1 {
			return 1
		}
		return v
	}
	updateInfo := func() {
		if infoLabel != nil {
			infoLabel.SetText(formatROI(roi, probed.Meta.Width, probed.Meta.Height, u.Lang != "en"))
		}
	}
	computeImageRect := func(bounds walk.Rectangle) walk.Rectangle {
		if imgW <= 0 || imgH <= 0 || bounds.Width <= 0 || bounds.Height <= 0 {
			return bounds
		}
		scaleX := float64(bounds.Width) / float64(imgW)
		scaleY := float64(bounds.Height) / float64(imgH)
		scale := math.Min(scaleX, scaleY)
		w := int(math.Round(float64(imgW) * scale))
		h := int(math.Round(float64(imgH) * scale))
		x := bounds.X + (bounds.Width-w)/2
		y := bounds.Y + (bounds.Height-h)/2
		return walk.Rectangle{X: x, Y: y, Width: w, Height: h}
	}
	toNorm := func(x, y int) (float64, float64) {
		if imgRect.Width <= 0 || imgRect.Height <= 0 {
			return 0, 0
		}
		nx := clamp01(float64(x-imgRect.X) / float64(imgRect.Width))
		ny := clamp01(float64(y-imgRect.Y) / float64(imgRect.Height))
		return nx, ny
	}
	setPreset := func(x, y, w, h float64) {
		roi = &FacecamROI{X: x, Y: y, W: w, H: h}
		updateInfo()
		if canvas != nil {
			canvas.Invalidate()
		}
	}

	initPct := 15
	if probed.Meta.Duration > 0 {
		initPct = int(math.Round(100 * probed.Timestamp / probed.Meta.Duration))
		if initPct < 0 {
			initPct = 0
		}
		if initPct > 98 {
			initPct = 98
		}
	}

	err = u.dialog(Dialog{
		AssignTo:      &dlg,
		Title:         t("Marcar região da Facecam", "Select Facecam region"),
		DefaultButton: &saveBtn,
		CancelButton:  &cancelBtn,
		Size:          Size{Width: 780, Height: 560},
		MinSize:       Size{Width: 600, Height: 440},
		Layout:        VBox{Margins: Margins{14, 12, 14, 12}, Spacing: 8},
		Children: []Widget{
			Label{Text: t("Clique e arraste sobre o quadro do vídeo para marcar a região da facecam.", "Click and drag on the video frame to mark the facecam region."), Font: font(10, true)},
			CustomWidget{
				AssignTo:            &canvas,
				MinSize:             Size{Width: 480, Height: 300},
				ClearsBackground:    false,
				InvalidatesOnResize: true,
				PaintMode:           PaintBuffered,
				PaintPixels: func(c *walk.Canvas, updateBounds walk.Rectangle) error {
					bounds := canvas.ClientBoundsPixels()
					bg, err := walk.NewSolidColorBrush(walk.RGB(20, 20, 24))
					if err == nil {
						defer bg.Dispose()
						_ = c.FillRectanglePixels(bg, bounds)
					}
					imgRect = computeImageRect(bounds)
					if bmp != nil && imgRect.Width > 0 && imgRect.Height > 0 {
						_ = c.DrawImageStretchedPixels(bmp, imgRect)
					}
					if roi != nil && roi.W >= 0.02 && roi.H >= 0.02 && imgRect.Width > 0 && imgRect.Height > 0 {
						rx := imgRect.X + int(math.Round(roi.X*float64(imgRect.Width)))
						ry := imgRect.Y + int(math.Round(roi.Y*float64(imgRect.Height)))
						rw := int(math.Round(roi.W * float64(imgRect.Width)))
						rh := int(math.Round(roi.H * float64(imgRect.Height)))
						rect := walk.Rectangle{X: rx, Y: ry, Width: rw, Height: rh}

						outerBrush, err := walk.NewSolidColorBrush(walk.RGB(255, 64, 88))
						if err == nil {
							defer outerBrush.Dispose()
							if pen, err := walk.NewGeometricPen(walk.PenSolid, 3, outerBrush); err == nil {
								defer pen.Dispose()
								_ = c.DrawRectanglePixels(pen, rect)
							}
						}
						innerPen, err := walk.NewCosmeticPen(walk.PenDash, walk.RGB(255, 245, 160))
						if err == nil {
							defer innerPen.Dispose()
							inner := walk.Rectangle{X: rx + 2, Y: ry + 2, Width: maxInt(1, rw-4), Height: maxInt(1, rh-4)}
							_ = c.DrawRectanglePixels(innerPen, inner)
						}
						// Corner handles for clear visual affordance.
						handleBrush, err := walk.NewSolidColorBrush(walk.RGB(255, 245, 160))
						if err == nil {
							defer handleBrush.Dispose()
							hs := 8
							corners := []walk.Rectangle{
								{X: rx - hs/2, Y: ry - hs/2, Width: hs, Height: hs},
								{X: rx + rw - hs/2, Y: ry - hs/2, Width: hs, Height: hs},
								{X: rx - hs/2, Y: ry + rh - hs/2, Width: hs, Height: hs},
								{X: rx + rw - hs/2, Y: ry + rh - hs/2, Width: hs, Height: hs},
							}
							for _, cr := range corners {
								_ = c.FillRectanglePixels(handleBrush, cr)
							}
						}
					}
					return nil
				},
				OnMouseDown: func(x, y int, button walk.MouseButton) {
					if button != walk.LeftButton {
						return
					}
					dragging = true
					anchorX, anchorY = toNorm(x, y)
					roi = &FacecamROI{X: anchorX, Y: anchorY, W: 0, H: 0}
					canvas.Invalidate()
				},
				OnMouseMove: func(x, y int, button walk.MouseButton) {
					if !dragging {
						return
					}
					nx, ny := toNorm(x, y)
					x0, x1 := math.Min(anchorX, nx), math.Max(anchorX, nx)
					y0, y1 := math.Min(anchorY, ny), math.Max(anchorY, ny)
					roi = &FacecamROI{X: x0, Y: y0, W: x1 - x0, H: y1 - y0}
					updateInfo()
					canvas.Invalidate()
				},
				OnMouseUp: func(x, y int, button walk.MouseButton) {
					if !dragging || button != walk.LeftButton {
						return
					}
					dragging = false
					if roi != nil && (roi.W < 0.02 || roi.H < 0.02) {
						roi = nil
					}
					updateInfo()
					canvas.Invalidate()
				},
			},
			Composite{Layout: HBox{MarginsZero: true}, Children: []Widget{
				Label{AssignTo: &frameLabel, Text: fmt.Sprintf(t("Quadro em %s", "Frame at %s"), clock(probed.Timestamp)), NoPrefix: true},
				Slider{
					AssignTo: &timeSlider,
					MinValue: 0,
					MaxValue: 98,
					Value:    initPct,
					OnValueChanged: func() {
						if frameLabel != nil && probed.Meta.Duration > 0 {
							ts := probed.Meta.Duration * float64(timeSlider.Value()) / 100.0
							frameLabel.SetText(fmt.Sprintf(t("Quadro em %s", "Frame at %s"), clock(ts)))
						}
					},
				},
				PushButton{AssignTo: &loadBtn, Text: t("Carregar quadro", "Load frame"), OnClicked: func() {
					ts := probed.Meta.Duration * float64(timeSlider.Value()) / 100.0
					loadBtn.SetEnabled(false)
					go func() {
						nextProbe, perr := probeVideoSync(videoPath, &ts, true)
						dlg.Synchronize(func() {
							loadBtn.SetEnabled(true)
							if perr != nil {
								walk.MsgBox(dlg, title, perr.Error(), walk.MsgBoxIconWarning)
								return
							}
							newBmp, nw, nh, lerr := loadJPEG(nextProbe.Frame)
							if lerr != nil {
								walk.MsgBox(dlg, title, lerr.Error(), walk.MsgBoxIconWarning)
								return
							}
							if bmp != nil {
								bmp.Dispose()
							}
							bmp, imgW, imgH = newBmp, nw, nh
							probed = nextProbe
							frameLabel.SetText(fmt.Sprintf(t("Quadro em %s", "Frame at %s"), clock(probed.Timestamp)))
							canvas.Invalidate()
						})
					}()
				}},
			}},
			Composite{Layout: HBox{MarginsZero: true}, Children: []Widget{
				Label{Text: t("Atalhos:", "Presets:")},
				PushButton{Text: t("Inf. Dir.", "Bottom-Right"), OnClicked: func() { setPreset(0.72, 0.66, 0.26, 0.31) }},
				PushButton{Text: t("Inf. Esq.", "Bottom-Left"), OnClicked: func() { setPreset(0.02, 0.66, 0.26, 0.31) }},
				PushButton{Text: t("Sup. Dir.", "Top-Right"), OnClicked: func() { setPreset(0.72, 0.03, 0.26, 0.31) }},
				PushButton{Text: t("Limpar", "Clear"), OnClicked: func() {
					roi = nil
					updateInfo()
					canvas.Invalidate()
				}},
				HSpacer{},
			}},
			Label{AssignTo: &infoLabel, Text: formatROI(roi, probed.Meta.Width, probed.Meta.Height, u.Lang != "en"), NoPrefix: true},
			Composite{Layout: HBox{MarginsZero: true}, Children: []Widget{
				HSpacer{},
				PushButton{AssignTo: &cancelBtn, Text: t("Cancelar", "Cancel"), OnClicked: func() { dlg.Cancel() }},
				PushButton{AssignTo: &saveBtn, Text: t("Salvar região", "Save region"), OnClicked: func() { dlg.Accept() }},
			}},
		},
	}).Create(owner)
	if err != nil {
		u.error(err)
		return current, false
	}
	if dlg.Run() != walk.DlgCmdOK {
		return current, false
	}
	return roi, true
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}
