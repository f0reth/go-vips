package vips

import (
	"errors"
	"fmt"
	"reflect"
	"runtime"
	"slices"
	"strings"
	"sync"
	"unsafe"
)

// Options holds arguments of a libvips operation by name, such as Options{"Q": 80, "keep": "none"}
// for jpegsave. Names are the ones of the libvips documentation, where '-' and '_' are the same
// ("output_profile" or "output-profile"); a nil Options sets no argument.
//
// Values are converted to the type of the argument:
//
//	bool                          gboolean
//	integer types                 int, uint64, double, enum and flags values
//	floating-point types          double
//	string types                  string, and enum and flags nicks such as "lanczos3" or "exif|xmp"
//	*Image                        VipsImage
//	[]byte                        VipsBlob (copied)
//	[]float64, numbers            VipsArrayDouble (a number is an array of one element)
//	[]int, integers               VipsArrayInt (an integer is an array of one element)
//	[]*Image                      VipsArrayImage
//
// Outputs are named with '_' and converted to bool, int, uint64, float64, string (also for enum
// nicks), *Image, []byte, []float64 and []int.
type Options map[string]any

// Call runs the libvips operation name, such as "resize" or "jpegsave_buffer", with args, which
// holds its required and optional input arguments by name. It returns the output arguments set by
// the operation by name. The caller owns the returned images and must close them.
func Call(name string, args Options) (Options, error) {
	if dynamicErr != nil {
		return nil, dynamicErr
	}
	// The images in args must stay alive until the operation holds its own references.
	defer runtime.KeepAlive(args)

	// The messages of libvips start with the name of the operation.
	op := vips_operation_new(name)
	if op == nil {
		return nil, fmt.Errorf("vips: %s", lastError())
	}

	arguments := operationArguments(name, op)
	for k, x := range args {
		if err := setArgument(op, arguments[strings.ReplaceAll(k, "-", "_")], x); err != nil {
			g_object_unref(op)
			return nil, fmt.Errorf("vips: %s: %s: %w", name, k, err)
		}
	}

	if vips_cache_operation_buildp(&op) != 0 {
		vips_object_unref_outputs(op)
		g_object_unref(op)
		return nil, fmt.Errorf("vips: %s", lastError())
	}
	defer func() {
		vips_object_unref_outputs(op)
		g_object_unref(op)
	}()
	// Operations that succeed may leave messages in the global error buffer, such as the failed
	// attempts of a loader. Clear them so they are not reported with the next error and do not fill
	// the buffer, whose size is limited. A concurrent failure may then lose its message.
	vips_error_clear()

	outputs := Options{}
	for k, a := range arguments {
		if a.flags&argOutput == 0 || a.flags&argDeprecated != 0 || vips_object_argument_isset(op, a.name) == 0 {
			continue
		}

		var v gValue
		g_value_init(&v, a.gType)
		g_object_get_property(op, a.name, &v)
		x, err := fromValue(&v)
		g_value_unset(&v)
		if err != nil {
			closeImages(outputs)
			return nil, fmt.Errorf("vips: %s: %s: %w", name, k, err)
		}
		outputs[k] = x
	}

	return outputs, nil
}

// callImage runs the operation name with opts and args (args take precedence) and returns its
// output image "out".
func callImage(name string, opts, args Options) (*Image, error) {
	outputs, err := Call(name, withArgs(opts, args))
	if err != nil {
		return nil, err
	}

	out, ok := outputs["out"].(*Image)
	delete(outputs, "out")
	closeImages(outputs)
	if !ok {
		return nil, fmt.Errorf("vips: %s: no output image", name)
	}

	return out, nil
}

// closeImages closes the images of outputs.
func closeImages(outputs Options) {
	for _, x := range outputs {
		if im, ok := x.(*Image); ok {
			im.Close()
		}
	}
}

// argument is an argument of a libvips operation.
type argument struct {
	name  string         // GObject property name
	flags int32          // VipsArgumentFlags
	pspec unsafe.Pointer // GParamSpec
	gType uintptr        // GType of the value
}

// operations caches the arguments of the operations by name. They belong to the class of the
// operation, which libvips never frees, and looking them up by name takes a lock of GObject shared
// by all threads.
var operations sync.Map // string → map[string]argument

