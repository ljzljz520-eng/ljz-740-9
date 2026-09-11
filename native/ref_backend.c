/*
 * ref_backend.c - 参考超分后端：加载"模型"（只要求文件存在、可读），
 * 使用双线性插值放大图像。它不做任何 AI 推理，仅用于：
 *
 *   1. 验证 Go 绑定的端到端流程（解码 -> ABI 调用 -> PNG 编码）；
 *   2. 演示 superres.h ABI 的最小实现，真实后端可直接照此替换。
 *
 * 构建（见仓库 Makefile）：
 *   gcc -DSR_BUILD_DLL -shared -fPIC -O2 ref_backend.c -o libsuperres.so
 */
#include "superres.h"

#include <stdio.h>
#include <stdlib.h>
#include <string.h>

struct sr_context {
    int scale;
    char model[256];
};

static void sr_set_error(char *errbuf, int errlen, const char *msg) {
    if (errbuf == NULL || errlen <= 0) return;
    strncpy(errbuf, msg, (size_t)(errlen - 1));
    errbuf[errlen - 1] = '\0';
}

SR_EXPORT const char *sr_version(void) {
    return "ref-bilinear ABIv1";
}

SR_EXPORT sr_context *sr_create(const char *model_path, int scale,
                                char *errbuf, int errlen) {
    if (model_path == NULL || model_path[0] == '\0') {
        sr_set_error(errbuf, errlen, "model path is empty");
        return NULL;
    }
    FILE *f = fopen(model_path, "rb");
    if (f == NULL) {
        char msg[512];
        snprintf(msg, sizeof(msg), "cannot open model file '%s'", model_path);
        sr_set_error(errbuf, errlen, msg);
        return NULL;
    }
    fclose(f);

    if (scale < 2 || scale > 8) {
        char msg[128];
        snprintf(msg, sizeof(msg), "scale %d not supported (want 2..8)", scale);
        sr_set_error(errbuf, errlen, msg);
        return NULL;
    }

    sr_context *ctx = (sr_context *)calloc(1, sizeof(sr_context));
    if (ctx == NULL) {
        sr_set_error(errbuf, errlen, "out of memory");
        return NULL;
    }
    ctx->scale = scale;
    strncpy(ctx->model, model_path, sizeof(ctx->model) - 1);
    return ctx;
}

SR_EXPORT int sr_set_scale(sr_context *ctx, int scale,
                           char *errbuf, int errlen) {
    if (ctx == NULL) {
        sr_set_error(errbuf, errlen, "null context");
        return SR_ERR_ARG;
    }
    if (scale < 2 || scale > 8) {
        char msg[128];
        snprintf(msg, sizeof(msg), "scale %d not supported (want 2..8)", scale);
        sr_set_error(errbuf, errlen, msg);
        return SR_ERR_ARG;
    }
    ctx->scale = scale;
    return SR_OK;
}

static inline unsigned char clamp_u8(int v) {
    if (v < 0) return 0;
    if (v > 255) return 255;
    return (unsigned char)v;
}

SR_EXPORT int sr_process(sr_context *ctx,
                         const unsigned char *in_pixels,
                         int in_w, int in_h, int in_stride,
                         unsigned char **out_pixels,
                         int *out_w, int *out_h, int *out_stride,
                         char *errbuf, int errlen) {
    if (ctx == NULL || in_pixels == NULL || out_pixels == NULL ||
        out_w == NULL || out_h == NULL || out_stride == NULL) {
        sr_set_error(errbuf, errlen, "null argument");
        return SR_ERR_ARG;
    }
    if (in_w <= 0 || in_h <= 0 || in_stride < in_w * 4) {
        sr_set_error(errbuf, errlen, "invalid input geometry");
        return SR_ERR_ARG;
    }

    const int s = ctx->scale;
    const int ow = in_w * s;
    const int oh = in_h * s;
    const int ostride = ow * 4;

    /* 尺寸溢出保护。 */
    if (ow / s != in_w || (long long)ostride * oh > (long long)1 << 31) {
        sr_set_error(errbuf, errlen, "output dimensions too large");
        return SR_ERR_NOMEM;
    }

    unsigned char *out = (unsigned char *)malloc((size_t)ostride * (size_t)oh);
    if (out == NULL) {
        sr_set_error(errbuf, errlen, "out of memory for output buffer");
        return SR_ERR_NOMEM;
    }

    /* 双线性重采样：输出像素中心映射回输入坐标。 */
    const double fx = (in_w > 1) ? (double)(in_w - 1) / (double)(ow - 1) : 0.0;
    const double fy = (in_h > 1) ? (double)(in_h - 1) / (double)(oh - 1) : 0.0;

    for (int y = 0; y < oh; y++) {
        double sy = (in_h > 1) ? y * fy : 0.0;
        int y0 = (int)sy;
        int y1 = y0 + 1;
        if (y1 > in_h - 1) y1 = in_h - 1;
        double ly = sy - y0;

        const unsigned char *r0 = in_pixels + (size_t)y0 * (size_t)in_stride;
        const unsigned char *r1 = in_pixels + (size_t)y1 * (size_t)in_stride;
        unsigned char *orow = out + (size_t)y * (size_t)ostride;

        for (int x = 0; x < ow; x++) {
            double sx = (in_w > 1) ? x * fx : 0.0;
            int x0 = (int)sx;
            int x1 = x0 + 1;
            if (x1 > in_w - 1) x1 = in_w - 1;
            double lx = sx - x0;

            const unsigned char *p00 = r0 + x0 * 4;
            const unsigned char *p10 = r0 + x1 * 4;
            const unsigned char *p01 = r1 + x0 * 4;
            const unsigned char *p11 = r1 + x1 * 4;

            for (int c = 0; c < 4; c++) {
                double top = p00[c] + (p10[c] - p00[c]) * lx;
                double bot = p01[c] + (p11[c] - p01[c]) * lx;
                orow[x * 4 + c] = clamp_u8((int)(top + (bot - top) * ly + 0.5));
            }
        }
    }

    *out_pixels = out;
    *out_w = ow;
    *out_h = oh;
    *out_stride = ostride;
    return SR_OK;
}

SR_EXPORT void sr_free_pixels(unsigned char *pixels) {
    free(pixels);
}

SR_EXPORT void sr_destroy(sr_context *ctx) {
    free(ctx);
}
