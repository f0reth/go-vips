//go:build unix || darwin || windows

package vips

import (
	"bytes"
	"fmt"
	"image"
	"image/color"
	"image/color/palette"
	"image/draw"
	"image/png"
	"math"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"runtime"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestLibrary(t *testing.T) {
	requireLibrary(t)

	if !regexp.MustCompile(`^\d+\.\d+\.\d+$`).MatchString(Version()) {
		t.Errorf("Version: got %q", Version())
	}
	if libVersion < minVersion {
		t.Errorf("libVersion: got %d, want >= %d", libVersion, minVersion)
	}
	t.Logf("libvips %s", Version())

	SetConcurrency(2)
	if n := vips_concurrency_get(); n != 2 {
		t.Errorf("SetConcurrency(2): %d threads", n)
	}
	SetConcurrency(0)
	if n := vips_concurrency_get(); n != defaultConcurrency || n < 1 {
		t.Errorf("SetConcurrency(0): %d threads, want the default %d", n, defaultConcurrency)
	}
	SetCacheMaxMem(100 << 20)
	SetCacheMaxFiles(100)
}

func TestLoad(t *testing.T) {
	requireLibrary(t)

	name := filepath.Join(t.TempDir(), "photo.jpg")
	if err := os.WriteFile(name, testJPEG, 0o644); err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		name   string
		load   func() (*Image, error)
		w, h   int
		loader string
	}{
		{"file", func() (*Image, error) { return Load(name, nil) }, 768, 512, "jpegload"},
		{"buffer", func() (*Image, error) { return LoadBuffer(testJPEG, nil) }, 768, 512, "jpegload_buffer"},
		{"shrink", func() (*Image, error) { return LoadBuffer(testJPEG, Options{"shrink": 2}) }, 384, 256, "jpegload_buffer"},
		{"thumbnail", func() (*Image, error) { return Thumbnail(name, 300, 0, nil) }, 300, 200, "jpegload"},
		{"thumbnail-buffer", func() (*Image, error) {
			return ThumbnailBuffer(testJPEG, 100, 100, Options{"crop": InterestingCentre})
		}, 100, 100, "jpegload_buffer"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			im := must(t)(tt.load())
			if im.Width() != tt.w || im.Height() != tt.h || im.Bands() != 3 ||
				im.Format() != BandFormatUchar || im.Interpretation() != InterpretationSRGB || im.HasAlpha() {
				t.Errorf("got %dx%d, %d bands, %s, %s, alpha %v, want %dx%d, 3 bands, uchar, srgb",
					im.Width(), im.Height(), im.Bands(), im.Format(), im.Interpretation(), im.HasAlpha(), tt.w, tt.h)
			}
			if loader, err := im.Get("vips-loader"); err != nil || loader != tt.loader {
				t.Errorf("vips-loader: got %v, %v, want %s", loader, err, tt.loader)
			}
		})
	}

	for name, err := range map[string]error{
		"missing file":    func() error { _, err := Load(filepath.Join(t.TempDir(), "missing.jpg"), nil); return err }(),
		"unknown format":  func() error { _, err := LoadBuffer([]byte("not an image"), nil); return err }(),
		"empty buffer":    func() error { _, err := LoadBuffer(nil, nil); return err }(),
		"zero width":      func() error { _, err := Thumbnail(name, 0, 0, nil); return err }(),
		"unknown options": func() error { _, err := LoadBuffer(testJPEG, Options{"nope": 1}); return err }(),
	} {
		if err == nil || !strings.HasPrefix(err.Error(), "vips: ") {
			t.Errorf("%s: got error %v", name, err)
		}
	}
}

