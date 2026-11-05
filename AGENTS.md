# AGENTS.md

## Development policy

This is an internal tool that is still under development. Do not keep backward compatibility or migrate anything. Always rewrite the code into its ideal, KISS form.

- **Public API**: Change or remove exported identifiers (image types, load/save functions, operations, option structs, and so on) whenever a better design calls for it. Do not leave deprecated aliases, wrappers, or old-name shims behind; update every caller, test, and benchmark in the same change.
- **Options**: Rename, merge, or drop fields in option structs freely. Do not keep unused fields or translate old field values for compatibility.
- **No dead code**: Remove code that has become unused instead of commenting it out or keeping it "just in case".
- **Tests and docs follow the code**: Rewrite tests, benchmarks, `testdata`, and `README.md` to match the new design rather than preserving old expectations.
- **libvips versions are not legacy**: Runtime checks on the version reported by `vips_version` (the minimum supported version and feature gates for newer releases) support the shared libraries users actually have installed. Keep them unless the minimum supported libvips version is raised on purpose; when it is, delete the obsolete branches.
- **Struct layouts**: Any purego mirror of a C struct (libvips, GLib, or GObject) must match the C layout and be checked by a layout test. Prefer treating libvips objects as opaque pointers so no mirror is needed. Simplicity never justifies breaking a layout.
- **Ownership**: Every `VipsImage` / `GObject` reference obtained from libvips must be released exactly once (`g_object_unref`), and memory libvips allocates for the caller (for example buffers from `vips_*_save_buffer`) must be freed with `g_free`. Simplicity never justifies leaking or double-freeing.

## Environment

- The shared library is loaded at runtime with [purego](https://github.com/ebitengine/purego); no cgo. Library names: `libvips-42.dll` (Windows), `libvips.so.42` (Linux), `libvips.42.dylib` (macOS).
- The Windows development machine has libvips 8.18.7 installed via vcpkg (`x64-windows`), with `libvips-42.dll` on `PATH`.
