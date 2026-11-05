//go:build unix || darwin || windows

package vips

import (
	"bytes"
	_ "embed"
	"image"
	"image/color"
	"image/jpeg"
	"sync"
	"testing"
	"unsafe"

	"github.com/ebitengine/purego"
)

// The tests of this package are written for a working libvips shared library: they fail, instead
// of being skipped, when it cannot be loaded. Only the test of layout_test.go does not need it.

// A 768x512 photo of the Kodak test image set (kodim23), as a JPEG.
//
//go:embed testdata/photo.jpg
var testJPEG []byte

// GObject and GLib functions only used by the tests.
var (
	g_object_add_weak_pointer    func(object unsafe.Pointer, location unsafe.Pointer)
	g_object_remove_weak_pointer func(object unsafe.Pointer, location unsafe.Pointer)
	g_malloc0                    func(size uint64) unsafe.Pointer
	testFuncs                    sync.Once
)

// requireLibrary fails the test if the libvips shared library cannot be used.
func requireLibrary(tb testing.TB) {
	tb.Helper()

	if err := Dynamic(); err != nil {
		tb.Fatalf("libvips is required by this test: %v\n"+
			"Put the directory of %s in the library search path (PATH, LD_LIBRARY_PATH or DYLD_LIBRARY_PATH), see README.",
			err, libvipsName)
	}

	testFuncs.Do(func() {
		purego.RegisterLibFunc(&g_object_add_weak_pointer, libgobject, "g_object_add_weak_pointer")
		purego.RegisterLibFunc(&g_object_remove_weak_pointer, libgobject, "g_object_remove_weak_pointer")
		purego.RegisterLibFunc(&g_malloc0, libglib, "g_malloc0")
	})
}

// weakRef tracks whether the GObject p is finalized, without holding a reference to it.
type weakRef struct {
	object   unsafe.Pointer
	location unsafe.Pointer // C memory, set to NULL by GObject when the object is finalized
}

func newWeakRef(t *testing.T, p unsafe.Pointer) *weakRef {
	t.Helper()

	w := &weakRef{object: p, location: g_malloc0(8)}
	*(*unsafe.Pointer)(w.location) = p
	g_object_add_weak_pointer(p, w.location)
	t.Cleanup(func() {
		if w.alive() {
			g_object_remove_weak_pointer(w.object, w.location)
		}
		g_free(w.location)
	})

	return w
}

func (w *weakRef) alive() bool {
	return *(*unsafe.Pointer)(w.location) != nil
}

func mustLoad(tb testing.TB) *Image {
	tb.Helper()
	requireLibrary(tb)

	im, err := LoadBuffer(testJPEG, nil)
	if err != nil {
		tb.Fatal(err)
	}
	tb.Cleanup(im.Close)

	return im
}

func mustFromImage(tb testing.TB, m image.Image) *Image {
	tb.Helper()
	requireLibrary(tb)

	im, err := FromImage(m)
	if err != nil {
		tb.Fatal(err)
	}
	tb.Cleanup(im.Close)

	return im
}

// must returns a function that fails the test on err and closes im at the end of the test, used
// as must(t)(im.Resize(0.5, nil)).
func must(tb testing.TB) func(im *Image, err error) *Image {
	return func(im *Image, err error) *Image {
		tb.Helper()

		if err != nil {
			tb.Fatal(err)
		}
		tb.Cleanup(im.Close)

		return im
	}
}

// pixel returns the values of the bands of im at (x, y).
func pixel(t *testing.T, im *Image, x, y int) []float64 {
	t.Helper()

	out, err := Call("getpoint", Options{"in": im, "x": x, "y": y})
	if err != nil {
		t.Fatal(err)
	}

	return out["out_array"].([]float64)
}

// decodedJPEG returns the test photo decoded by image/jpeg, an *image.YCbCr.
func decodedJPEG(tb testing.TB) image.Image {
	tb.Helper()

	m, err := jpeg.Decode(bytes.NewReader(testJPEG))
	if err != nil {
		tb.Fatal(err)
	}

	return m
}

// noiseImage returns an image of random pixels, with random alpha if alpha is set.
func noiseImage(w, h int, seed uint64, alpha bool) *image.NRGBA {
	m := image.NewNRGBA(image.Rect(0, 0, w, h))
	x := seed
	for i := range m.Pix {
		// xorshift64
		x ^= x << 13
		x ^= x >> 7
		x ^= x << 17
		m.Pix[i] = uint8(x >> 32)
		if i%4 == 3 && !alpha {
			m.Pix[i] = 0xff
		}
	}

	return m
}

// samePixels fails the test if a and b, compared from their top-left corners, do not have the same
// size and the same non-premultiplied 16-bit colors.
func samePixels(t *testing.T, a, b image.Image) {
	t.Helper()

	ab, bb := a.Bounds(), b.Bounds()
	if ab.Size() != bb.Size() {
		t.Fatalf("size: got %v, want %v", bb.Size(), ab.Size())
	}
	for y := range ab.Dy() {
		for x := range ab.Dx() {
			ca := color.NRGBA64Model.Convert(a.At(ab.Min.X+x, ab.Min.Y+y))
			cb := color.NRGBA64Model.Convert(b.At(bb.Min.X+x, bb.Min.Y+y))
			if ca != cb {
				t.Fatalf("pixel (%d, %d): got %v, want %v", x, y, cb, ca)
			}
		}
	}
}
