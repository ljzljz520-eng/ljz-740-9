// Package supersr is a Go binding for local image super-resolution engines.
//
// The native library is loaded at runtime with dlopen/dlsym (see native/sr.h
// for the ABI contract), so a missing library or a missing exported function
// is reported as a regular Go error instead of a link-time or start-up
// failure.
//
// Typical use:
//
//	lib, err := supersr.Open("./libsr.so")          // load native library
//	model, err := lib.LoadModel("realesrgan.bin", 4) // load model, x4
//	pngBytes, err := model.UpscalePNG(img)           // run, get PNG bytes
package supersr

/*
#cgo LDFLAGS: -ldl

#include <stdlib.h>
#include <string.h>
#include <dlfcn.h>

// Function-pointer types matching native/sr.h.
typedef void       *(*sr_load_model_fn)(const char *, int, char *, int);
typedef int         (*sr_upscale_fn)(void *, const unsigned char *, int, int, int,
                                     unsigned char **, int *, int *, int *, char *, int);
typedef void        (*sr_free_fn)(void *);
typedef void        (*sr_unload_model_fn)(void *);
typedef const char *(*sr_version_fn)(void);

static void       *x_dlopen(const char *p)  { return dlopen(p, RTLD_NOW | RTLD_LOCAL); }
static void       *x_dlsym(void *h, const char *n) { return dlsym(h, n); }
static const char *x_dlerror(void)          { return dlerror(); }
static void        x_dlclose(void *h)       { dlclose(h); }

// Result structs let us do a whole native call in one cgo crossing,
// including error text, without malloc'ing buffers from Go.

typedef struct {
	void *model;
	char  err[512];
} x_load_result;

static x_load_result x_load_model(sr_load_model_fn fn, const char *path, int scale) {
	x_load_result r;
	memset(&r, 0, sizeof r);
	r.model = fn(path, scale, r.err, (int)sizeof r.err);
	return r;
}

typedef struct {
	unsigned char *data;
	int            width, height, stride;
	int            code; // 0 = success
	char           err[512];
} x_upscale_result;

static x_upscale_result x_upscale(sr_upscale_fn fn, void *m,
                                  const unsigned char *rgba, int w, int h, int stride) {
	x_upscale_result r;
	memset(&r, 0, sizeof r);
	r.code = fn(m, rgba, w, h, stride, &r.data, &r.width, &r.height, &r.stride,
	            r.err, (int)sizeof r.err);
	return r;
}

static void        x_call_free(sr_free_fn fn, void *p)            { fn(p); }
static void        x_call_unload(sr_unload_model_fn fn, void *m)  { fn(m); }
static const char *x_call_version(sr_version_fn fn)               { return fn(); }
*/
import "C"

import (
	"bytes"
	"fmt"
	"image"
	"image/png"
	"os"
	"runtime"
	"sync"
	"unsafe"
)

// MaxScale is the largest upscale factor accepted by LoadModel.
const MaxScale = 16

// Library is a loaded native super-resolution shared library.
//
// All native calls are serialized with an internal mutex, so a Library (and
// its Models) may be shared by goroutines; the native code itself never runs
// concurrently.
type Library struct {
	mu      sync.Mutex
	handle  unsafe.Pointer // dlopen handle
	closed  bool
	version string

	fnLoadModel C.sr_load_model_fn
	fnUpscale   C.sr_upscale_fn
	fnFree      C.sr_free_fn
	fnUnload    C.sr_unload_model_fn
}

// Open loads the native library at path and resolves the required symbols.
//
// Errors: *LoadError (ErrLibraryNotFound) if the file is missing or not a
// loadable shared object; *SymbolError (ErrFunctionMissing) if a required
// sr_* function is not exported.
func Open(path string) (*Library, error) {
	if _, err := os.Stat(path); err != nil {
		return nil, &LoadError{Path: path, Msg: err.Error()}
	}

	cpath := C.CString(path)
	defer C.free(unsafe.Pointer(cpath))

	h := C.x_dlopen(cpath)
	if h == nil {
		return nil, &LoadError{Path: path, Msg: C.GoString(C.x_dlerror())}
	}

	lib := &Library{handle: h}
	if err := lib.resolve(); err != nil {
		C.x_dlclose(h)
		return nil, err
	}

	// sr_version is optional.
	if p := lib.lookup("sr_version"); p != nil {
		lib.version = C.GoString(C.x_call_version(C.sr_version_fn(p)))
	}

	runtime.SetFinalizer(lib, func(l *Library) { _ = l.Close() })
	return lib, nil
}

// lookup resolves a single symbol; returns nil if absent.
func (l *Library) lookup(name string) unsafe.Pointer {
	cname := C.CString(name)
	defer C.free(unsafe.Pointer(cname))
	C.x_dlerror() // clear stale error
	return C.x_dlsym(l.handle, cname)
}

// resolve binds all required entry points.
func (l *Library) resolve() error {
	required := []struct {
		name string
		dst  *unsafe.Pointer
	}{
		{"sr_load_model", (*unsafe.Pointer)(unsafe.Pointer(&l.fnLoadModel))},
		{"sr_upscale_rgba", (*unsafe.Pointer)(unsafe.Pointer(&l.fnUpscale))},
		{"sr_free", (*unsafe.Pointer)(unsafe.Pointer(&l.fnFree))},
		{"sr_unload_model", (*unsafe.Pointer)(unsafe.Pointer(&l.fnUnload))},
	}
	for _, r := range required {
		p := l.lookup(r.name)
		if p == nil {
			return &SymbolError{Name: r.name}
		}
		*r.dst = p
	}
	return nil
}

