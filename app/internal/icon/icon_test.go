package icon

import (
	"encoding/binary"
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"testing"
)

// TestICOStructure 校验 ICO 封装格式。
//
// 这里踩过一次坑：条目表应该紧跟在 6 字节的 ICONDIR 之后，
// 曾经误写成从"图像数据偏移"开始，导致越界 panic。
func TestICOStructure(t *testing.T) {
	for _, s := range []Scheme{Brand, StateScheme(StateConnected), StateScheme(StateFailed)} {
		ico := ICOFor(s, 16, 32)

		if got := binary.LittleEndian.Uint16(ico[0:]); got != 0 {
			t.Fatalf("reserved 应为 0，得到 %d", got)
		}
		if got := binary.LittleEndian.Uint16(ico[2:]); got != 1 {
			t.Fatalf("type 应为 1，得到 %d", got)
		}
		count := int(binary.LittleEndian.Uint16(ico[4:]))
		if count != 2 {
			t.Fatalf("期望 2 张图，得到 %d", count)
		}

		wantOffset := 6 + 16*count
		for i := 0; i < count; i++ {
			e := 6 + i*16
			size := int(ico[e])
			imgLen := int(binary.LittleEndian.Uint32(ico[e+8:]))
			imgOff := int(binary.LittleEndian.Uint32(ico[e+12:]))

			if size != 16 && size != 32 {
				t.Fatalf("第 %d 张图尺寸异常 %d", i, size)
			}
			if imgOff != wantOffset {
				t.Fatalf("第 %d 张图偏移应为 %d，得到 %d", i, wantOffset, imgOff)
			}
			andLen := ((size + 31) / 32) * 4 * size
			wantLen := 40 + size*size*4 + andLen
			if imgLen != wantLen {
				t.Fatalf("第 %d 张图长度应为 %d，得到 %d", i, wantLen, imgLen)
			}
			if imgOff+imgLen > len(ico) {
				t.Fatalf("第 %d 张图越界", i)
			}
			if got := int(binary.LittleEndian.Uint32(ico[imgOff+8:])); got != size*2 {
				t.Fatalf("header 高度应为 %d，得到 %d", size*2, got)
			}
			wantOffset += imgLen
		}
	}
}

// TestICOSizeByteFor256 确认 256 按 ICO 规范写成 0。
func TestICOSizeByteFor256(t *testing.T) {
	ico := ICOFor(Brand, 256)
	if ico[6] != 0 || ico[7] != 0 {
		t.Fatalf("256 的宽高字节应为 0，得到 %d/%d", ico[6], ico[7])
	}
}

// TestRenderBasics 检查画出来的图确实有内容、四角透明、斜缝是深色底板。
func TestRenderBasics(t *testing.T) {
	const size = 64
	img := Render(size, Brand)

	for _, p := range [][2]int{{0, 0}, {size - 1, 0}, {0, size - 1}, {size - 1, size - 1}} {
		if a := img.RGBAAt(p[0], p[1]).A; a != 0 {
			t.Errorf("角 (%d,%d) 应该是透明的，alpha=%d", p[0], p[1], a)
		}
	}
	if !hasColorNear(img, 0xFB, 0xB0, 0x3B, 40) {
		t.Error("找不到橙色（右上那一半）")
	}
	if !hasColorNear(img, 0x1B, 0x75, 0xBC, 60) {
		t.Error("找不到蓝色（左下那一半）")
	}
	// 正中心落在斜缝上，应该是深底板色而不是橙或蓝
	if c := img.RGBAAt(size/2, size/2); c.R > 115 {
		t.Errorf("斜缝应该露出深色底板，实际 rgb(%d,%d,%d)", c.R, c.G, c.B)
	}
}

// TestDiagonalIsOriented 确认斜分方向：右上暖、左下冷。
func TestDiagonalIsOriented(t *testing.T) {
	const size = 128
	img := Render(size, Brand)

	tr := img.RGBAAt(size*3/4, size/4)
	if !(tr.R > 190 && tr.B < 140) {
		t.Errorf("右上应该是暖色（橙），实际 rgb(%d,%d,%d)", tr.R, tr.G, tr.B)
	}
	bl := img.RGBAAt(size/4, size*3/4)
	if !(bl.B > 130 && bl.R < 140) {
		t.Errorf("左下应该是冷色（蓝），实际 rgb(%d,%d,%d)", bl.R, bl.G, bl.B)
	}
}

// TestRenderIsResolutionIndependent 确认不同尺寸画的是同一个图形。
// 斜缝穿过中心这一点在每种尺寸下都必须成立。
func TestRenderIsResolutionIndependent(t *testing.T) {
	for _, size := range []int{16, 32, 64, 256} {
		img := Render(size, Brand)
		c := img.RGBAAt(size/2, size/2)
		if c.A < 200 {
			t.Errorf("%dpx: 中心不该透明，alpha=%d", size, c.A)
			continue
		}
		if c.R > 115 {
			t.Errorf("%dpx: 中心应该在斜缝上（深色），实际 rgb(%d,%d,%d)", size, c.R, c.G, c.B)
		}
	}
}

