//go:build cgo

package supersr

/*
#cgo CFLAGS: -I${SRCDIR}
#cgo !windows LDFLAGS: -ldl
#include <stdlib.h>
#include <string.h>
#include "native/superres.h"
#include "native/loader.h"
*/
import "C"

import (
	"fmt"
	"image"
	"strings"
	"unsafe"
)

const errBufSize = 1024

type cgoBackend struct {
	api    C.sr_api
	ctx    *C.sr_context
	closed bool
}

// openNativeBackend 按候选顺序尝试加载；汇总每一条 dlopen 失败原因。
func openNativeBackend(candidates []string, modelPath string, scale int) (nativeBackend, error) {
	if len(candidates) == 0 {
		return nil, fmt.Errorf("%w: no candidate library path configured", ErrLibraryNotFound)
	}

	// 构造 C 字符串数组。
	cStrs := make([]*C.char, len(candidates))
	for i, p := range candidates {
		cStrs[i] = C.CString(p)
	}
	defer func() {
		for _, s := range cStrs {
			C.free(unsafe.Pointer(s))
		}
	}()

	loadErr, freeLoadErr := newCErrBuf()
	defer freeLoadErr()

	handle := C.sr_load_library((**C.char)(unsafe.Pointer(&cStrs[0])),
		C.int(len(candidates)), loadErr, errBufSize)
	if handle == nil {
		return nil, fmt.Errorf("%w: tried [%s] (%s)",
			ErrLibraryNotFound, strings.Join(candidates, ", "), goString(loadErr))
	}

	b := &cgoBackend{}
	missing := (*C.char)(C.malloc(256))
	defer C.free(unsafe.Pointer(missing))
	C.memset(unsafe.Pointer(missing), 0, 256)

	if rc := C.sr_load_symbols(handle, &b.api, missing, 256); rc != 0 {
		C.sr_unload(handle)
		return nil, fmt.Errorf("%w: symbol %q not found while loading candidates [%s]",
			ErrSymbolMissing, C.GoString(missing), strings.Join(candidates, ", "))
	}

	// 创建原生会话。
	cModel := C.CString(modelPath)
	defer C.free(unsafe.Pointer(cModel))
	createErr, freeCreateErr := newCErrBuf()
	defer freeCreateErr()

	ctx := C.sr_call_create(&b.api, cModel, C.int(scale), createErr, errBufSize)
	if ctx == nil {
		C.sr_unload(handle)
		// 模型在 Go 侧已检查过；走到这里说明后端无法使用该模型文件。
		return nil, fmt.Errorf("%w: backend failed to load model %q: %s",
			ErrBackend, modelPath, goString(createErr))
	}
	b.ctx = ctx
	return b, nil
}

func (b *cgoBackend) version() string {
	if b.closed {
		return ""
	}
	return C.GoString(C.sr_call_version(&b.api))
}

func (b *cgoBackend) setScale(scale int) error {
	if b.closed {
		return ErrClosed
	}
	errBuf, freeBuf := newCErrBuf()
	defer freeBuf()

	rc := C.sr_call_set_scale(&b.api, b.ctx, C.int(scale), errBuf, errBufSize)
	if rc != C.SR_OK {
		return mapCError(rc, "set scale", errBuf)
	}
	return nil
}

func (b *cgoBackend) processRGBA(img *image.RGBA, scale int) (*image.RGBA, error) {
	if b.closed {
		return nil, ErrClosed
	}
	w := img.Bounds().Dx()
	h := img.Bounds().Dy()
	if w <= 0 || h <= 0 {
		return nil, fmt.Errorf("%w: empty image", ErrInvalidImage)
	}

	errBuf, freeBuf := newCErrBuf()
	defer freeBuf()

	var outPix *C.uchar
	var outW, outH, outStride C.int

	// img.Pix 在本次 C 调用期间保持存活；toRGBA 已保证数据从 (0,0) 开始。
	inPix := (*C.uchar)(unsafe.Pointer(&img.Pix[0]))
	rc := C.sr_call_process(&b.api, b.ctx,
		inPix, C.int(w), C.int(h), C.int(img.Stride),
		&outPix, &outW, &outH, &outStride,
		errBuf, errBufSize)
	if rc != C.SR_OK {
		return nil, mapCError(rc, "super-resolution", errBuf)
	}
	if outPix == nil || outW <= 0 || outH <= 0 || outStride < outW*4 {
		if outPix != nil {
			C.sr_call_free_pixels(&b.api, outPix)
		}
		return nil, fmt.Errorf("%w: backend returned invalid output (%dx%d stride=%d)",
			ErrBackend, int(outW), int(outH), int(outStride))
	}

	// 立刻把数据拷贝进 Go 管理的内存，再让后端释放原缓冲，避免 CGO
	// 调用方持有 malloc 指针。
	ow, oh, ost := int(outW), int(outH), int(outStride)
	src := unsafe.Slice((*byte)(unsafe.Pointer(outPix)), ost*oh)

	dst := image.NewRGBA(image.Rect(0, 0, ow, oh))
	if ost == 4*ow {
		copy(dst.Pix, src)
	} else {
		for y := 0; y < oh; y++ {
			copy(dst.Pix[y*dst.Stride:y*dst.Stride+4*ow],
				src[y*ost:y*ost+4*ow])
		}
	}
	C.sr_call_free_pixels(&b.api, outPix)
	return dst, nil
}

func (b *cgoBackend) close() error {
	if b.closed {
		return nil
	}
	b.closed = true
	if b.ctx != nil {
		C.sr_call_destroy(&b.api, b.ctx)
		b.ctx = nil
	}
	if b.api.handle != nil {
		C.sr_unload(b.api.handle)
		b.api.handle = nil
	}
	return nil
}

// newCErrBuf 分配一块零初始化的 C 错误缓冲区，返回释放函数。
func newCErrBuf() (*C.char, func()) {
	p := (*C.char)(C.malloc(errBufSize))
	C.memset(unsafe.Pointer(p), 0, errBufSize)
	return p, func() { C.free(unsafe.Pointer(p)) }
}

func goString(p *C.char) string {
	if p == nil {
		return ""
	}
	return C.GoString(p)
}

// mapCError 把后端状态码映射为带正确类别的 Go 错误。
func mapCError(rc C.int, op string, errBuf *C.char) error {
	msg := goString(errBuf)
	switch rc {
	case C.SR_ERR_ARG:
		return fmt.Errorf("%w: %s: %s", ErrInvalidScale, op, fallbackMsg(msg, "invalid argument"))
	case C.SR_ERR_MODEL:
		return fmt.Errorf("%w: %s: %s", ErrModelNotFound, op, fallbackMsg(msg, "model error"))
	case C.SR_ERR_UNSUPPORTED:
		return fmt.Errorf("%w: %s: %s", ErrSymbolMissing, op, fallbackMsg(msg, "unsupported operation"))
	case C.SR_ERR_NOMEM, C.SR_ERR_INFER:
		return fmt.Errorf("%w: %s: %s", ErrBackend, op, fallbackMsg(msg, "inference failed"))
	default:
		return fmt.Errorf("%w: %s: code=%d %s", ErrBackend, op, int(rc), fallbackMsg(msg, "unknown error"))
	}
}

func fallbackMsg(msg, fallback string) string {
	if strings.TrimSpace(msg) == "" {
		return fallback
	}
	return msg
}

var _ nativeBackend = (*cgoBackend)(nil)
