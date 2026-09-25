package gosr

import (
	"errors"
	"fmt"
	"os"
	"strings"
	"sync"

	"github.com/solo-manager/gosr/internal/nativelib"
)

// Backend wraps one loaded native super-resolution shared library.
type Backend struct {
	lib *nativelib.Library
}

var (
	defaultMu      sync.Mutex
	defaultBackend *Backend
)

// OpenBackend loads the native backend from the given shared library path.
// Pass an empty path to use $GOSR_NATIVE_LIB and the platform search paths.
func OpenBackend(path string) (*Backend, error) {
	path, err := resolveLibrary(path)
	if err != nil {
		return nil, err
	}
	lib, err := nativelib.Open(path)
	if err != nil {
		switch {
		case errors.Is(err, nativelib.ErrLibraryNotFound):
			return nil, fmt.Errorf("%w: %w", ErrBackendNotFound, err)
		case errors.Is(err, nativelib.ErrFunctionMissing):
			return nil, fmt.Errorf("%w: %w", ErrBackendFunctionMissing, err)
		case errors.Is(err, nativelib.ErrABIMismatch):
			return nil, fmt.Errorf("%w: %w", ErrBackendABIMismatch, err)
		default:
			return nil, fmt.Errorf("gosr: open backend: %w", err)
		}
	}
	return &Backend{lib: lib}, nil
}

// Default returns the process-wide backend, locating and loading it on first
// use. Close it with CloseDefault if you need explicit resource cleanup.
func Default() (*Backend, error) {
	defaultMu.Lock()
	defer defaultMu.Unlock()
	if defaultBackend != nil {
		return defaultBackend, nil
	}
	b, err := OpenBackend("")
	if err != nil {
		return nil, err
	}
	defaultBackend = b
	return b, nil
}

// CloseDefault unloads the process-wide backend if one was loaded.
func CloseDefault() error {
	defaultMu.Lock()
	b := defaultBackend
	defaultBackend = nil
	defaultMu.Unlock()
	if b == nil {
		return nil
	}
	return b.Close()
}

// Close releases the native library. Resolvers created from it must be
// closed first.
func (b *Backend) Close() error {
	if b == nil || b.lib == nil {
		return nil
	}
	err := b.lib.Close()
	b.lib = nil
	return err
}

// HasFunction reports whether the backend exports the given optional symbol.
// Mandatory symbols are guaranteed present once OpenBackend succeeded.
func (b *Backend) HasFunction(name string) bool {
	return b.lib.HasFunction(name)
}

// SupportsCUDA reports whether the backend was built with CUDA support.
func (b *Backend) SupportsCUDA() bool { return b.lib.SupportsCUDA() }

// SupportsCPU always reflects the "gosr_supports" capability query.
func (b *Backend) SupportsCPU() bool { return b.lib.SupportsCPU() }

func resolveLibrary(explicit string) (string, error) {
	if explicit != "" {
		return explicit, nil
	}
	if env := os.Getenv("GOSR_NATIVE_LIB"); env != "" {
		return env, nil
	}
	for _, c := range nativelib.DefaultCandidates() {
		if c == "" {
			continue
		}
		if fi, err := os.Stat(c); err == nil && !fi.IsDir() {
			return c, nil
		}
	}
	return "", fmt.Errorf("%w: searched $GOSR_NATIVE_LIB and standard paths "+
		"(build native/ or set GOSR_NATIVE_LIB)", ErrBackendNotFound)
}

// translateNative converts nativelib status errors into package errors.
func translateNative(err error) error {
	if err == nil {
		return nil
	}
	switch {
	case errors.Is(err, nativelib.ErrModelNotFound):
		return joinErr(ErrModelNotFound, err)
	case errors.Is(err, nativelib.ErrModelCorrupt):
		return joinErr(ErrModelCorrupt, err)
	case errors.Is(err, nativelib.ErrImageCorrupt):
		return joinErr(ErrImageCorrupt, err)
	case errors.Is(err, nativelib.ErrFunctionMissing):
		return joinErr(ErrBackendFunctionMissing, err)
	default:
		return err
	}
}

// joinErr wraps the package sentinel with the native layer's detail while
// keeping errors.Is working. It removes the native-layer prefix and any
// phrase that merely repeats the sentinel's own wording.
func joinErr(sentinel, native error) error {
	return fmt.Errorf("%w: %s", sentinel, detail(sentinel, native.Error()))
}

func detail(sentinel error, msg string) string {
	msg = strings.TrimPrefix(msg, "nativelib: ")
	phrase := strings.TrimPrefix(sentinel.Error(), "gosr: ")
	if i := strings.Index(msg, ": "); i >= 0 && msg[:i] == phrase {
		return strings.TrimSpace(msg[i+2:])
	}
	return msg
}
