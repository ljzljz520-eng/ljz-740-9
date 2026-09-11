//go:build windows

package supersr

// defaultLibraryNames 返回 Windows 平台的默认库文件名候选。
func defaultLibraryNames() []string {
	return []string{
		".\\superres.dll",
		"superres.dll",
	}
}
