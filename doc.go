// Package gosr provides Go bindings for local image super-resolution.
//
// The package wraps a native backend (OpenCV 4 dnn_superres) that is loaded
// at runtime from a shared library, so super-resolution runs fully offline:
//
//	r, err := gosr.LoadModelFile(gosr.ESPCN, 4, "ESPCN_x4.pb")
//	if err != nil { ... }
//	defer r.Close()
//
//	pngBytes, err := r.ResolveFile("photo.png")
//
// Required failure classes are sentinel errors usable with errors.Is:
// ErrModelNotFound, ErrModelCorrupt, ErrImageCorrupt and
// ErrBackendFunctionMissing (plus ErrBackendNotFound / ErrBackendABIMismatch).
package gosr
