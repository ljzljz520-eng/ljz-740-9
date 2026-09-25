//go:build cgo && windows

package nativelib

/*
#include "gosr_rt.h"
*/
import "C"

import (
	"os"
	"path/filepath"
)

func dlError() string {
	return C.GoString(C.gosr_rt_last_error())
}

func gosrDL() uintptr {
	return uintptr(C.gosr_rt_dl_handle())
}

func DefaultCandidates() []string {
	const name = "gosr_native.dll"
	exeDir := ""
	if exe, err := os.Executable(); err == nil {
		exeDir = filepath.Dir(exe)
	}
	return []string{
		filepath.Join(exeDir, name),
		filepath.Join(exeDir, "..", "lib", name),
		`C:\Program Files\gosr\bin\` + name,
		`C:\Program Files\gosr\lib\` + name,
	}
}
