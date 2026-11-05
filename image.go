package vips

import (
	"encoding/binary"
	"errors"
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"maps"
	"runtime"
	"unsafe"
)

// Image is a libvips image. Its methods may be called from several goroutines, but not
// concurrently with Close.
type Image struct {
	p       unsafe.Pointer // VipsImage, of which Image owns one reference
	cleanup runtime.Cleanup
}

// newImage returns an Image owning the reference p.
func newImage(p unsafe.Pointer) *Image {
	im := &Image{p: p}
	// The reference is released when im is garbage collected if it was not closed.
	im.cleanup = runtime.AddCleanup(im, func(p unsafe.Pointer) { g_object_unref(p) }, p)

	return im
}

// Close releases the image. The memory of libvips is not seen by the Go garbage collector, so
// images should be closed when they are no longer needed. Close does nothing on a closed image.
func (im *Image) Close() {
	if im == nil || im.p == nil {
		return
	}

	im.cleanup.Stop()
	g_object_unref(im.p)
	im.p = nil
}

// ptr returns the VipsImage of im. The caller must keep im alive while using it.
func (im *Image) ptr() unsafe.Pointer {
	if im == nil || im.p == nil {
		panic("vips: use of a nil or closed Image")
	}

	return im.p
}

// Load loads the image file filename, in any format supported by libvips. opts holds the
// options of the loader, such as Options{"access": "sequential"}.
func Load(filename string, opts Options) (*Image, error) {
	if dynamicErr != nil {
		return nil, dynamicErr
	}

	loader := vips_foreign_find_load(filename)
	if loader == "" {
		return nil, fmt.Errorf("vips: load %s: %s", filename, lastError())
	}

	return callImage(loader, opts, Options{"filename": filename})
}

// LoadBuffer loads the image file contained in buf, in any format supported by libvips. opts holds
// the options of the loader, such as Options{"n": -1} to load all the pages of an animation.
func LoadBuffer(buf []byte, opts Options) (*Image, error) {
	if dynamicErr != nil {
		return nil, dynamicErr
	}

	loader := vips_foreign_find_load_buffer(buf, uint64(len(buf)))
	if loader == "" {
		return nil, fmt.Errorf("vips: load buffer: %s", lastError())
	}

	return callImage(loader, opts, Options{"buffer": buf})
}

// Save saves im to the file filename, in the format given by its suffix. opts holds the options
// of the saver, such as Options{"Q": 90} for JPEG.
func (im *Image) Save(filename string, opts Options) error {
	saver := vips_foreign_find_save(filename)
	if saver == "" {
		return fmt.Errorf("vips: save %s: %s", filename, lastError())
	}

	outputs, err := Call(saver, withArgs(opts, Options{"in": im, "filename": filename}))
	closeImages(outputs)

	return err
}

// SaveBuffer returns im saved in the format given by suffix, such as ".jpg", ".png", ".webp",
// ".avif" or ".gif". opts holds the options of the saver, such as Options{"Q": 90} for JPEG.
func (im *Image) SaveBuffer(suffix string, opts Options) ([]byte, error) {
	saver := vips_foreign_find_save_buffer(suffix)
	if saver == "" {
		return nil, fmt.Errorf("vips: save %s: %s", suffix, lastError())
	}

	outputs, err := Call(saver, withArgs(opts, Options{"in": im}))
	if err != nil {
		return nil, err
	}
	closeImages(outputs)

	buf, ok := outputs["buffer"].([]byte)
	if !ok {
		return nil, fmt.Errorf("vips: %s: no output buffer", saver)
	}

	return buf, nil
}

// withArgs returns a copy of opts with args added.
func withArgs(opts, args Options) Options {
	all := maps.Clone(opts)
	if all == nil {
		all = Options{}
	}
	maps.Copy(all, args)

	return all
}

// CopyMemory returns a copy of im computed in memory. The images loaded with the "sequential"
// access are computed while their file is read, once only: a copy in memory can be saved or
// processed several times.
func (im *Image) CopyMemory() (*Image, error) {
	defer runtime.KeepAlive(im)

	p := vips_image_copy_memory(im.ptr())
	if p == nil {
		return nil, fmt.Errorf("vips: copy memory: %s", lastError())
	}

	return newImage(p), nil
}

// Width returns the width of im in pixels.
func (im *Image) Width() int {
	defer runtime.KeepAlive(im)

	return int(vips_image_get_width(im.ptr()))
}

// Height returns the height of im in pixels. For a multi-page image, it is the height of all the
// pages; the height of a page is the "page-height" field.
func (im *Image) Height() int {
	defer runtime.KeepAlive(im)

	return int(vips_image_get_height(im.ptr()))
}

