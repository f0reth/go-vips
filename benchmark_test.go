//go:build unix || darwin || windows

package vips

import (
	"fmt"
	"testing"
)

// Benchmarks of the public API, run with:
//
//	go test -run '^$' -bench . -benchmem
//
// Besides ns/op, they report the throughput in megapixels per second (Mpx/s) of the input image.
// The libvips operation cache is disabled so that every iteration runs the operations.

// reportPixels reports the throughput of a benchmark processing pixels pixels per iteration.
func reportPixels(b *testing.B, pixels int) {
	b.Helper()

	if s := b.Elapsed().Seconds(); s > 0 {
		b.ReportMetric(float64(pixels)*float64(b.N)/s/1e6, "Mpx/s")
	}
}

// benchSetup checks the library and disables the operation cache during the benchmark.
func benchSetup(b *testing.B) {
	b.Helper()
	requireLibrary(b)

	SetCacheMax(0)
	b.Cleanup(func() { SetCacheMax(100) })
}

// memoryImage returns the test photo decoded in memory, so that operations on it do not include
// the JPEG decoding.
func memoryImage(b *testing.B) *Image {
	b.Helper()

	return mustFromImage(b, decodedJPEG(b))
}

const photoPixels = 768 * 512

func BenchmarkLoadBuffer(b *testing.B) {
	benchSetup(b)

	for b.Loop() {
		im, err := LoadBuffer(testJPEG, nil)
		if err != nil {
			b.Fatal(err)
		}
		// Decode all the pixels.
		if _, err := Call("avg", Options{"in": im}); err != nil {
			b.Fatal(err)
		}
		im.Close()
	}
	reportPixels(b, photoPixels)
}

// largeJPEG returns a 6144x4096 (25 Mpx) JPEG, like the photos of cameras.
func largeJPEG(b *testing.B) []byte {
	b.Helper()

	buf, err := must(b)(memoryImage(b).Resize(8, nil)).SaveBuffer(".jpg", Options{"Q": 85})
	if err != nil {
		b.Fatal(err)
	}

	return buf
}

// thumbnail makes a thumbnail of width w of the JPEG buf and saves it as JPEG.
func thumbnail(b *testing.B, buf []byte, w int) {
	im, err := ThumbnailBuffer(buf, w, 0, nil)
	if err != nil {
		b.Error(err)
		return
	}
	defer im.Close()
	if _, err := im.SaveBuffer(".jpg", nil); err != nil {
		b.Error(err)
	}
}

func BenchmarkThumbnailBuffer(b *testing.B) {
	benchSetup(b)

	for b.Loop() {
		thumbnail(b, testJPEG, 256)
	}
	reportPixels(b, photoPixels)
}

func BenchmarkThumbnailBufferParallel(b *testing.B) {
	benchSetup(b)

	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			thumbnail(b, testJPEG, 256)
		}
	})
	reportPixels(b, photoPixels)
}

func BenchmarkThumbnailBufferLarge(b *testing.B) {
	benchSetup(b)
	large := largeJPEG(b)

	for _, w := range []int{256, 2048} {
		b.Run(fmt.Sprint(w), func(b *testing.B) {
			for b.Loop() {
				thumbnail(b, large, w)
			}
			reportPixels(b, 64*photoPixels)
		})
		b.Run(fmt.Sprintf("%d-parallel", w), func(b *testing.B) {
			b.RunParallel(func(pb *testing.PB) {
				for pb.Next() {
					thumbnail(b, large, w)
				}
			})
			reportPixels(b, 64*photoPixels)
		})
	}
}

func BenchmarkResize(b *testing.B) {
	benchSetup(b)
	photo := memoryImage(b)
	// A white disk on a transparent background, 512x512 and 4096x4096.
	logo := must(b)(must(b)(Load("testdata/logo.png", nil)).CopyMemory())
	largeLogo := must(b)(must(b)(logo.Resize(8, nil)).CopyMemory())

	resize := func(b *testing.B, src *Image, opts Options) {
		for b.Loop() {
			im, err := src.Resize(0.4, opts)
			if err != nil {
				b.Fatal(err)
			}
			if _, err := Call("avg", Options{"in": im}); err != nil {
				b.Fatal(err)
			}
			im.Close()
		}
		reportPixels(b, src.Width()*src.Height())
	}
	for _, kernel := range []Kernel{KernelNearest, KernelLinear, KernelLanczos3} {
		b.Run(string(kernel), func(b *testing.B) { resize(b, photo, Options{"kernel": kernel}) })
	}
	b.Run("alpha", func(b *testing.B) { resize(b, logo, nil) })
	b.Run("alpha-large", func(b *testing.B) { resize(b, largeLogo, nil) })
}

