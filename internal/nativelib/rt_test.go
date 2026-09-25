package nativelib

import (
	"bytes"
	"errors"
	"fmt"
	"image"
	"image/png"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"
)

var (
	pngInput []byte
	libFull  string
	libMiss  string
	libBad   string
)

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

func ccBin() string {
	if c := os.Getenv("CC"); c != "" {
		return c
	}
	if p, err := exec.LookPath("gcc"); err == nil {
		return p
	}
	return "cc"
}

func buildMock(t *testing.T, name string) string {
	t.Helper()
	out := filepath.Join(t.TempDir(), name+libExt())
	src := filepath.Join("testdata", name+".c")
	cmd := exec.Command(ccBin(), "-shared", "-fPIC", "-O0", "-g",
		"-o", out, src)
	if b, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("build %s: %v\n%s", name, err, b)
	}
	return out
}

func TestMain(m *testing.M) {
	var buf bytes.Buffer
	img := image.NewNRGBA(image.Rect(0, 0, 2, 2))
	for i := range img.Pix {
		img.Pix[i] = 255
	}
	if err := png.Encode(&buf, img); err != nil {
		fmt.Fprint(os.Stderr, err)
		os.Exit(1)
	}
	pngInput = buf.Bytes()

	// Mocks are needed by every test; build in a throwaway process step.
	tmp, err := os.MkdirTemp("", "gosr-mocks-")
	if err != nil {
		fmt.Fprint(os.Stderr, err)
		os.Exit(1)
	}
	for _, name := range []string{"mock_full", "mock_missing", "mock_badabi"} {
		out := filepath.Join(tmp, name+libExt())
		cmd := exec.Command(ccBin(), "-shared", "-fPIC", "-O0", "-g",
			"-o", out, filepath.Join("testdata", name+".c"))
		if b, err := cmd.CombinedOutput(); err != nil {
			fmt.Fprintf(os.Stderr, "build %s: %v\n%s", name, err, b)
			os.Exit(1)
		}
		switch name {
		case "mock_full":
			libFull = out
		case "mock_missing":
			libMiss = out
		case "mock_badabi":
			libBad = out
		}
	}
	os.Exit(m.Run())
}

func reset() { resetForTest() }

func TestOpen_MissingFile(t *testing.T) {
	reset()
	_, err := Open(filepath.Join(t.TempDir(), "nope"+libExt()))
	if !errors.Is(err, ErrLibraryNotFound) {
		t.Fatalf("want ErrLibraryNotFound, got %v", err)
	}
}

func TestOpen_FunctionMissing(t *testing.T) {
	reset()
	_, err := Open(libMiss)
	if !errors.Is(err, ErrFunctionMissing) {
		t.Fatalf("want ErrFunctionMissing, got %v", err)
	}
}

func TestOpen_ABIMismatch(t *testing.T) {
	reset()
	_, err := Open(libBad)
	if !errors.Is(err, ErrABIMismatch) {
		t.Fatalf("want ErrABIMismatch, got %v", err)
	}
}

func TestFullLifecycle(t *testing.T) {
	reset()
	l, err := Open(libFull)
	if err != nil {
		t.Fatal(err)
	}
	if !l.HasFunction("gosr_supports") {
		t.Fatal("gosr_supports should be present in mock_full")
	}
	if !l.SupportsCPU() {
		t.Fatal("mock should report CPU support")
	}
	if l.SupportsCUDA() {
		t.Fatal("mock should not report CUDA support")
	}

	h, err := l.Create("espcn", 4)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		l.Destroy(h)
		_ = l.Close()
	}()

	// Model not found: both Go-side path and backend status.
	err = l.LoadModel(h, filepath.Join(t.TempDir(), "missing.pb"))
	if !errors.Is(err, ErrModelNotFound) {
		t.Fatalf("want ErrModelNotFound, got %v", err)
	}

	// Existing-but-bad model.
	bad := filepath.Join(t.TempDir(), "garbage.pb")
	if err := os.WriteFile(bad, []byte("not a model"), 0o644); err != nil {
		t.Fatal(err)
	}
	err = l.LoadModel(h, bad)
	if !errors.Is(err, ErrModelCorrupt) {
		t.Fatalf("want ErrModelCorrupt, got %v", err)
	}

	// Valid load.
	good := filepath.Join(t.TempDir(), "weights.pb")
	if err := os.WriteFile(good, []byte("weights"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := l.LoadModel(h, good); err != nil {
		t.Fatal(err)
	}

	// Corrupt input.
	_, err = l.Upsample(h, []byte("this is not an image"))
	if !errors.Is(err, ErrImageCorrupt) {
		t.Fatalf("want ErrImageCorrupt, got %v", err)
	}

	// Happy path: PNG in -> PNG out.
	out, err := l.Upsample(h, pngInput)
	if err != nil {
		t.Fatal(err)
	}
	if len(out) == 0 || !bytes.Equal(out[:8], pngInput[:8]) {
		t.Fatalf("unexpected output: %d bytes", len(out))
	}
	if _, err := png.Decode(bytes.NewReader(out)); err != nil {
		t.Fatalf("output is not decodable PNG: %v", err)
	}
}

func TestEmptyInput(t *testing.T) {
	reset()
	l, err := Open(libFull)
	if err != nil {
		t.Fatal(err)
	}
	defer l.Close()
	h, _ := l.Create("espcn", 2)
	defer l.Destroy(h)
	if _, err := l.Upsample(h, nil); err == nil {
		t.Fatal("expected error on empty input")
	}
}
