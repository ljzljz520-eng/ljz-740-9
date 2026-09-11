package supersr

import "image"

// nativeBackend 是原生后端在 Go 侧的最小接口。
// 真正的实现在 backend_cgo.go（CGO）或 backend_nocgo.go（CGO_DISABLED），
// openNativeBackend 工厂函数同样由这两个带构建标签的文件提供。
type nativeBackend interface {
	setScale(scale int) error
	processRGBA(img *image.RGBA, scale int) (*image.RGBA, error)
	version() string
	close() error
}