func BenchmarkSaveBuffer(b *testing.B) {
	benchSetup(b)
	src := memoryImage(b)

	for _, suffix := range []string{".jpg", ".png", ".webp", ".gif"} {
		b.Run(suffix[1:], func(b *testing.B) {
			var n int
			for b.Loop() {
				buf, err := src.SaveBuffer(suffix, nil)
				if err != nil {
					b.Fatal(err)
				}
				n = len(buf)
			}
			reportPixels(b, photoPixels)
			b.ReportMetric(float64(n), "out-bytes")
		})
	}
}

func BenchmarkFromImage(b *testing.B) {
	benchSetup(b)
	m := decodedJPEG(b)

	for b.Loop() {
		im, err := FromImage(m)
		if err != nil {
			b.Fatal(err)
		}
		im.Close()
	}
	reportPixels(b, photoPixels)
}

func BenchmarkToImage(b *testing.B) {
	benchSetup(b)
	src := memoryImage(b)

	for b.Loop() {
		if _, err := src.ToImage(); err != nil {
			b.Fatal(err)
		}
	}
	reportPixels(b, photoPixels)
}

// BenchmarkConcurrency runs common functions with several numbers of libvips threads
// (SetConcurrency), one call at a time ("serial") and from GOMAXPROCS goroutines ("parallel"). It
// shows how many threads to use for one large image and for many images processed in parallel.
func BenchmarkConcurrency(b *testing.B) {
	benchSetup(b)
	b.Cleanup(func() { SetConcurrency(0) })

	photo := memoryImage(b)
	large := must(b)(must(b)(photo.Resize(8, nil)).CopyMemory())
	largeJPEG := largeJPEG(b)
	// avg decodes all the pixels of im.
	avg := func(im *Image, err error) error {
		if err != nil {
			return err
		}
		defer im.Close()
		_, err = Call("avg", Options{"in": im})

		return err
	}
	// save saves the thumbnail im as JPEG.
	save := func(im *Image, err error) error {
		if err != nil {
			return err
		}
		defer im.Close()
		_, err = im.SaveBuffer(".jpg", nil)

		return err
	}

	for _, op := range []struct {
		name   string
		pixels int
		run    func() error
	}{
		{"LoadBuffer", photoPixels, func() error { return avg(LoadBuffer(testJPEG, nil)) }},
		{"ThumbnailBuffer", photoPixels, func() error { return save(ThumbnailBuffer(testJPEG, 256, 0, nil)) }},
		{"ThumbnailBufferLarge", 64 * photoPixels, func() error { return save(ThumbnailBuffer(largeJPEG, 2048, 0, nil)) }},
		{"Resize", photoPixels, func() error { return avg(photo.Resize(0.4, nil)) }},
		{"ResizeLarge", 64 * photoPixels, func() error { return avg(large.Resize(0.25, nil)) }},
		{"SaveBuffer", photoPixels, func() error { _, err := photo.SaveBuffer(".jpg", nil); return err }},
		{"ToImage", photoPixels, func() error { _, err := photo.ToImage(); return err }},
	} {
		for _, threads := range []int{1, 2, 4, 8, 0} {
			name := fmt.Sprintf("%s/threads=%d", op.name, threads)
			if threads == 0 {
				name = op.name + "/threads=default"
			}
			b.Run(name+"/serial", func(b *testing.B) {
				SetConcurrency(threads)
				for b.Loop() {
					if err := op.run(); err != nil {
						b.Fatal(err)
					}
				}
				reportPixels(b, op.pixels)
			})
			b.Run(name+"/parallel", func(b *testing.B) {
				SetConcurrency(threads)
				b.RunParallel(func(pb *testing.PB) {
					for pb.Next() {
						if err := op.run(); err != nil {
							b.Error(err)
							return
						}
					}
				})
				reportPixels(b, op.pixels)
			})
		}
	}
}
