//go:build unix || darwin || windows

package vips

import (
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

func BenchmarkThumbnailBuffer(b *testing.B) {
	benchSetup(b)

	for b.Loop() {
		im, err := ThumbnailBuffer(testJPEG, 256, 0, nil)
		if err != nil {
			b.Fatal(err)
		}
		if _, err := im.SaveBuffer(".jpg", nil); err != nil {
			b.Fatal(err)
		}
		im.Close()
	}
	reportPixels(b, photoPixels)
}

func BenchmarkThumbnailBufferParallel(b *testing.B) {
	benchSetup(b)

	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			im, err := ThumbnailBuffer(testJPEG, 256, 0, nil)
			if err != nil {
				b.Error(err)
				return
			}
			if _, err := im.SaveBuffer(".jpg", nil); err != nil {
				b.Error(err)
			}
			im.Close()
		}
	})
	reportPixels(b, photoPixels)
}

func BenchmarkResize(b *testing.B) {
	benchSetup(b)
	src := memoryImage(b)

	for _, kernel := range []Kernel{KernelNearest, KernelLinear, KernelLanczos3} {
		b.Run(string(kernel), func(b *testing.B) {
			for b.Loop() {
				im, err := src.Resize(0.4, Options{"kernel": kernel})
				if err != nil {
					b.Fatal(err)
				}
				if _, err := Call("avg", Options{"in": im}); err != nil {
					b.Fatal(err)
				}
				im.Close()
			}
			reportPixels(b, photoPixels)
		})
	}
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
