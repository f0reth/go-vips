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
	"slices"
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

	// data holds the w x h pixels of bands samples, of 8 bits or 16 bits in native byte order.
	var data []byte
	var bands int
	format, space := int32(formatUchar), InterpretationSRGB
	if y, ok := m.(*image.YCbCr); ok {
		data, bands = ycbcrToRGB(y), 3
	} else {
		switch m.ColorModel() {
		case color.GrayModel:
			g := convert(m, image.NewGray)
			data, bands, space = pack(g.Pix, g.Stride, w, h, 1, 1, 1), 1, InterpretationBW
		case color.Gray16Model:
			g := convert(m, image.NewGray16)
			data, bands, space = pack(g.Pix, g.Stride, w, h, 1, 1, 2), 1, InterpretationGrey16
			format = formatUshort
		case color.RGBA64Model, color.NRGBA64Model:
			n := convert(m, image.NewNRGBA64)
			bands = alphaBands(n.Opaque())
			data, format, space = pack(n.Pix, n.Stride, w, h, 4, bands, 2), formatUshort, InterpretationRGB16
		default:
			n := toNRGBA(m)
			bands = alphaBands(n.Opaque())
			data = pack(n.Pix, n.Stride, w, h, 4, bands, 1)
		}
	}

	p := vips_image_new_from_memory_copy(data, uint64(len(data)), int32(w), int32(h), int32(bands), format)
	if p == nil {
		return nil, fmt.Errorf("vips: from image: %s", lastError())
	}
	im := newImage(p)
	// libvips guesses the interpretation from the bands and the format: right for 8-bit gray and
	// RGB ("b-w" and "srgb"), "multiband" or "b-w" otherwise.
	if im.Interpretation() == space {
		return im, nil
	}
	defer im.Close()

	return im.Copy(Options{"interpretation": space})
}

