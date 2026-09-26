// Command genicon 生成程序图标。
//
//	go run ./cmd/genicon            # 写入 build/windows/icon.ico 和 build/appicon.png
//	go run ./cmd/genicon -preview p.png   # 额外输出一张放大预览，方便肉眼检查
//
// 之所以做成命令而不是把 .ico 直接放进仓库：图标是按状态/尺寸算出来的，
// 改一次几何定义就能重新生成一整套，不用手工维护一堆图片。
package main

import (
	"flag"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"

	"frpcgui/internal/icon"
)

func main() {
	root := flag.String("root", ".", "模块根目录（build/ 所在的目录）")
	preview := flag.String("preview", "", "额外输出一张放大预览图到指定路径")
	flag.Parse()

	if err := run(*root, *preview); err != nil {
		fmt.Fprintln(os.Stderr, "生成失败:", err)
		os.Exit(1)
	}
}

func run(root, preview string) error {
	// 1) Windows 程序图标（会嵌进 exe，也用作窗口左上角图标）
	icoPath := filepath.Join(root, "build", "windows", "icon.ico")
	if err := os.MkdirAll(filepath.Dir(icoPath), 0o755); err != nil {
		return err
	}
	ico := icon.AppICO()
	if err := os.WriteFile(icoPath, ico, 0o644); err != nil {
		return err
	}
	fmt.Printf("写入 %s（%d 字节，含尺寸 %v）\n", icoPath, len(ico), icon.StandardSizes)

	// 2) appicon.png —— Wails 在非 Windows 平台用它，部分场景也当备用
	appIconPath := filepath.Join(root, "build", "appicon.png")
	big := icon.Render(1024, icon.Brand)
	data, err := icon.PNG(big)
	if err != nil {
		return err
	}
	if err := os.WriteFile(appIconPath, data, 0o644); err != nil {
		return err
	}
	fmt.Printf("写入 %s（%d 字节，1024×1024）\n", appIconPath, len(data))

	// 3) 可选：放大预览，用来肉眼确认小尺寸下还看得清
	if preview != "" {
		if err := writePreview(preview); err != nil {
			return err
		}
		fmt.Printf("写入预览 %s\n", preview)
	}
	return nil
}

// writePreview 把 16/24/32/48/64/128 各尺寸按整数倍放大并排画出来，
// 同时放一行"实际大小"的对照。
func writePreview(path string) error {
	sizes := []int{16, 24, 32, 48, 64, 128}
	scale := 4
	pad := 12

	width := pad
	for _, s := range sizes {
		width += s*scale + pad
	}
	height := pad + 128*scale + pad + 128 + pad

	canvas := image.NewRGBA(image.Rect(0, 0, width, height))
	fill(canvas, color.RGBA{0xF7, 0xF9, 0xFB, 0xFF})

	x := pad
	for _, s := range sizes {
		img := icon.Render(s, icon.Brand)
		drawScaled(canvas, img, x, pad, scale)
		// 下面贴一张原尺寸的，看看真实观感
		drawScaled(canvas, img, x, pad+128*scale+pad, 1)
		x += s*scale + pad
	}

	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	return png.Encode(f, canvas)
}

func fill(img *image.RGBA, c color.RGBA) {
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
			// 与底色做一次普通 alpha 混合，免得预览里出现黑洞
			bg := dst.RGBAAt(ox+x, oy+y)
			a := float64(c.A) / 255
			dst.SetRGBA(ox+x, oy+y, color.RGBA{
				R: uint8(float64(c.R)*a + float64(bg.R)*(1-a)),
				G: uint8(float64(c.G)*a + float64(bg.G)*(1-a)),
				B: uint8(float64(c.B)*a + float64(bg.B)*(1-a)),
				A: 255,
			})
		}
	}
}
