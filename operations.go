package vips

import (
	"cmp"
	"errors"
	"fmt"
	"reflect"
)

// Typed wrappers of common libvips operations. Each one is named after the operation, takes its
// required arguments as parameters and its optional arguments in opts (nil for none), and returns
// its "out" image. Call runs the other operations.

// BandFormat is the format of the pixel values of an image (VipsBandFormat).
type BandFormat string

// Band formats.
const (
	BandFormatUchar     BandFormat = "uchar"
	BandFormatChar      BandFormat = "char"
	BandFormatUshort    BandFormat = "ushort"
	BandFormatShort     BandFormat = "short"
	BandFormatUint      BandFormat = "uint"
	BandFormatInt       BandFormat = "int"
	BandFormatFloat     BandFormat = "float"
	BandFormatComplex   BandFormat = "complex"
	BandFormatDouble    BandFormat = "double"
	BandFormatDpComplex BandFormat = "dpcomplex"
)

// Interpretation is how the bands of an image are interpreted (VipsInterpretation).
type Interpretation string

// Interpretations.
const (
	InterpretationMultiband Interpretation = "multiband"
	InterpretationBW        Interpretation = "b-w"
	InterpretationHistogram Interpretation = "histogram"
	InterpretationXYZ       Interpretation = "xyz"
	InterpretationLab       Interpretation = "lab"
	InterpretationCMYK      Interpretation = "cmyk"
	InterpretationLabQ      Interpretation = "labq"
	InterpretationRGB       Interpretation = "rgb"
	InterpretationCMC       Interpretation = "cmc"
	InterpretationLCh       Interpretation = "lch"
	InterpretationLabS      Interpretation = "labs"
	InterpretationSRGB      Interpretation = "srgb"
	InterpretationYxy       Interpretation = "yxy"
	InterpretationFourier   Interpretation = "fourier"
	InterpretationRGB16     Interpretation = "rgb16"
	InterpretationGrey16    Interpretation = "grey16"
	InterpretationMatrix    Interpretation = "matrix"
	InterpretationScRGB     Interpretation = "scrgb"
	InterpretationHSV       Interpretation = "hsv"
)

// Angle is a rotation by a multiple of 90 degrees, clockwise (VipsAngle).
type Angle string

// Angles.
const (
	AngleD0   Angle = "d0"
	AngleD90  Angle = "d90"
	AngleD180 Angle = "d180"
	AngleD270 Angle = "d270"
)

// Direction is a flip direction (VipsDirection).
type Direction string

// Directions.
const (
	DirectionHorizontal Direction = "horizontal"
	DirectionVertical   Direction = "vertical"
)

// CompassDirection is a position in an area (VipsCompassDirection).
type CompassDirection string

// Compass directions.
const (
	CompassDirectionCentre    CompassDirection = "centre"
	CompassDirectionNorth     CompassDirection = "north"
	CompassDirectionEast      CompassDirection = "east"
	CompassDirectionSouth     CompassDirection = "south"
	CompassDirectionWest      CompassDirection = "west"
	CompassDirectionNorthEast CompassDirection = "north-east"
	CompassDirectionSouthEast CompassDirection = "south-east"
	CompassDirectionSouthWest CompassDirection = "south-west"
	CompassDirectionNorthWest CompassDirection = "north-west"
)

// BlendMode is how an overlay is composited on a base image (VipsBlendMode).
type BlendMode string

// Blend modes.
const (
	BlendModeClear       BlendMode = "clear"
	BlendModeSource      BlendMode = "source"
	BlendModeOver        BlendMode = "over"
	BlendModeIn          BlendMode = "in"
	BlendModeOut         BlendMode = "out"
	BlendModeAtop        BlendMode = "atop"
	BlendModeDest        BlendMode = "dest"
	BlendModeDestOver    BlendMode = "dest-over"
	BlendModeDestIn      BlendMode = "dest-in"
	BlendModeDestOut     BlendMode = "dest-out"
	BlendModeDestAtop    BlendMode = "dest-atop"
	BlendModeXor         BlendMode = "xor"
	BlendModeAdd         BlendMode = "add"
	BlendModeSaturate    BlendMode = "saturate"
	BlendModeMultiply    BlendMode = "multiply"
	BlendModeScreen      BlendMode = "screen"
	BlendModeOverlay     BlendMode = "overlay"
	BlendModeDarken      BlendMode = "darken"
	BlendModeLighten     BlendMode = "lighten"
	BlendModeColourDodge BlendMode = "colour-dodge"
	BlendModeColourBurn  BlendMode = "colour-burn"
	BlendModeHardLight   BlendMode = "hard-light"
	BlendModeSoftLight   BlendMode = "soft-light"
	BlendModeDifference  BlendMode = "difference"
	BlendModeExclusion   BlendMode = "exclusion"
)

// Kernel is a resampling kernel, the "kernel" option of Resize (VipsKernel).
type Kernel string

// Kernels.
const (
	KernelNearest  Kernel = "nearest"
	KernelLinear   Kernel = "linear"
	KernelCubic    Kernel = "cubic"
	KernelMitchell Kernel = "mitchell"
	KernelLanczos2 Kernel = "lanczos2"
	KernelLanczos3 Kernel = "lanczos3"
)

