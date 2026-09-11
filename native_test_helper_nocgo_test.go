//go:build !cgo

package supersr

import "testing"

func testLibrary(t *testing.T) string {
	t.Skip("CGO 禁用，无法加载原生库")
	return ""
}

func buildDummyLib(t *testing.T) string {
	t.Skip("CGO 禁用，无法编译原生库")
	return ""
}
