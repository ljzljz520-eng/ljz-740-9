/*
 * superres.h - 图像超分辨率原生后端的稳定 C ABI。
 *
 * Go 绑定在运行时 dlopen/LoadLibrary 一个实现本头文件约定的共享库
 * （Linux: libsuperres.so, macOS: libsuperres.dylib, Windows: superres.dll），
 * 并按符号名解析下列函数。参考实现见 native/ref_backend.c。
 *
 * 所有像素数据均为 8 位预乘无关（straight alpha）的 RGBA，逐行紧密排列：
 * 第 y 行第 x 个像素位于 data[(y*stride + x*4) .. +3]。
 *
 * 错误处理约定：函数返回 int 状态码；失败时若 errbuf 非 NULL，后端应把
 * NUL 结尾的错误描述写入其中（不超过 errlen-1 字节）。
 */
#ifndef SUPERSR_SUPERRES_H
#define SUPERSR_SUPERRES_H

#include <stddef.h>

#ifdef __cplusplus
extern "C" {
#endif

#if defined(_WIN32) && defined(SR_BUILD_DLL)
#  define SR_EXPORT __declspec(dllexport)
#else
#  define SR_EXPORT
#endif

#define SR_ABI_VERSION 1

/* 返回状态码 */
#define SR_OK            0
#define SR_ERR_ARG       1  /* 参数非法（倍率不支持、尺寸为 0 等） */
#define SR_ERR_MODEL     2  /* 模型文件缺失或无法加载 */
#define SR_ERR_NOMEM     3  /* 内存 / 显存不足 */
#define SR_ERR_INFER     4  /* 推理执行失败 */
#define SR_ERR_UNSUPPORTED 5 /* 该后端不支持的操作 */

typedef struct sr_context sr_context;

/*
 * sr_version 返回 ABI 版本与后端描述字符串，例如
 * "ref-bilinear ABIv1"。返回的字符串由后端持有，调用方不得释放。
 *
 * 命名约定：仅供开发/测试、不执行 AI 推理的占位或参考后端必须以
 * "ref-" 前缀开头（如 "ref-bilinear ABIv1"）；真实推理引擎的版本串
 * 不得使用该前缀，以便调用方（如 cmd/sr）识别并提示当前并非 AI 超分。
 */
SR_EXPORT const char *sr_version(void);

/*
 * sr_create 加载模型并创建推理上下文。
 *   model_path : 模型文件路径（参考后端只要求是普通文件）
 *   scale      : 初始倍率
 *   errbuf     : 可选的错误描述缓冲区
 */
SR_EXPORT sr_context *sr_create(const char *model_path, int scale,
                                char *errbuf, int errlen);

/* 运行期修改倍率。不支持时可返回 SR_ERR_UNSUPPORTED。 */
SR_EXPORT int sr_set_scale(sr_context *ctx, int scale,
                           char *errbuf, int errlen);

/*
 * sr_process 执行超分。
 *   in_pixels/in_w/in_h/in_stride : 输入 RGBA
 *   out_* : 输出参数，后端用 malloc/calloc 分配；调用方通过 sr_free_pixels 释放
 */
SR_EXPORT int sr_process(sr_context *ctx,
                         const unsigned char *in_pixels,
                         int in_w, int in_h, int in_stride,
                         unsigned char **out_pixels,
                         int *out_w, int *out_h, int *out_stride,
                         char *errbuf, int errlen);

/* 释放 sr_process 返回的像素缓冲。 */
SR_EXPORT void sr_free_pixels(unsigned char *pixels);

/* 销毁上下文。ctx 为 NULL 时为空操作。 */
SR_EXPORT void sr_destroy(sr_context *ctx);

#ifdef __cplusplus
}
#endif

#endif /* SUPERSR_SUPERRES_H */
