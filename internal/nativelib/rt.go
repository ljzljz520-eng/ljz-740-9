//go:build cgo

// Package nativelib loads and wraps the native gosr super-resolution
// backend through its stable C ABI. The backend is resolved at runtime
// (dlopen), so the package builds with only a C compiler and every kind of
// backend failure (missing file, missing symbol, bad ABI) is an ordinary
// error.
package nativelib

/*
#cgo linux LDFLAGS: -ldl
#cgo freebsd LDFLAGS: -ldl

#include <stdlib.h>
#include <string.h>
#include "gosr.h"
#include "gosr_rt.h"
*/
import "C"

import (
	"errors"
	"fmt"
	"sync"
	"unsafe"
)

// Status codes mirrored from gosr.h.
const (
	StatusOK       = C.GOSR_OK
	StatusInvalid  = C.GOSR_ERR_INVALID
	StatusModel    = C.GOSR_ERR_MODEL
	StatusBadModel = C.GOSR_ERR_BAD_MODEL
	StatusBadImage = C.GOSR_ERR_BAD_IMAGE
	StatusNoFunc   = C.GOSR_ERR_NO_FUNC
	StatusInternal = C.GOSR_ERR_INTERNAL
	ABIExpectation = C.GOSR_ABI_VERSION
	CapCPU         = C.GOSR_CAP_CPU
	CapCUDA        = C.GOSR_CAP_CUDA
	errBufCap      = 512
)

var (
	// ErrLibraryNotFound means no backend shared library could be located.
	ErrLibraryNotFound = errors.New("nativelib: native backend library not found")
	// ErrFunctionMissing means the backend lacks a required ABI symbol.
	ErrFunctionMissing = errors.New("nativelib: required function missing in backend")
	// ErrABIMismatch means backend and bindings speak different ABI versions.
	ErrABIMismatch = errors.New("nativelib: ABI version mismatch")

	mu  sync.Mutex
	lib *Library
)

// Library is one loaded native backend. Only one may be open per process.
type Library struct {
	hasSupports bool
	closed      bool
}

// Open loads the shared library at path and binds all mandatory symbols.
func Open(path string) (*Library, error) {
	mu.Lock()
	defer mu.Unlock()
	if lib != nil {
		return nil, errors.New("nativelib: a backend library is already loaded")
	}

	cpath := C.CString(path)
	defer C.free(unsafe.Pointer(cpath))

	dl := C.gosr_rt_open(cpath)
	if dl == nil {
		return nil, fmt.Errorf("%w: %s: %s", ErrLibraryNotFound, path, dlError())
	}

	var errbuf [errBufCap]C.char
	rc := C.gosr_rt_bind(dl, &errbuf[0], C.size_t(len(errbuf)))
	if rc != StatusOK {
		C.gosr_rt_close(dl)
		msg := C.GoString(&errbuf[0])
		if rc == StatusNoFunc {
			return nil, fmt.Errorf("%w (%s): %s", ErrFunctionMissing, path, msg)
		}
		return nil, fmt.Errorf("nativelib: bind failed: %s", msg)
	}

	abi := int(C.gosr_call_abi_version())
	if abi != ABIExpectation {
		C.gosr_rt_close(dl)
		return nil, fmt.Errorf("%w: backend=%d bindings=%d (%s)",
			ErrABIMismatch, abi, ABIExpectation, path)
	}

	l := &Library{hasSupports: C.gosr_rt_has_supports() != 0}
	lib = l
	return l, nil
}

// Close unloads the backend. It is safe to call multiple times.
func (l *Library) Close() error {
	mu.Lock()
	defer mu.Unlock()
	if l != lib {
		return errors.New("nativelib: library is not the active backend")
	}
	if l.closed {
		return nil
	}
	l.closed = true
	lib = nil
	// Close the bound dlopen handle via C (avoids uintptr/pointer casts in Go).
	C.gosr_rt_close_bound()
	return nil
}

// HasFunction reports whether the optional gosr_supports symbol is present.
func (l *Library) HasFunction(name string) bool {
	switch name {
	case "gosr_supports":
		return l.hasSupports
	default:
		return false
	}
}