// Version returns the native library's version string, or "" if the library
// does not export the optional sr_version symbol.
func (l *Library) Version() string { return l.version }

// Close unloads the native library. Models created from it must be closed
// first. Safe to call more than once.
func (l *Library) Close() error {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.closed {
		return nil
	}
	l.closed = true
	if l.handle != nil {
		C.x_dlclose(l.handle)
		l.handle = nil
	}
	return nil
}

// Model is a super-resolution model loaded into the native library.
type Model struct {
	lib    *Library
	handle unsafe.Pointer
	scale  int
	closed bool
}

// LoadModel loads a model file with the given integer upscale factor
// (1..MaxScale).
//
// Errors: ErrInvalidScale; *ModelError (ErrModelNotFound) if the file does
// not exist; *NativeError (ErrNative) if the library rejects the model.
func (l *Library) LoadModel(modelPath string, scale int) (*Model, error) {
	if scale < 1 || scale > MaxScale {
		return nil, fmt.Errorf("%w: got %d, want 1..%d", ErrInvalidScale, scale, MaxScale)
	}
	if _, err := os.Stat(modelPath); err != nil {
		return nil, &ModelError{Path: modelPath, Err: err}
	}

	l.mu.Lock()
	defer l.mu.Unlock()
	if l.closed {
		return nil, ErrClosed
	}

	cpath := C.CString(modelPath)
	defer C.free(unsafe.Pointer(cpath))

	res := C.x_load_model(l.fnLoadModel, cpath, C.int(scale))
	if res.model == nil {
		return nil, &NativeError{Op: "sr_load_model", Msg: C.GoString(&res.err[0])}
	}

	m := &Model{lib: l, handle: res.model, scale: scale}
	runtime.SetFinalizer(m, func(m *Model) { _ = m.Close() })
	return m, nil
}

// Scale returns the upscale factor the model was loaded with.
func (m *Model) Scale() int { return m.scale }

// Close releases the native model handle. Safe to call more than once.
func (m *Model) Close() error {
	m.lib.mu.Lock()
	defer m.lib.mu.Unlock()
	if m.closed {
		return nil
	}
	m.closed = true
	if m.handle != nil && !m.lib.closed {
		C.x_call_unload(m.lib.fnUnload, m.handle)
	}
	m.handle = nil
	return nil
}

// Upscale runs super-resolution on src and returns the enlarged image.
// The input is converted to NRGBA first; the output dimensions are
// (width*scale) x (height*scale).
func (m *Model) Upscale(src image.Image) (*image.NRGBA, error) {
	in := toNRGBA(src)
	w, h := in.Bounds().Dx(), in.Bounds().Dy()
	if w == 0 || h == 0 {
		return nil, fmt.Errorf("%w: empty image", ErrImageCorrupt)
	}

	m.lib.mu.Lock()
	defer m.lib.mu.Unlock()
	if m.closed || m.lib.closed {
		return nil, ErrClosed
	}

	res := C.x_upscale(m.lib.fnUpscale, m.handle,
		(*C.uchar)(unsafe.Pointer(&in.Pix[0])),
		C.int(w), C.int(h), C.int(in.Stride))
	runtime.KeepAlive(in)

	if res.code != 0 {
		return nil, &NativeError{Op: "sr_upscale_rgba", Msg: C.GoString(&res.err[0])}
	}

	outW, outH, outStride := int(res.width), int(res.height), int(res.stride)
	if outW <= 0 || outH <= 0 || outStride < outW*4 {
		C.x_call_free(m.lib.fnFree, unsafe.Pointer(res.data))
		return nil, &NativeError{Op: "sr_upscale_rgba", Msg: "library returned invalid geometry"}
	}

	// Copy out of the native buffer, then release it immediately.
	raw := C.GoBytes(unsafe.Pointer(res.data), C.int(outStride*outH))
	C.x_call_free(m.lib.fnFree, unsafe.Pointer(res.data))

	dst := image.NewNRGBA(image.Rect(0, 0, outW, outH))
	for y := 0; y < outH; y++ {
		copy(dst.Pix[y*dst.Stride:y*dst.Stride+outW*4],
			raw[y*outStride:y*outStride+outW*4])
	}
	return dst, nil
}

// UpscalePNG runs super-resolution on src and encodes the result as PNG,
// returning the PNG bytes.
func (m *Model) UpscalePNG(src image.Image) ([]byte, error) {
	out, err := m.Upscale(src)
	if err != nil {
		return nil, err
	}
	var buf bytes.Buffer
	enc := png.Encoder{CompressionLevel: png.DefaultCompression}
	if err := enc.Encode(&buf, out); err != nil {
		return nil, fmt.Errorf("supersr: encode png: %w", err)
	}
	return buf.Bytes(), nil
}

// UpscalePNGBytes is a convenience wrapper: decode srcBytes (PNG/JPEG/GIF),
// upscale, and return PNG bytes.
func (m *Model) UpscalePNGBytes(srcBytes []byte) ([]byte, error) {
	img, err := DecodeBytes(srcBytes)
	if err != nil {
		return nil, err
	}
	return m.UpscalePNG(img)
}
