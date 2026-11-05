//go:build amd64 || arm64

package vips

import (
	"testing"
	"unsafe"
)

// TestStructLayout checks the Go mirror of GValue against the layout of the C struct on 64-bit
// platforms: a GType (gsize) followed by a union of two 8-byte members (gobject/gvalue.h).
func TestStructLayout(t *testing.T) {
	var v gValue

	tests := []struct {
		name      string
		got, want uintptr
	}{
		{"GValue.g_type", unsafe.Offsetof(v.gType), 0},
		{"GValue.data", unsafe.Offsetof(v.data), 8},
		{"sizeof(GValue)", unsafe.Sizeof(v), 24},
	}

	for _, tt := range tests {
		if tt.got != tt.want {
			t.Errorf("%s: got %d, want %d", tt.name, tt.got, tt.want)
		}
	}
}
