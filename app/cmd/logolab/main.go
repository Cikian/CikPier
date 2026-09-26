// Command logolab 用来比较几版候选图标。
//
//	go run ./cmd/logolab -out logo-candidates.png
//
// 这是个"设计试验台"，不参与打包。定稿之后把选中的图形搬进 internal/icon。
package main

import (
	"flag"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"math"
	"os"

	"frpcgui/internal/icon"
)

// ---------------------------------------------------------------- 基础工具

const tileRadius = 0.22

func inTile(x, y float64) bool {
	if x < 0 || x > 1 || y < 0 || y > 1 {
		return false
	}
	r := tileRadius
	cx := math.Min(math.Max(x, r), 1-r)
	cy := math.Min(math.Max(y, r), 1-r)
	dx, dy := x-cx, y-cy
	return dx*dx+dy*dy <= r*r
}

func clamp8(v float64) uint8 {
	if v <= 0 {
		return 0
	}
	if v >= 255 {
		return 255
	}
	return uint8(v + 0.5)
}

func lerp(a, b icon.RGB, t float64) icon.RGB {
	return icon.RGB{
		R: clamp8(float64(a.R) + (float64(b.R)-float64(a.R))*t),
		G: clamp8(float64(a.G) + (float64(b.G)-float64(a.G))*t),
		B: clamp8(float64(a.B) + (float64(b.B)-float64(a.B))*t),
	}
}

func grad(a, b icon.RGB, y float64) icon.RGB {
	return lerp(a, b, math.Min(math.Max(y, 0), 1))
}

// 品牌色（取自 Cikian 标识）
var (
	navyTop = icon.RGB{R: 0x1E, G: 0x40, B: 0x72}
	navyBot = icon.RGB{R: 0x10, G: 0x22, B: 0x3F}
	amber   = icon.RGB{R: 0xFB, G: 0xB0, B: 0x3B}
	orange  = icon.RGB{R: 0xF5, G: 0x82, B: 0x1F}
	skyBlue = icon.RGB{R: 0x59, G: 0xA9, B: 0xE6}
	blue    = icon.RGB{R: 0x1B, G: 0x75, B: 0xBC}
)

// paint 返回某个点的颜色；covered=false 表示这里不该画（在圆角外）。
type paintFunc func(x, y float64) (icon.RGB, bool)

const ss = 4 // 超采样倍数

func raster(size int, paint paintFunc) *image.RGBA {
	img := image.NewRGBA(image.Rect(0, 0, size, size))
	inv := 1.0 / float64(size)
	total := float64(ss * ss)
	for py := 0; py < size; py++ {
		for px := 0; px < size; px++ {
			var r, g, b, a float64
			for sy := 0; sy < ss; sy++ {
				for sx := 0; sx < ss; sx++ {
					x := (float64(px) + (float64(sx)+0.5)/ss) * inv
					y := (float64(py) + (float64(sy)+0.5)/ss) * inv
					if !inTile(x, y) {
						continue
					}
					c, ok := paint(x, y)
					if !ok {
						continue
					}
					r += float64(c.R)
					g += float64(c.G)
					b += float64(c.B)
					a++
				}
			}
			i := img.PixOffset(px, py)
			if a == 0 {
				continue
			}
			img.Pix[i+0] = clamp8(r / a)
			img.Pix[i+1] = clamp8(g / a)
			img.Pix[i+2] = clamp8(b / a)
			img.Pix[i+3] = clamp8(a / total * 255)
		}
	}
	return img
}

// chevronBand 判断点是否落在"向上的折带"里（顶端在 apexY）。
func chevronBand(x, y, apexY, halfW, h, stroke float64) bool {
	dy := y - apexY
	if dy < 0 || dy > h+stroke {
		return false
	}
	slope := halfW / h
	ax := math.Abs(x - 0.5)
	if dy <= h {
		if ax > slope*dy {
			return false
		}
		inner := dy - stroke
		return !(inner > 0 && ax <= slope*inner)
	}
	// 两条"脚"：内顶点以下的部分
	inner := dy - stroke
	if inner <= 0 || inner > h {
		return false
	}
	return ax >= slope*inner && ax <= slope*h
}