func TestSave(t *testing.T) {
	src := mustLoad(t)
	want, err := src.ToImage()
	if err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		suffix   string
		opts     Options
		lossless bool
	}{
		{".jpg", Options{"Q": 90}, false},
		{".png", nil, true},
		{".webp", Options{"lossless": true}, true},
		{".gif", nil, false},
		{".tif", Options{"compression": "lzw"}, true},
	}

	for _, tt := range tests {
		t.Run(tt.suffix, func(t *testing.T) {
			buf, err := src.SaveBuffer(tt.suffix, tt.opts)
			if err != nil {
				t.Fatal(err)
			}

			name := filepath.Join(t.TempDir(), "out"+tt.suffix)
			if err := src.Save(name, tt.opts); err != nil {
				t.Fatal(err)
			}
			file, err := os.ReadFile(name)
			if err != nil {
				t.Fatal(err)
			}

			for _, data := range [][]byte{buf, file} {
				im := must(t)(LoadBuffer(data, nil))
				if im.Width() != 768 || im.Height() != 512 {
					t.Fatalf("got %dx%d", im.Width(), im.Height())
				}
				if tt.lossless {
					got, err := im.ToImage()
					if err != nil {
						t.Fatal(err)
					}
					samePixels(t, want, got)
				}
			}
		})
	}

	// The options reach the saver.
	low, err := src.SaveBuffer(".jpg", Options{"Q": 10})
	if err != nil {
		t.Fatal(err)
	}
	high, err := src.SaveBuffer(".jpg", Options{"Q": 95})
	if err != nil {
		t.Fatal(err)
	}
	if len(low) >= len(high) {
		t.Errorf("Q 10: %d bytes, Q 95: %d bytes", len(low), len(high))
	}

	if _, err := src.SaveBuffer(".nope", nil); err == nil {
		t.Error(".nope: no error")
	}
	if err := src.Save(filepath.Join(t.TempDir(), "out.nope"), nil); err == nil {
		t.Error("out.nope: no error")
	}
	if _, err := src.SaveBuffer(".jpg", Options{"Q": "high"}); err == nil {
		t.Error("Q high: no error")
	}
}

func TestCall(t *testing.T) {
	src := mustLoad(t)

	// Number outputs and array outputs.
	out, err := Call("max", Options{"in": src})
	if err != nil {
		t.Fatal(err)
	}
	if out["out"] != 255.0 || len(out["x_array"].([]int)) != 1 {
		t.Errorf("max: got %v", out)
	}
	x, y := out["x"].(int), out["y"].(int)
	if p := pixel(t, src, x, y); !slices.Contains(p, 255) {
		t.Errorf("max at (%d, %d): got %v", x, y, p)
	}

	// Image array input.
	bands := make([]*Image, 3)
	for i := range bands {
		bands[i] = must(t)(src.ExtractBand(i, nil))
	}
	joined := must(t)(callImage("bandjoin", nil, Options{"in": []*Image{bands[2], bands[1], bands[0]}}))
	if got, want := pixel(t, joined, 5, 5), pixel(t, src, 5, 5); got[0] != want[2] || got[2] != want[0] {
		t.Errorf("bandjoin: got %v, want the bands of %v reversed", got, want)
	}

	// Integer array input and boolean input.
	affine := must(t)(callImage("affine", nil, Options{"in": src, "matrix": []float64{2, 0, 0, 2}, "oarea": []int{0, 0, 10, 20}}))
	if affine.Width() != 10 || affine.Height() != 20 {
		t.Errorf("affine: got %dx%d", affine.Width(), affine.Height())
	}
	linear := must(t)(src.Linear([]float64{1}, []float64{0}, Options{"uchar": true}))
	if linear.Format() != BandFormatUchar {
		t.Errorf("linear uchar: got %s", linear.Format())
	}

	// A number is an array of one element, an enum may be given by value.
	black := must(t)(callImage("black", nil, Options{"width": 3, "height": 2, "bands": 1}))
	white := must(t)(callImage("linear", nil, Options{"in": black, "a": 1, "b": 255.0}))
	cast := must(t)(callImage("cast", nil, Options{"in": white, "format": 0}))
	if p := pixel(t, cast, 2, 1); cast.Format() != BandFormatUchar || p[0] != 255 {
		t.Errorf("cast: got %s %v", cast.Format(), p)
	}

	// Flags, by nick or by value.
	if libVersion >= 81500 {
		for _, keep := range []any{"none", "exif|icc", 0} {
			if _, err := src.SaveBuffer(".jpg", Options{"keep": keep}); err != nil {
				t.Errorf("keep %v: %v", keep, err)
			}
		}
	}

	// Arguments are not modified.
	opts := Options{"kernel": KernelNearest}
	must(t)(src.Resize(0.5, opts))
	if len(opts) != 1 {
		t.Errorf("opts modified: %v", opts)
	}
}

