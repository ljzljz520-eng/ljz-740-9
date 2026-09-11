// Package supersr 提供本地图像超分辨率（Super-Resolution）原生库的 Go 绑定。
//
// 绑定本身不实现超分算法，而是在运行时通过 dlopen/LoadLibrary 加载一个实现了
// native/superres.h C ABI 的原生动态库（如 Real-ESRGAN / Real-CUGAN 的封装），
// 并负责：
//
//   - 加载/校验模型文件；
//   - 设置放大倍率；
//   - 解码常见图片格式（PNG/JPEG/GIF）并转为 RGBA 传入后端；
//   - 执行超分推理；
//   - 将结果编码为 PNG 字节返回。
//
// 仓库自带一个参考后端 native/ref_backend.c（双线性放大），用于开发、测试与
// 演示；接入真实推理引擎时，只需让自己的共享库导出 superres.h 中约定的
// 6 个 C 函数即可。
package supersr

import (
	"bytes"
	"errors"
	"fmt"
	"image"
	"image/draw"
	_ "image/gif" // 注册 GIF 解码器
	_ "image/jpeg"
	"image/png"
	"io"
	"os"
	"path/filepath"
	"sync"
)

// MinScale / MaxScale 是绑定层允许的倍率范围。具体后端可能进一步收窄。
const (
	MinScale = 2
	MaxScale = 8

	// LibraryEnv 可用于指定原生库路径（多个候选路径用操作系统的分隔符
	// 分隔：Linux/macOS 用 ':'，Windows 用 ';'）。
	LibraryEnv = "SUPERSR_LIBRARY"

	// DefaultScale 是未显式设置倍率时使用的默认值。
	DefaultScale = 2
)

// Option 用于配置 SuperResolver。
type Option func(*config)

type config struct {
	scale     int
	libraries []string // 显式指定的动态库候选路径
}

// WithScale 设置初始放大倍率（2~8）。之后也可以用 SetScale 修改。
func WithScale(s int) Option {
	return func(c *config) { c.scale = s }
}

// WithLibrary 显式指定原生动态库的路径（可多次调用形成候选列表）。
// 不指定时按 SUPERSR_LIBRARY 环境变量、可执行文件同目录、系统库搜索
// 路径的顺序查找默认库名。
func WithLibrary(path string) Option {
	return func(c *config) {
		if path != "" {
			c.libraries = append(c.libraries, path)
		}
	}
}

// SuperResolver 封装一个原生超分推理会话。它不是并发安全的：同一时刻
// 只应有一个 goroutine 调用 Process*；Close 之后所有方法返回 ErrClosed。
type SuperResolver struct {
	mu      sync.Mutex
	backend nativeBackend
	model   string
	scale   int
	closed  bool
}

// Open 加载模型并打开原生后端，返回一个超分会话。
//
// 可能返回的错误（可用 errors.Is 判断）：
//   - ErrModelNotFound:    模型文件不存在；
//   - ErrLibraryNotFound:  找不到原生动态库；
//   - ErrSymbolMissing:    动态库缺少必需的导出函数；
//   - ErrInvalidScale:     倍率非法；
//   - ErrCGODisabled:      CGO 被禁用的构建。
func Open(modelPath string, opts ...Option) (*SuperResolver, error) {
	cfg := config{scale: DefaultScale}
	for _, opt := range opts {
		opt(&cfg)
	}
	if cfg.scale < MinScale || cfg.scale > MaxScale {
		return nil, fmt.Errorf("%w: scale=%d, want %d..%d", ErrInvalidScale, cfg.scale, MinScale, MaxScale)
	}

	// 在 Go 侧先做一次模型文件检查，保证错误类别稳定，不依赖后端实现。
	if modelPath == "" {
		return nil, fmt.Errorf("%w: empty model path", ErrModelNotFound)
	}
	info, err := os.Stat(modelPath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) || errors.Is(err, os.ErrPermission) {
			return nil, fmt.Errorf("%w: %s: %v", ErrModelNotFound, modelPath, err)
		}
		return nil, fmt.Errorf("%w: %s: %v", ErrModelNotFound, modelPath, err)
	}
	if info.IsDir() {
		return nil, fmt.Errorf("%w: %s is a directory", ErrModelNotFound, modelPath)
	}

	candidates := resolveLibraryCandidates(cfg.libraries)
	nb, err := openNativeBackend(candidates, modelPath, cfg.scale)
	if err != nil {
		return nil, err
	}

	return &SuperResolver{
		backend: nb,
		model:   modelPath,
		scale:   cfg.scale,
	}, nil
}

// Model 返回打开时使用的模型路径。
func (s *SuperResolver) Model() string { return s.model }

// Scale 返回当前倍率。
func (s *SuperResolver) Scale() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.scale
}

