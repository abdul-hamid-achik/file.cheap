// Command genframes writes deterministic PNG frame fixtures (plus a README.txt)
// for the studio image-preview / file-scroll e2e flow. Pure-Go, CGO-free.
//
// Usage: go run ./e2e/gen/genframes <dir> [count]
package main

import (
	"fmt"
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"strconv"
)

func main() {
	if len(os.Args) < 2 {
		fmt.Fprintln(os.Stderr, "usage: genframes <dir> [count]")
		os.Exit(2)
	}
	dir := os.Args[1]
	n := 30
	if len(os.Args) > 2 {
		if v, err := strconv.Atoi(os.Args[2]); err == nil {
			n = v
		}
	}
	if err := os.MkdirAll(filepath.Join(dir, "frames"), 0o755); err != nil {
		panic(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "README.txt"),
		[]byte("frames captured from screen recording\n"), 0o644); err != nil {
		panic(err)
	}
	for i := 1; i <= n; i++ {
		img := image.NewRGBA(image.Rect(0, 0, 48, 32))
		for y := 0; y < 32; y++ {
			for x := 0; x < 48; x++ {
				img.Set(x, y, color.RGBA{
					R: uint8((x*5 + i*10) % 256),
					G: uint8((y * 8) % 256),
					B: uint8((i * 97) % 256),
					A: 255,
				})
			}
		}
		f, err := os.Create(filepath.Join(dir, "frames", fmt.Sprintf("frame_%04d.png", i)))
		if err != nil {
			panic(err)
		}
		if err := png.Encode(f, img); err != nil {
			panic(err)
		}
		_ = f.Close()
	}
	fmt.Printf("generated %d frames in %s\n", n, dir)
}