// SupportsCPU / SupportsCUDA query the optional capability entry point.
func (l *Library) SupportsCPU() bool {
	return C.gosr_call_supports(CapCPU) != 0
}
func (l *Library) SupportsCUDA() bool {
	return C.gosr_call_supports(CapCUDA) != 0
}

// Handle is an opaque native super-resolution session.
type Handle = *C.gosr_handle

// Create allocates a native session.
func (l *Library) Create(algorithm string, scale int) (Handle, error) {
	var errbuf [errBufCap]C.char
	ca := C.CString(algorithm)
	defer C.free(unsafe.Pointer(ca))

	h := C.gosr_call_create(ca, C.int(scale), &errbuf[0], C.size_t(len(errbuf)))
	if h == nil {
		return nil, fmt.Errorf("nativelib: create(%s,%d): %s",
			algorithm, scale, C.GoString(&errbuf[0]))
	}
	return h, nil
}

// Destroy releases a native session.
func (l *Library) Destroy(h Handle) {
	if h == nil {
		return
	}
	C.gosr_call_destroy(h)
}

// LoadModel loads model weights via the backend.
func (l *Library) LoadModel(h Handle, modelPath string) error {
	var errbuf [errBufCap]C.char
	cp := C.CString(modelPath)
	defer C.free(unsafe.Pointer(cp))

	rc := C.gosr_call_load_model(h, cp, &errbuf[0], C.size_t(len(errbuf)))
	return mapStatus(rc, &errbuf)
}

// Result holds PNG bytes produced by the backend.
type Result struct {
	PNG []byte
}

// Upsample runs super-resolution over encoded image bytes and returns PNG.
func (l *Library) Upsample(h Handle, image []byte) ([]byte, error) {
	if len(image) == 0 {
		return nil, errors.New("nativelib: empty input image")
	}
	var errbuf [errBufCap]C.char
	var out *C.uint8_t
	var outLen C.size_t

	in := (*C.uint8_t)(unsafe.Pointer(&image[0]))
	rc := C.gosr_call_upsample(h, in, C.size_t(len(image)),
		&out, &outLen, &errbuf[0], C.size_t(len(errbuf)))
	if err := mapStatus(rc, &errbuf); err != nil {
		return nil, err
	}
	if out == nil || outLen == 0 {
		return nil, errors.New("nativelib: backend reported success but returned no data")
	}
	defer C.gosr_call_free(unsafe.Pointer(out))

	n := C.int(0)
	if outLen <= 0x7fffffff {
		n = C.int(outLen)
	}
	return C.GoBytes(unsafe.Pointer(out), n), nil
}

func mapStatus(rc C.int, errbuf *[errBufCap]C.char) error {
	if rc == StatusOK {
		return nil
	}
	msg := C.GoString(&errbuf[0])
	switch rc {
	case StatusModel:
		return fmt.Errorf("%w: %s", ErrModelNotFound, msg)
	case StatusBadModel:
		return fmt.Errorf("%w: %s", ErrModelCorrupt, msg)
	case StatusBadImage:
		return fmt.Errorf("%w: %s", ErrImageCorrupt, msg)
	case StatusNoFunc:
		return fmt.Errorf("%w: %s", ErrFunctionMissing, msg)
	case StatusInvalid:
		return fmt.Errorf("nativelib: invalid argument/state: %s", msg)
	default:
		return fmt.Errorf("nativelib: internal error: %s", msg)
	}
}

// Sentinel errors surfaced by status translation.
var (
	ErrModelNotFound = errors.New("nativelib: model file not found")
	ErrModelCorrupt  = errors.New("nativelib: model file corrupt or unsupported")
	ErrImageCorrupt  = errors.New("nativelib: input image corrupt or undecodable")
)

// resetForTest drops the active backend; test-only.
func resetForTest() {
	mu.Lock()
	defer mu.Unlock()
	if lib != nil && !lib.closed {
		lib.closed = true
	}
	lib = nil
}