// operationArguments returns the arguments of the operation op named name by name, written with
// '_'.
func operationArguments(name string, op unsafe.Pointer) map[string]argument {
	if m, ok := operations.Load(name); ok {
		return m.(map[string]argument)
	}

	var names, flags unsafe.Pointer
	var n int32
	if vips_object_get_args(op, &names, &flags, &n) != 0 {
		lastError()
		return nil
	}

	// The arrays and the GParamSpecs belong to the class of op.
	m := make(map[string]argument, n)
	for i, p := range cSlice[unsafe.Pointer](names, int(n)) {
		a := argument{name: goString(p), flags: cSlice[int32](flags, int(n))[i]}
		var argumentClass, argumentInstance unsafe.Pointer
		if vips_object_get_argument(op, a.name, &a.pspec, &argumentClass, &argumentInstance) != 0 {
			lastError()
			continue
		}
		a.gType = g_param_spec_get_default_value(a.pspec).gType
		m[strings.ReplaceAll(a.name, "-", "_")] = a
	}
	operations.Store(name, m)

	return m
}

func setArgument(op unsafe.Pointer, a argument, x any) error {
	switch {
	case a.flags == 0:
		return errors.New("no such argument")
	case a.flags&argModify != 0:
		return errors.New("arguments modified in place are not supported")
	case a.flags&argInput == 0:
		return errors.New("not an input argument")
	}

	var v gValue
	if err := toValue(&v, a.gType, x); err != nil {
		return err
	}
	defer g_value_unset(&v)

	// g_object_set_property ignores invalid values with a warning.
	if g_param_value_validate(a.pspec, &v) != 0 {
		return fmt.Errorf("invalid value %v", x)
	}
	g_object_set_property(op, a.name, &v)

	return nil
}

// toValue initializes v as a GValue of type t holding x.
func toValue(v *gValue, t uintptr, x any) error {
	g_value_init(v, t)
	if err := setValue(v, t, x); err != nil {
		g_value_unset(v)
		if err == errInvalid {
			return fmt.Errorf("cannot use %T as %s", x, g_type_name(t))
		}
		return err
	}

	return nil
}

// nickDomain is the domain of the errors of vips_enum_from_nick and vips_flags_from_nick.
const nickDomain = "nick"

// nickError returns the error of vips_enum_from_nick or vips_flags_from_nick, which lists the
// valid nicks.
func nickError() error {
	return errors.New(strings.TrimPrefix(lastError(), nickDomain+": "))
}

// errInvalid is returned by setValue when x cannot be converted to the type of v.
var errInvalid = errors.New("invalid value")

func setValue(v *gValue, t uintptr, x any) error {
	r := reflect.ValueOf(x)

	switch g_type_fundamental(t) {
	case gTypeBoolean:
		if r.Kind() != reflect.Bool {
			return errInvalid
		}
		g_value_set_boolean(v, boolInt(r.Bool()))
	case gTypeInt, gTypeUint, gTypeInt64, gTypeUint64:
		i, ok := asInt(r)
		if !ok {
			return errInvalid
		}
		switch g_type_fundamental(t) {
		case gTypeInt:
			g_value_set_int(v, int32(i))
		case gTypeUint:
			g_value_set_uint(v, uint32(i))
		case gTypeInt64:
			g_value_set_int64(v, i)
		default:
			g_value_set_uint64(v, uint64(i))
		}
	case gTypeFloat, gTypeDouble:
		f, ok := asFloat(r)
		if !ok {
			return errInvalid
		}
		if g_type_fundamental(t) == gTypeFloat {
			g_value_set_float(v, float32(f))
		} else {
			g_value_set_double(v, f)
		}
	case gTypeEnum:
		i, ok := asInt(r)
		if r.Kind() == reflect.String {
			if i = int64(vips_enum_from_nick(nickDomain, t, r.String())); i < 0 {
				return nickError()
			}
		} else if !ok {
			return errInvalid
		}
		g_value_set_enum(v, int32(i))
	case gTypeFlags:
		i, ok := asInt(r)
		if r.Kind() == reflect.String {
			if i = int64(vips_flags_from_nick(nickDomain, t, r.String())); i < 0 {
				return nickError()
			}
		} else if !ok {
			return errInvalid
		}
		g_value_set_flags(v, uint32(i))
	case gTypeString:
		if r.Kind() != reflect.String {
			return errInvalid
		}
		g_value_set_string(v, r.String())
	case gTypeObject:
		im, ok := x.(*Image)
		if !ok || g_type_is_a(t, typeImage) == 0 {
			return errInvalid
		}
		if im == nil || im.p == nil {
			return errors.New("nil or closed image")
		}
		g_value_set_object(v, im.p)
	case gTypeBoxed:
		area, err := newArea(t, x, r)
		if err != nil {
			return err
		}
		g_value_set_boxed(v, area)
		vips_area_unref(area)
	default:
		return errInvalid
	}

	return nil
}

