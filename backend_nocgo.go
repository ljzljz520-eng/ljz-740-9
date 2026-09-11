//go:build !cgo

package supersr

import "image"

// CGO 被禁用（CGO_ENABLED=0）时无法加载任何原生库；给出明确的错误类别，
// 让调用方的 errors.Is(err, ErrCGODisabled) 能够命中。
func openNativeBackend(candidates []string, modelPath string, scale int) (nativeBackend, error) {
	return nil, ErrCGODisabled
}

type noCGOBackend struct{}

func (noCGOBackend) setScale(int) error { return ErrCGODisabled }
func (noCGOBackend) processRGBA(*image.RGBA, int) (*image.RGBA, error) {
	return nil, ErrCGODisabled
}
func (noCGOBackend) version() string { return "" }
func (noCGOBackend) close() error    { return nil }

var _ nativeBackend = noCGOBackend{}
