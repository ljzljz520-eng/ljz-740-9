/*
 * loader.h - 跨平台动态库加载与符号解析的薄封装，以及对后端函数指针的
 * 跳板调用（Go 侧无法直接通过 CGO 调用 C 函数指针，需要 C 跳板）。
 */
#ifndef SUPERSR_LOADER_H
#define SUPERSR_LOADER_H

#include <stddef.h>
#include "superres.h"

#ifdef __cplusplus
extern "C" {
#endif

/* 解析后的后端函数表。 */
typedef struct sr_api {
    void *handle;
    const char *(*version)(void);
    sr_context *(*create)(const char *, int, char *, int);
    int (*set_scale)(sr_context *, int, char *, int);
    int (*process)(sr_context *, const unsigned char *, int, int, int,
                   unsigned char **, int *, int *, int *, char *, int);
    void (*free_pixels)(unsigned char *);
    void (*destroy)(sr_context *);
} sr_api;

/*
 * sr_load_library 按顺序尝试候选路径加载动态库，全部失败时返回 NULL，
 * 并把各路径的失败原因（dlerror/GetLastError）拼接写入 errbuf。
 */
void *sr_load_library(char **candidates, int count, char *errbuf, int errlen);

/*
 * sr_load_symbols 解析全部必需符号。缺少任意一个符号时返回 -1 并在
 * missing 中写入第一个缺失的符号名；成功返回 0。
 */
int sr_load_symbols(void *handle, sr_api *api, char *missing, int missing_len);

void sr_unload(void *handle);

/* ---- C 跳板：Go 通过这些静态包装间接调用函数指针 ---- */
const char *sr_call_version(sr_api *api);
sr_context *sr_call_create(sr_api *api, const char *path, int scale,
                           char *errbuf, int errlen);
int sr_call_set_scale(sr_api *api, sr_context *ctx, int scale,
                      char *errbuf, int errlen);
int sr_call_process(sr_api *api, sr_context *ctx,
                    const unsigned char *in, int w, int h, int stride,
                    unsigned char **out, int *ow, int *oh, int *ostride,
                    char *errbuf, int errlen);
void sr_call_free_pixels(sr_api *api, unsigned char *p);
void sr_call_destroy(sr_api *api, sr_context *ctx);

#ifdef __cplusplus
}
#endif

#endif /* SUPERSR_LOADER_H */