func TestCallErrors(t *testing.T) {
	src := mustLoad(t)
	closed := must(t)(src.Copy(nil))
	closed.Close()

	tests := []struct {
		name string
		op   string
		args Options
		want string
	}{
		{"unknown operation", "nope", nil, `class "nope" not found`},
		{"unknown argument", "invert", Options{"in": src, "nope": 1}, "nope: no such argument"},
		{"output argument", "invert", Options{"in": src, "out": src}, "out: not an input argument"},
		{"modified argument", "draw_rect", Options{"image": src}, "image: arguments modified in place are not supported"},
		{"missing argument", "resize", Options{"in": src}, "scale"},
		{"wrong type", "resize", Options{"in": src, "scale": "half"}, "scale: cannot use string as gdouble"},
		{"wrong object", "invert", Options{"in": 1}, "in: cannot use int as VipsImage"},
		{"nil image", "invert", Options{"in": (*Image)(nil)}, "in: nil or closed image"},
		{"closed image", "invert", Options{"in": closed}, "in: nil or closed image"},
		{"nil value", "resize", Options{"in": src, "scale": nil}, "cannot use <nil>"},
		{"out of range", "resize", Options{"in": src, "scale": 0.5, "kernel": 99}, "kernel: invalid value 99"},
		{"out of range int", "thumbnail_image", Options{"in": src, "width": 0}, "width: invalid value 0"},
		{"unknown nick", "resize", Options{"in": src, "scale": 0.5, "kernel": "nope"}, "has no member 'nope', should be one of: nearest,"},
		{"closed image in array", "bandjoin", Options{"in": []*Image{src, closed}}, "in: nil or closed image"},
		{"wrong array", "bandjoin", Options{"in": []int{1}}, "in: cannot use []int as VipsArrayImage"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := Call(tt.op, tt.args)
			if err == nil || !strings.HasPrefix(err.Error(), "vips: ") || !strings.Contains(err.Error(), tt.want) {
				t.Errorf("got error %v, want %q", err, tt.want)
			}
		})
	}

	// The error buffer does not keep messages of previous operations.
	_, err1 := Call("nope1", nil)
	_, err2 := Call("nope2", nil)
	if strings.Contains(err2.Error(), "nope1") {
		t.Errorf("got errors %v, then %v", err1, err2)
	}
}

func TestClose(t *testing.T) {
	im := must(t)(mustLoad(t).Copy(nil))
	im.Close()
	im.Close()
	(*Image)(nil).Close()

	defer func() {
		if r := recover(); r == nil {
			t.Error("Width of a closed image: no panic")
		}
	}()
	im.Width()
}

