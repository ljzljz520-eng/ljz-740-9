# supersr — Go 本地图像超分辨率原生库绑定

纯 Go API + CGO 动态绑定：运行时加载一个实现固定 C ABI 的本地超分库
（Real-ESRGAN / Real-CUGAN / ncnn 等引擎的封装均可），完成
**加载模型 → 传入图片 → 设置倍率 → 执行超分 → 返回 PNG 字节** 的完整流程。
绑定层负责图片解码（PNG/JPEG/GIF）、RGBA 转换、内存管理与错误分类。

> 仓库自带一个 **参考后端**（`native/ref_backend.c`，双线性放大，非 AI 推理），
> 用于开箱即用地演示与测试；真实引擎只需实现同一组 C 导出函数即可热替换。

## 特性

- 运行时 `dlopen` / `LoadLibrary` 加载，**不与任何推理框架编译期绑定**；
- 支持加载模型、设置倍率（x2~x8，可运行时修改）、图像输入、PNG 字节输出；
- 输入支持 PNG / JPEG / GIF（Go 标准库注册的格式均可）；
- 完整错误分类，`errors.Is` 可判定：
  - `ErrModelNotFound` — 模型不存在 / 是目录 / 不可读；
  - `ErrInvalidImage` — 图片不存在、损坏或格式不支持；
  - `ErrLibraryNotFound` — 原生动态库找不到 / 不是有效共享库；
  - `ErrSymbolMissing` — 库缺少绑定所需的导出函数（ABI 不兼容）；
  - `ErrInvalidScale` / `ErrBackend` / `ErrClosed` / `ErrCGODisabled`；
- `CGO_ENABLED=0` 也能编译，调用时明确返回 `ErrCGODisabled`；
- 输出缓冲由后端分配、Go 拷贝后立即通过 `sr_free_pixels` 归还，无泄漏。

## 目录结构

```
supersr/
├── go.mod
├── supersr.go                 # 公共 API：Open / Process / SetScale / Close
├── errors.go                  # 全部哨兵错误
├── backend.go                 # nativeBackend 接口
├── backend_cgo.go             # CGO 动态加载 + 函数指针跳板（!cgo 时排除）
├── backend_nocgo.go           # CGO_ENABLED=0 的明确报错实现
├── library_{unix,windows}.go  # 平台默认库名
├── loader.c                   # dlopen/LoadLibrary 封装 + C 跳板
├── native/
│   ├── superres.h             # 后端必须实现的稳定 C ABI（6 个导出）
│   ├── loader.h
│   └── ref_backend.c          # 参考后端：模型校验 + 双线性放大
└── cmd/
    ├── sr/main.go             # 命令行示例（处理单张图片）
    └── genimg/main.go         # 生成测试 PNG
```

## 快速开始

```bash
make            # 编译 libsuperres.so 和 bin/sr
make demo       # 端到端演示
```

手动执行：

```bash
# 1. 编译参考原生库
gcc -DSR_BUILD_DLL -shared -fPIC -O2 -o libsuperres.so native/ref_backend.c

# 2. 生成一张测试图，造一个"模型文件"（参考后端只检查文件存在）
./bin/genimg -o photo.png -w 40 -h 30
echo "fake weights" > model.bin

# 3. x2 超分，输出 PNG
./bin/sr -model model.bin -input photo.png -output photo_2x.png \
         -scale 2 -lib ./libsuperres.so -version
# 后端版本: ref-bilinear ABIv1
# 完成: photo.png -> photo_2x.png (xxxx 字节, 倍率 x2, 后端 ref-bilinear ABIv1)
```

CLI 参数：

| 参数 | 说明 |
| --- | --- |
| `-model` | 模型文件路径（必填） |
| `-input` | 输入图片 PNG/JPEG/GIF（必填） |
| `-output` | 输出 PNG 路径（必填） |
| `-scale` | 倍率 2~8（默认 2，可运行期由库校验） |
| `-lib` | 显式指定动态库路径（默认查 `SUPERSR_LIBRARY`、当前目录与系统路径） |
| `-version` | 打印后端版本字符串 |

预期错误以退出码 **2** 区分（模型/图片/库问题），其他错误退出码 1。

## 作为库使用

```go
import "supersr"

r, err := supersr.Open("models/realesrgan-x4.bin",
    supersr.WithScale(4),
    supersr.WithLibrary("/usr/local/lib/libsuperres.so"), // 可选
)
if err != nil {
    switch {
    case errors.Is(err, supersr.ErrModelNotFound):   // …
    case errors.Is(err, supersr.ErrLibraryNotFound): // …
    case errors.Is(err, supersr.ErrSymbolMissing):   // …
    }
}
defer r.Close()

fmt.Println(r.Version()) // 后端版本

// 文件 -> PNG 字节
pngBytes, err := r.ProcessFile("photo.jpg")

// 任意 io.Reader / 字节
pngBytes, err = r.ProcessReader(file)
pngBytes, err = r.ProcessBytes(data)

// 或传入已解码的 image.Image，拿到 *image.RGBA（尺寸 = 原图 × 倍率）
out, err := r.Process(img)

// 运行期改倍率
if err := r.SetScale(2); err != nil { /* … */ }
```

### 动态库查找顺序

1. `WithLibrary(...)` 显式候选（可多个）；
2. `SUPERSR_LIBRARY` 环境变量（`:` / `;` 分隔多个）；
3. 平台默认名：
   - Linux：`./libsuperres.so`、`libsuperres.so`（后者走 `LD_LIBRARY_PATH`/rpath/系统目录）；
   - macOS：`./libsuperres.dylib`、`libsuperres.dylib`；
   - Windows：`superres.dll`。

显式指定过路径时不会回退到默认库，避免静默加载错误的库。

## 接入真实超分引擎

让你的共享库导出 `native/superres.h` 中的 6 个 C 函数即可，无需改动 Go 代码：

| 导出函数 | 职责 |
| --- | --- |
| `sr_version` | 返回 ABI/引擎版本字符串 |
| `sr_create` | 打开模型、创建上下文（模型缺失返回 `SR_ERR_MODEL`） |
| `sr_set_scale` | 切换倍率（不支持可返回 `SR_ERR_UNSUPPORTED`） |
| `sr_process` | RGBA 输入 → RGBA 输出（输出用 `malloc` 分配并回填宽高/stride） |
| `sr_free_pixels` | 释放输出缓冲 |
| `sr_destroy` | 销毁上下文 |

像素约定：8-bit straight-alpha RGBA；输入/输出均通过 stride 描述行跨度。
状态码 `SR_OK / SR_ERR_ARG / SR_ERR_MODEL / SR_ERR_NOMEM / SR_ERR_INFER /
SR_ERR_UNSUPPORTED` 会被绑定层映射到对应的 Go 错误类别。错误描述可写入
`errbuf`（NUL 结尾），会原样出现在 Go 错误信息里。

典型接入方式：用 C/C++ 实现这层薄壳，内部调用 ncnn / ONNX Runtime /
Real-ESRGAN 的推理 API，编译成 `libsuperres.so`。

## 测试

```bash
make test          # CGO 测试（自动用 gcc 编译参考库）
go test -race ./…  # 竞态检测
CGO_ENABLED=0 go build ./...   # 无 CGO 构建验证
```

测试覆盖：候选库解析与优先级、模型不存在、库不存在、库缺符号、
损坏图片、端到端放大尺寸、运行期改倍率、Close 幂等、带偏移子图。

## 平台

Linux（已测试）、macOS、Windows（`LoadLibraryA`/`GetProcAddress`，
mingw 构建参考 DLL）。需要可用的 C 编译器（CGO）。
