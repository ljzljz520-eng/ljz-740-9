// gosr_backend.cpp — native super-resolution backend implemented on top of
// OpenCV 4's dnn_superres module (EDSR / ESPCN / FSRCNN / LapSRN models).
//
// Built as a standalone shared library; the Go bindings load it at runtime
// via the C ABI in internal/nativelib/gosr.h, so this file is the only place
// OpenCV is required.
//
// Build: see Makefile (pkg-config --cflags --libs opencv4).

#include <cstdio>
#include <cstring>
#include <fstream>
#include <memory>
#include <string>
#include <vector>

#include <opencv2/core.hpp>
#include <opencv2/imgcodecs.hpp>
#include <opencv2/dnn_superres.hpp>

#include "../internal/nativelib/gosr.h"

namespace {

void write_err(char *buf, size_t len, const std::string &msg) {
    if (!buf || len == 0) return;
    // snprintf truncates safely; keep messages short and single-line.
    std::string m = msg;
    for (char &c : m) {
        if (c == '\n' || c == '\r') c = ' ';
    }
    snprintf(buf, len, "%s", m.c_str());
}

bool file_exists(const std::string &path) {
    std::ifstream f(path, std::ios::binary);
    return f.good();
}

bool valid_algorithm(const std::string &a) {
    return a == "edsr" || a == "espcn" || a == "fsrcnn" || a == "lapsrn";
}

bool valid_scale(const std::string &a, int scale) {
    if (scale < 2 || scale > 8) return false;
    if (a == "lapsrn") return scale == 2 || scale == 4 || scale == 8;
    return scale == 2 || scale == 3 || scale == 4;
}

struct Session {
    cv::dnn_superres::DnnSuperResImpl sr;
    std::string algorithm;
    int scale = 0;
    bool model_loaded = false;
};

}  // namespace

extern "C" {

int gosr_abi_version(void) { return GOSR_ABI_VERSION; }

gosr_handle *gosr_create(const char *algorithm, int scale,
                         char *errbuf, size_t errlen) {
    if (!algorithm) {
        write_err(errbuf, errlen, "algorithm is null");
        return nullptr;
    }
    std::string algo(algorithm);
    if (!valid_algorithm(algo)) {
        write_err(errbuf, errlen, "unsupported algorithm: " + algo);
        return nullptr;
    }
    if (!valid_scale(algo, scale)) {
        write_err(errbuf, errlen,
                  "invalid scale " + std::to_string(scale) + " for " + algo);
        return nullptr;
    }
    try {
        auto *s = new Session();
        s->algorithm = algo;
        s->scale = scale;
        // EDSR ships per-scale model variants; the others carry the scale
        // in their weights but setModel is still required.
        s->sr.setModel(algo, scale);
        return reinterpret_cast<gosr_handle *>(s);
    } catch (const cv::Exception &e) {
        write_err(errbuf, errlen, std::string("create failed: ") + e.what());
        return nullptr;
    } catch (const std::exception &e) {
        write_err(errbuf, errlen, std::string("create failed: ") + e.what());
        return nullptr;
    }
}

void gosr_destroy(gosr_handle *h) {
    delete reinterpret_cast<Session *>(h);
}

int gosr_load_model(gosr_handle *h, const char *model_path,
                    char *errbuf, size_t errlen) {
    auto *s = reinterpret_cast<Session *>(h);
    if (!s || !model_path) {
        write_err(errbuf, errlen, "null handle or model path");
        return GOSR_ERR_INVALID;
    }
    std::string path(model_path);
    if (!file_exists(path)) {
        write_err(errbuf, errlen, "model file not found: " + path);
        return GOSR_ERR_MODEL;
    }
    try {
        s->sr.readModel(path);
        // setModel must follow readModel (weights are scale-specific).
        s->sr.setModel(s->algorithm, s->scale);
        s->model_loaded = true;
        return GOSR_OK;
    } catch (const cv::Exception &e) {
        write_err(errbuf, errlen,
                  std::string("cannot parse model: ") + e.what());
        return GOSR_ERR_BAD_MODEL;
    } catch (const std::exception &e) {
        write_err(errbuf, errlen,
                  std::string("cannot parse model: ") + e.what());
        return GOSR_ERR_BAD_MODEL;
    }
}

int gosr_upsample(gosr_handle *h,
                  const uint8_t *in, size_t in_len,
                  uint8_t **out, size_t *out_len,
                  char *errbuf, size_t errlen) {
    auto *s = reinterpret_cast<Session *>(h);
    if (!s || !in || in_len == 0 || !out || !out_len) {
        write_err(errbuf, errlen, "null/empty upsample argument");
        return GOSR_ERR_INVALID;
    }
    if (!s->model_loaded) {
        write_err(errbuf, errlen, "no model loaded");
        return GOSR_ERR_INVALID;
    }

    try {
        cv::Mat raw(1, static_cast<int>(in_len), CV_8UC1,
                    const_cast<uint8_t *>(in));
        cv::Mat img = cv::imdecode(raw, cv::IMREAD_COLOR);
        if (img.empty()) {
            write_err(errbuf, errlen,
                      "image decode failed: data is not a valid PNG/JPEG/...");
            return GOSR_ERR_BAD_IMAGE;
        }

        cv::Mat upscaled;
        s->sr.upsample(img, upscaled);
        if (upscaled.empty()) {
            write_err(errbuf, errlen, "super-resolution produced no image");
            return GOSR_ERR_INTERNAL;
        }

        std::vector<uint8_t> encoded;
        std::vector<int> params = {cv::IMWRITE_PNG_COMPRESSION, 6};
        if (!cv::imencode(".png", upscaled, encoded, params) || encoded.empty()) {
            write_err(errbuf, errlen, "PNG encoding failed");
            return GOSR_ERR_INTERNAL;
        }

        uint8_t *buf = static_cast<uint8_t *>(std::malloc(encoded.size()));
        if (!buf) {
            write_err(errbuf, errlen, "out of memory");
            return GOSR_ERR_INTERNAL;
        }
        std::memcpy(buf, encoded.data(), encoded.size());
        *out = buf;
        *out_len = encoded.size();
        return GOSR_OK;
    } catch (const cv::Exception &e) {
        write_err(errbuf, errlen, std::string("inference failed: ") + e.what());
        return GOSR_ERR_INTERNAL;
    } catch (const std::exception &e) {
        write_err(errbuf, errlen, std::string("inference failed: ") + e.what());
        return GOSR_ERR_INTERNAL;
    }
}

void gosr_free(void *p) { std::free(p); }

int gosr_supports(int capability) {
    switch (capability) {
        case GOSR_CAP_CPU:
            return 1;
        case GOSR_CAP_CUDA:
#ifdef GOSR_WITH_CUDA
            return 1;
#else
            return 0;
#endif
        default:
            return 0;
    }
}

}  // extern "C"