// pageHeight returns the height of a page of im: the "page-height" field if it divides the height,
// the height otherwise.
func (im *Image) pageHeight() int {
	defer runtime.KeepAlive(im)

	return int(vips_image_get_page_height(im.ptr()))
}

// Bands returns the number of bands (channels) of im.
func (im *Image) Bands() int {
	defer runtime.KeepAlive(im)

	return int(vips_image_get_bands(im.ptr()))
}

// Format returns the format of the pixel values of im.
func (im *Image) Format() BandFormat {
	defer runtime.KeepAlive(im)

	return BandFormat(vips_enum_nick(typeBandFormat, vips_image_get_format(im.ptr())))
}

// Interpretation returns how the bands of im are interpreted.
func (im *Image) Interpretation() Interpretation {
	defer runtime.KeepAlive(im)

	return Interpretation(vips_enum_nick(typeInterpretation, vips_image_get_interpretation(im.ptr())))
}

// HasAlpha reports whether the last band of im looks like an alpha channel.
func (im *Image) HasAlpha() bool {
	defer runtime.KeepAlive(im)

	return vips_image_hasalpha(im.ptr()) != 0
}

// Get returns the metadata field name of im, such as "orientation", "n-pages", "exif-data" or
// "icc-profile-data", converted like the outputs of Call.
func (im *Image) Get(name string) (any, error) {
	defer runtime.KeepAlive(im)

	var v gValue
	if vips_image_get(im.ptr(), name, &v) != 0 {
		return nil, fmt.Errorf("vips: get %s: %s", name, lastError())
	}
	defer g_value_unset(&v)

	x, err := fromValue(&v)
	if err != nil {
		return nil, fmt.Errorf("vips: get %s: %w", name, err)
	}

	return x, nil
}

// Fields returns the names of the metadata fields of im.
func (im *Image) Fields() []string {
	defer runtime.KeepAlive(im)

	p := vips_image_get_fields(im.ptr())
	defer g_strfreev(p)

	var names []string
	for ; *(*unsafe.Pointer)(p) != nil; p = unsafe.Add(p, unsafe.Sizeof(p)) {
		names = append(names, goString(*(*unsafe.Pointer)(p)))
	}

	return names
}

// VipsBandFormat values used with vips_image_new_from_memory_copy.
const (
	formatUchar  = 0
	formatUshort = 2
)

// FromImage returns a copy of m. Gray images become one-band images ("b-w" or "grey16"), the other
// images sRGB images ("srgb" or "rgb16") with an alpha band unless they are opaque. Images of 16-bit
// color models keep their 16 bits.
func FromImage(m image.Image) (*Image, error) {
	if dynamicErr != nil {
		return nil, dynamicErr
	}

	r := m.Bounds()
	w, h := r.Dx(), r.Dy()
	if w <= 0 || h <= 0 {
		return nil, errors.New("vips: empty image")
	}

	// pix holds the rows of channels samples of depth bytes per pixel.
	var pix []byte
	var stride, channels, depth int
	var opaque bool
	var space Interpretation
	switch m.ColorModel() {
	case color.GrayModel:
		g := convert(m, image.NewGray)
		pix, stride, channels, depth, opaque, space = g.Pix, g.Stride, 1, 1, true, InterpretationBW
	case color.Gray16Model:
		g := convert(m, image.NewGray16)
		pix, stride, channels, depth, opaque, space = g.Pix, g.Stride, 1, 2, true, InterpretationGrey16
	case color.RGBA64Model, color.NRGBA64Model:
		n := convert(m, image.NewNRGBA64)
		pix, stride, channels, depth, opaque, space = n.Pix, n.Stride, 4, 2, n.Opaque(), InterpretationRGB16
	default:
		n := toNRGBA(m)
		pix, stride, channels, depth, opaque, space = n.Pix, n.Stride, 4, 1, n.Opaque(), InterpretationSRGB
	}

	bands := channels
	if channels == 4 && opaque {
		bands = 3
	}
	// Pack the rows and drop the alpha channel of opaque images.
	px := bands * depth
	data := make([]byte, w*h*px)
	for y := range h {
		src, dst := pix[y*stride:], data[y*w*px:(y+1)*w*px]
		if bands == channels {
			copy(dst, src)
			continue
		}
		for x := range w {
			copy(dst[x*px:(x+1)*px], src[x*channels*depth:])
		}
	}

	format := int32(formatUchar)
	if depth == 2 {
		format = formatUshort
		// libvips stores 16-bit samples in native byte order, the image package in big-endian.
		for i := 0; i < len(data); i += 2 {
			binary.NativeEndian.PutUint16(data[i:], binary.BigEndian.Uint16(data[i:]))
		}
	}

	p := vips_image_new_from_memory_copy(data, uint64(len(data)), int32(w), int32(h), int32(bands), format)
	if p == nil {
		return nil, fmt.Errorf("vips: from image: %s", lastError())
	}
	im := newImage(p)
	defer im.Close()

	return im.Copy(Options{"interpretation": space})
}

