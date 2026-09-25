package gosr

import (
	"bytes"
	"errors"
	"image"
	"image/color"
	"image/png"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func ccBin() string {
	if c := os.Getenv("CC"); c != "" {
		return c
	}
	if p, err := exec.LookPath("gcc"); err == nil {
		return p
	}
	return "cc"
}

func libExt() string {
	switch runtime.GOOS {
	case "darwin":
		return ".dylib"
	case "windows":
		return ".dll"
	default:
		return ".so"
	}
}

func buildMockLib(t *testing.T, src, name string) string {
	t.Helper()
	dir := t.TempDir()
	out := filepath.Join(dir, name+libExt())
	cmd := exec.Command(ccBin(), "-shared", "-fPIC", "-O0", "-g",
		"-I", "internal/nativelib",
		"-o", out, src)
	if b, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("build %s: %v\n%s", name, err, b)
	}
	return out
}

func mockFull(t *testing.T) *Backend {
	t.Helper()
	path := buildMockLib(t,
		filepath.Join("internal", nativelibTestdata(), "mock_full.c"), "mock_full")
	b, err := OpenBackend(path)
	if err != nil {
		t.Fatalf("open backend: %v", err)
	}
	t.Cleanup(func() { _ = b.Close() })
	return b
}

func nativelibTestdata() string { return filepath.Join("nativelib", "testdata") }

