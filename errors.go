package supersr

import "errors"

// 绑定层可以区分的错误类别。底层库返回的错误会用 fmt.Errorf("%w: ...", ErrXxx)
// 的方式包装，因此调用方可以用 errors.Is 判断。
var (
	// ErrLibraryNotFound 表示找不到原生动态库（.so / .dylib / .dll），
	// 或者指定的文件不是有效的共享库。
	ErrLibraryNotFound = errors.New("supersr: native super-resolution library not found")

	// ErrSymbolMissing 表示动态库已成功加载，但缺少绑定所需的导出函数，
	// 即库版本过旧 / 不是兼容的 superres ABI 实现。
	ErrSymbolMissing = errors.New("supersr: native library is missing required functions")

	// ErrModelNotFound 表示模型文件不存在，或不是一个可读的普通文件。
	ErrModelNotFound = errors.New("supersr: model file not found")

	// ErrInvalidImage 表示输入图片无法解码（文件损坏、格式不支持等）。
	ErrInvalidImage = errors.New("supersr: invalid or corrupted input image")

	// ErrInvalidScale 表示倍率超出了后端允许的范围。
	ErrInvalidScale = errors.New("supersr: invalid scale factor")

	// ErrBackend 表示原生后端执行超分时返回了错误（显存不足、推理失败等）。
	ErrBackend = errors.New("supersr: native backend error")

	// ErrClosed 表示 SuperResolver 已经 Close，不能再使用。
	ErrClosed = errors.New("supersr: resolver is closed")

	// ErrCGODisabled 表示构建时关闭了 CGO，无法加载任何原生库。
	ErrCGODisabled = errors.New("supersr: this build was compiled with CGO_ENABLED=0; native bindings are unavailable")
)