func TestOperations(t *testing.T) {
	src := mustLoad(t)
	small := must(t)(src.ExtractArea(0, 0, 100, 50))

	tests := []struct {
		name   string
		op     func() (*Image, error)
		w, h   int
		bands  int
		format BandFormat
	}{
		{"Resize", func() (*Image, error) { return src.Resize(0.5, nil) }, 384, 256, 3, BandFormatUchar},
		{"Resize/vscale", func() (*Image, error) {
			return src.Resize(0.5, Options{"vscale": 0.25, "kernel": KernelNearest})
		}, 384, 128, 3, BandFormatUchar},
		{"ThumbnailImage", func() (*Image, error) { return src.ThumbnailImage(150, 0, nil) }, 150, 100, 3, BandFormatUchar},
		{"ThumbnailImage/crop", func() (*Image, error) {
			return src.ThumbnailImage(100, 100, Options{"crop": InterestingAttention})
		}, 100, 100, 3, BandFormatUchar},
		{"ExtractArea", func() (*Image, error) { return src.ExtractArea(10, 20, 30, 40) }, 30, 40, 3, BandFormatUchar},
		{"Smartcrop", func() (*Image, error) {
			return src.Smartcrop(200, 100, Options{"interesting": InterestingEntropy})
		}, 200, 100, 3, BandFormatUchar},
		{"Embed", func() (*Image, error) {
			return src.Embed(10, 10, 800, 600, Options{"extend": ExtendWhite})
		}, 800, 600, 3, BandFormatUchar},
		{"Gravity", func() (*Image, error) {
			return src.Gravity(CompassDirectionCentre, 1000, 1000, Options{"background": []float64{0, 128, 255}})
		}, 1000, 1000, 3, BandFormatUchar},
		{"Rot", func() (*Image, error) { return src.Rot(AngleD90) }, 512, 768, 3, BandFormatUchar},
		{"Rotate", func() (*Image, error) { return src.Rotate(90, nil) }, 512, 768, 3, BandFormatUchar},
		{"Flip", func() (*Image, error) { return src.Flip(DirectionHorizontal) }, 768, 512, 3, BandFormatUchar},
		{"Autorot", func() (*Image, error) { return src.Autorot() }, 768, 512, 3, BandFormatUchar},
		{"Colourspace", func() (*Image, error) { return src.Colourspace(InterpretationBW, nil) }, 768, 512, 1, BandFormatUchar},
		{"Cast", func() (*Image, error) { return src.Cast(BandFormatUshort, nil) }, 768, 512, 3, BandFormatUshort},
		{"Copy", func() (*Image, error) { return src.Copy(Options{"xres": 10.0}) }, 768, 512, 3, BandFormatUchar},
		{"Gaussblur", func() (*Image, error) { return src.Gaussblur(2, nil) }, 768, 512, 3, BandFormatUchar},
		{"Sharpen", func() (*Image, error) { return src.Sharpen(Options{"sigma": 1.5}) }, 768, 512, 3, BandFormatUchar},
		{"Invert", func() (*Image, error) { return src.Invert() }, 768, 512, 3, BandFormatUchar},
		{"Linear", func() (*Image, error) { return src.Linear([]float64{2}, []float64{1, 2, 3}, nil) }, 768, 512, 3, BandFormatFloat},
		{"ExtractBand", func() (*Image, error) { return src.ExtractBand(1, Options{"n": 2}) }, 768, 512, 2, BandFormatUchar},
		{"Composite2", func() (*Image, error) {
			return src.Composite2(small, BlendModeOver, Options{"x": 700, "y": 500})
		}, 768, 512, 4, BandFormatUchar},
		{"Insert", func() (*Image, error) { return src.Insert(small, 700, 500, Options{"expand": true}) }, 800, 550, 3, BandFormatUchar},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			im := must(t)(tt.op())
			if im.Width() != tt.w || im.Height() != tt.h || im.Bands() != tt.bands || im.Format() != tt.format {
				t.Errorf("got %dx%d, %d bands, %s, want %dx%d, %d bands, %s",
					im.Width(), im.Height(), im.Bands(), im.Format(), tt.w, tt.h, tt.bands, tt.format)
			}
			// The pixels can be computed.
			if _, err := im.SaveBuffer(".png", Options{"compression": 0}); err != nil {
				t.Error(err)
			}
		})
	}

	// The operations do what their name says.
	at := func(op func() (*Image, error), x, y int) []float64 { return pixel(t, must(t)(op()), x, y) }
	if got, want := at(func() (*Image, error) { return src.Flip(DirectionHorizontal) }, 0, 3), pixel(t, src, 767, 3); !slices.Equal(got, want) {
		t.Errorf("Flip: got %v, want %v", got, want)
	}
	if got, want := at(func() (*Image, error) { return src.Rot(AngleD90) }, 511, 0), pixel(t, src, 0, 0); !slices.Equal(got, want) {
		t.Errorf("Rot: got %v, want %v", got, want)
	}
	if got, want := at(func() (*Image, error) { return src.ExtractArea(10, 20, 30, 40) }, 0, 0), pixel(t, src, 10, 20); !slices.Equal(got, want) {
		t.Errorf("ExtractArea: got %v, want %v", got, want)
	}
	if got, want := at(func() (*Image, error) { return src.Invert() }, 7, 9), pixel(t, src, 7, 9); got[0] != 255-want[0] {
		t.Errorf("Invert: got %v, want 255 - %v", got, want)
	}
	if got := at(func() (*Image, error) { return src.Embed(10, 10, 800, 600, Options{"extend": ExtendWhite}) }, 0, 0); !slices.Equal(got, []float64{255, 255, 255}) {
		t.Errorf("Embed: got %v, want white", got)
	}
	if got := at(func() (*Image, error) {
		return src.Gravity(CompassDirectionCentre, 1000, 1000, Options{"background": []float64{0, 128, 255}})
	}, 0, 0); !slices.Equal(got, []float64{0, 128, 255}) {
		t.Errorf("Gravity: got %v, want the background", got)
	}

	// Flatten removes alpha.
	alpha := must(t)(callImage("bandjoin_const", nil, Options{"in": src, "c": 128}))
	flat := must(t)(alpha.Flatten(Options{"background": 255}))
	if !alpha.HasAlpha() || flat.Bands() != 3 || flat.HasAlpha() {
		t.Errorf("Flatten: got %d bands from %d", flat.Bands(), alpha.Bands())
	}
}

