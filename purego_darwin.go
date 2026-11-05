//go:build darwin

package vips

import (
	"fmt"

	"github.com/ebitengine/purego"
)

const (
	libvipsName    = "libvips.42.dylib"
	libgobjectName = "libgobject-2.0.0.dylib"
	libglibName    = "libglib-2.0.0.dylib"
)

func loadLibrary(name string) (uintptr, error) {
	handle, err := purego.Dlopen(name, purego.RTLD_NOW|purego.RTLD_GLOBAL)
	if err != nil {
		return 0, fmt.Errorf("cannot load library %s: %w", name, err)
	}

	return handle, nil
}
