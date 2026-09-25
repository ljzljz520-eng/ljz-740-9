package supersr

import (
	"bytes"
	"fmt"
	"image"
	"image/draw"
	"io"

	// Register decoders for the common formats accepted by Decode.
	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"
)

// Decode reads an image (PNG, JPEG or GIF) from r. Corrupt or unsupported
// data yields an error matching ErrImageCorrupt.
func Decode(r io.Reader) (image.Image, error) {
	img, _, err := image.Decode(r)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrImageCorrupt, err)
	}
	return img, nil
}

// DecodeBytes decodes an image from an in-memory byte slice.
func DecodeBytes(b []byte) (image.Image, error) {
	return Decode(bytes.NewReader(b))
}

// toNRGBA converts any image to a tightly packed, non-premultiplied RGBA
// buffer starting at the origin, as required by the native ABI.
func toNRGBA(src image.Image) *image.NRGBA {
	if n, ok := src.(*image.NRGBA); ok {
		b := n.Bounds()
		if b.Min == (image.Point{}) && n.Stride == 4*b.Dx() {
			return n // already in the exact layout we need
		}
	}
	b := src.Bounds()
	dst := image.NewNRGBA(image.Rect(0, 0, b.Dx(), b.Dy()))
	draw.Draw(dst, dst.Bounds(), src, b.Min, draw.Src)
	return dst
}