// TestStateSchemeKeepsStateVisible 确认托盘图标"状态色占大面积"。
//
// 斜分图形几乎铺满整个方块。如果照搬品牌配色（底板色当缝、两半当图形），
// 状态色就只剩一条细线看不见了 —— 这条测试防止那种回归。
func TestStateSchemeKeepsStateVisible(t *testing.T) {
	cases := map[string]RGB{
		"connected": StateConnected,
		"starting":  StateStarting,
		"failed":    StateFailed,
	}
	for name, st := range cases {
		const size = 64
		img := Render(size, StateScheme(st))

		colored, total := 0, 0
		for y := 0; y < size; y++ {
			for x := 0; x < size; x++ {
				c := img.RGBAAt(x, y)
				if c.A < 200 {
					continue
				}
				total++
				// 状态色是饱和的：不是白，也不是很深
				isWhite := c.R > 244 && c.G > 244 && c.B > 244
				if !isWhite && int(c.R)+int(c.G)+int(c.B) > 130 {
					colored++
				}
			}
		}
		if total == 0 {
			t.Fatalf("%s: 没有不透明像素", name)
		}
		if r := float64(colored) / float64(total); r < 0.70 {
			t.Errorf("%s: 状态色只占 %.0f%%，应该占大部分（>=70%%）", name, r*100)
		}
	}
}

// TestTrayStatePreview 把四种托盘状态并排画出来，方便肉眼确认"状态一眼能分辨"。
// 设 FRPCGUI_ICON_PREVIEW=1 才会写文件。
func TestTrayStatePreview(t *testing.T) {
	if os.Getenv("FRPCGUI_ICON_PREVIEW") != "1" {
		t.Skip("未设置 FRPCGUI_ICON_PREVIEW=1")
	}
	states := []struct {
		name string
		c    RGB
	}{
		{"connected", StateConnected},
		{"starting", StateStarting},
		{"stopped", StateStopped},
		{"failed", StateFailed},
	}

	const cell = 96
	canvas := image.NewRGBA(image.Rect(0, 0, cell*len(states), 200))
	fillRGBA(canvas, color.RGBA{0xF7, 0xF9, 0xFB, 0xFF})
	for i, st := range states {
		s := StateScheme(st.c)
		drawScaled(canvas, Render(16, s), i*cell+8, 8, 4)  // 16px 放大 4 倍
		drawScaled(canvas, Render(32, s), i*cell+8, 80, 3) // 32px 放大 3 倍
	}

	out := filepath.Join(os.TempDir(), "frpc-tray-states.png")
	f, err := os.Create(out)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	if err := png.Encode(f, canvas); err != nil {
		t.Fatal(err)
	}
	t.Logf("托盘状态预览已写入 %s", out)
}

func fillRGBA(img *image.RGBA, c color.RGBA) {
	b := img.Bounds()
	for y := b.Min.Y; y < b.Max.Y; y++ {
		for x := b.Min.X; x < b.Max.X; x++ {
			img.SetRGBA(x, y, c)
		}
	}
}

func drawScaled(dst *image.RGBA, src *image.RGBA, ox, oy, scale int) {
	size := src.Bounds().Dx()
	for y := 0; y < size*scale; y++ {
		for x := 0; x < size*scale; x++ {
			c := src.RGBAAt(x/scale, y/scale)
			if c.A == 0 {
				continue
			}
			px, py := ox+x, oy+y
			if !(image.Point{px, py}).In(dst.Bounds()) {
				continue
			}
			bg := dst.RGBAAt(px, py)
			a := float64(c.A) / 255
			dst.SetRGBA(px, py, color.RGBA{
				R: uint8(float64(c.R)*a + float64(bg.R)*(1-a)),
				G: uint8(float64(c.G)*a + float64(bg.G)*(1-a)),
				B: uint8(float64(c.B)*a + float64(bg.B)*(1-a)),
				A: 255,
			})
		}
	}
}

// hasColorNear 在图里找有没有和给定颜色接近的不透明像素。
func hasColorNear(img *image.RGBA, r, g, b uint8, tol int) bool {
	near := func(a, want uint8) bool {
		d := int(a) - int(want)
		if d < 0 {
			d = -d
		}
		return d <= tol
	}
	bounds := img.Bounds()
	for y := bounds.Min.Y; y < bounds.Max.Y; y++ {
		for x := bounds.Min.X; x < bounds.Max.X; x++ {
			c := img.RGBAAt(x, y)
			if c.A > 200 && near(c.R, r) && near(c.G, g) && near(c.B, b) {
				return true
			}
		}
	}
	return false
}
