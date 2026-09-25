//go:build !cgo

// Package nativelib requires cgo to load the native backend shared library.
// When built with CGO_ENABLED=0 the small surface below returns a clear error
// instead of failing with an opaque "build constraints exclude all Go files".
package nativelib

import "errors"

// ErrCgoDisabled is returned by every entry point in a cgo-free build.
var ErrCgoDisabled = errors.New("nativelib: built with CGO_ENABLED=0; " +
	"the super-resolution bindings require cgo")

// Sentinel errors keep their names so the parent package compiles unchanged.
var (
	ErrLibraryNotFound = ErrCgoDisabled
	ErrFunctionMissing = ErrCgoDisabled
	ErrABIMismatch     = ErrCgoDisabled
	ErrModelNotFound   = ErrCgoDisabled
	ErrModelCorrupt    = ErrCgoDisabled
	ErrImageCorrupt    = ErrCgoDisabled
)

// Handle mirrors the cgo Handle shape (an opaque pointer-like value).
type Handle *struct{}

// Library is a no-op placeholder in cgo-free builds.
type Library struct{}

const (
	CapCPU  = 1
	CapCUDA = 2
)

func Open(string) (*Library, error)                        { return nil, ErrCgoDisabled }
func (l *Library) Close() error                            { return nil }
func (l *Library) HasFunction(string) bool                 { return false }
func (l *Library) SupportsCPU() bool                       { return false }
func (l *Library) SupportsCUDA() bool                      { return false }
func (l *Library) Create(string, int) (Handle, error)      { return nil, ErrCgoDisabled }
func (l *Library) Destroy(Handle)                          {}
func (l *Library) LoadModel(Handle, string) error          { return ErrCgoDisabled }
func (l *Library) Upsample(Handle, []byte) ([]byte, error) { return nil, ErrCgoDisabled }
func DefaultCandidates() []string                          { return nil }
