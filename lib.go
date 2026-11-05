package vips

import (
	"errors"
	"fmt"
	"strings"
	"unsafe"
)

// minVersion is the minimum supported libvips version (major*10000 + minor*100 + micro).
const minVersion = 81200

var (
	libvips    uintptr
	libgobject uintptr
	libglib    uintptr
	libVersion int   // major*10000 + minor*100 + micro
	dynamicErr error // why the library cannot be used, if it cannot
)

// GTypes of libvips, set by init.
var (
	typeImage          uintptr
	typeBlob           uintptr
	typeRefString      uintptr
	typeArrayDouble    uintptr
	typeArrayInt       uintptr
	typeArrayImage     uintptr
	typeBandFormat     uintptr
	typeInterpretation uintptr
)

func init() {
	if err := load(); err != nil {
		dynamicErr = fmt.Errorf("vips: %w", err)
	}
}

func load() error {
	if unsafe.Sizeof(uintptr(0)) != 8 {
		// gValue only mirrors the GValue of 64-bit platforms.
		return errors.New("only 64-bit platforms are supported")
	}
	if err := register(); err != nil {
		return err
	}

	major, minor, micro := vips_version(0), vips_version(1), vips_version(2)
	libVersion = int(major)*10000 + int(minor)*100 + int(micro)
	if libVersion < minVersion {
		return fmt.Errorf("minimum required libvips version is 8.12.0, got %d.%d.%d", major, minor, micro)
	}

	// Package initialization runs on the main thread, as vips_init requires.
	if vips_init("go-vips") != 0 {
		return fmt.Errorf("vips_init: %s", lastError())
	}

	typeImage = vips_image_get_type()
	typeBlob = vips_blob_get_type()
	typeRefString = vips_ref_string_get_type()
	typeArrayDouble = vips_array_double_get_type()
	typeArrayInt = vips_array_int_get_type()
	typeArrayImage = vips_array_image_get_type()
	typeBandFormat = vips_band_format_get_type()
	typeInterpretation = vips_interpretation_get_type()

	return nil
}

// lastError returns and clears the messages of the libvips error buffer.
func lastError() string {
	p := vips_error_buffer_copy()
	defer g_free(p)

	if msg := strings.TrimSpace(goString(p)); msg != "" {
		return msg
	}

	return "unknown error"
}

// goString returns a copy of the NUL-terminated C string p.
func goString(p unsafe.Pointer) string {
	if p == nil {
		return ""
	}

	n := 0
	for *(*byte)(unsafe.Add(p, n)) != 0 {
		n++
	}

	return string(unsafe.Slice((*byte)(p), n))
}

// gValue mirrors GValue on 64-bit platforms: a GType followed by a union of two 8-byte members.
// Only g_type is read directly, the data is accessed through the g_value_* functions.
type gValue struct {
	gType uintptr
	data  [2]uint64
}

// Fundamental GTypes (G_TYPE_MAKE_FUNDAMENTAL(n) is n << 2).
const (
	gTypeBoolean = 5 << 2
	gTypeInt     = 6 << 2
	gTypeUint    = 7 << 2
	gTypeInt64   = 10 << 2
	gTypeUint64  = 11 << 2
	gTypeEnum    = 12 << 2
	gTypeFlags   = 13 << 2
	gTypeFloat   = 14 << 2
	gTypeDouble  = 15 << 2
	gTypeString  = 16 << 2
	gTypeBoxed   = 18 << 2
	gTypeObject  = 20 << 2
)

// VipsArgumentFlags.
const (
	argInput      = 16
	argOutput     = 32
	argDeprecated = 64
	argModify     = 128
)

