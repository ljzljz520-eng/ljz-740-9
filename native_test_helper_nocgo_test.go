//go:build !cgo

package supersr

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

// cgoEnabled 供与构建标签无关的测试选择对应的预期错误类别。
const cgoEnabled = false

func testLibrary(t *testing.T) string {
	t.Skip("CGO 禁用，无法加载原生库")
	return ""
}

func buildDummyLib(t *testing.T) string {
	t.Skip("CGO 禁用，无法编译原生库")
	return ""
}

// TestOpenReturnsCGODisabled 验证无 CGO 构建下 Open 的错误分类：
// 即使模型存在、显式指定了库路径，也必须返回 ErrCGODisabled，
// 且不返回任何可用会话。
func TestOpenReturnsCGODisabled(t *testing.T) {
	model := filepath.Join(t.TempDir(), "model.bin")
	if err := os.WriteFile(model, []byte("fake"), 0o644); err != nil {
		t.Fatal(err)
	}

	r, err := Open(model, WithLibrary(filepath.Join(t.TempDir(), "libnope.so")))
	if !errors.Is(err, ErrCGODisabled) {
		t.Fatalf("want ErrCGODisabled, got %v", err)
	}
	if r != nil {
		t.Fatalf("CGO 禁用时不应返回会话, got %v", r)
	}
}
