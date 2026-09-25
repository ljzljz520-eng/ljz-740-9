/*
 * gosr.h — stable C ABI between the Go bindings and the native super
 * resolution backend.
 *
 * The native backend (native/...) is built as a separate shared library
 * (libgosr_native.so / .dylib / gosr_native.dll) so that OpenCV is a runtime
 * dependency only: the Go package compiles with a plain C toolchain.
 */
#ifndef GOSR_H
#define GOSR_H

#include <stddef.h>
#include <stdint.h>

#ifdef __cplusplus
extern "C" {
#endif

#define GOSR_ABI_VERSION 1

/* Status codes returned by every ABI entry point. */
#define GOSR_OK              0
#define GOSR_ERR_INVALID     1   /* invalid argument / state */
#define GOSR_ERR_MODEL       2   /* model file missing or unreadable */
#define GOSR_ERR_BAD_MODEL   3   /* model file corrupt / wrong format */
#define GOSR_ERR_BAD_IMAGE   4   /* input image bytes corrupt / undecodable */
#define GOSR_ERR_NO_FUNC     5   /* backend does not provide a needed symbol */
#define GOSR_ERR_INTERNAL    9   /* unspecified backend failure */

/* Capability flags reported by gosr_supports(). */
#define GOSR_CAP_CPU  (1 << 0)
#define GOSR_CAP_CUDA (1 << 1)

typedef struct gosr_handle gosr_handle;

/* ABI handshake: native code must return GOSR_ABI_VERSION. */
int gosr_abi_version(void);

/*
 * Create / destroy a super-resolution session.
 * algorithm: "edsr", "espcn", "fsrcnn" or "lapsrn" (case sensitive).
 * scale:     upscaling factor, e.g. 2, 3, 4, 8.
 */
gosr_handle *gosr_create(const char *algorithm, int scale,
                         char *errbuf, size_t errlen);
void gosr_destroy(gosr_handle *h);

/*
 * Load model weights from disk. Must be called before upsample.
 * Returns GOSR_ERR_MODEL when the file does not exist and
 * GOSR_ERR_BAD_MODEL when it cannot be parsed by the network backend.
 */
int gosr_load_model(gosr_handle *h, const char *model_path,
                    char *errbuf, size_t errlen);

/*
 * Run super-resolution on encoded image bytes (PNG/JPEG/...).
 * On GOSR_OK *out points to malloc()ed PNG bytes of *out_len length.
 * Ownership transfers to the caller, which frees with gosr_free().
 */
int gosr_upsample(gosr_handle *h,
                  const uint8_t *in, size_t in_len,
                  uint8_t **out, size_t *out_len,
                  char *errbuf, size_t errlen);

void gosr_free(void *p);

/* Optional: returns GOSR_CAP_* bitmask; 0 if unavailable. */
int gosr_supports(int capability);

#ifdef __cplusplus
}
#endif

#endif /* GOSR_H */
