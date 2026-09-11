//go:build !windows

package supersr

// defaultLibraryNames 返回 POSIX 平台的默认库文件名候选。
// dlopen 对裸 soname 会自动搜索 LD_LIBRARY_PATH、rpath、系统目录；
// 也会尝试可执行文件旁边的 ./ 路径（由 loader 按字符串原样 dlopen）。
func defaultLibraryNames() []string {
	return []string{
		"./libsuperres.so",
		"libsuperres.so",
		"./libsuperres.dylib",
		"libsuperres.dylib",
	}
}
