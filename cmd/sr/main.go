// 命令行示例：使用 supersr 绑定对单张图片做超分并输出 PNG。
//
// 用法：
//
//	sr -model model.bin -input photo.jpg -output photo_2x.png [-scale 2] [-lib ./libsuperres.so]
package main

import (
	"errors"
	"flag"
	"fmt"
	"os"
	"strings"

	"supersr"
)

// refBackendVersionPrefix 是参考（非 AI 推理）后端的版本串约定前缀，
// 与 native/superres.h 中 sr_version 的说明保持一致
// （仓库自带后端报告 "ref-bilinear ABIv1"）。
const refBackendVersionPrefix = "ref-"

func main() {
	model := flag.String("model", "", "模型文件路径（必填）")
	input := flag.String("input", "", "输入图片路径（PNG/JPEG/GIF，必填）")
	output := flag.String("output", "", "输出 PNG 路径（必填）")
	scale := flag.Int("scale", supersr.DefaultScale, "放大倍率（2~8）")
	library := flag.String("lib", "", "原生动态库路径（可选，默认按 SUPERSR_LIBRARY / 系统路径查找）")
	showVersion := flag.Bool("version", false, "打印后端版本后退出")
	requireInference := flag.Bool("require-inference", false,
		"要求加载真实 AI 推理后端；若加载到双线性参考后端则直接报错退出")
	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "sr - 本地图像超分命令行示例\n\n")
		fmt.Fprintf(os.Stderr, "用法: %s -model 模型 -input 输入图 -output 输出图 [-scale 2] [-lib 动态库]\n\n", os.Args[0])
		flag.PrintDefaults()
	}
	flag.Parse()

	if *model == "" || *input == "" || *output == "" {
		flag.Usage()
		os.Exit(2)
	}

	var opts []supersr.Option
	opts = append(opts, supersr.WithScale(*scale))
	if *library != "" {
		opts = append(opts, supersr.WithLibrary(*library))
	}

	r, err := supersr.Open(*model, opts...)
	if err != nil {
		fatal("初始化超分会话", err)
	}
	defer r.Close()

	backendVersion := r.Version()
	if isReferenceBackend(backendVersion) {
		if *requireInference {
			fmt.Fprintf(os.Stderr,
				"错误: 当前加载的是非 AI 推理的参考后端（版本 %q），仅做双线性放大。\n"+
					"  请用 -lib 指定真实推理引擎的动态库（实现 native/superres.h ABI），或设置 SUPERSR_LIBRARY。\n",
				backendVersion)
			os.Exit(2)
		}
		// 防止开箱示例被误认为跑了 AI 超分：显著提示当前是参考后端，
		// 并指明如何替换成真实引擎。
		fmt.Fprintf(os.Stderr,
			"⚠ 注意: 当前使用的是非 AI 推理的参考后端（版本 %q），输出仅为双线性插值放大。\n"+
				"  需要真实超分效果时，请用 -lib 指定 AI 推理动态库（Real-ESRGAN / Real-CUGAN / ncnn 等的 ABI 封装）。\n\n",
			backendVersion)
	}

	if *showVersion {
		fmt.Printf("后端版本: %s\n", backendVersion)
	}

	pngBytes, err := r.ProcessFile(*input)
	if err != nil {
		fatal("处理图片", err)
	}

	if err := os.WriteFile(*output, pngBytes, 0o644); err != nil {
		fmt.Fprintf(os.Stderr, "错误: 写出结果失败: %v\n", err)
		os.Exit(1)
	}

	inInfo, _ := os.Stat(*input)
	fmt.Fprintf(os.Stderr, "完成: %s -> %s (%d 字节, 倍率 x%d, 后端 %s)\n",
		*input, *output, len(pngBytes), r.Scale(), backendVersion)
	_ = inInfo
}

// isReferenceBackend 根据后端报告的版本串判断它是否为非 AI 推理的参考后端。
// 真实引擎不应使用 "ref-" 前缀（见 native/superres.h 的 sr_version 约定）。
func isReferenceBackend(version string) bool {
	return strings.HasPrefix(strings.TrimSpace(version), refBackendVersionPrefix)
}

// fatal 把三类可预期错误翻译成清晰的中文提示，并使用退出码 2 区分
// （模型不存在 / 图片损坏 / 库或其函数缺失），其余错误退出码 1。
func fatal(stage string, err error) {
	switch {
	case errors.Is(err, supersr.ErrModelNotFound):
		fmt.Fprintf(os.Stderr, "错误: 模型不存在或不可读\n  阶段: %s\n  详情: %v\n", stage, err)
		os.Exit(2)
	case errors.Is(err, supersr.ErrInvalidImage):
		fmt.Fprintf(os.Stderr, "错误: 输入图片不存在、已损坏或格式不受支持\n  阶段: %s\n  详情: %v\n", stage, err)
		os.Exit(2)
	case errors.Is(err, supersr.ErrLibraryNotFound):
		fmt.Fprintf(os.Stderr, "错误: 找不到原生超分动态库（可用 -lib 指定或设置 SUPERSR_LIBRARY）\n  阶段: %s\n  详情: %v\n", stage, err)
		os.Exit(2)
	case errors.Is(err, supersr.ErrSymbolMissing):
		fmt.Fprintf(os.Stderr, "错误: 动态库缺少绑定所需的导出函数（ABI 不兼容）\n  阶段: %s\n  详情: %v\n", stage, err)
		os.Exit(2)
	case errors.Is(err, supersr.ErrCGODisabled):
		fmt.Fprintf(os.Stderr, "错误: 当前程序在 CGO_ENABLED=0 下构建，无法加载原生库\n  详情: %v\n", err)
		os.Exit(2)
	case errors.Is(err, supersr.ErrInvalidScale):
		fmt.Fprintf(os.Stderr, "错误: 非法倍率（允许 %d~%d）\n  详情: %v\n", supersr.MinScale, supersr.MaxScale, err)
		os.Exit(2)
	default:
		fmt.Fprintf(os.Stderr, "错误: %s失败: %v\n", stage, err)
		os.Exit(1)
	}
}