func tinyPNG(t *testing.T) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, 2, 2))
	for y := 0; y < 2; y++ {
		for x := 0; x < 2; x++ {
			img.Set(x, y, color.RGBA{R: 10, G: 20, B: 30, A: 255})
		}
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func TestBackendNotFound(t *testing.T) {
	resetDefaultForTest()
	t.Setenv("GOSR_NATIVE_LIB", filepath.Join(t.TempDir(), "absent"+libExt()))
	_, err := Default()
	if !errors.Is(err, ErrBackendNotFound) {
		t.Fatalf("want ErrBackendNotFound, got %v", err)
	}
}

func TestBackendFunctionMissing(t *testing.T) {
	path := buildMockLib(t,
		filepath.Join("internal", "nativelib", "testdata", "mock_missing.c"),
		"mock_missing")
	_, err := OpenBackend(path)
	if !errors.Is(err, ErrBackendFunctionMissing) {
		t.Fatalf("want ErrBackendFunctionMissing, got %v", err)
	}
	if !strings.Contains(err.Error(), "gosr_upsample") {
		t.Fatalf("error should name missing symbol, got %v", err)
	}
}

func TestBackendABIMismatch(t *testing.T) {
	path := buildMockLib(t,
		filepath.Join("internal", "nativelib", "testdata", "mock_badabi.c"),
		"mock_badabi")
	_, err := OpenBackend(path)
	if !errors.Is(err, ErrBackendABIMismatch) {
		t.Fatalf("want ErrBackendABIMismatch, got %v", err)
	}
}

func TestInvalidAlgorithmAndScale(t *testing.T) {
	b := mockFull(t)
	if _, err := b.NewResolver("waifu2x", 2); !errors.Is(err, ErrInvalidAlgorithm) {
		t.Fatalf("want ErrInvalidAlgorithm, got %v", err)
	}
	if _, err := b.NewResolver(ESPCN, 5); !errors.Is(err, ErrInvalidScale) {
		t.Fatalf("want ErrInvalidScale, got %v", err)
	}
	if _, err := b.NewResolver(LapSRN, 3); !errors.Is(err, ErrInvalidScale) {
		t.Fatalf("want ErrInvalidScale, got %v", err)
	}
}

func TestModelNotFound(t *testing.T) {
	b := mockFull(t)
	r, err := b.NewResolver(ESPCN, 4)
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()

	missing := filepath.Join(t.TempDir(), "ESPCN_x4.pb")
	err = r.LoadModel(missing)
	if !errors.Is(err, ErrModelNotFound) {
		t.Fatalf("want ErrModelNotFound, got %v", err)
	}

	// Directory is not a model either.
	if err := r.LoadModel(t.TempDir()); !errors.Is(err, ErrModelNotFound) {
		t.Fatalf("dir: want ErrModelNotFound, got %v", err)
	}
}

func TestModelCorrupt(t *testing.T) {
	b := mockFull(t)
	r, err := b.NewResolver(FSRCNN, 3)
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()

	bad := filepath.Join(t.TempDir(), "garbage.pb")
	if err := os.WriteFile(bad, []byte("definitely not protobuf"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := r.LoadModel(bad); !errors.Is(err, ErrModelCorrupt) {
		t.Fatalf("want ErrModelCorrupt, got %v", err)
	}
}

func TestUpsampleCorruptImage(t *testing.T) {
	b := mockFull(t)
	r, err := b.NewResolver(ESPCN, 4)
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()

	good := filepath.Join(t.TempDir(), "weights.pb")
	if err := os.WriteFile(good, []byte("ok"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := r.LoadModel(good); err != nil {
		t.Fatal(err)
	}

	if _, err := r.UpsampleBytes(nil); !errors.Is(err, ErrImageCorrupt) {
		t.Fatalf("nil: want ErrImageCorrupt, got %v", err)
	}
	if _, err := r.UpsampleBytes([]byte("garbage")); !errors.Is(err, ErrImageCorrupt) {
		t.Fatalf("garbage: want ErrImageCorrupt, got %v", err)
	}
	// image.Image path is always encodable; nil image must still fail cleanly.
	if _, err := r.UpsampleImage(nil); !errors.Is(err, ErrImageCorrupt) {
		t.Fatalf("nil image: want ErrImageCorrupt, got %v", err)
	}
}

func TestUpsampleBeforeLoad(t *testing.T) {
	b := mockFull(t)
	r, err := b.NewResolver(ESPCN, 2)
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()
	if _, err := r.UpsampleBytes(tinyPNG(t)); err == nil {
		t.Fatal("expected error when no model is loaded")
	}
}

func TestHappyPath(t *testing.T) {
	b := mockFull(t)
	r, err := b.NewResolver(EDSR, 2)
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()

	good := filepath.Join(t.TempDir(), "EDSR_x2.pb")
	if err := os.WriteFile(good, []byte("weights"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := r.LoadModel(good); err != nil {
		t.Fatal(err)
	}

	// Bytes API.
	out, err := r.UpsampleBytes(tinyPNG(t))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := png.Decode(bytes.NewReader(out)); err != nil {
		t.Fatalf("result not a valid PNG: %v", err)
	}

	// image.Image API.
	img := image.NewRGBA(image.Rect(0, 0, 1, 1))
	out2, err := r.UpsampleImage(img)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := png.Decode(bytes.NewReader(out2)); err != nil {
		t.Fatalf("image result not a valid PNG: %v", err)
	}
}

func TestSetScaleReloadsModel(t *testing.T) {
	b := mockFull(t)
	r, err := b.NewResolver(ESPCN, 2)
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()

	good := filepath.Join(t.TempDir(), "ESPCN.pb")
	if err := os.WriteFile(good, []byte("weights"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := r.LoadModel(good); err != nil {
		t.Fatal(err)
	}
	if err := r.SetScale(4); err != nil {
		t.Fatal(err)
	}
	if r.Scale() != 4 {
		t.Fatalf("scale = %d, want 4", r.Scale())
	}
	// Model must still be loaded at the new scale.
	if _, err := r.UpsampleBytes(tinyPNG(t)); err != nil {
		t.Fatalf("upsample after SetScale: %v", err)
	}
	if err := r.SetScale(5); !errors.Is(err, ErrInvalidScale) {
		t.Fatalf("want ErrInvalidScale, got %v", err)
	}
}

func TestCloseIdempotentAndGuarded(t *testing.T) {
	b := mockFull(t)
	r, err := b.NewResolver(ESPCN, 2)
	if err != nil {
		t.Fatal(err)
	}
	if err := r.Close(); err != nil {
		t.Fatal(err)
	}
	if err := r.Close(); err != nil {
		t.Fatalf("second close: %v", err)
	}
	if err := r.LoadModel("whatever"); !errors.Is(err, ErrClosed) {
		t.Fatalf("want ErrClosed, got %v", err)
	}
	if _, err := r.UpsampleBytes(tinyPNG(t)); !errors.Is(err, ErrClosed) {
		t.Fatalf("want ErrClosed, got %v", err)
	}
}
