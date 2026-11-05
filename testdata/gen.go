//go:build ignore

// gen generates the test images derived from photo.jpg (kodim23 of the Kodak test image set).
// Run it from the root of the module:
//
//	go run testdata/gen.go
package main

import (
	"bytes"
	"image"
	"image/color"
	"image/color/palette"
	"image/draw"
	"image/gif"
	"image/jpeg"
	"image/png"
	"log"
	"math"
	"os"

	"github.com/f0reth/go-vips"
)

func main() {
	photo, err := os.ReadFile("testdata/photo.jpg")
	check(err)
	src, err := vips.LoadBuffer(photo, nil)
	check(err)

	// orientation6.jpg: photo.jpg with an EXIF orientation of 6 (displayed rotated by 90 degrees
	// clockwise), like the photos of a phone held vertically.
	write("orientation6.jpg", withOrientation(photo, 6))

	// p3.jpg: Display P3 JPEG with its profile embedded, like the photos of recent phones.
	p3, err := vips.Call("icc_transform", vips.Options{"in": src, "output_profile": "p3"})
	check(err)
	write("p3.jpg", save(p3["out"].(*vips.Image), ".jpg", vips.Options{"Q": 90}))

	// photo16.png: 16-bit PNG, the photo halved in 16-bit precision.
	deep, err := src.Cast(vips.BandFormatUshort, nil)
	check(err)
	deep, err = deep.Linear([]float64{257}, []float64{0}, nil)
	check(err)
	deep, err = deep.Cast(vips.BandFormatUshort, nil)
	check(err)
	deep, err = deep.Copy(vips.Options{"interpretation": vips.InterpretationRGB16})
	check(err)
	deep, err = deep.Resize(0.5, nil)
	check(err)
	write("photo16.png", save(deep, ".png", nil))

	// logo.png: white disk with an antialiased edge on a transparent black background, like logos
	// and icons.
	write("logo.png", encodePNG(disk(512, 200)))

	// anim.gif and anim.webp: 4 frames of 160x120 panning over the photo.
	m, err := jpeg.Decode(bytes.NewReader(photo))
	check(err)
	anim := &gif.GIF{LoopCount: 0}
	for i := range 4 {
		frame := image.NewPaletted(image.Rect(0, 0, 160, 120), palette.Plan9)
		draw.FloydSteinberg.Draw(frame, frame.Rect, m, image.Pt(300+i*40, 150))
		anim.Image = append(anim.Image, frame)
		anim.Delay = append(anim.Delay, 10)
	}
	var buf bytes.Buffer
	check(gif.EncodeAll(&buf, anim))
	write("anim.gif", buf.Bytes())
	frames, err := vips.LoadBuffer(buf.Bytes(), vips.Options{"n": -1})
	check(err)
	write("anim.webp", save(frames, ".webp", vips.Options{"Q": 90}))
}

// withOrientation returns the JPEG data with an EXIF APP1 segment holding orientation inserted
// after the SOI marker.
func withOrientation(data []byte, orientation byte) []byte {
	exif := []byte{
		0xff, 0xe1, 0, 34, // APP1, length
		'E', 'x', 'i', 'f', 0, 0,
		'M', 'M', 0, 42, 0, 0, 0, 8, // big-endian TIFF header, IFD0 at 8
		0, 1, // 1 entry
		0x01, 0x12, 0, 3, 0, 0, 0, 1, 0, orientation, 0, 0, // Orientation, SHORT, 1 value
		0, 0, 0, 0, // no next IFD
	}

	return append(append(append([]byte{}, data[:2]...), exif...), data[2:]...)
}

// disk returns a size x size image of a white disk of radius r with an antialiased edge, on a
// transparent black background.
func disk(size int, r float64) *image.NRGBA {
	m := image.NewNRGBA(image.Rect(0, 0, size, size))
	c := float64(size) / 2
	for y := range size {
		for x := range size {
			d := math.Hypot(float64(x)+0.5-c, float64(y)+0.5-c)
			if a := math.Min(math.Max(r-d+0.5, 0), 1); a > 0 {
				m.SetNRGBA(x, y, color.NRGBA{255, 255, 255, uint8(math.Round(a * 255))})
			}
		}
	}

	return m
}

func save(im *vips.Image, suffix string, opts vips.Options) []byte {
	buf, err := im.SaveBuffer(suffix, opts)
	check(err)

	return buf
}

func encodePNG(m image.Image) []byte {
	var buf bytes.Buffer
	check(png.Encode(&buf, m))

	return buf.Bytes()
}

func write(name string, data []byte) {
	check(os.WriteFile("testdata/"+name, data, 0o644))
	log.Printf("%s: %d bytes", name, len(data))
}

func check(err error) {
	if err != nil {
		log.Fatal(err)
	}
}