// SetScale 修改放大倍率。原生库不支持该操作时返回 ErrSymbolMissing；
// 倍率非法返回 ErrInvalidScale；后端拒绝时返回包装后的 ErrBackend。
func (s *SuperResolver) SetScale(scale int) error {
	if scale < MinScale || scale > MaxScale {
		return fmt.Errorf("%w: scale=%d, want %d..%d", ErrInvalidScale, scale, MinScale, MaxScale)
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return ErrClosed
	}
	if err := s.backend.setScale(scale); err != nil {
		return err
	}
	s.scale = scale
	return nil
}

// Process 对一张解码后的图像执行超分，返回放大后的图像（始终为 *image.RGBA）。
func (s *SuperResolver) Process(img image.Image) (*image.RGBA, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return nil, ErrClosed
	}
	if img == nil {
		return nil, fmt.Errorf("%w: nil image", ErrInvalidImage)
	}
	b := img.Bounds()
	if b.Dx() <= 0 || b.Dy() <= 0 {
		return nil, fmt.Errorf("%w: empty image bounds %v", ErrInvalidImage, b)
	}
	rgba := toRGBA(img)
	return s.backend.processRGBA(rgba, s.scale)
}

// ProcessReader 从 r 解码图片（PNG/JPEG/GIF），执行超分，返回 PNG 字节。
// 图片损坏或格式不支持时返回 ErrInvalidImage。
func (s *SuperResolver) ProcessReader(r io.Reader) ([]byte, error) {
	src, _, err := image.Decode(r)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrInvalidImage, err)
	}
	out, err := s.Process(src)
	if err != nil {
		return nil, err
	}
	return encodePNG(out)
}

// ProcessBytes 对内存中的压缩图片字节执行超分，返回 PNG 字节。
func (s *SuperResolver) ProcessBytes(data []byte) ([]byte, error) {
	return s.ProcessReader(bytes.NewReader(data))
}

// ProcessFile 读取磁盘上的图片文件，执行超分，返回 PNG 字节。
// 文件不存在同样归类为 ErrInvalidImage（输入侧问题），并附带文件路径。
func (s *SuperResolver) ProcessFile(path string) ([]byte, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("%w: %s: %v", ErrInvalidImage, path, err)
	}
	defer f.Close()
	return s.ProcessReader(f)
}

// Version 返回原生库通过 sr_version 报告的版本字符串。
func (s *SuperResolver) Version() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return ""
	}
	return s.backend.version()
}

// Close 释放原生会话与动态库句柄。可重复调用，第二次起返回 nil。
func (s *SuperResolver) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return nil
	}
	s.closed = true
	return s.backend.close()
}

func encodePNG(img *image.RGBA) ([]byte, error) {
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		// 正常情况下编码内存中的 RGBA 不会失败。
		return nil, fmt.Errorf("%w: png encode: %v", ErrBackend, err)
	}
	return buf.Bytes(), nil
}

// toRGBA 把任意 image.Image 转成左上角从 (0,0) 开始、Stride == 4*w 的
// 紧凑 RGBA。不能直接把带子原点的 SubImage 指针交给 C，所以统一绘制一遍。
func toRGBA(img image.Image) *image.RGBA {
	if src, ok := img.(*image.RGBA); ok {
		b := src.Bounds()
		if b.Min.X == 0 && b.Min.Y == 0 && src.Stride == 4*b.Dx() {
			return src
		}
		dst := image.NewRGBA(image.Rect(0, 0, b.Dx(), b.Dy()))
		draw.Draw(dst, dst.Bounds(), src, b.Min, draw.Src)
		return dst
	}
	b := img.Bounds()
	dst := image.NewRGBA(image.Rect(0, 0, b.Dx(), b.Dy()))
	draw.Draw(dst, dst.Bounds(), img, b.Min, draw.Src)
	return dst
}

// resolveLibraryCandidates 按优先级汇总动态库候选路径并去重：
// 显式 WithLibrary > SUPERSR_LIBRARY 环境变量 > 平台默认库名。
//
// 一旦用户显式指定了路径（或设置了环境变量），就不再追加默认库名，
// 避免"指定的库打不开却静默使用了别的库"这种难以排查的情况。
func resolveLibraryCandidates(explicit []string) []string {
	var out []string
	seen := map[string]bool{}
	add := func(p string) {
		if p == "" || seen[p] {
			return
		}
		seen[p] = true
		out = append(out, p)
	}
	for _, p := range explicit {
		add(p)
	}
	for _, p := range filepath.SplitList(os.Getenv(LibraryEnv)) {
		add(p)
	}
	if len(out) == 0 {
		for _, name := range defaultLibraryNames() {
			add(name)
		}
	}
	return out
}
