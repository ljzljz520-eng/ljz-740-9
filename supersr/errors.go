package supersr

import (
	"errors"
	"fmt"
)

// Sentinel errors. Use errors.Is to match them; concrete error types carry
// additional context (paths, symbol names, native messages).
var (
	// ErrLibraryNotFound: the shared library file is missing or cannot be
	// loaded by the dynamic linker.
	ErrLibraryNotFound = errors.New("supersr: native library not found or unloadable")

	// ErrFunctionMissing: the library loaded fine but does not export one of
	// the required sr_* functions (wrong library, or ABI version mismatch).
	ErrFunctionMissing = errors.New("supersr: required function missing in native library")

	// ErrModelNotFound: the model file does not exist on disk.
	ErrModelNotFound = errors.New("supersr: model file not found")

	// ErrImageCorrupt: the input bytes could not be decoded as an image.
	ErrImageCorrupt = errors.New("supersr: image data corrupt or unsupported")

	// ErrInvalidScale: scale factor outside the supported range [1, 16].
	ErrInvalidScale = errors.New("supersr: invalid scale factor")

	// ErrNative: a native library call returned a failure code.
	ErrNative = errors.New("supersr: native call failed")

	// ErrClosed: operation attempted on a closed Library or Model.
	ErrClosed = errors.New("supersr: handle already closed")
)

// LoadError reports a dlopen failure for Path.
type LoadError struct {
	Path string
	Msg  string
}

func (e *LoadError) Error() string {
	return fmt.Sprintf("supersr: cannot load native library %q: %s", e.Path, e.Msg)
}

func (e *LoadError) Unwrap() error { return ErrLibraryNotFound }

// SymbolError reports that the native library does not export a required
// function.
type SymbolError struct {
	Name string
}

func (e *SymbolError) Error() string {
	return fmt.Sprintf("supersr: required symbol %q not exported by the native library", e.Name)
}

func (e *SymbolError) Unwrap() error { return ErrFunctionMissing }

// ModelError reports that a model file is missing/unreadable on disk.
type ModelError struct {
	Path string
	Err  error
}

func (e *ModelError) Error() string {
	return fmt.Sprintf("supersr: cannot access model %q: %v", e.Path, e.Err)
}

// Unwrap exposes both the sentinel and the underlying OS error.
func (e *ModelError) Unwrap() []error { return []error{ErrModelNotFound, e.Err} }

// NativeError reports a failure returned by the native library itself.
type NativeError struct {
	Op  string // native entry point, e.g. "sr_upscale_rgba"
	Msg string // message provided by the library
}

func (e *NativeError) Error() string {
	if e.Msg == "" {
		return fmt.Sprintf("supersr: %s failed", e.Op)
	}
	return fmt.Sprintf("supersr: %s failed: %s", e.Op, e.Msg)
}

func (e *NativeError) Unwrap() error { return ErrNative }
