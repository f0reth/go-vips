//go:build windows

package vips

import (
	"fmt"
	"syscall"
)

const (
	libvipsName    = "libvips-42.dll"
	libgobjectName = "libgobject-2.0-0.dll"
	libglibName    = "libglib-2.0-0.dll"
)

func loadLibrary(name string) (uintptr, error) {
	handle, err := syscall.LoadLibrary(name)
	if err != nil {
		return 0, fmt.Errorf("cannot load library %s: %w", name, err)
	}

	return uintptr(handle), nil
}
