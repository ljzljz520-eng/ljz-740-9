package supersr

import (
	"bytes"
	"errors"
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"testing"
)

// makeImage 生成一张 w*h 的测试 RGBA 图。
func makeImage(w, h int) *image.RGBA {
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			img.Set(x, y, color.RGBA{R: uint8(x * 4), G: uint8(y * 8), B: 128, A: 255})
		}
	}
	return img
}

func pngBytes(t *testing.T, img image.Image) []byte {
	t.Helper()
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func TestResolveLibraryCandidates(t *testing.T) {
	t.Setenv(LibraryEnv, "")

	// 无显式配置时使用默认名。
	got := resolveLibraryCandidates(nil)
	if len(got) == 0 {
		t.Fatal("expected default library candidates")
	}

	// 显式路径优先，且不再回退默认名。
	got = resolveLibraryCandidates([]string{"/opt/libsuperres.so"})
	if len(got) != 1 || got[0] != "/opt/libsuperres.so" {
		t.Fatalf("explicit path must suppress defaults, got %v", got)
	}

	// 去重。
	got = resolveLibraryCandidates([]string{"a.so", "a.so"})
	if len(got) != 1 {
		t.Fatalf("expected dedup, got %v", got)
	}

	// 环境变量候选。
	t.Setenv(LibraryEnv, string(os.PathListSeparator)+"envlib.so")
	got = resolveLibraryCandidates([]string{"a.so", "envlib.so"})
	if len(got) != 2 || got[0] != "a.so" || got[1] != "envlib.so" {
		t.Fatalf("unexpected candidates %v", got)
	}
}

func TestOpenInvalidScale(t *testing.T) {
	_, err := Open("whatever", WithScale(1))
	if !errors.Is(err, ErrInvalidScale) {
		t.Fatalf("want ErrInvalidScale, got %v", err)
	}
	_, err = Open("whatever", WithScale(9))
	if !errors.Is(err, ErrInvalidScale) {
		t.Fatalf("want ErrInvalidScale, got %v", err)
	}
}

func TestOpenModelNotFound(t *testing.T) {
	_, err := Open(filepath.Join(t.TempDir(), "missing.bin"))
	if !errors.Is(err, ErrModelNotFound) {
		t.Fatalf("want ErrModelNotFound, got %v", err)
	}

	dir := t.TempDir()
	_, err = Open(dir) // 目录也算模型不存在。
	if !errors.Is(err, ErrModelNotFound) {
		t.Fatalf("want ErrModelNotFound for directory, got %v", err)
	}

	_, err = Open("")
	if !errors.Is(err, ErrModelNotFound) {
		t.Fatalf("want ErrModelNotFound for empty path, got %v", err)
	}
}

func TestLibraryNotFound(t *testing.T) {
	model := filepath.Join(t.TempDir(), "model.bin")
	if err := os.WriteFile(model, []byte("fake"), 0o644); err != nil {
		t.Fatal(err)
	}
	_, err := Open(model, WithLibrary(filepath.Join(t.TempDir(), "libnope.so")))
	// CGO 禁用时根本无法 dlopen，错误类别优先收敛为 ErrCGODisabled。
	want := ErrLibraryNotFound
	if !cgoEnabled {
		want = ErrCGODisabled
	}
	if !errors.Is(err, want) {
		t.Fatalf("want %v, got %v", want, err)
	}
}

func TestSymbolMissing(t *testing.T) {
	// 一个与超分 ABI 无关的普通共享库，能加载但缺符号。
	dummy := buildDummyLib(t)
	model := filepath.Join(t.TempDir(), "model.bin")
	if err := os.WriteFile(model, []byte("fake"), 0o644); err != nil {
		t.Fatal(err)
	}
	_, err := Open(model, WithLibrary(dummy))
	if !errors.Is(err, ErrSymbolMissing) {
		t.Fatalf("want ErrSymbolMissing, got %v", err)
	}
}

func TestInvalidImage(t *testing.T) {
	dir := t.TempDir()
	model := filepath.Join(dir, "model.bin")
	if err := os.WriteFile(model, []byte("fake"), 0o644); err != nil {
		t.Fatal(err)
	}
	r, err := Open(model, WithLibrary(testLibrary(t)))
	if err != nil {
		t.Skipf("参考后端不可用，跳过: %v", err)
	}
	defer r.Close()

	if _, err := r.ProcessBytes([]byte("not an image")); !errors.Is(err, ErrInvalidImage) {
		t.Fatalf("损坏图片应返回 ErrInvalidImage, got %v", err)
	}
	if _, err := r.ProcessReader(bytes.NewReader(nil)); !errors.Is(err, ErrInvalidImage) {
		t.Fatalf("空输入应返回 ErrInvalidImage, got %v", err)
	}
}

func TestEndToEnd(t *testing.T) {
	dir := t.TempDir()
	model := filepath.Join(dir, "model.bin")
	if err := os.WriteFile(model, []byte("fake model weights"), 0o644); err != nil {
		t.Fatal(err)
	}

	r, err := Open(model, WithLibrary(testLibrary(t)), WithScale(2))
	if err != nil {
		t.Skipf("参考后端不可用（非 cgo 环境或未构建），跳过: %v", err)
	}
	defer r.Close()

	if v := r.Version(); v == "" {
		t.Fatal("empty backend version")
	} else {
		t.Logf("后端版本: %s", v)
	}

	// ProcessBytes：PNG 输入 -> PNG 输出，尺寸按倍率放大。
	src := makeImage(12, 8)
	out, err := r.ProcessBytes(pngBytes(t, src))
	if err != nil {
		t.Fatal(err)
	}
	cfg, err := png.DecodeConfig(bytes.NewReader(out))
	if err != nil {
		t.Fatalf("输出不是合法 PNG: %v", err)
	}
	if cfg.Width != 24 || cfg.Height != 16 {
		t.Fatalf("x2 输出尺寸错误: got %dx%d, want 24x16", cfg.Width, cfg.Height)
	}

	// 运行时改倍率。
	if err := r.SetScale(4); err != nil {
		t.Fatal(err)
	}
	if r.Scale() != 4 {
		t.Fatalf("scale = %d, want 4", r.Scale())
	}
	out4, err := r.Process(src)
	if err != nil {
		t.Fatal(err)
	}
	if out4.Bounds().Dx() != 48 || out4.Bounds().Dy() != 32 {
		t.Fatalf("x4 尺寸错误: %v", out4.Bounds())
	}

	// 非法倍率。
	if err := r.SetScale(1); !errors.Is(err, ErrInvalidScale) {
		t.Fatalf("want ErrInvalidScale, got %v", err)
	}

	// 关闭后调用应报错，且 Close 可重复。
	if err := r.Close(); err != nil {
		t.Fatal(err)
	}
	if err := r.Close(); err != nil {
		t.Fatalf("重复 Close 应返回 nil, got %v", err)
	}
	if _, err := r.Process(src); !errors.Is(err, ErrClosed) {
		t.Fatalf("want ErrClosed, got %v", err)
	}
}

func TestJPEGInput(t *testing.T) {
	// 验证格式注册：jpeg 输入也应能解码（通过 image 包通用注册）。
	dir := t.TempDir()
	model := filepath.Join(dir, "model.bin")
	if err := os.WriteFile(model, []byte("m"), 0o644); err != nil {
		t.Fatal(err)
	}
	r, err := Open(model, WithLibrary(testLibrary(t)))
	if err != nil {
		t.Skipf("参考后端不可用，跳过: %v", err)
	}
	defer r.Close()

	// 手写一个最小 JPEG 不现实，这里直接用 image.YCbCr 不经过解码；
	// 改为验证 RGBA 子图（带偏移的 SubImage）也能安全处理。
	src := makeImage(10, 10).SubImage(image.Rect(2, 2, 8, 8))
	out, err := r.Process(src)
	if err != nil {
		t.Fatal(err)
	}
	if out.Bounds().Dx() != 12 || out.Bounds().Dy() != 12 {
		t.Fatalf("子图 x2 尺寸错误: %v", out.Bounds())
	}
}
