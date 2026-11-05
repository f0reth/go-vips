//go:build unix && !darwin

package vips

import (
	"debug/elf"
	"fmt"
	"os"
	"runtime"

	"github.com/ebitengine/purego"
)

const (
	libvipsName    = "libvips.so.42"
	libgobjectName = "libgobject-2.0.so.0"
	libglibName    = "libglib-2.0.so.0"
)

func loadLibrary(name string) (uintptr, error) {
	if runtime.GOOS == "linux" {
		if err := checkDynamicBinary(); err != nil {
			return 0, err
		}
	}

	handle, err := purego.Dlopen(name, purego.RTLD_NOW|purego.RTLD_GLOBAL)
	if err != nil {
		return 0, fmt.Errorf("cannot load library %s: %w", name, err)
	}

	return handle, nil
}

// checkDynamicBinary returns an error if the executable is statically linked, which cannot load
// shared libraries.
func checkDynamicBinary() error {
	fileName, err := os.Executable()
	if err != nil {
		return fmt.Errorf("cannot check the executable: %w", err)
	}

	f, err := elf.Open(fileName)
	if err != nil {
		return fmt.Errorf("cannot check the executable: %w", err)
	}
	defer f.Close()

	if _, err := f.DynamicSymbols(); err != nil {
		return fmt.Errorf("not a dynamic binary: %w", err)
	}

	return nil
}
