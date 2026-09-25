/*
 * gosr_rt.h — runtime loader for the gosr native backend.
 *
 * The shared library is opened with dlopen()/LoadLibrary() instead of being
 * linked at build time, so:
 *   1. the Go package builds without OpenCV installed;
 *   2. a missing/incompatible backend becomes an ordinary Go error;
 *   3. a backend missing one of the required symbols is reported as
 *      "function missing" instead of a linker failure.
 *
 * The small static inline wrappers exist because cgo cannot call C function
 * pointers directly.
 */
#ifndef GOSR_RT_H
#define GOSR_RT_H

#include <stddef.h>
#include <stdint.h>
#include "gosr.h"

/* ---- platform dynamic-loader glue ---- */
#if defined(_WIN32)
#include <windows.h>
#include <stdio.h>
typedef HMODULE gosr_dl_t;
#define GOSR_DLOPEN(path) LoadLibraryA(path)
#define GOSR_DLSYM(handle, name) (void *)(uintptr_t)GetProcAddress((HMODULE)(handle), (name))
#define GOSR_DLCLOSE(handle) FreeLibrary((HMODULE)(handle))

static inline gosr_dl_t gosr_rt_open(const char *path) {
    return LoadLibraryA(path);
}
static inline void *gosr_rt_sym(gosr_dl_t handle, const char *name) {
    return (void *)(uintptr_t)GetProcAddress((HMODULE)(handle), name);
}
static inline void gosr_rt_close(gosr_dl_t handle) {
    FreeLibrary((HMODULE)(handle));
}
static inline const char *gosr_rt_last_error(void) {
    static char buf[256];
    unsigned long code = GetLastError();
    if (code == 0) return "LoadLibrary failed";
    snprintf(buf, sizeof(buf), "LoadLibrary failed, GetLastError=%lu", code);
    return buf;
}
#else
#include <dlfcn.h>
typedef void *gosr_dl_t;
#define GOSR_DLOPEN(path) dlopen((path), RTLD_NOW | RTLD_LOCAL)
#define GOSR_DLSYM(handle, name) dlsym((handle), (name))
#define GOSR_DLCLOSE(handle) dlclose((handle))

static inline gosr_dl_t gosr_rt_open(const char *path) {
    return dlopen(path, RTLD_NOW | RTLD_LOCAL);
}
static inline void *gosr_rt_sym(gosr_dl_t handle, const char *name) {
    return dlsym(handle, name);
}
static inline void gosr_rt_close(gosr_dl_t handle) {
    dlclose(handle);
}
static inline const char *gosr_rt_last_error(void) {
    const char *e = dlerror();
    return e ? e : "dlopen failed";
}
#endif

typedef int (*gosr_abi_version_fn)(void);
typedef gosr_handle *(*gosr_create_fn)(const char *, int, char *, size_t);
typedef void (*gosr_destroy_fn)(gosr_handle *);
typedef int (*gosr_load_model_fn)(gosr_handle *, const char *, char *, size_t);
typedef int (*gosr_upsample_fn)(gosr_handle *, const uint8_t *, size_t,
                                uint8_t **, size_t *, char *, size_t);
typedef void (*gosr_free_fn)(void *);
typedef int (*gosr_supports_fn)(int);

/* Resolved function pointers, populated by gosr_rt_bind(). */
extern gosr_abi_version_fn gosr_rt_p_abi_version;
extern gosr_create_fn      gosr_rt_p_create;
extern gosr_destroy_fn     gosr_rt_p_destroy;
extern gosr_load_model_fn  gosr_rt_p_load_model;
extern gosr_upsample_fn    gosr_rt_p_upsample;
extern gosr_free_fn        gosr_rt_p_free;
extern gosr_supports_fn    gosr_rt_p_supports;

/*
 * Bind all mandatory symbols from an already opened library.
 * Returns GOSR_OK, GOSR_ERR_NO_FUNC (a required symbol was not found),
 * or GOSR_ERR_INVALID (dl == NULL). On failure no symbols are trusted.
 */
int gosr_rt_bind(gosr_dl_t dl, char *errbuf, size_t errlen);

/* Returns the platform dynamic-loader handle passed to the last bind, or 0. */
gosr_dl_t gosr_rt_dl_handle(void);

/* Drops resolved function pointers; defined in gosr_rt.c. */
void gosr_rt_reset_pointers(void);

/* Closes the currently bound backend handle (used from Go). */
static inline void gosr_rt_close_bound(void) {
    gosr_rt_close(gosr_rt_dl_handle());
    gosr_rt_reset_pointers();
}

/* Reports whether the optional gosr_supports symbol resolved. */
int gosr_rt_has_supports(void);

static inline int gosr_call_abi_version(void) {
    return gosr_rt_p_abi_version();
}
static inline gosr_handle *gosr_call_create(const char *algorithm, int scale,
                                            char *errbuf, size_t errlen) {
    return gosr_rt_p_create(algorithm, scale, errbuf, errlen);
}
static inline void gosr_call_destroy(gosr_handle *h) {
    gosr_rt_p_destroy(h);
}
static inline int gosr_call_load_model(gosr_handle *h, const char *path,
                                       char *errbuf, size_t errlen) {
    return gosr_rt_p_load_model(h, path, errbuf, errlen);
}
static inline int gosr_call_upsample(gosr_handle *h,
                                     const uint8_t *in, size_t in_len,
                                     uint8_t **out, size_t *out_len,
                                     char *errbuf, size_t errlen) {
    return gosr_rt_p_upsample(h, in, in_len, out, out_len, errbuf, errlen);
}
static inline void gosr_call_free(void *p) { gosr_rt_p_free(p); }
static inline int gosr_call_supports(int cap) {
    if (!gosr_rt_has_supports()) return 0;
    return gosr_rt_p_supports(cap);
}

#endif /* GOSR_RT_H */
