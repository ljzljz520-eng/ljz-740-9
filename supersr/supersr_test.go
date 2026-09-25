package supersr_test

import (
	"bytes"
	"errors"
	"image"
	"image/color"
	"image/png"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/example/supersr/supersr"
)

// buildRefLib compiles native/sr.c into a temp .so; skipped if gcc is absent.
func buildRefLib(t *testing.T, extraArgs ...string) string {
	t.Helper()
	gcc, err := exec.LookPath("gcc")
	if err != nil {
		t.Skip("gcc not available, skipping native test")
	}
	so := filepath.Join(t.TempDir(), "libsr_ref.so")
	src, err := filepath.Abs(filepath.Join("..", "native", "sr.c"))
	if err != nil {
		t.Fatal(err)
	}
	args := append([]string{"-shared", "-fPIC", "-O2", "-o", so, src}, extraArgs...)
	if out, err := exec.Command(gcc, args...).CombinedOutput(); err != nil {
		t.Fatalf("build reference library: %v\n%s", err, out)
	}
	return so
}

func writeModel(t *testing.T) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "model.bin")
	if err := os.WriteFile(p, []byte("SRSIMPLE-WEIGHTS"), 0o644); err != nil {
		t.Fatal(err)
	}
	return p
}

func gradient(w, h int) *image.NRGBA {
	img := image.NewNRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			img.SetNRGBA(x, y, color.NRGBA{
				R: uint8(x * 255 / max(w-1, 1)),
				G: uint8(y * 255 / max(h-1, 1)),
				B: 128, A: 255,
			})
		}
	}
	return img
}

func TestOpenMissingLibrary(t *testing.T) {
	_, err := supersr.Open(filepath.Join(t.TempDir(), "does-not-exist.so"))
	if !errors.Is(err, supersr.ErrLibraryNotFound) {
		t.Fatalf("want ErrLibraryNotFound, got %v", err)
	}
	var le *supersr.LoadError
	if !errors.As(err, &le) {
		t.Fatalf("want *LoadError, got %T", err)
	}
}

func TestOpenMissingSymbol(t *testing.T) {
	// Reference library built without sr_upscale_rgba.
	so := buildRefLib(t, "-DSR_OMIT_UPSCALE")
	_, err := supersr.Open(so)
	if !errors.Is(err, supersr.ErrFunctionMissing) {
		t.Fatalf("want ErrFunctionMissing, got %v", err)
	}
	var se *supersr.SymbolError
	if !errors.As(err, &se) || se.Name != "sr_upscale_rgba" {
		t.Fatalf("want SymbolError{sr_upscale_rgba}, got %v", err)
	}
}

func TestLoadModelNotFound(t *testing.T) {
	lib, err := supersr.Open(buildRefLib(t))
	if err != nil {
		t.Fatal(err)
	}
	defer lib.Close()

	_, err = lib.LoadModel(filepath.Join(t.TempDir(), "no-such-model.bin"), 4)
	if !errors.Is(err, supersr.ErrModelNotFound) {
		t.Fatalf("want ErrModelNotFound, got %v", err)
	}
}

func TestLoadModelInvalidScale(t *testing.T) {
	lib, err := supersr.Open(buildRefLib(t))
	if err != nil {
		t.Fatal(err)
	}
	defer lib.Close()

	for _, s := range []int{0, -1, 17, 1000} {
		if _, err := lib.LoadModel(writeModel(t), s); !errors.Is(err, supersr.ErrInvalidScale) {
			t.Fatalf("scale %d: want ErrInvalidScale, got %v", s, err)
		}
	}
}

func TestDecodeCorruptImage(t *testing.T) {
	for _, bad := range [][]byte{
		[]byte("this is not an image"),
		{0x89, 0x50, 0x4E, 0x47, 0x0D, 0x0A, 0x1A, 0x0A, 0x00, 0x01}, // truncated PNG
		{},
	} {
		if _, err := supersr.DecodeBytes(bad); !errors.Is(err, supersr.ErrImageCorrupt) {
			t.Fatalf("want ErrImageCorrupt, got %v", err)
		}
	}
}

func TestUpscaleEndToEnd(t *testing.T) {
	lib, err := supersr.Open(buildRefLib(t))
	if err != nil {
		t.Fatal(err)
	}
	defer lib.Close()
	if lib.Version() == "" {
		t.Fatal("expected non-empty version from reference library")
	}

	const scale = 4
	model, err := lib.LoadModel(writeModel(t), scale)
	if err != nil {
		t.Fatal(err)
	}
	defer model.Close()
	if model.Scale() != scale {
		t.Fatalf("Scale() = %d, want %d", model.Scale(), scale)
	}

	src := gradient(24, 16)
	pngBytes, err := model.UpscalePNG(src)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.HasPrefix(pngBytes, []byte{0x89, 'P', 'N', 'G'}) {
		t.Fatal("output is not a PNG")
	}

	out, err := png.Decode(bytes.NewReader(pngBytes))
	if err != nil {
		t.Fatal(err)
	}
	if got := out.Bounds().Size(); got != (image.Point{24 * scale, 16 * scale}) {
		t.Fatalf("output size = %v, want %v", got, image.Point{96, 64})
	}
}

func TestUpscalePNGBytesCorruptInput(t *testing.T) {
	lib, err := supersr.Open(buildRefLib(t))
	if err != nil {
		t.Fatal(err)
	}
	defer lib.Close()

	model, err := lib.LoadModel(writeModel(t), 2)
	if err != nil {
		t.Fatal(err)
	}
	defer model.Close()

	if _, err := model.UpscalePNGBytes([]byte("garbage")); !errors.Is(err, supersr.ErrImageCorrupt) {
		t.Fatalf("want ErrImageCorrupt, got %v", err)
	}
}

func TestUseAfterClose(t *testing.T) {
	lib, err := supersr.Open(buildRefLib(t))
	if err != nil {
		t.Fatal(err)
	}
	model, err := lib.LoadModel(writeModel(t), 2)
	if err != nil {
		t.Fatal(err)
	}
	model.Close()
	lib.Close()
	if _, err := model.Upscale(gradient(4, 4)); !errors.Is(err, supersr.ErrClosed) {
		t.Fatalf("want ErrClosed, got %v", err)
	}
	if err := lib.Close(); err != nil { // double close must be a no-op
		t.Fatal(err)
	}
}
