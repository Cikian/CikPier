// Package icon 负责生成程序图标和托盘图标。
//
// 为什么要自己画：托盘图标需要按连接状态换颜色（绿/黄/灰/红），
// 程序图标需要一整套尺寸（16 到 256）。放一堆 .ico 文件既容易在升级时丢，
// 也没法按状态变色。用一个几何定义 + 光栅化器，两件事一起解决。
//
// 图形语言取自 Cikian 品牌标识：直角、斜切、橙 + 蓝。
// 定稿的图形是"一道斜缝把方块分成两半"—— 左上橙、右下蓝，
// 斜缝本身留着底板色，形成一条干净的分界。
package icon

import (
	"bytes"
	"image"
	"image/png"
	"math"
)

// RGB 是很朴素的三通道颜色，避免调用方引入 color 包。
type RGB struct{ R, G, B uint8 }

// Scheme 描述一套配色。
//
// Tile 是底板（斜缝露出来的就是它），Warm / Cool 是斜分出来的两半。
// 斜缝方向沿 y = x 这条主对角线。
type Scheme struct {
	TileTop, TileBottom RGB
	WarmTop, WarmBottom RGB // 右上那一半
	CoolTop, CoolBottom RGB // 左下那一半
}

// Brand 是程序图标配色，取自 Cikian 标识里的橙和蓝。
var Brand = Scheme{
	TileTop:    RGB{0x1E, 0x40, 0x72}, // 藏青（标识里的深蓝）
	TileBottom: RGB{0x10, 0x22, 0x3F},
	WarmTop:    RGB{0xFB, 0xB0, 0x3B}, // 琥珀
	WarmBottom: RGB{0xF5, 0x82, 0x1F}, // 橙
	CoolTop:    RGB{0x59, 0xA9, 0xE6}, // 亮蓝
	CoolBottom: RGB{0x1B, 0x75, 0xBC}, // 蓝
}

// StateScheme 是托盘图标配色：整块用状态色，斜缝用白色。
//
// 托盘只有 16px，靠形状去分辨状态太吃力，所以让**整块底板**承担状态。
// 斜分图形几乎铺满整个方块，如果照搬品牌配色（底板色当缝），
// 状态色就只剩一条细线看不见了 —— 所以这里把两者对调：
// 两半用状态色，缝用白色。效果是"一个状态色方块 + 一道白斜线"，一眼可辨。
func StateScheme(base RGB) Scheme {
	white := RGB{0xFF, 0xFF, 0xFF}
	return Scheme{
		TileTop:    white,
		TileBottom: white,
		WarmTop:    mix(base, white, 0.16),
		WarmBottom: base,
		CoolTop:    base,
		CoolBottom: scale(base, 0.78),
	}
}

// 托盘用的四种状态色。
var (
	StateConnected = RGB{0x10, 0xB9, 0x81} // 薄荷绿：已连接
	StateStarting  = RGB{0xF5, 0x9E, 0x0B} // 琥珀：连接中
	StateStopped   = RGB{0x94, 0xA3, 0xB8} // 灰：未运行
	StateFailed    = RGB{0xEF, 0x44, 0x44} // 红：失败
)

// ---------------------------------------------------------------- 几何定义
//
// 所有坐标都是 0..1 的归一化值，这样任意尺寸都能算出同一张图。
const (
	tileRadius = 0.22  // 底板圆角半径
	splitGap   = 0.085 // 斜缝的半宽（沿 x-y 方向度量）
)

// inTile 判断点是否落在圆角方形底板上。
func inTile(x, y float64) bool {
	r := tileRadius
	if x < 0 || x > 1 || y < 0 || y > 1 {
		return false
	}
	cx := math.Min(math.Max(x, r), 1-r)
	cy := math.Min(math.Max(y, r), 1-r)
	dx, dy := x-cx, y-cy
	return dx*dx+dy*dy <= r*r
}

// ---------------------------------------------------------------- 光栅化

// supersample 是每个像素在每个方向上的采样数。4 已经足够平滑，
// 再高只是浪费时间 —— 16px 的托盘图标总渲染量也就 16*16*16 个采样点。
const supersample = 4

// Render 渲染一张 size×size 的图标。
func Render(size int, s Scheme) *image.RGBA {
	img := image.NewRGBA(image.Rect(0, 0, size, size))
	inv := 1.0 / float64(size)
	total := float64(supersample * supersample)

	for py := 0; py < size; py++ {
		for px := 0; px < size; px++ {
			var accR, accG, accB, accA float64

			for sy := 0; sy < supersample; sy++ {
				for sx := 0; sx < supersample; sx++ {
					x := (float64(px) + (float64(sx)+0.5)/supersample) * inv
					y := (float64(py) + (float64(sy)+0.5)/supersample) * inv

					if !inTile(x, y) {
						continue
					}
					// 以 y = x 这条主对角线为界：右下（x-y 大）是暖色，左上是冷色，
					// 中间留一条斜缝露出底板色。
					var c RGB
					switch d := x - y; {
					case d > splitGap:
						c = gradient(s.WarmTop, s.WarmBottom, y)
					case d < -splitGap:
						c = gradient(s.CoolTop, s.CoolBottom, y)
					default:
						c = gradient(s.TileTop, s.TileBottom, y)
					}
					accR += float64(c.R)
					accG += float64(c.G)
					accB += float64(c.B)
					accA++
				}
			}

			i := img.PixOffset(px, py)
			if accA == 0 {
				continue
			}
			// 颜色按"有覆盖的采样点"取平均，避免边缘被透明采样点拉暗；
			// alpha 则按全部采样点算，这才是抗锯齿。
			img.Pix[i+0] = clamp8(accR / accA)
			img.Pix[i+1] = clamp8(accG / accA)
			img.Pix[i+2] = clamp8(accB / accA)
			img.Pix[i+3] = clamp8(accA / total * 255)
		}
	}
	return img
}