// ---------------------------------------------------------------- 候选图形

// A：两枚向上的箭头（当前方案）
func paintChevrons(x, y float64) (icon.RGB, bool) {
	const halfW, h, stroke = 0.255, 0.175, 0.125
	if chevronBand(x, y, 0.155, halfW, h, stroke) {
		return grad(amber, orange, y), true
	}
	if chevronBand(x, y, 0.545, halfW, h, stroke) {
		return grad(skyBlue, blue, y), true
	}
	return grad(navyTop, navyBot, y), true
}

// inRoundRect 判断点是否落在圆角矩形里（坐标 0..1）。
func inRoundRect(x, y, x0, y0, x1, y1, r float64) bool {
	if x < x0 || x > x1 || y < y0 || y > y1 {
		return false
	}
	cx := math.Min(math.Max(x, x0+r), x1-r)
	cy := math.Min(math.Max(y, y0+r), y1-r)
	dx, dy := x-cx, y-cy
	return dx*dx+dy*dy <= r*r
}

// B：折角 C —— 粗方环在右边开口，上半橙下半蓝
//
// 第一版把 C 画成"上横 + 左竖 + 下横 + 斜切端头"，斜切太陡，
// 小尺寸糊成一团、大尺寸看着像数字 7。改成"粗方环开口"就稳了。
func paintAngularC(x, y float64) (icon.RGB, bool) {
	outer := inRoundRect(x, y, 0.20, 0.20, 0.80, 0.80, 0.13)
	inner := inRoundRect(x, y, 0.41, 0.41, 0.59, 0.59, 0.05)
	gap := x > 0.66 && math.Abs(y-0.5) < 0.155 // 右边开口，才是 C 而不是 O
	if !outer || inner || gap {
		return grad(navyTop, navyBot, y), true
	}
	if y < 0.5 {
		return grad(amber, orange, y), true
	}
	return grad(skyBlue, blue, y), true
}

// C：同心 —— 一粗环 + 中心实心点
func paintRings(x, y float64) (icon.RGB, bool) {
	dx, dy := x-0.5, y-0.5
	d := math.Sqrt(dx*dx + dy*dy)
	if d >= 0.195 && d <= 0.325 {
		return grad(amber, orange, y), true
	}
	if d <= 0.115 {
		return grad(skyBlue, blue, y), true
	}
	return grad(navyTop, navyBot, y), true
}

// D：等距立方体（呼应标识里的折角）
func paintCube(x, y float64) (icon.RGB, bool) {
	inPoly := func(pts [][2]float64) bool {
		n := len(pts)
		inside := false
		for i, j := 0, n-1; i < n; j, i = i, i+1 {
			xi, yi := pts[i][0], pts[i][1]
			xj, yj := pts[j][0], pts[j][1]
			if (yi > y) != (yj > y) && x < (xj-xi)*(y-yi)/(yj-yi)+xi {
				inside = !inside
			}
		}
		return inside
	}
	top := [][2]float64{{0.50, 0.175}, {0.795, 0.335}, {0.50, 0.495}, {0.205, 0.335}}
	left := [][2]float64{{0.205, 0.335}, {0.50, 0.495}, {0.50, 0.815}, {0.205, 0.655}}
	right := [][2]float64{{0.50, 0.495}, {0.795, 0.335}, {0.795, 0.655}, {0.50, 0.815}}

	if inPoly(top) {
		return grad(amber, orange, y), true
	}
	if inPoly(right) {
		return grad(skyBlue, blue, y), true
	}
	if inPoly(left) {
		return lerp(icon.RGB{R: 0x2A, G: 0x5A, B: 0x99}, navyBot, (y-0.335)/0.48), true
	}
	return grad(navyTop, navyBot, y), true
}