func TestGoImage(t *testing.T) {
	requireLibrary(t)

	gray := image.NewGray(image.Rect(0, 0, 61, 37))
	gray16 := image.NewGray16(gray.Rect)
	nrgba64 := image.NewNRGBA64(gray.Rect)
	for y := range gray.Rect.Dy() {
		for x := range gray.Rect.Dx() {
			gray.SetGray(x, y, color.Gray{uint8(x*4 + y)})
			gray16.SetGray16(x, y, color.Gray16{uint16(x*1000 + y*7)})
			nrgba64.SetNRGBA64(x, y, color.NRGBA64{uint16(x * 999), uint16(y * 1700), 0x1234, uint16(x*y*31 + 1)})
		}
	}
	rgba64 := image.NewRGBA64(gray.Rect)
	draw.Draw(rgba64, rgba64.Rect, nrgba64, image.Point{}, draw.Src)
	opaque64 := image.NewRGBA64(gray.Rect)
	draw.Draw(opaque64, opaque64.Rect, gray16, image.Point{}, draw.Src)

	noise := noiseImage(61, 37, 1, true)
	opaque := noiseImage(61, 37, 2, false)
	rgba := image.NewRGBA(opaque.Rect)
	draw.Draw(rgba, rgba.Rect, noise, image.Point{}, draw.Src)
	opaqueRGBA := image.NewRGBA(opaque.Rect)
	draw.Draw(opaqueRGBA, opaqueRGBA.Rect, opaque, image.Point{}, draw.Src)
	paletted := image.NewPaletted(opaque.Rect, append(palette.Plan9[:255:255], color.Transparent))
	draw.Draw(paletted, paletted.Rect, noise, image.Point{}, draw.Src)

	tests := []struct {
		name  string
		m     image.Image
		bands int
		space Interpretation
		got   image.Image // type of ToImage
	}{
		{"Gray", gray, 1, InterpretationBW, &image.Gray{}},
		{"Gray16", gray16, 1, InterpretationGrey16, &image.Gray16{}},
		{"NRGBA", noise, 4, InterpretationSRGB, &image.NRGBA{}},
		{"NRGBA/opaque", opaque, 3, InterpretationSRGB, &image.RGBA{}},
		{"RGBA", rgba, 4, InterpretationSRGB, &image.NRGBA{}},
		{"RGBA/opaque", opaqueRGBA, 3, InterpretationSRGB, &image.RGBA{}},
		{"NRGBA64", nrgba64, 4, InterpretationRGB16, &image.NRGBA64{}},
		{"RGBA64", rgba64, 4, InterpretationRGB16, &image.NRGBA64{}},
		{"RGBA64/opaque", opaque64, 3, InterpretationRGB16, &image.RGBA64{}},
		{"YCbCr", decodedJPEG(t), 3, InterpretationSRGB, &image.RGBA{}},
		{"Paletted", paletted, 4, InterpretationSRGB, &image.NRGBA{}},
		{"SubImage", noise.SubImage(image.Rect(5, 7, 30, 20)), 4, InterpretationSRGB, &image.NRGBA{}},
	}
	// YCbCr images of every subsampling ratio, cut at odd coordinates.
	for _, ratio := range []image.YCbCrSubsampleRatio{
		image.YCbCrSubsampleRatio444, image.YCbCrSubsampleRatio422, image.YCbCrSubsampleRatio420,
		image.YCbCrSubsampleRatio440, image.YCbCrSubsampleRatio411, image.YCbCrSubsampleRatio410,
	} {
		m := image.NewYCbCr(image.Rect(-3, -5, 58, 32), ratio)
		for i, s := range [][]byte{m.Y, m.Cb, m.Cr} {
			copy(s, noiseImage(61, 37, uint64(i+3), true).Pix)
		}
		tests = append(tests, struct {
			name  string
			m     image.Image
			bands int
			space Interpretation
			got   image.Image
		}{"YCbCr/" + ratio.String(), m.SubImage(image.Rect(-1, -3, 51, 29)), 3, InterpretationSRGB, &image.RGBA{}})
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			im := mustFromImage(t, tt.m)
			w, h := tt.m.Bounds().Dx(), tt.m.Bounds().Dy()
			if im.Width() != w || im.Height() != h || im.Bands() != tt.bands || im.Interpretation() != tt.space {
				t.Errorf("got %dx%d, %d bands, %s, want %dx%d, %d bands, %s",
					im.Width(), im.Height(), im.Bands(), im.Interpretation(), w, h, tt.bands, tt.space)
			}

			got, err := im.ToImage()
			if err != nil {
				t.Fatal(err)
			}
			if reflect.TypeOf(got) != reflect.TypeOf(tt.got) {
				t.Errorf("ToImage: got %T, want %T", got, tt.got)
			}
			if got.Bounds().Min != (image.Point{}) {
				t.Errorf("ToImage: got bounds %v", got.Bounds())
			}

			// The colors are compared with their 8-bit or 16-bit NRGBA conversion by the image
			// package, which FromImage uses for the other types.
			var want image.Image = convert(tt.m, image.NewNRGBA)
			switch got.(type) {
			case *image.Gray16, *image.RGBA64, *image.NRGBA64:
				want = convert(tt.m, image.NewNRGBA64)
			}
			samePixels(t, want, got)
		})
	}

	if _, err := FromImage(image.NewRGBA(image.Rect(0, 0, 0, 10))); err == nil {
		t.Error("empty image: no error")
	}
}

