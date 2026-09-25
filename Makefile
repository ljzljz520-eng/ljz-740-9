# gosr — Go bindings for local image super-resolution.
#
# `make native` builds the OpenCV-based shared library the bindings load at
# runtime. The Go packages themselves build and test without OpenCV.

GO ?= go
CC ?= cc
CXX ?= c++

GOOS ?= $(shell $(GO) env GOOS)

ifeq ($(GOOS),darwin)
NATIVE_LIB := libgosr_native.dylib
OPENCV_CFLAGS := $(shell pkg-config --cflags opencv4 2>/dev/null)
OPENCV_LIBS   := $(shell pkg-config --libs opencv4 2>/dev/null)
EXTRA_LDFLAGS := -Wl,-install_name,@rpath/libgosr_native.dylib
else ifeq ($(GOOS),windows)
NATIVE_LIB := gosr_native.dll
OPENCV_CFLAGS := $(shell pkg-config --cflags opencv4 2>/dev/null)
OPENCV_LIBS   := $(shell pkg-config --libs opencv4 2>/dev/null) -lws2_32
EXTRA_LDFLAGS :=
else
NATIVE_LIB := libgosr_native.so
OPENCV_CFLAGS := $(shell pkg-config --cflags opencv4 2>/dev/null)
OPENCV_LIBS   := $(shell pkg-config --libs opencv4 2>/dev/null)
EXTRA_LDFLAGS := -Wl,-soname,libgosr_native.so
endif

# Set CUSE_CUDA=1 to build the backend with CUDA capability reporting.
ifdef WITH_CUDA
CUDA_DEFINE := -DGOSR_WITH_CUDA=1
endif

.PHONY: all native cli test check fmt vet clean install

all: native cli

native: $(NATIVE_LIB)

$(NATIVE_LIB): native/gosr_backend.cpp internal/nativelib/gosr.h
	@pkg-config --exists opencv4 || \
	  (echo "error: OpenCV 4 (with contrib dnn_superres) is required to build the native backend.\n" \
	       "  Debian/Ubuntu: apt install libopencv-dev\n" \
	       "  macOS:         brew install opencv\n" \
	       "  Fedora:        dnf install opencv opencv-contrib\n" >&2; exit 1)
	$(CXX) -std=c++17 -O2 -fPIC -shared $(CUDA_DEFINE) \
	  $(OPENCV_CFLAGS) native/gosr_backend.cpp \
	  -o $(NATIVE_LIB) $(OPENCV_LIBS) $(EXTRA_LDFLAGS)
	@echo "built $(NATIVE_LIB)"

cli:
	$(GO) build -o bin/gosr-cli ./cmd/gosr-cli

# Tests build tiny mock backends; point CC at a real C compiler if needed.
test:
	$(GO) test ./...

check: vet test

vet:
	$(GO) vet ./...

fmt:
	$(GO) fmt ./...

install: native cli
	install -d $(DESTDIR)/usr/local/lib $(DESTDIR)/usr/local/bin
	install -m 0755 $(NATIVE_LIB) $(DESTDIR)/usr/local/lib/
	install -m 0755 bin/gosr-cli $(DESTDIR)/usr/local/bin/

clean:
	rm -f libgosr_native.so libgosr_native.dylib gosr_native.dll
	rm -rf bin