// newArea returns a new VipsArea of the boxed type t holding x, or errInvalid if x cannot be
// converted.
func newArea(t uintptr, x any, r reflect.Value) (unsafe.Pointer, error) {
	switch t {
	case typeBlob:
		if b, ok := x.([]byte); ok {
			return vips_blob_copy(b, uint64(len(b))), nil
		}
	case typeArrayDouble:
		if a, ok := x.([]float64); ok {
			return vips_array_double_new(a, int32(len(a))), nil
		}
		if f, ok := asFloat(r); ok {
			return vips_array_double_new([]float64{f}, 1), nil
		}
	case typeArrayInt:
		if a, ok := x.([]int); ok {
			a32 := make([]int32, len(a))
			for i, n := range a {
				a32[i] = int32(n)
			}
			return vips_array_int_new(a32, int32(len(a32))), nil
		}
		if i, ok := asInt(r); ok {
			return vips_array_int_new([]int32{int32(i)}, 1), nil
		}
	case typeArrayImage:
		if a, ok := x.([]*Image); ok {
			ptrs := make([]unsafe.Pointer, len(a))
			for i, im := range a {
				if im == nil || im.p == nil {
					return nil, errors.New("nil or closed image")
				}
				ptrs[i] = im.p
			}
			// The array holds its own reference to each image.
			return vips_array_image_new(ptrs, int32(len(ptrs))), nil
		}
	}

	return nil, errInvalid
}

// fromValue converts the GValue v to a Go value. Images get their own reference.
func fromValue(v *gValue) (any, error) {
	t := v.gType

	switch g_type_fundamental(t) {
	case gTypeBoolean:
		return g_value_get_boolean(v) != 0, nil
	case gTypeInt:
		return int(g_value_get_int(v)), nil
	case gTypeUint:
		return int(g_value_get_uint(v)), nil
	case gTypeInt64:
		return int(g_value_get_int64(v)), nil
	case gTypeUint64:
		return g_value_get_uint64(v), nil
	case gTypeFloat:
		return float64(g_value_get_float(v)), nil
	case gTypeDouble:
		return g_value_get_double(v), nil
	case gTypeEnum:
		return vips_enum_nick(t, g_value_get_enum(v)), nil
	case gTypeFlags:
		return int(g_value_get_flags(v)), nil
	case gTypeString:
		return g_value_get_string(v), nil
	case gTypeObject:
		if g_type_is_a(t, typeImage) != 0 {
			if p := g_value_get_object(v); p != nil {
				return newImage(g_object_ref(p)), nil
			}
			return (*Image)(nil), nil
		}
	case gTypeBoxed:
		switch t {
		case typeBlob:
			var n uint64
			p := vips_value_get_blob(v, &n)
			return cBytes(p, int(n)), nil
		case typeRefString:
			var n uint64
			p := vips_value_get_ref_string(v, &n)
			return string(cBytes(p, int(n))), nil
		case typeArrayDouble:
			var n int32
			p := vips_value_get_array_double(v, &n)
			return slices.Clone(cSlice[float64](p, int(n))), nil
		case typeArrayInt:
			var n int32
			p := vips_value_get_array_int(v, &n)
			a := make([]int, n)
			for i, x := range cSlice[int32](p, int(n)) {
				a[i] = int(x)
			}
			return a, nil
		}
	}

	return nil, fmt.Errorf("unsupported type %s", g_type_name(t))
}

// cSlice returns the C array p of n elements as a slice, which must not outlive the array.
func cSlice[T any](p unsafe.Pointer, n int) []T {
	if p == nil || n <= 0 {
		return nil
	}

	return unsafe.Slice((*T)(p), n)
}

// cBytes returns a copy of the n bytes at p.
func cBytes(p unsafe.Pointer, n int) []byte {
	return slices.Clone(cSlice[byte](p, n))
}

func asInt(r reflect.Value) (int64, bool) {
	switch r.Kind() {
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return r.Int(), true
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uintptr:
		return int64(r.Uint()), true
	}

	return 0, false
}

func asFloat(r reflect.Value) (float64, bool) {
	if r.Kind() == reflect.Float32 || r.Kind() == reflect.Float64 {
		return r.Float(), true
	}
	i, ok := asInt(r)

	return float64(i), ok
}

func boolInt(b bool) int32 {
	if b {
		return 1
	}

	return 0
}
