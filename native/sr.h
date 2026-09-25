/*
 * sr.h — ABI contract between the Go binding and a native super-resolution
 * library (e.g. a Real-ESRGAN / waifu2x style engine).
 *
 * Required exported symbols:
 *   sr_load_model, sr_upscale_rgba, sr_free, sr_unload_model
 * Optional:
 *   sr_version
 *
 * Threading: the Go side serializes all calls with a mutex, so the native
 * implementation does not need to be thread-safe.
 */
#ifndef SR_H
#define SR_H

#ifdef _WIN32
#define SR_API __declspec(dllexport)
#else
#define SR_API __attribute__((visibility("default")))
#endif

#ifdef __cplusplus
extern "C" {
#endif

/* Opaque handle to a loaded model. */
typedef void *sr_model;

/* Optional. Returns a human-readable version string (static storage). */
SR_API const char *sr_version(void);

/*
 * Loads a model file with the given integer upscale factor.
 * Returns NULL on failure and writes a message into errbuf.
 */
SR_API sr_model sr_load_model(const char *path, int scale,
                              char *errbuf, int errbuf_size);

/*
 * Upscales an NRGBA image (8-bit, 4 channels, not premultiplied).
 *   rgba/width/height/stride : input pixels (stride = bytes per row)
 *   out/out_width/out_height/out_stride : on success, *out is a malloc'd
 *      buffer of (*out_stride * *out_height) bytes; caller frees with sr_free.
 * Returns 0 on success, non-zero on failure (message in errbuf).
 * The input buffer is only borrowed for the duration of the call.
 */
SR_API int sr_upscale_rgba(sr_model m,
                           const unsigned char *rgba,
                           int width, int height, int stride,
                           unsigned char **out,
                           int *out_width, int *out_height, int *out_stride,
                           char *errbuf, int errbuf_size);

/* Frees buffers returned by sr_upscale_rgba. */
SR_API void sr_free(void *p);

/* Releases a model handle. */
SR_API void sr_unload_model(sr_model m);

#ifdef __cplusplus
}
#endif

#endif /* SR_H */