func TestToImage(t *testing.T) {
	gray := image.NewGray(image.Rect(0, 0, 16, 8))
	for i := range gray.Pix {
		gray.Pix[i] = uint8(i * 2)
	}
	g := mustFromImage(t, gray)
	gray16 := image.NewGray16(gray.Rect)
	for i := range gray16.Pix {
		gray16.Pix[i] = uint8(i * 7)
	}
	g16 := mustFromImage(t, gray16)
	src := mustLoad(t)
	want := decodedJPEG(t)

	tests := []struct {
		name string
		im   *Image
		want image.Image
	}{
		// Gray with alpha gives NRGBA.
		{"b-w+alpha", must(t)(callImage("bandjoin_const", nil, Options{"in": g, "c": 128})), func() image.Image {
			m := image.NewNRGBA(gray.Rect)
			for i, v := range gray.Pix {
				copy(m.Pix[4*i:], []byte{v, v, v, 128})
			}
			return m
		}()},
		{"grey16+alpha", must(t)(callImage("bandjoin_const", nil, Options{"in": g16, "c": 1000})), func() image.Image {
			m := image.NewNRGBA64(gray.Rect)
			for i := range gray16.Pix[:len(gray16.Pix)/2] {
				v := gray16.Gray16At(i%16, i/16).Y
				m.SetNRGBA64(i%16, i/16, color.NRGBA64{v, v, v, 1000})
			}
			return m
		}()},
		// Other formats and color spaces are converted to 8-bit sRGB.
		{"float", must(t)(g.Cast(BandFormatFloat, nil)), gray},
		{"multiband", must(t)(g.Copy(Options{"interpretation": InterpretationMultiband})), gray},
		{"lab", must(t)(g.Colourspace(InterpretationLab, nil)), gray},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := tt.im.ToImage()
			if err != nil {
				t.Fatal(err)
			}
			samePixels(t, tt.want, got)
		})
	}

	// libvips decodes the JPEG almost like image/jpeg; they upsample the chroma differently.
	got, err := src.ToImage()
	if err != nil {
		t.Fatal(err)
	}
	if p := psnr(want, got); p < 40 {
		t.Errorf("JPEG: PSNR with image/jpeg %.1f dB", p)
	}

	// Images with too many bands cannot be converted.
	five := must(t)(callImage("bandjoin_const", nil, Options{"in": src, "c": []float64{1, 2}}))
	if _, err := five.ToImage(); err == nil {
		t.Error("5 bands: no error")
	}
}

// psnr returns the PSNR in dB of the 8-bit RGB channels of b compared to a.
func psnr(a, b image.Image) float64 {
	var se float64
	r := a.Bounds()
	for y := r.Min.Y; y < r.Max.Y; y++ {
		for x := r.Min.X; x < r.Max.X; x++ {
			r1, g1, b1, _ := a.At(x, y).RGBA()
			r2, g2, b2, _ := b.At(x, y).RGBA()
			for _, v := range [][2]uint32{{r1, r2}, {g1, g2}, {b1, b2}} {
				e := float64(v[0]>>8) - float64(v[1]>>8)
				se += e * e
			}
		}
	}
	if se == 0 {
		return math.Inf(1)
	}

	return 10 * math.Log10(255*255/(se/float64(3*r.Dx()*r.Dy())))
}

