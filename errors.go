package gosr

import "errors"

// Sentinel errors. All errors returned by the package wrap one of these;
// callers can use errors.Is to handle the required failure classes.
var (
	// ErrModelNotFound: the model path does not exist or is unreadable.
	ErrModelNotFound = errors.New("gosr: model file not found")
	// ErrModelCorrupt: the model exists but cannot be parsed as a network.
	ErrModelCorrupt = errors.New("gosr: model file corrupt or unsupported")
	// ErrImageCorrupt: input bytes do not decode as a supported image.
	ErrImageCorrupt = errors.New("gosr: input image corrupt or undecodable")
	// ErrBackendNotFound: no native backend shared library was located.
	ErrBackendNotFound = errors.New("gosr: native backend library not found")
	// ErrBackendFunctionMissing: the backend lacks a function the bindings need.
	ErrBackendFunctionMissing = errors.New("gosr: required function missing in backend")
	// ErrBackendABIMismatch: backend and bindings have incompatible ABIs.
	ErrBackendABIMismatch = errors.New("gosr: native backend ABI mismatch")
	// ErrInvalidScale/Algorithm: bad session parameters.
	ErrInvalidScale     = errors.New("gosr: invalid scale for algorithm")
	ErrInvalidAlgorithm = errors.New("gosr: unsupported algorithm")
	// ErrClosed: a resolver/backend is used after Close.
	ErrClosed = errors.New("gosr: resolver closed")
)
