#include "native/loader.h"

#include <stdio.h>
#include <string.h>

#if defined(_WIN32)

#include <windows.h>

void *sr_load_library(char **candidates, int count, char *errbuf, int errlen) {
    if (errbuf != NULL && errlen > 0) errbuf[0] = '\0';
    for (int i = 0; i < count; i++) {
        HMODULE h = LoadLibraryA(candidates[i]);
        if (h != NULL) {
            return (void *)h;
        }
        if (errbuf != NULL && errlen > 1) {
            char msg[256];
            snprintf(msg, sizeof(msg), "%s: LoadLibrary error %lu; ",
                     candidates[i], (unsigned long)GetLastError());
            strncat(errbuf, msg, (size_t)(errlen - 1 - (int)strlen(errbuf)));
        }
    }
    return NULL;
}

int sr_load_symbols(void *handle, sr_api *api, char *missing, int missing_len) {
    HMODULE h = (HMODULE)handle;
    struct { const char *name; void **slot; } syms[] = {
        {"sr_version",     (void **)&api->version},
        {"sr_create",      (void **)&api->create},
        {"sr_set_scale",   (void **)&api->set_scale},
        {"sr_process",     (void **)&api->process},
        {"sr_free_pixels", (void **)&api->free_pixels},
        {"sr_destroy",     (void **)&api->destroy},
    };
    for (size_t i = 0; i < sizeof(syms) / sizeof(syms[0]); i++) {
        void *p = (void *)GetProcAddress(h, syms[i].name);
        if (p == NULL) {
            if (missing != NULL && missing_len > 0)
                strncpy(missing, syms[i].name, (size_t)(missing_len - 1));
            return -1;
        }
        *syms[i].slot = p;
    }
    api->handle = handle;
    return 0;
}

void sr_unload(void *handle) {
    if (handle != NULL) FreeLibrary((HMODULE)handle);
}

#else /* POSIX */

#include <dlfcn.h>

void *sr_load_library(char **candidates, int count, char *errbuf, int errlen) {
    if (errbuf != NULL && errlen > 0) errbuf[0] = '\0';
    for (int i = 0; i < count; i++) {
        (void)dlerror();
        void *h = dlopen(candidates[i], RTLD_NOW | RTLD_LOCAL);
        if (h != NULL) {
            return h;
        }
        if (errbuf != NULL && errlen > 1) {
            const char *e = dlerror();
            char msg[512];
            snprintf(msg, sizeof(msg), "%s: %s; ", candidates[i],
                     e != NULL ? e : "unknown error");
            strncat(errbuf, msg, (size_t)(errlen - 1 - (int)strlen(errbuf)));
        }
    }
    return NULL;
}

int sr_load_symbols(void *handle, sr_api *api, char *missing, int missing_len) {
    struct { const char *name; void **slot; } syms[] = {
        {"sr_version",     (void **)&api->version},
        {"sr_create",      (void **)&api->create},
        {"sr_set_scale",   (void **)&api->set_scale},
        {"sr_process",     (void **)&api->process},
        {"sr_free_pixels", (void **)&api->free_pixels},
        {"sr_destroy",     (void **)&api->destroy},
    };
    for (size_t i = 0; i < sizeof(syms) / sizeof(syms[0]); i++) {
        (void)dlerror();
        void *p = dlsym(handle, syms[i].name);
        if (p == NULL) {
            if (missing != NULL && missing_len > 0)
                strncpy(missing, syms[i].name, (size_t)(missing_len - 1));
            return -1;
        }
        *syms[i].slot = p;
    }
    api->handle = handle;
    return 0;
}

void sr_unload(void *handle) {
    if (handle != NULL) dlclose(handle);
}

#endif /* platform */

/* ---- 跳板实现 ---- */

const char *sr_call_version(sr_api *api) {
    return api->version();
}

sr_context *sr_call_create(sr_api *api, const char *path, int scale,
                           char *errbuf, int errlen) {
    return api->create(path, scale, errbuf, errlen);
}

int sr_call_set_scale(sr_api *api, sr_context *ctx, int scale,
                      char *errbuf, int errlen) {
    return api->set_scale(ctx, scale, errbuf, errlen);
}

int sr_call_process(sr_api *api, sr_context *ctx,
                    const unsigned char *in, int w, int h, int stride,
                    unsigned char **out, int *ow, int *oh, int *ostride,
                    char *errbuf, int errlen) {
    return api->process(ctx, in, w, h, stride, out, ow, oh, ostride,
                        errbuf, errlen);
}

void sr_call_free_pixels(sr_api *api, unsigned char *p) {
    api->free_pixels(p);
}

void sr_call_destroy(sr_api *api, sr_context *ctx) {
    api->destroy(ctx);
}
