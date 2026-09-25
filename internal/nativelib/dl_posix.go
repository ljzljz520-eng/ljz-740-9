//go:build cgo && !windows

package nativelib

/*
#include <stdlib.h>
#include "gosr_rt.h"
*/
import "C"

import (
	"os"
	"path/filepath"
	"runtime"
)

func dlError() string {
	return C.GoString(C.gosr_rt_last_error())
}

// gosrDL returns the currently bound platform handle. The C loader stores it
// in a file-static, so for Close we re-resolve via the function table is not
// possible; instead we keep the handle accessible through a tiny C accessor
// below.
func gosrDL() uintptr {
	return uintptr(C.gosr_rt_dl_handle())
}

// DefaultCandidates lists platform-standard locations for the backend, after
// the GOSR_NATIVE_LIB environment override.
func DefaultCandidates() []string {
	name := "libgosr_native.so"
	switch runtime.GOOS {
	case "darwin":
		name = "libgosr_native.dylib"
	case "freebsd":
		name = "libgosr_native.so"
	}
	exeDir := ""
	if exe, err := os.Executable(); err == nil {
		exeDir = filepath.Dir(exe)
	}
	candidates := []string{
		filepath.Join(exeDir, name),
		filepath.Join(exeDir, "..", "lib", name),
		"/usr/local/lib/" + name,
		"/usr/lib/" + name,
		"/usr/lib64/" + name,
		"/usr/local/lib/gosr/" + name,
		"/opt/gosr/lib/" + name,
	}
	if runtime.GOOS == "darwin" {
		candidates = append(candidates,
			"/opt/homebrew/lib/"+name,
			"/usr/local/opt/opencv/lib/"+name)
	}
	return candidates
}