// Interesting is how to find the interesting part of an image, the "crop" option of the thumbnail
// operations and the "interesting" option of Smartcrop (VipsInteresting).
type Interesting string

// Interesting strategies.
const (
	InterestingNone      Interesting = "none"
	InterestingCentre    Interesting = "centre"
	InterestingEntropy   Interesting = "entropy"
	InterestingAttention Interesting = "attention"
	InterestingLow       Interesting = "low"
	InterestingHigh      Interesting = "high"
	InterestingAll       Interesting = "all"
)

// Size is how the thumbnail operations may resize, their "size" option (VipsSize).
type Size string

// Sizes.
const (
	SizeBoth  Size = "both"
	SizeUp    Size = "up"
	SizeDown  Size = "down"
	SizeForce Size = "force"
)

// Extend is how to fill new pixels, the "extend" option of Embed and Gravity (VipsExtend).
type Extend string

// Extends.
const (
	ExtendBlack      Extend = "black"
	ExtendCopy       Extend = "copy"
	ExtendRepeat     Extend = "repeat"
	ExtendMirror     Extend = "mirror"
	ExtendWhite      Extend = "white"
	ExtendBackground Extend = "background"
)

// maxCoord is VIPS_MAX_COORD, the largest image dimension of libvips.
const maxCoord = 10000000

// Thumbnail loads the image file filename and resizes it to fit within width x height pixels,
// keeping its aspect ratio. A width or height of 0 leaves that dimension free: Thumbnail(name, 800,
// 0, nil) makes an image 800 pixels wide. It shrinks while loading when the format allows it (JPEG,
// WebP, ...), which makes it the fastest way to resize a file.
//
// Like the libvips thumbnail operation, it rotates the image upright according to its EXIF
// orientation, premultiplies the alpha channel while resizing, and converts CMYK images to sRGB
// and 16-bit images to 8 bits. Options include:
//
//	"size"            SizeDown to never enlarge, SizeUp to never shrink, SizeForce to resize to
//	                  exactly width x height without keeping the aspect ratio
//	"crop"            fill width x height and crop the part chosen by an Interesting
//	"no_rotate"       true to ignore the EXIF orientation
//	"option_string"   options of the loader, such as "n=-1" to resize all the frames of an animation
//	"output_profile"  ICC profile to convert to, such as "srgb"
//
// The result is computed while the file is read, once only: use CopyMemory to save it several
// times.
func Thumbnail(filename string, width, height int, opts Options) (*Image, error) {
	args, err := thumbnailArgs(width, height, opts)
	if err != nil {
		return nil, err
	}
	args["filename"] = filename

	return callImage("thumbnail", opts, args)
}

// ThumbnailBuffer is Thumbnail for the image file contained in buf.
func ThumbnailBuffer(buf []byte, width, height int, opts Options) (*Image, error) {
	args, err := thumbnailArgs(width, height, opts)
	if err != nil {
		return nil, err
	}
	args["buffer"] = buf

	return callImage("thumbnail_buffer", opts, args)
}

// ThumbnailImage is Thumbnail for an image already loaded, which it can read several times.
// Thumbnail and ThumbnailBuffer are faster since they can shrink while loading.
func (im *Image) ThumbnailImage(width, height int, opts Options) (*Image, error) {
	args, err := thumbnailArgs(width, height, opts)
	if err != nil {
		return nil, err
	}
	args["in"] = im

	return callImage("thumbnail_image", opts, args)
}

// thumbnailArgs returns the "width" and "height" arguments of the thumbnail operations, where 0 is
// a free dimension.
func thumbnailArgs(width, height int, opts Options) (Options, error) {
	if width < 0 || height < 0 || width == 0 && height == 0 {
		return nil, fmt.Errorf("vips: thumbnail: invalid size %dx%d", width, height)
	}
	// They would fill a box of the largest dimension of libvips.
	if (width == 0 || height == 0) && (isNick(opts["size"], string(SizeForce)) ||
		opts["crop"] != nil && !isNick(opts["crop"], string(InterestingNone))) {
		return nil, fmt.Errorf("vips: thumbnail: size %dx%d: forcing the size and cropping need a width and a height", width, height)
	}

	return Options{"width": cmp.Or(width, maxCoord), "height": cmp.Or(height, maxCoord)}, nil
}

// isNick reports whether x is the string nick.
func isNick(x any, nick string) bool {
	r := reflect.ValueOf(x)

	return r.Kind() == reflect.String && r.String() == nick
}