// libvips functions, registered by init. libvips objects are opaque pointers.
var (
	vips_init                       func(argv0 string) int32
	vips_version                    func(flag int32) int32
	vips_error_buffer_copy          func() unsafe.Pointer
	vips_error_clear                func()
	vips_concurrency_set            func(n int32)
	vips_cache_set_max              func(n int32)
	vips_cache_set_max_mem          func(bytes uint64)
	vips_cache_set_max_files        func(n int32)
	vips_operation_new              func(name string) unsafe.Pointer
	vips_cache_operation_buildp     func(operation *unsafe.Pointer) int32
	vips_object_unref_outputs       func(object unsafe.Pointer)
	vips_object_get_args            func(object unsafe.Pointer, names, flags *unsafe.Pointer, n *int32) int32
	vips_object_get_argument        func(object unsafe.Pointer, name string, pspec, argumentClass, argumentInstance *unsafe.Pointer) int32
	vips_object_argument_isset      func(object unsafe.Pointer, name string) int32
	vips_foreign_find_load          func(filename string) string
	vips_foreign_find_load_buffer   func(data []byte, size uint64) string
	vips_foreign_find_save          func(filename string) string
	vips_foreign_find_save_buffer   func(suffix string) string
	vips_image_new_from_memory_copy func(data []byte, size uint64, width, height, bands, format int32) unsafe.Pointer
	vips_image_write_to_memory      func(image unsafe.Pointer, size *uint64) unsafe.Pointer
	vips_image_copy_memory          func(image unsafe.Pointer) unsafe.Pointer
	vips_image_get_width            func(image unsafe.Pointer) int32
	vips_image_get_height           func(image unsafe.Pointer) int32
	vips_image_get_bands            func(image unsafe.Pointer) int32
	vips_image_get_format           func(image unsafe.Pointer) int32
	vips_image_get_interpretation   func(image unsafe.Pointer) int32
	vips_image_hasalpha             func(image unsafe.Pointer) int32
	vips_image_get                  func(image unsafe.Pointer, name string, value *gValue) int32
	vips_image_get_fields           func(image unsafe.Pointer) unsafe.Pointer
	vips_enum_nick                  func(enum uintptr, value int32) string
	vips_enum_from_nick             func(domain string, enum uintptr, nick string) int32
	vips_flags_from_nick            func(domain string, flags uintptr, nick string) int32
	vips_image_get_type             func() uintptr
	vips_blob_get_type              func() uintptr
	vips_ref_string_get_type        func() uintptr
	vips_array_double_get_type      func() uintptr
	vips_array_int_get_type         func() uintptr
	vips_array_image_get_type       func() uintptr
	vips_band_format_get_type       func() uintptr
	vips_interpretation_get_type    func() uintptr
	vips_blob_copy                  func(data []byte, length uint64) unsafe.Pointer
	vips_array_double_new           func(array []float64, n int32) unsafe.Pointer
	vips_array_int_new              func(array []int32, n int32) unsafe.Pointer
	vips_array_image_new            func(array []unsafe.Pointer, n int32) unsafe.Pointer
	vips_area_unref                 func(area unsafe.Pointer)
	vips_value_get_blob             func(value *gValue, length *uint64) unsafe.Pointer
	vips_value_get_ref_string       func(value *gValue, length *uint64) unsafe.Pointer
	vips_value_get_array_double     func(value *gValue, n *int32) unsafe.Pointer
	vips_value_get_array_int        func(value *gValue, n *int32) unsafe.Pointer
)

// GObject and GLib functions, registered by init.
var (
	g_object_ref                   func(object unsafe.Pointer) unsafe.Pointer
	g_object_unref                 func(object unsafe.Pointer)
	g_object_set_property          func(object unsafe.Pointer, name string, value *gValue)
	g_object_get_property          func(object unsafe.Pointer, name string, value *gValue)
	g_param_spec_get_default_value func(pspec unsafe.Pointer) *gValue
	g_param_value_validate         func(pspec unsafe.Pointer, value *gValue) int32
	g_type_fundamental             func(t uintptr) uintptr
	g_type_is_a                    func(t, isA uintptr) int32
	g_type_name                    func(t uintptr) string
	g_value_init                   func(value *gValue, t uintptr) *gValue
	g_value_unset                  func(value *gValue)
	g_value_set_boolean            func(value *gValue, v int32)
	g_value_set_int                func(value *gValue, v int32)
	g_value_set_uint               func(value *gValue, v uint32)
	g_value_set_int64              func(value *gValue, v int64)
	g_value_set_uint64             func(value *gValue, v uint64)
	g_value_set_float              func(value *gValue, v float32)
	g_value_set_double             func(value *gValue, v float64)
	g_value_set_string             func(value *gValue, v string)
	g_value_set_enum               func(value *gValue, v int32)
	g_value_set_flags              func(value *gValue, v uint32)
	g_value_set_object             func(value *gValue, v unsafe.Pointer)
	g_value_set_boxed              func(value *gValue, v unsafe.Pointer)
	g_value_get_boolean            func(value *gValue) int32
	g_value_get_int                func(value *gValue) int32
	g_value_get_uint               func(value *gValue) uint32
	g_value_get_int64              func(value *gValue) int64
	g_value_get_uint64             func(value *gValue) uint64
	g_value_get_float              func(value *gValue) float32
	g_value_get_double             func(value *gValue) float64
	g_value_get_string             func(value *gValue) string
	g_value_get_enum               func(value *gValue) int32
	g_value_get_flags              func(value *gValue) uint32
	g_value_get_object             func(value *gValue) unsafe.Pointer
	g_free                         func(p unsafe.Pointer)
	g_strfreev                     func(p unsafe.Pointer)
)