// E：斜分双色 —— 一道斜缝把方块分成橙蓝两半
func paintDiagonal(x, y float64) (icon.RGB, bool) {
	s := x - y // 以 y=x 这条对角线为界
	const gap = 0.085
	switch {
	case s > gap:
		return grad(amber, orange, y), true
	case s < -gap:
		return grad(skyBlue, blue, y), true
	default:
		return grad(navyTop, navyBot, y), true
	}
}

// ---------------------------------------------------------------- 拼图输出

type candidate struct {
	name  string
	paint paintFunc
}

func main() {
	out := flag.String("out", "logo-candidates.png", "输出文件")
	flag.Parse()

	cands := []candidate{
		{"A 双箭头（当前）", paintChevrons},
		{"B 折角C", paintAngularC},
		{"C 同心环", paintRings},
		{"D 等距立方", paintCube},
		{"E 斜分双色", paintDiagonal},
	}

	const (
		big    = 132
		mid    = 34
		small  = 16
		pad    = 16
		zoom   = 7
		colGap = 16
	)
	colW := big + colGap
	rowH := big + pad + mid + pad + small + pad + small*zoom

	w := pad + len(cands)*colW
	h := pad + rowH + pad
	canvas := image.NewRGBA(image.Rect(0, 0, w, h))
	fill(canvas, color.RGBA{0xF7, 0xF9, 0xFB, 0xFF})

	for i, c := range cands {
		x0 := pad + i*colW
		y := pad

		blit(canvas, raster(big, c.paint), x0, y) // 132：看整体造型
		y += big + pad

		blit(canvas, raster(mid, c.paint), x0, y) // 34：任务栏大小
		y += mid + pad

		blit(canvas, raster(small, c.paint), x0, y) // 16：托盘真实大小
		y += small + pad

		zoomInto(canvas, raster(small, c.paint), x0, y, zoom) // 16 放大 7 倍
	}

	f, err := os.Create(*out)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	defer f.Close()
	if err := png.Encode(f, canvas); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	fmt.Printf("写入 %s（%d×%d）\n", *out, w, h)
	fmt.Println("每列从上到下：132px / 34px / 16px 原尺寸 / 16px 放大 7 倍")
	fmt.Println("列顺序从左到右：")
	for i, c := range cands {
		fmt.Printf("  %d. %s\n", i+1, c.name)
	}
}

func fill(img *image.RGBA, c color.RGBA) {
	b := img.Bounds()
	for y := b.Min.Y; y < b.Max.Y; y++ {
		for x := b.Min.X; x < b.Max.X; x++ {
			img.SetRGBA(x, y, c)
		}
	}
}

func blit(dst, src *image.RGBA, ox, oy int) {
	size := src.Bounds().Dx()
	for y := 0; y < size; y++ {
		for x := 0; x < size; x++ {
			blend(dst, ox+x, oy+y, src.RGBAAt(x, y))
		}
	}
}

func zoomInto(dst, src *image.RGBA, ox, oy, scale int) {
	size := src.Bounds().Dx()
	for y := 0; y < size*scale; y++ {
		for x := 0; x < size*scale; x++ {
			blend(dst, ox+x, oy+y, src.RGBAAt(x/scale, y/scale))
		}
	}
}

func blend(dst *image.RGBA, x, y int, c color.RGBA) {
	if !(image.Point{x, y}).In(dst.Bounds()) || c.A == 0 {
		return
	}
	bg := dst.RGBAAt(x, y)
	a := float64(c.A) / 255
	dst.SetRGBA(x, y, color.RGBA{
		R: uint8(float64(c.R)*a + float64(bg.R)*(1-a)),
		G: uint8(float64(c.G)*a + float64(bg.G)*(1-a)),
		B: uint8(float64(c.B)*a + float64(bg.B)*(1-a)),
		A: 255,
	})
}