// Resize resizes im by scale. Options include "vscale" (vertical scale, default scale) and
// "kernel". Unlike the libvips resize operation alone, it premultiplies the alpha channel while
// resizing, so that the color of transparent pixels does not darken the edges. ThumbnailImage
// resizes to a size in pixels.
//
// Resize returns an error for multi-page images (animations), whose page height it would not
// resize: use ThumbnailImage.
func (im *Image) Resize(scale float64, opts Options) (*Image, error) {
	if ph, err := im.Get("page-height"); err == nil && ph.(int) < im.Height() {
		return nil, errors.New("vips: resize: multi-page images must be resized with ThumbnailImage")
	}
	if !im.HasAlpha() {
		return callImage("resize", opts, Options{"in": im, "scale": scale})
	}

	premultiplied, err := callImage("premultiply", nil, Options{"in": im})
	if err != nil {
		return nil, err
	}
	defer premultiplied.Close()
	resized, err := callImage("resize", opts, Options{"in": premultiplied, "scale": scale})
	if err != nil {
		return nil, err
	}
	defer resized.Close()
	unpremultiplied, err := callImage("unpremultiply", nil, Options{"in": resized})
	if err != nil {
		return nil, err
	}
	defer unpremultiplied.Close()

	return unpremultiplied.Cast(im.Format(), nil)
}

// ExtractArea returns the area of im of size width x height at (left, top).
func (im *Image) ExtractArea(left, top, width, height int) (*Image, error) {
	return callImage("extract_area", nil, Options{"input": im, "left": left, "top": top, "width": width, "height": height})
}

// Smartcrop returns the most interesting area of im of size width x height. Options include
// "interesting" (default "attention").
func (im *Image) Smartcrop(width, height int, opts Options) (*Image, error) {
	return callImage("smartcrop", opts, Options{"input": im, "width": width, "height": height})
}

// Embed places im at (x, y) in an image of size width x height. Options include "extend" and
// "background".
func (im *Image) Embed(x, y, width, height int, opts Options) (*Image, error) {
	return callImage("embed", opts, Options{"in": im, "x": x, "y": y, "width": width, "height": height})
}

// Gravity places im at direction in an image of size width x height. Options include "extend"
// and "background".
func (im *Image) Gravity(direction CompassDirection, width, height int, opts Options) (*Image, error) {
	return callImage("gravity", opts, Options{"in": im, "direction": direction, "width": width, "height": height})
}

// Rot rotates im by a multiple of 90 degrees.
func (im *Image) Rot(angle Angle) (*Image, error) {
	return callImage("rot", nil, Options{"in": im, "angle": angle})
}

// Rotate rotates im by angle degrees, clockwise. Options include "background".
func (im *Image) Rotate(angle float64, opts Options) (*Image, error) {
	return callImage("rotate", opts, Options{"in": im, "angle": angle})
}

// Flip flips im in direction.
func (im *Image) Flip(direction Direction) (*Image, error) {
	return callImage("flip", nil, Options{"in": im, "direction": direction})
}

// Autorot rotates and flips im upright according to its EXIF orientation, which it removes.
func (im *Image) Autorot() (*Image, error) {
	return callImage("autorot", nil, Options{"in": im})
}

// Colourspace converts im to the color space space.
func (im *Image) Colourspace(space Interpretation, opts Options) (*Image, error) {
	return callImage("colourspace", opts, Options{"in": im, "space": space})
}

// Cast converts the pixel values of im to format, clipping them to its range.
func (im *Image) Cast(format BandFormat, opts Options) (*Image, error) {
	return callImage("cast", opts, Options{"in": im, "format": format})
}

// Copy returns a copy of im with the fields given in opts changed, such as "interpretation",
// "xres" or "yres".
func (im *Image) Copy(opts Options) (*Image, error) {
	return callImage("copy", opts, Options{"in": im})
}

// Flatten removes the alpha band of im by blending it with the "background" option (default
// black).
func (im *Image) Flatten(opts Options) (*Image, error) {
	return callImage("flatten", opts, Options{"in": im})
}

// Gaussblur blurs im with a Gaussian of standard deviation sigma.
func (im *Image) Gaussblur(sigma float64, opts Options) (*Image, error) {
	return callImage("gaussblur", opts, Options{"in": im, "sigma": sigma})
}

// Sharpen sharpens im. Options include "sigma".
func (im *Image) Sharpen(opts Options) (*Image, error) {
	return callImage("sharpen", opts, Options{"in": im})
}

// Invert inverts the pixel values of im.
func (im *Image) Invert() (*Image, error) {
	return callImage("invert", nil, Options{"in": im})
}

// Linear computes a*im + b for each band, with a and b of one element or one per band. Options
// include "uchar" to output 8-bit values.
func (im *Image) Linear(a, b []float64, opts Options) (*Image, error) {
	return callImage("linear", opts, Options{"in": im, "a": a, "b": b})
}

// ExtractBand returns the band band of im. Options include "n", the number of bands to extract.
func (im *Image) ExtractBand(band int, opts Options) (*Image, error) {
	return callImage("extract_band", opts, Options{"in": im, "band": band})
}

// Composite2 composites overlay on im with mode. Options include "x" and "y", the position of
// overlay.
func (im *Image) Composite2(overlay *Image, mode BlendMode, opts Options) (*Image, error) {
	return callImage("composite2", opts, Options{"base": im, "overlay": overlay, "mode": mode})
}

// Insert inserts sub into im at (x, y). Options include "expand" to enlarge im to contain sub.
func (im *Image) Insert(sub *Image, x, y int, opts Options) (*Image, error) {
	return callImage("insert", opts, Options{"main": im, "sub": sub, "x": x, "y": y})
}
