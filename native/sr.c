/*
 * sr.c — reference implementation of the sr.h ABI.
 *
 * A dependency-free nearest-neighbour upscaler. It exists so the Go binding
 * can be built, tested and run end-to-end without shipping a real neural
 * network; swap it for a real engine (Real-ESRGAN, waifu2x, ...) exposing
 * the same symbols.
 *
 * Build:  gcc -shared -fPIC -O2 -o libsr_ref.so sr.c
 * Build a "broken" variant missing sr_upscale_rgba (for error-path tests):
 *         gcc -shared -fPIC -O2 -DSR_OMIT_UPSCALE -o libsr_broken.so sr.c
 */
#include <stdio.h>
#include <stdlib.h>
#include <string.h>

#include "sr.h"

typedef struct {
    int scale;
} simple_model;

const char *sr_version(void) { return "sr-reference/1.0 (nearest-neighbour)"; }

static void set_err(char *buf, int size, const char *msg) {
    if (buf && size > 0) {
        snprintf(buf, (size_t)size, "%s", msg);
    }
}

sr_model sr_load_model(const char *path, int scale, char *errbuf, int errbuf_size) {
    if (scale < 1 || scale > 16) {
        set_err(errbuf, errbuf_size, "scale out of range (1..16)");
        return NULL;
    }
    /* A real engine would parse weights here; we just require the file. */
    FILE *f = fopen(path, "rb");
    if (!f) {
        set_err(errbuf, errbuf_size, "cannot open model file");
        return NULL;
    }
    fclose(f);

    simple_model *m = (simple_model *)malloc(sizeof(simple_model));
    if (!m) {
        set_err(errbuf, errbuf_size, "out of memory");
        return NULL;
    }
    m->scale = scale;
    return (sr_model)m;
}

#ifndef SR_OMIT_UPSCALE
int sr_upscale_rgba(sr_model handle,
                    const unsigned char *rgba, int width, int height, int stride,
                    unsigned char **out,
                    int *out_width, int *out_height, int *out_stride,
                    char *errbuf, int errbuf_size) {
    simple_model *m = (simple_model *)handle;
    if (!m || !rgba || !out || width <= 0 || height <= 0 || stride < width * 4) {
        set_err(errbuf, errbuf_size, "invalid argument");
        return 1;
    }
    int s = m->scale;
    int ow = width * s, oh = height * s, os = ow * 4;
    unsigned char *dst = (unsigned char *)malloc((size_t)os * (size_t)oh);
    if (!dst) {
        set_err(errbuf, errbuf_size, "out of memory");
        return 2;
    }
    for (int y = 0; y < oh; y++) {
        const unsigned char *srow = rgba + (size_t)(y / s) * (size_t)stride;
        unsigned char *drow = dst + (size_t)y * (size_t)os;
        for (int x = 0; x < ow; x++) {
            const unsigned char *sp = srow + (size_t)(x / s) * 4;
            unsigned char *dp = drow + (size_t)x * 4;
            dp[0] = sp[0];
            dp[1] = sp[1];
            dp[2] = sp[2];
            dp[3] = sp[3];
        }
    }
    *out = dst;
    *out_width = ow;
    *out_height = oh;
    *out_stride = os;
    return 0;
}
#endif /* SR_OMIT_UPSCALE */

void sr_free(void *p) { free(p); }

void sr_unload_model(sr_model m) { free(m); }
