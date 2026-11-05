// Package vips provides image processing with the libvips shared library, used via purego
// (no cgo).
//
// Images are immutable: operations return new images, which libvips computes lazily when they
// are saved or converted. The most common operations are methods of Image named after the libvips
// operation; Call runs any operation by name.
package vips

import "fmt"

// Dynamic returns the error, if any, that makes the libvips shared library unusable.
func Dynamic() error {
	return dynamicErr
}

// Version returns the version of the libvips shared library, such as "8.18.7", or "" if it
// cannot be used.
func Version() string {
	if dynamicErr != nil {
		return ""
	}

	return fmt.Sprintf("%d.%d.%d", libVersion/10000, libVersion/100%100, libVersion%100)
}

// SetConcurrency sets the number of worker threads libvips uses to compute an image. 0 restores
// the default: the VIPS_CONCURRENCY environment variable, or the number of CPUs.
func SetConcurrency(n int) {
	if dynamicErr == nil {
		vips_concurrency_set(int32(n))
	}
}

// SetCacheMax sets the maximum number of operations kept in the libvips operation cache
// (default 100). 0 disables the cache.
func SetCacheMax(n int) {
	if dynamicErr == nil {
		vips_cache_set_max(int32(n))
	}
}

// SetCacheMaxMem sets the maximum memory in bytes held by the operations of the libvips operation
// cache (default 100 MiB).
func SetCacheMaxMem(bytes int) {
	if dynamicErr == nil {
		vips_cache_set_max_mem(uint64(bytes))
	}
}

// SetCacheMaxFiles sets the maximum number of files held open by the operations of the libvips
// operation cache (default 100).
func SetCacheMaxFiles(n int) {
	if dynamicErr == nil {
		vips_cache_set_max_files(int32(n))
	}
}
