# gosr — Go 本地图像超分辨率绑定

`gosr` 是一个**纯本地运行**的图像超分辨率（Super-Resolution）Go 库绑定。
底层基于 OpenCV 4 的 `dnn_superres` 模块，支持 `EDSR / ESPCN / FSRCNN / LapSRN`
系列模型；原生后端编译为独立动态库，在运行时通过 `dlopen` 加载，因此：

- 编译 Go 包**不需要安装 OpenCV**，只需要 C 工具链；
- 后端缺失、函数符号缺失、ABI 版本不符都是普通 Go 错误，而非链接/崩溃；
- 模型推理完全离线，不发起任何网络请求。

## 功能

| 能力 | API |
| --- | --- |
| 加载/打开原生后端 | `gosr.OpenBackend(path)`、`gosr.Default()` |
| 创建会话 | `b.NewResolver("espcn", 4)` / `gosr.New(...)` |
| 加载模型 | `(*SuperResolver).LoadModel(path)` |
| 设置倍率 | `(*SuperResolver).SetScale(4)`（已加载模型会自动重新加载） |
| 传入图片 | `UpsampleImage(image.Image)` / `UpsampleBytes([]byte)` |
| 执行超分 | 同上，内部完成 decode → DNN 推理 → PNG 编码 |
| 返回 PNG 字节 | `[]byte`（可直接 `os.WriteFile`） |

## 架构

```
Go 应用 / gosr-cli
        │
  包 gosr (resolver.go, backend.go)        ← 对外 API、错误分类
        │
internal/nativelib (cgo + dlopen)          ← 稳定 C ABI、运行时装载
        │
libgosr_native.so / .dylib / .dll          ← C++ 后端（唯一依赖 OpenCV 处）
        └── opencv2/dnn_superres (EDSR/ESPCN/FSRCNN/LapSRN)
```

C ABI 定义见 `internal/nativelib/gosr.h`，带版本号（`GOSR_ABI_VERSION`），
版本不匹配时返回 `gosr.ErrBackendABIMismatch`。

## 构建

### 1. 编译原生后端（需要 OpenCV 4 + contrib）

```bash
# Debian/Ubuntu
sudo apt install libopencv-dev
# macOS
brew install opencv
# Fedora
sudo dnf install opencv opencv-contrib

make native        # 生成 libgosr_native.so / .dylib
sudo make install  # 安装到 /usr/local/lib 与 /usr/local/bin
```

不想安装到系统目录时，用环境变量指定后端位置：

```bash
export GOSR_NATIVE_LIB=$PWD/libgosr_native.so
```

### 2. 编译 CLI

```bash
make cli           # bin/gosr-cli
```

### 3. 运行测试（不需要 OpenCV）

测试会用纯 C 编译三个 mock 动态库（完整功能 / 缺少符号 / ABI 不符），
端到端验证绑定与错误路径：

```bash
make test          # 等价于 CC=/usr/bin/gcc go test ./...
```

## 命令行示例：处理单张图片

```bash
export GOSR_NATIVE_LIB=$PWD/libgosr_native.so

bin/gosr-cli \
  -algo espcn \
  -scale 4 \
  -model ./models/ESPCN_x4.pb \
  -in ./photo.png \
  -out ./photo_x4.png \
  -v
```

参数：

- `-algo`：`edsr | espcn | fsrcnn | lapsrn`
- `-scale`：倍率。edsr/espcn/fsrcnn 支持 2/3/4；lapsrn 支持 2/4/8
- `-model`：模型权重（`.pb` / `.onnx`）
- `-in` / `-out`：输入图片（PNG/JPEG 等）与输出 PNG；省略 `-out` 时自动命名
- `-backend`：显式指定动态库路径（默认读 `$GOSR_NATIVE_LIB` 与系统库目录）

退出码：`0` 成功；`3` 后端缺失/函数缺失/ABI 不符；`4` 模型不存在；
`5` 模型损坏；`6` 图片损坏。

模型权重可从 OpenCV 官方仓库获取：

- <https://github.com/fannymonori/TF-ESPCN>
- <https://github.com/Saafke/FSRCNN_Tensorflow>
- <https://github.com/Saafke/EDSR_Tensorflow>
- <https://github.com/fannymonori/TF-LapSRN>

## 作为库使用

```go
package main

import (
	"errors"
	"log"
	"os"

	"github.com/solo-manager/gosr"
)

func main() {
	r, err := gosr.LoadModelFile(gosr.ESPCN, 4, "models/ESPCN_x4.pb")
	if err != nil {
		log.Fatal(err)
	}
	defer r.Close()

	pngBytes, err := r.ResolveFile("photo.png")
	switch {
	case errors.Is(err, gosr.ErrModelNotFound):
		log.Fatal("模型不存在")
	case errors.Is(err, gosr.ErrModelCorrupt):
		log.Fatal("模型损坏")
	case errors.Is(err, gosr.ErrImageCorrupt):
		log.Fatal("图片损坏")
	case errors.Is(err, gosr.ErrBackendFunctionMissing):
		log.Fatal("原生库函数缺失，请重新编译 native/")
	case err != nil:
		log.Fatal(err)
	}

	if err := os.WriteFile("photo_x4.png", pngBytes, 0o644); err != nil {
		log.Fatal(err)
	}

	// 运行时切换倍率（会自动重新加载已设置的模型）
	if err := r.SetScale(2); err != nil {
		log.Fatal(err)
	}
}
```

也可以直接传 `image.Image`：

```go
out, err := r.UpsampleImage(img) // img image.Image -> PNG []byte
```

## 错误处理

| 哨兵错误 | 触发场景 |
| --- | --- |
| `ErrBackendNotFound` | 找不到 `libgosr_native` 动态库 |
| `ErrBackendFunctionMissing` | 动态库缺少绑定所需的导出函数（如裁剪过的库） |
| `ErrBackendABIMismatch` | 后端与绑定的 ABI 版本不一致 |
| `ErrModelNotFound` | 模型路径不存在 / 是目录 / 不可读 |
| `ErrModelCorrupt` | 模型文件存在但无法被网络后端解析 |
| `ErrImageCorrupt` | 输入字节为空或无法解码为图片 |
| `ErrInvalidScale` / `ErrInvalidAlgorithm` | 倍率或算法参数非法 |
| `ErrClosed` | 在 `Close` 之后继续使用 |

所有错误都可用 `errors.Is` 判断；后端返回的具体诊断信息通过 `%w` 包装保留。

## 线程安全

单个 `*SuperResolver` 的调用互斥（可在多 goroutine 间共享，内部加锁）。
进程同一时间只能加载一个后端动态库；如需切换后端，先 `Backend.Close()`。

## License

MIT
