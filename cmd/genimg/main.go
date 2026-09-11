// genimg 生成一张带彩色渐变与网格的测试 PNG，供演示与测试使用。
package main

import (
	"flag"
	"image"
	"image/color"
	"image/png"
	"os"
)

func main() {
	out := flag.String("o", "input.png", "输出路径")
	w := flag.Int("w", 32, "宽度")
	h := flag.Int("h", 24, "高度")
	flag.Parse()

	img := image.NewRGBA(image.Rect(0, 0, *w, *h))
	for y := 0; y < *h; y++ {
		for x := 0; x < *w; x++ {
			img.Set(x, y, color.RGBA{
				R: uint8(x * 255 / max(*w-1, 1)),
				G: uint8(y * 255 / max(*h-1, 1)),
				B: uint8((x + y) * 255 / max(*w+*h-2, 1)),
				A: 255,
			})
		}
	}
	// 画几条白色网格线，方便肉眼观察放大质量。
	for x := 0; x < *w; x += 8 {
		for y := 0; y < *h; y++ {
			img.Set(x, y, color.RGBA{255, 255, 255, 255})
		}
	}
	for y := 0; y < *h; y += 8 {
		for x := 0; x < *w; x++ {
			img.Set(x, y, color.RGBA{255, 255, 255, 255})
		}
	}

	f, err := os.Create(*out)
	if err != nil {
		panic(err)
	}
	defer f.Close()
	if err := png.Encode(f, img); err != nil {
		panic(err)
	}
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}
