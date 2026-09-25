/*
 * gosr_rt.c — resolves ABI symbols of the gosr native backend.
 *
 * Function pointers are stored in a single translation unit; a process only
 * ever binds one backend. Opening a second library without closing the first
 * is rejected on the Go side.
 */
#include "gosr_rt.h"
#include <stdarg.h>
#include <stdio.h>
#include <string.h>
#include <stdio.h>

static gosr_dl_t g_dl;

static gosr_abi_version_fn p_abi_version;
static gosr_create_fn      p_create;
static gosr_destroy_fn     p_destroy;
static gosr_load_model_fn  p_load_model;
static gosr_upsample_fn    p_upsample;
static gosr_free_fn        p_free;
static gosr_supports_fn    p_supports;

/* Names referenced by the static-inline wrappers in gosr_rt.h. */
gosr_abi_version_fn gosr_rt_p_abi_version;
gosr_create_fn      gosr_rt_p_create;
gosr_destroy_fn     gosr_rt_p_destroy;
gosr_load_model_fn  gosr_rt_p_load_model;
gosr_upsample_fn    gosr_rt_p_upsample;
gosr_free_fn        gosr_rt_p_free;
gosr_supports_fn    gosr_rt_p_supports;

int gosr_rt_has_supports(void) { return p_supports != NULL; }

gosr_dl_t gosr_rt_dl_handle(void) { return g_dl; }

void gosr_rt_reset_pointers(void) {
    p_abi_version = NULL;
    p_create = NULL;
    p_destroy = NULL;
    p_load_model = NULL;
    p_upsample = NULL;
    p_free = NULL;
    p_supports = NULL;
    gosr_rt_p_abi_version = NULL;
    gosr_rt_p_create = NULL;
    gosr_rt_p_destroy = NULL;
    gosr_rt_p_load_model = NULL;
    gosr_rt_p_upsample = NULL;
    gosr_rt_p_free = NULL;
    gosr_rt_p_supports = NULL;
    g_dl = NULL;
}

static void errf(char *errbuf, size_t errlen, const char *fmt, ...) {
    if (errbuf == NULL || errlen == 0) return;
    va_list ap;
    va_start(ap, fmt);
    vsnprintf(errbuf, errlen, fmt, ap);
    va_end(ap);
}

int gosr_rt_bind(gosr_dl_t dl, char *errbuf, size_t errlen) {
    static const char *required[] = {
        "gosr_abi_version", "gosr_create", "gosr_destroy",
        "gosr_load_model", "gosr_upsample", "gosr_free",
    };
    void *resolved[sizeof(required) / sizeof(required[0])];
    size_t i;

    if (dl == NULL) {
        errf(errbuf, errlen, "bind called with null library handle");
        return GOSR_ERR_INVALID;
    }

    for (i = 0; i < sizeof(required) / sizeof(required[0]); i++) {
        resolved[i] = gosr_rt_sym(dl, required[i]);
        if (resolved[i] == NULL) {
#if !defined(_WIN32)
            const char *dlerr = dlerror();
#endif
            errf(errbuf, errlen, "backend missing required function: %s%s",
                 required[i],
#if !defined(_WIN32)
                 (dlerr != NULL ? dlerr : "")
#else
                 ""
#endif
            );
            return GOSR_ERR_NO_FUNC;
        }
    }

    p_abi_version = (gosr_abi_version_fn)resolved[0];
    p_create      = (gosr_create_fn)resolved[1];
    p_destroy     = (gosr_destroy_fn)resolved[2];
    p_load_model  = (gosr_load_model_fn)resolved[3];
    p_upsample    = (gosr_upsample_fn)resolved[4];
    p_free        = (gosr_free_fn)resolved[5];
    /* Optional: its absence must not be fatal. */
    p_supports    = (gosr_supports_fn)gosr_rt_sym(dl, "gosr_supports");

    gosr_rt_p_abi_version = p_abi_version;
    gosr_rt_p_create      = p_create;
    gosr_rt_p_destroy     = p_destroy;
    gosr_rt_p_load_model  = p_load_model;
    gosr_rt_p_upsample    = p_upsample;
    gosr_rt_p_free        = p_free;
    gosr_rt_p_supports    = p_supports;

    g_dl = dl;
    return GOSR_OK;
}