// alphaBands returns the number of bands of a color image: 3 if it is opaque, 4 otherwise.
func alphaBands(opaque bool) int {
	if opaque {
		return 3
	}

	return 4
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

// ycbcrToRGB returns the pixels of m as 8-bit RGB samples, converted like color.YCbCrToRGB.
func ycbcrToRGB(m *image.YCbCr) []byte {
	r := m.Rect
	w := r.Dx()
	// The offset of the chroma samples of every subsampling ratio is the sum of an offset depending
	// on the row and one depending on the column: cx holds the latter.
	cx := make([]int, w)
	for i := range cx {
		cx[i] = m.COffset(r.Min.X+i, r.Min.Y) - m.COffset(r.Min.X, r.Min.Y)
	}

	data := make([]byte, 3*w*r.Dy())
	for y := r.Min.Y; y < r.Max.Y; y++ {
		ys := m.Y[m.YOffset(r.Min.X, y):][:w]
		ci := m.COffset(r.Min.X, y)
		cbs, crs := m.Cb[ci:], m.Cr[ci:]
		dst := data[3*w*(y-r.Min.Y):][:3*w]
		for i, yv := range ys {
			// color.YCbCrToRGB, which is not inlined.
			yy := int32(yv) * 0x10101
			cb := int32(cbs[cx[i]]) - 128
			cr := int32(crs[cx[i]]) - 128
			d := dst[3*i : 3*i+3 : 3*i+3]
			d[0] = clamp16(yy + 91881*cr)
			d[1] = clamp16(yy - 22554*cb - 46802*cr)
			d[2] = clamp16(yy + 116130*cb)
		}
	}

	return data
}

// clamp16 returns v >> 16 clamped to [0, 255].
func clamp16(v int32) uint8 {
	if uint32(v)&0xff000000 == 0 {
		return uint8(v >> 16)
	}

	return uint8(^(v >> 31))
}

// pack returns the w x h pixels of pix, rows of stride bytes of pixels of channels big-endian
// samples of depth bytes, as unpadded rows of the first bands samples in native byte order. It
// returns pix itself if it already has this layout.
func pack(pix []byte, stride, w, h, channels, bands, depth int) []byte {
	in, out := channels*depth, bands*depth
	if depth == 1 && channels == bands && stride == w*out {
		return pix[:h*stride]
	}

	data := make([]byte, w*h*out)
	for y := range h {
		src, dst := pix[y*stride:y*stride+w*in], data[y*w*out:(y+1)*w*out]
		switch {
		case depth == 2:
			for x := range w {
				s, d := src[x*in:], dst[x*out:]
				for c := range bands {
					binary.NativeEndian.PutUint16(d[2*c:], binary.BigEndian.Uint16(s[2*c:]))
				}
			}
		case channels == bands:
			copy(dst, src)
		default: // 4 to 3 samples of 8 bits
			for s, d := 0, 0; d < len(dst); s, d = s+4, d+3 {
				dst[d], dst[d+1], dst[d+2] = src[s], src[s+1], src[s+2]
			}
		}
	}

	return data
}

// ToImage returns im as an image.Image of the image package, converted to sRGB or gray:
// *image.Gray or *image.Gray16 for gray images without alpha, *image.RGBA or *image.RGBA64 for
// images without alpha, *image.NRGBA or *image.NRGBA64 otherwise. 16-bit images ("rgb16" and
// "grey16") give 16-bit images, all others 8-bit images.
func (im *Image) ToImage() (image.Image, error) {
	space := im.Interpretation()
	gray := space == InterpretationBW || space == InterpretationGrey16
	deep := space == InterpretationRGB16 || space == InterpretationGrey16

	target, color, format := InterpretationSRGB, InterpretationSRGB, BandFormatUchar
	switch {
	case gray && deep:
		target, color, format = InterpretationGrey16, InterpretationRGB16, BandFormatUshort
	case gray:
		target = InterpretationBW
	case deep:
		target, color, format = InterpretationRGB16, InterpretationRGB16, BandFormatUshort
	}

	// c is im converted step by step; steps holds the images to close.
	c := im
	var steps []*Image
	defer func() {
		for _, s := range steps {
			s.Close()
		}
	}()
	step := func(out *Image, err error) error {
		if err == nil {
			steps = append(steps, out)
			c = out
		}

		return err
	}

	if space != target {
		if err := step(c.Colourspace(target, nil)); err != nil {
			return nil, err
		}
	}
	if c.Format() != format {
		if err := step(c.Cast(format, nil)); err != nil {
			return nil, err
		}
	}

	colors := 3
	if gray {
		colors = 1
	}
	bands := c.Bands()
	if bands != colors && bands != colors+1 {
		return nil, fmt.Errorf("vips: cannot convert a %d-band %s image", bands, target)
	}
	// The image package has no gray image with alpha: libvips converts it to color.
	if gray && bands == 2 {
		if err := step(c.Colourspace(color, nil)); err != nil {
			return nil, err
		}
	}

	data, err := c.writeToMemory(deep)
	if err != nil {
		return nil, err
	}

	r := image.Rect(0, 0, c.Width(), c.Height())
	switch {
	case bands == 1 && !deep:
		return &image.Gray{Pix: data, Stride: r.Dx(), Rect: r}, nil
	case bands == 1:
		return &image.Gray16{Pix: data, Stride: 2 * r.Dx(), Rect: r}, nil
	case bands == 3 && !deep:
		return &image.RGBA{Pix: data, Stride: 4 * r.Dx(), Rect: r}, nil
	case bands == 3:
		return &image.RGBA64{Pix: data, Stride: 8 * r.Dx(), Rect: r}, nil
	case !deep:
		return &image.NRGBA{Pix: data, Stride: 4 * r.Dx(), Rect: r}, nil
	}

	return &image.NRGBA64{Pix: data, Stride: 8 * r.Dx(), Rect: r}, nil
}

// writeToMemory returns the pixels of im as samples of the image package: 16-bit samples (if deep)
// in big-endian order, and RGB pixels of 3 samples with an opaque alpha sample added.
func (im *Image) writeToMemory(deep bool) ([]byte, error) {
	defer runtime.KeepAlive(im)

	rgb := im.Bands() == 3
	var n uint64
	p := vips_image_write_to_memory(im.ptr(), &n)
	if p == nil {
		return nil, fmt.Errorf("vips: write to memory: %s", lastError())
	}
	defer g_free(p)

	src := cSlice[byte](p, int(n))
	switch {
	case !rgb && !deep:
		return slices.Clone(src), nil
	case !rgb:
		data := make([]byte, len(src))
		for i := 0; i < len(src); i += 2 {
			binary.BigEndian.PutUint16(data[i:], binary.NativeEndian.Uint16(src[i:]))
		}
		return data, nil
	case !deep:
		data := make([]byte, len(src)/3*4)
		for s, d := 0, 0; s < len(src); s, d = s+3, d+4 {
			in, out := src[s:s+3:s+3], data[d:d+4:d+4]
			out[0], out[1], out[2], out[3] = in[0], in[1], in[2], 0xff
		}
		return data, nil
	}

	data := make([]byte, len(src)/6*8)
	for s, d := 0, 0; s < len(src); s, d = s+6, d+8 {
		in, out := src[s:s+6:s+6], data[d:d+8:d+8]
		binary.BigEndian.PutUint16(out[0:], binary.NativeEndian.Uint16(in[0:]))
		binary.BigEndian.PutUint16(out[2:], binary.NativeEndian.Uint16(in[2:]))
		binary.BigEndian.PutUint16(out[4:], binary.NativeEndian.Uint16(in[4:]))
		out[6], out[7] = 0xff, 0xff
	}

	return data, nil
}
