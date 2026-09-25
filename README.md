# supersr — Go 本地图像超分库绑定

通过 cgo + `dlopen`/`dlsym` 在**运行时**加载原生超分库（Real-ESRGAN / waifu2x 风格引擎均可，
只要导出约定符号），因此"库不存在 / 库函数缺失"都是可捕获的 Go error，而不是链接期或启动崩溃。

## 功能

- `supersr.Open(path)` — 加载原生共享库并解析必需符号
- `lib.LoadModel(modelPath, scale)` — 加载模型并设置倍率（1–16）
- `model.Upscale(img)` — 执行超分，返回 `*image.NRGBA`
- `model.UpscalePNG(img)` / `model.UpscalePNGBytes(b)` — 执行超分并返回 **PNG 字节**
- `supersr.Decode(r)` — 解码 PNG/JPEG/GIF，损坏数据映射为 `ErrImageCorrupt`
- 所有原生调用经互斥锁串行化，goroutine 安全；模型与库均有 finalizer 兜底

## 原生 ABI（native/sr.h）

必需符号：`sr_load_model`、`sr_upscale_rgba`、`sr_free`、`sr_unload_model`；可选：`sr_version`。
像素格式为 NRGBA（8bit×4 通道，非预乘），输出缓冲区由库 `malloc`、调用方用 `sr_free` 释放。

`native/sr.c` 是一个零依赖的最近邻参考实现，用于端到端跑通与测试；
接入真实引擎时按同一 ABI 导出符号即可。

## 构建与运行

```sh
# 1. 构建参考原生库（或替换为真实超分引擎的 .so）
gcc -shared -fPIC -O2 -o libsr_ref.so native/sr.c

# 2. 构建 CLI 并处理单张图片
go build -o srcli ./cmd/srcli
./srcli -lib ./libsr_ref.so -model model.bin -scale 4 -in input.png -out output.png
```

## 错误处理与退出码

| 场景             | 哨兵错误（errors.Is）     | 具体类型        | CLI 退出码 |
|------------------|---------------------------|-----------------|-----------|
| 库文件缺失/不可加载 | `ErrLibraryNotFound`      | `*LoadError`    | 2         |
| 库函数缺失        | `ErrFunctionMissing`      | `*SymbolError`  | 3         |
| 模型不存在        | `ErrModelNotFound`        | `*ModelError`   | 4         |
| 图片损坏/格式不支持 | `ErrImageCorrupt`         | —               | 5         |
| 倍率非法         | `ErrInvalidScale`         | —               | 1         |
| 原生调用失败      | `ErrNative`               | `*NativeError`  | 6         |

## 测试

```sh
go test ./supersr/        # 8 个用例：全部错误路径 + 端到端 x4 超分
go test -race ./supersr/
```
