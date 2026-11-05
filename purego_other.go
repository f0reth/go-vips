//go:build !unix && !darwin && !windows

package vips

import "errors"

func register() error {
	return errors.New("platform not supported")
}