// convert returns m as a T, converting it with newImage and draw.Draw if it is not one.
func convert[T draw.Image](m image.Image, newImage func(image.Rectangle) T) T {
	if t, ok := m.(T); ok {
		return t
	}

	t := newImage(m.Bounds())
	draw.Draw(t, m.Bounds(), m, m.Bounds().Min, draw.Src)

	return t
}

// toNRGBA returns m as an *image.NRGBA, without conversion for an opaque *image.RGBA.
func toNRGBA(m image.Image) *image.NRGBA {
	if rgba, ok := m.(*image.RGBA); ok && rgba.Opaque() {
		return &image.NRGBA{Pix: rgba.Pix, Stride: rgba.Stride, Rect: rgba.Rect}
	}

	return convert(m, image.NewNRGBA)
}

// ToImage returns im as an image.Image of the image package, converted to sRGB or gray:
// *image.Gray or *image.Gray16 for gray images without alpha, *image.RGBA or *image.RGBA64 for
// images without alpha, *image.NRGBA or *image.NRGBA64 otherwise. 16-bit images ("rgb16" and
// "grey16") give 16-bit images, all others 8-bit images.
func (im *Image) ToImage() (image.Image, error) {
	space := im.Interpretation()
	gray := space == InterpretationBW || space == InterpretationGrey16
	deep := space == InterpretationRGB16 || space == InterpretationGrey16

	target, format := InterpretationSRGB, BandFormatUchar
	switch {
	case gray && deep:
		target, format = InterpretationGrey16, BandFormatUshort
	case gray:
		target = InterpretationBW
	case deep:
		target, format = InterpretationRGB16, BandFormatUshort
	}

	c, err := im.Colourspace(target, nil)
	if err != nil {
		return nil, err
	}
	defer c.Close()
	if c.Format() != format {
		cast, err := c.Cast(format, nil)
		if err != nil {
			return nil, err
		}
		defer cast.Close()
		c = cast
	}

	colors := 3
	if gray {
		colors = 1
	}
	bands := c.Bands()
	if bands != colors && bands != colors+1 {
		return nil, fmt.Errorf("vips: cannot convert a %d-band %s image", bands, target)
	}

	data, err := c.writeToMemory()
	if err != nil {
		return nil, err
	}

	return goImage(data, c.Width(), c.Height(), bands, deep), nil
}

// writeToMemory returns the pixels of im.
func (im *Image) writeToMemory() ([]byte, error) {
	defer runtime.KeepAlive(im)

	var n uint64
	p := vips_image_write_to_memory(im.ptr(), &n)
	if p == nil {
		return nil, fmt.Errorf("vips: write to memory: %s", lastError())
	}
	defer g_free(p)

	return cBytes(p, int(n)), nil
}

// goImage returns the w x h pixels data of bands 8-bit or 16-bit (if deep) samples as an image of
// the image package.
func goImage(data []byte, w, h, bands int, deep bool) image.Image {
	r := image.Rect(0, 0, w, h)
	depth := 1
	if deep {
		depth = 2
		for i := 0; i < len(data); i += 2 {
			binary.BigEndian.PutUint16(data[i:], binary.NativeEndian.Uint16(data[i:]))
		}
	}

	switch {
	case bands == 1 && !deep:
		return &image.Gray{Pix: data, Stride: w, Rect: r}
	case bands == 1:
		return &image.Gray16{Pix: data, Stride: 2 * w, Rect: r}
	case bands == 4 && !deep:
		return &image.NRGBA{Pix: data, Stride: 4 * w, Rect: r}
	case bands == 4:
		return &image.NRGBA64{Pix: data, Stride: 8 * w, Rect: r}
	}

	// Expand gray with alpha and RGB to RGBA, where -1 is an opaque alpha.
	channels := [4]int{0, 0, 0, 1}
	if bands == 3 {
		channels = [4]int{0, 1, 2, -1}
	}
	pix := make([]byte, w*h*4*depth)
	for i := range w * h {
		for c, s := range channels {
			for d := range depth {
				v := byte(0xff)
				if s >= 0 {
					v = data[(i*bands+s)*depth+d]
				}
				pix[(i*4+c)*depth+d] = v
			}
		}
	}

	switch {
	case bands == 3 && !deep:
		return &image.RGBA{Pix: pix, Stride: 4 * w, Rect: r}
	case bands == 3:
		return &image.RGBA64{Pix: pix, Stride: 8 * w, Rect: r}
	case !deep:
		return &image.NRGBA{Pix: pix, Stride: 4 * w, Rect: r}
	}

	return &image.NRGBA64{Pix: pix, Stride: 8 * w, Rect: r}
}
