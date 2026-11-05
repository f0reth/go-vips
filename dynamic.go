//go:build unix || darwin || windows

package vips

import (
	"fmt"

	"github.com/ebitengine/purego"
)

// register loads libvips, GObject and GLib and registers their functions.
func register() (err error) {
	// purego.RegisterLibFunc panics when a function is missing.
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("%v", r)
		}
	}()

	if libvips, err = loadLibrary(libvipsName); err != nil {
		return err
	}
	// GetProcAddress does not search the dependencies of a DLL, so GObject and GLib are loaded on
	// their own (they are already loaded as dependencies of libvips).
	if libgobject, err = loadLibrary(libgobjectName); err != nil {
		return err
	}
	if libglib, err = loadLibrary(libglibName); err != nil {
		return err
	}

	for _, f := range []struct {
		lib  uintptr
		fptr any
		name string
	}{
		{libvips, &vips_init, "vips_init"},
		{libvips, &vips_version, "vips_version"},
		{libvips, &vips_error_buffer_copy, "vips_error_buffer_copy"},
		{libvips, &vips_error_clear, "vips_error_clear"},
		{libvips, &vips_concurrency_set, "vips_concurrency_set"},
		{libvips, &vips_cache_set_max, "vips_cache_set_max"},
		{libvips, &vips_cache_set_max_mem, "vips_cache_set_max_mem"},
		{libvips, &vips_cache_set_max_files, "vips_cache_set_max_files"},
		{libvips, &vips_operation_new, "vips_operation_new"},
		{libvips, &vips_cache_operation_buildp, "vips_cache_operation_buildp"},
		{libvips, &vips_object_unref_outputs, "vips_object_unref_outputs"},
		{libvips, &vips_object_get_args, "vips_object_get_args"},
		{libvips, &vips_object_get_argument, "vips_object_get_argument"},
		{libvips, &vips_object_argument_isset, "vips_object_argument_isset"},
		{libvips, &vips_foreign_find_load, "vips_foreign_find_load"},
		{libvips, &vips_foreign_find_load_buffer, "vips_foreign_find_load_buffer"},
		{libvips, &vips_foreign_find_save, "vips_foreign_find_save"},
		{libvips, &vips_foreign_find_save_buffer, "vips_foreign_find_save_buffer"},
		{libvips, &vips_image_new_from_memory_copy, "vips_image_new_from_memory_copy"},
		{libvips, &vips_image_write_to_memory, "vips_image_write_to_memory"},
		{libvips, &vips_image_copy_memory, "vips_image_copy_memory"},
		{libvips, &vips_image_get_width, "vips_image_get_width"},
		{libvips, &vips_image_get_height, "vips_image_get_height"},
		{libvips, &vips_image_get_bands, "vips_image_get_bands"},
		{libvips, &vips_image_get_format, "vips_image_get_format"},
		{libvips, &vips_image_get_interpretation, "vips_image_get_interpretation"},
		{libvips, &vips_image_hasalpha, "vips_image_hasalpha"},
		{libvips, &vips_image_get, "vips_image_get"},
		{libvips, &vips_image_get_fields, "vips_image_get_fields"},
		{libvips, &vips_enum_nick, "vips_enum_nick"},
		{libvips, &vips_enum_from_nick, "vips_enum_from_nick"},
		{libvips, &vips_flags_from_nick, "vips_flags_from_nick"},
		{libvips, &vips_image_get_type, "vips_image_get_type"},
		{libvips, &vips_blob_get_type, "vips_blob_get_type"},
		{libvips, &vips_ref_string_get_type, "vips_ref_string_get_type"},
		{libvips, &vips_array_double_get_type, "vips_array_double_get_type"},
		{libvips, &vips_array_int_get_type, "vips_array_int_get_type"},
		{libvips, &vips_array_image_get_type, "vips_array_image_get_type"},
		{libvips, &vips_band_format_get_type, "vips_band_format_get_type"},
		{libvips, &vips_interpretation_get_type, "vips_interpretation_get_type"},
		{libvips, &vips_blob_copy, "vips_blob_copy"},
		{libvips, &vips_array_double_new, "vips_array_double_new"},
		{libvips, &vips_array_int_new, "vips_array_int_new"},
		{libvips, &vips_array_image_new, "vips_array_image_new"},
		{libvips, &vips_area_unref, "vips_area_unref"},
		{libvips, &vips_value_get_blob, "vips_value_get_blob"},
		{libvips, &vips_value_get_ref_string, "vips_value_get_ref_string"},
		{libvips, &vips_value_get_array_double, "vips_value_get_array_double"},
		{libvips, &vips_value_get_array_int, "vips_value_get_array_int"},

		{libgobject, &g_object_ref, "g_object_ref"},
		{libgobject, &g_object_unref, "g_object_unref"},
		{libgobject, &g_object_set_property, "g_object_set_property"},
		{libgobject, &g_object_get_property, "g_object_get_property"},
		{libgobject, &g_param_spec_get_default_value, "g_param_spec_get_default_value"},
		{libgobject, &g_param_value_validate, "g_param_value_validate"},
		{libgobject, &g_type_fundamental, "g_type_fundamental"},
		{libgobject, &g_type_is_a, "g_type_is_a"},
		{libgobject, &g_type_name, "g_type_name"},
		{libgobject, &g_value_init, "g_value_init"},
		{libgobject, &g_value_unset, "g_value_unset"},
		{libgobject, &g_value_set_boolean, "g_value_set_boolean"},
		{libgobject, &g_value_set_int, "g_value_set_int"},
		{libgobject, &g_value_set_uint, "g_value_set_uint"},
		{libgobject, &g_value_set_int64, "g_value_set_int64"},
		{libgobject, &g_value_set_uint64, "g_value_set_uint64"},
		{libgobject, &g_value_set_float, "g_value_set_float"},
		{libgobject, &g_value_set_double, "g_value_set_double"},
		{libgobject, &g_value_set_string, "g_value_set_string"},
		{libgobject, &g_value_set_enum, "g_value_set_enum"},
		{libgobject, &g_value_set_flags, "g_value_set_flags"},
		{libgobject, &g_value_set_object, "g_value_set_object"},
		{libgobject, &g_value_set_boxed, "g_value_set_boxed"},
		{libgobject, &g_value_get_boolean, "g_value_get_boolean"},
		{libgobject, &g_value_get_int, "g_value_get_int"},
		{libgobject, &g_value_get_uint, "g_value_get_uint"},
		{libgobject, &g_value_get_int64, "g_value_get_int64"},
		{libgobject, &g_value_get_uint64, "g_value_get_uint64"},
		{libgobject, &g_value_get_float, "g_value_get_float"},
		{libgobject, &g_value_get_double, "g_value_get_double"},
		{libgobject, &g_value_get_string, "g_value_get_string"},
		{libgobject, &g_value_get_enum, "g_value_get_enum"},
		{libgobject, &g_value_get_flags, "g_value_get_flags"},
		{libgobject, &g_value_get_object, "g_value_get_object"},

		{libglib, &g_free, "g_free"},
		{libglib, &g_strfreev, "g_strfreev"},
	} {
		purego.RegisterLibFunc(f.fptr, f.lib, f.name)
	}

	return nil
}