// ---------------------------------------------------------------- 输出格式

// PNG 把图编码成 PNG 字节。
func PNG(img *image.RGBA) ([]byte, error) {
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// ICO 把若干张不同尺寸的图打包成 Windows ICO。
//
// 结构：ICONDIR(6) + N×ICONDIRENTRY(16) + N×图像数据
// 图像数据 = BITMAPINFOHEADER(40) + XOR 位图（自下而上 BGRA）+ AND 掩码（全 0，靠 alpha 透明）
//
// 这里踩过一次坑：条目表要紧跟在 6 字节的 ICONDIR 之后，
// 曾经误写成从"图像数据偏移"开始，直接越界 panic。
func ICO(imgs []*image.RGBA) []byte {
	const entrySize = 16
	const dirSize = 6
	const headerSize = 40

	offset := dirSize + entrySize*len(imgs)
	out := make([]byte, offset)

	le16(out[0:], 0) // reserved
	le16(out[2:], 1) // type = 1 (icon)
	le16(out[4:], uint16(len(imgs)))

	for idx, img := range imgs {
		size := img.Bounds().Dx()
		andRow := ((size + 31) / 32) * 4
		andLen := andRow * size
		imgLen := headerSize + size*size*4 + andLen

		body := make([]byte, imgLen)
		le32(body[0:], headerSize)
		le32(body[4:], uint32(size))
		le32(body[8:], uint32(size*2)) // 高度写两倍：XOR 位图 + AND 掩码
		le16(body[12:], 1)             // planes
		le16(body[14:], 32)            // 位深

		// ICO 内部是自下而上存的，要翻过来
		for y := 0; y < size; y++ {
			src := img.PixOffset(0, size-1-y)
			dst := headerSize + y*size*4
			for x := 0; x < size; x++ {
				p := src + x*4
				q := dst + x*4
				body[q+0] = img.Pix[p+2] // B
				body[q+1] = img.Pix[p+1] // G
				body[q+2] = img.Pix[p+0] // R
				body[q+3] = img.Pix[p+3] // A
			}
		}
		// AND 掩码保持全 0（透明度完全由 alpha 通道决定）

		e := dirSize + idx*entrySize
		// ⚠ 256 在这里写成 0，这是 ICO 的规定（0 表示 256），不是笔误
		out[e+0] = sizeByte(size)
		out[e+1] = sizeByte(size)
		out[e+2] = 0 // 调色板色数（32bpp 时为 0）
		out[e+3] = 0 // reserved
		le16(out[e+4:], 1)
		le16(out[e+6:], 32)
		le32(out[e+8:], uint32(imgLen))
		le32(out[e+12:], uint32(offset))

		out = append(out, body...)
		offset += imgLen
	}
	return out
}

// StandardSizes 是 Windows 图标常用的一组尺寸。
// 16 托盘/小列表，32 桌面，48 中等图标，256 大图标和任务栏预览。
var StandardSizes = []int{16, 32, 48, 64, 128, 256}

// ICOFor 按给定尺寸渲染同一套配色并打包成 ICO。
func ICOFor(s Scheme, sizes ...int) []byte {
	imgs := make([]*image.RGBA, 0, len(sizes))
	for _, size := range sizes {
		imgs = append(imgs, Render(size, s))
	}
	return ICO(imgs)
}

// AppICO 按 StandardSizes 渲染并打包程序图标。
func AppICO() []byte {
	return ICOFor(Brand, StandardSizes...)
}

// ---------------------------------------------------------------- 颜色工具

func gradient(top, bottom RGB, y float64) RGB {
	if y < 0 {
		y = 0
	}
	if y > 1 {
		y = 1
	}
	return RGB{
		R: clamp8(float64(top.R) + (float64(bottom.R)-float64(top.R))*y),
		G: clamp8(float64(top.G) + (float64(bottom.G)-float64(top.G))*y),
		B: clamp8(float64(top.B) + (float64(bottom.B)-float64(top.B))*y),
	}
}

// mix 把 base 往 other 方向插值，t=0 得到 base，t=1 得到 other。
func mix(base, other RGB, t float64) RGB {
	return RGB{
		R: clamp8(float64(base.R) + (float64(other.R)-float64(base.R))*t),
		G: clamp8(float64(base.G) + (float64(other.G)-float64(base.G))*t),
		B: clamp8(float64(base.B) + (float64(other.B)-float64(base.B))*t),
	}
}

// scale 把颜色整体调暗/调亮（乘一个系数）。
func scale(c RGB, f float64) RGB {
	return RGB{R: clamp8(float64(c.R) * f), G: clamp8(float64(c.G) * f), B: clamp8(float64(c.B) * f)}
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

func sizeByte(size int) byte {
	if size >= 256 {
		return 0 // ICO 里 0 表示 256
	}
	return byte(size)
}

func le16(b []byte, v uint16) {
	b[0] = byte(v)
	b[1] = byte(v >> 8)
}

func le32(b []byte, v uint32) {
	b[0] = byte(v)
	b[1] = byte(v >> 8)
	b[2] = byte(v >> 16)
	b[3] = byte(v >> 24)
}
