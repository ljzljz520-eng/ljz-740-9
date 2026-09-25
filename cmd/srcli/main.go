// Command srcli upscales a single image with a native super-resolution
// library and writes the result as PNG.
//
//	srcli -lib ./libsr_ref.so -model model.bin -scale 4 -in in.png -out out.png
//
// Exit codes let scripts distinguish failure classes:
//
//	0 ok | 1 usage | 2 library missing/unloadable | 3 required symbol missing
//	4 model not found | 5 input image corrupt | 6 native call failed | 7 i/o
package main

import (
	"errors"
	"flag"
	"fmt"
	"os"

	"github.com/example/supersr/supersr"
)

const (
	exitOK = iota
	exitUsage
	exitLibrary
	exitSymbol
	exitModel
	exitImage
	exitNative
	exitIO
)

func main() {
	os.Exit(realMain())
}

func realMain() int {
	libPath := flag.String("lib", "", "path to the native super-resolution shared library (.so/.dylib)")
	modelPath := flag.String("model", "", "path to the super-resolution model file")
	scale := flag.Int("scale", 4, "integer upscale factor (1-16)")
	inPath := flag.String("in", "", "input image file (PNG/JPEG/GIF)")
	outPath := flag.String("out", "", "output PNG file")
	flag.Parse()

	if *libPath == "" || *modelPath == "" || *inPath == "" || *outPath == "" {
		fmt.Fprintln(os.Stderr, "usage: srcli -lib LIB -model MODEL -in INPUT -out OUTPUT [-scale N]")
		fmt.Fprintln(os.Stderr)
		flag.PrintDefaults()
		return exitUsage
	}

	if err := run(*libPath, *modelPath, *inPath, *outPath, *scale); err != nil {
		code, hint := classify(err)
		fmt.Fprintf(os.Stderr, "srcli: error: %v\n       hint: %s (exit %d)\n", err, hint, code)
		return code
	}
	return exitOK
}

func run(libPath, modelPath, inPath, outPath string, scale int) error {
	// 1. Load the native library (fails on missing file / missing symbols).
	lib, err := supersr.Open(libPath)
	if err != nil {
		return err
	}
	defer lib.Close()
	fmt.Fprintf(os.Stderr, "[lib]   %s (abi: %s)\n", libPath, orUnknown(lib.Version()))

	// 2. Load the model with the requested scale factor.
	model, err := lib.LoadModel(modelPath, scale)
	if err != nil {
		return err
	}
	defer model.Close()
	fmt.Fprintf(os.Stderr, "[model] %s (scale x%d)\n", modelPath, model.Scale())

	// 3. Read and decode the input image.
	raw, err := os.ReadFile(inPath)
	if err != nil {
		return fmt.Errorf("read input: %w", err)
	}
	img, err := supersr.DecodeBytes(raw)
	if err != nil {
		return err
	}
	b := img.Bounds()
	fmt.Fprintf(os.Stderr, "[input] %s: %dx%d\n", inPath, b.Dx(), b.Dy())

	// 4. Run super-resolution; the binding returns ready-to-write PNG bytes.
	pngBytes, err := model.UpscalePNG(img)
	if err != nil {
		return err
	}

	// 5. Write the output file.
	if err := os.WriteFile(outPath, pngBytes, 0o644); err != nil {
		return fmt.Errorf("write output: %w", err)
	}
	fmt.Fprintf(os.Stderr, "[done]  %s: %dx%d, %d bytes PNG\n",
		outPath, b.Dx()*scale, b.Dy()*scale, len(pngBytes))
	return nil
}

// classify maps an error to an exit code plus a human-readable hint.
func classify(err error) (int, string) {
	switch {
	case errors.Is(err, supersr.ErrLibraryNotFound):
		return exitLibrary, "native library file not found or not a loadable shared object"
	case errors.Is(err, supersr.ErrFunctionMissing):
		return exitSymbol, "the library does not export the required sr_* functions (wrong library or ABI mismatch)"
	case errors.Is(err, supersr.ErrModelNotFound):
		return exitModel, "model file does not exist; check the -model path"
	case errors.Is(err, supersr.ErrInvalidScale):
		return exitUsage, "scale must be an integer between 1 and 16"
	case errors.Is(err, supersr.ErrImageCorrupt):
		return exitImage, "input is not a valid PNG/JPEG/GIF image (file corrupt or unsupported format)"
	case errors.Is(err, supersr.ErrNative):
		return exitNative, "the native library reported a failure"
	default:
		return exitIO, "unexpected i/o error"
	}
}

func orUnknown(s string) string {
	if s == "" {
		return "unknown"
	}
	return s
}
