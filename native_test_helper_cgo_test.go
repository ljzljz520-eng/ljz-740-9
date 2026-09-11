//go:build cgo

package supersr

import (
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"testing"
)

var (
	buildSharedOnce sync.Once
	sharedLibPath   string
	sharedLibErr    error
)

// buildTestLibraries 用系统 cc 编译参考后端与一个"空壳"共享库。
// 若环境没有 C 编译器，测试会被 Skip。
func buildTestLibraries(t *testing.T) (ref, dummy string) {
	t.Helper()
	buildSharedOnce.Do(func() {
		dir, err := os.MkdirTemp("", "supersr-libs-")
		if err != nil {
			sharedLibErr = err
			return
		}
		ref = filepath.Join(dir, "libsuperres.so")
		src := filepath.Join("native", "ref_backend.c")
		if cmd := exec.Command("gcc", "-DSR_BUILD_DLL", "-shared", "-fPIC", "-O2",
			"-o", ref, src); cmd.Run() != nil {
			sharedLibErr = cmd.Err
		}
		sharedLibPath = ref
	})
	if sharedLibErr != nil || sharedLibPath == "" {
		t.Skipf("无法构建参考共享库: %v", sharedLibErr)
	}

	dummySrc := filepath.Join(t.TempDir(), "dummy.c")
	dummy = filepath.Join(t.TempDir(), "libdummy.so")
	if err := os.WriteFile(dummySrc, []byte("int sr_unrelated(void){return 0;}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := exec.Command("gcc", "-shared", "-fPIC", "-o", dummy, dummySrc).Run(); err != nil {
		t.Skipf("无法构建空壳共享库: %v", err)
	}
	return sharedLibPath, dummy
}

func testLibrary(t *testing.T) string {
	ref, _ := buildTestLibraries(t)
	return ref
}

func buildDummyLib(t *testing.T) string {
	_, dummy := buildTestLibraries(t)
	return dummy
}