func TestMetadata(t *testing.T) {
	src := mustLoad(t)

	fields := src.Fields()
	for _, name := range []string{"width", "height", "interpretation", "xres", "vips-loader"} {
		if !slices.Contains(fields, name) {
			t.Errorf("Fields: no %s in %v", name, fields)
		}
	}

	for name, want := range map[string]any{
		"width":                 768,
		"bands":                 3,
		"interpretation":        "srgb",
		"vips-loader":           "jpegload_buffer",
		"jpeg-chroma-subsample": "4:2:0",
	} {
		if got, err := src.Get(name); err != nil || got != want {
			t.Errorf("Get(%s): got %#v, %v, want %#v", name, got, err, want)
		}
	}
	if xres, err := src.Get("xres"); err != nil || xres.(float64) <= 0 {
		t.Errorf("Get(xres): got %v, %v", xres, err)
	}
	if _, err := src.Get("nope"); err == nil {
		t.Error("Get(nope): no error")
	}

	// Blobs: the ICC profile of a converted image.
	withICC := must(t)(callImage("icc_transform", nil, Options{"in": src, "output_profile": "srgb", "input-profile": "srgb"}))
	icc, err := withICC.Get("icc-profile-data")
	if b, _ := icc.([]byte); err != nil || len(b) < 128 || string(b[36:40]) != "acsp" {
		t.Errorf("Get(icc-profile-data): got %d bytes, %v", len(b), err)
	}
}

func TestOwnership(t *testing.T) {
	requireLibrary(t)

	// The operation cache holds references to the inputs and outputs of the operations.
	SetCacheMax(0)
	t.Cleanup(func() { SetCacheMax(100) })

	in, err := FromImage(noiseImage(64, 48, 3, false))
	if err != nil {
		t.Fatal(err)
	}
	inRef := newWeakRef(t, in.p)

	out, err := in.Resize(0.5, nil)
	if err != nil {
		t.Fatal(err)
	}
	outRef := newWeakRef(t, out.p)

	band, err := out.ExtractBand(0, nil)
	if err != nil {
		t.Fatal(err)
	}
	bandRef := newWeakRef(t, band.p)
	joined, err := callImage("bandjoin", nil, Options{"in": []*Image{band, band, band}})
	if err != nil {
		t.Fatal(err)
	}
	joinedRef := newWeakRef(t, joined.p)
	if _, err := joined.SaveBuffer(".png", nil); err != nil {
		t.Fatal(err)
	}

	joined.Close()
	band.Close()
	out.Close()
	in.Close()
	for name, ref := range map[string]*weakRef{"in": inRef, "out": outRef, "band": bandRef, "joined": joinedRef} {
		if ref.alive() {
			t.Errorf("%s: not finalized after Close", name)
		}
	}

	// Images that are not closed are released by the garbage collector.
	var gcRef *weakRef
	func() {
		im, err := FromImage(noiseImage(8, 8, 4, false))
		if err != nil {
			t.Fatal(err)
		}
		gcRef = newWeakRef(t, im.p)
	}()
	for i := 0; i < 100 && gcRef.alive(); i++ {
		runtime.GC()
		time.Sleep(time.Millisecond)
	}
	if gcRef.alive() {
		t.Error("not finalized after garbage collection")
	}
}

func TestConcurrent(t *testing.T) {
	src := mustLoad(t)

	var wg sync.WaitGroup
	errs := make(chan error, 16)
	for i := range 16 {
		wg.Go(func() {
			errs <- func() error {
				thumb, err := ThumbnailBuffer(testJPEG, 100+i, 0, nil)
				if err != nil {
					return err
				}
				defer thumb.Close()
				buf, err := thumb.SaveBuffer(".png", nil)
				if err != nil {
					return err
				}
				if m, err := png.Decode(bytes.NewReader(buf)); err != nil || m.Bounds().Dx() != 100+i {
					return fmt.Errorf("thumbnail %d: got %v, %v", i, m.Bounds(), err)
				}

				// An input image shared by the goroutines.
				resized, err := src.Resize(1/float64(i+1), nil)
				if err != nil {
					return err
				}
				defer resized.Close()
				_, err = resized.SaveBuffer(".jpg", nil)

				return err
			}()
		})
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Error(err)
		}
	}
}
