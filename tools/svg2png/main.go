package main

import (
	"flag"
	"image"
	"image/png"
	"log"
	"os"

	"github.com/srwiley/oksvg"
	"github.com/srwiley/rasterx"
)

func main() {
	var in string
	var out string
	var size int
	flag.StringVar(&in, "in", "", "input SVG path")
	flag.StringVar(&out, "out", "", "output PNG path")
	flag.IntVar(&size, "size", 1024, "PNG size (width=height)")
	flag.Parse()
	if in == "" || out == "" {
		log.Fatal("usage: svg2png -in assets/icon.svg -out icon.png -size 1024")
	}

	f, err := os.Open(in)
	if err != nil {
		log.Fatal(err)
	}
	defer f.Close()

	icon, err := oksvg.ReadIconStream(f)
	if err != nil {
		log.Fatal(err)
	}
	icon.SetTarget(0, 0, float64(size), float64(size))
	rgba := image.NewRGBA(image.Rect(0, 0, size, size))
	dc := rasterx.NewDasher(size, size, rasterx.NewScannerGV(size, size, rgba, rgba.Bounds()))
	icon.Draw(dc, 1.0)

	outF, err := os.Create(out)
	if err != nil {
		log.Fatal(err)
	}
	defer outF.Close()
	if err := png.Encode(outF, rgba); err != nil {
		log.Fatal(err)
	}
}
