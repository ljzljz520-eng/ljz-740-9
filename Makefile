# supersr 构建脚本
#
#   make            构建参考动态库 + 命令行工具
#   make lib        只构建参考后端 libsuperres.so
#   make test       运行全部 Go 测试（自动编译参考库）
#   make demo       端到端演示：生成测试图 -> x2 超分
#   make clean      清理产物

GO ?= go
CC = gcc
CFLAGS := -O2 -Wall -Wextra -fPIC -shared

LIB_UNIX := libsuperres.so
LIB_MAC  := libsuperres.dylib
LIB_WIN  := superres.dll

BIN_DIR := bin

.PHONY: all lib test demo clean vet

all: lib $(BIN_DIR)/sr $(BIN_DIR)/genimg

lib: $(LIB_UNIX)

$(LIB_UNIX): native/ref_backend.c native/superres.h
	$(CC) $(CFLAGS) -DSR_BUILD_DLL -o $@ native/ref_backend.c

$(LIB_MAC): native/ref_backend.c native/superres.h
	$(CC) $(CFLAGS) -DSR_BUILD_DLL -o $@ native/ref_backend.c

$(LIB_WIN): native/ref_backend.c native/superres.h
	$(CC) -O2 -Wall -shared -DSR_BUILD_DLL \
		-Wl,--out-implib,libsuperres.a -o $@ native/ref_backend.c

$(BIN_DIR)/sr: $(wildcard *.go) go.mod cmd/sr/main.go
	CGO_ENABLED=1 $(GO) build -o $@ ./cmd/sr

$(BIN_DIR)/genimg: go.mod cmd/genimg/main.go
	$(GO) build -o $@ ./cmd/genimg

vet:
	CGO_ENABLED=1 $(GO) vet ./...

test:
	CGO_ENABLED=1 $(GO) test ./...

demo: all
	$(BIN_DIR)/genimg -o /tmp/sr_input.png -w 40 -h 30
	@echo "fake model weights" > /tmp/sr_model.bin
	$(BIN_DIR)/sr -model /tmp/sr_model.bin \
		-input /tmp/sr_input.png -output /tmp/sr_output_x2.png \
		-scale 2 -lib ./$(LIB_UNIX) -version

clean:
	rm -rf $(BIN_DIR) $(LIB_UNIX) $(LIB_MAC) $(LIB_WIN) libsuperres.a
