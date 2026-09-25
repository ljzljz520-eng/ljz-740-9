// Command gosr-cli super-resolves a single local image using a DNN model
// (EDSR/ESPCN/FSRCNN/LapSRN) and writes the result as PNG.
//
// Usage:
//
//	gosr-cli -model ESPCN_x4.pb -algo espcn -scale 4 \
//	         -in photo.png -out photo_x4.png
//
// The native backend (libgosr_native) is located through $GOSR_NATIVE_LIB
// or the platform's standard library paths.
package main

import (
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/solo-manager/gosr"
)

// Exit codes let scripts distinguish the required error classes.
const (
	exitOK            = 0
	exitUsage         = 2
	exitBackend       = 3
	exitModelNotFound = 4
	exitModelCorrupt  = 5
	exitImageCorrupt  = 6
	exitOther         = 1
)

func main() {
	model := flag.String("model", "", "path to super-resolution model (.pb/.onnx)")
	algo := flag.String("algo", "espcn", "algorithm: edsr|espcn|fsrcnn|lapsrn")
	scale := flag.Int("scale", 4, "upscale factor (2/3/4/8 depending on model)")
	in := flag.String("in", "", "input image path (PNG/JPEG/...)")
	out := flag.String("out", "", "output PNG path (default: <in>_<algo>x<scale>.png)")
	backendPath := flag.String("backend", os.Getenv("GOSR_NATIVE_LIB"),
		"path to native backend shared library (defaults to $GOSR_NATIVE_LIB)")
	verbose := flag.Bool("v", false, "print backend capabilities")
	flag.Parse()

	if *model == "" || *in == "" {
		usage("both -model and -in are required")
		os.Exit(exitUsage)
	}

	code := run(*backendPath, *algo, *scale, *model, *in, *out, *verbose)
	os.Exit(code)
}

func run(backendPath, algo string, scale int, modelPath, inPath, outPath string, verbose bool) int {
	var (
		b   *gosr.Backend
		err error
	)
	if backendPath != "" {
		b, err = gosr.OpenBackend(backendPath)
	} else {
		b, err = gosr.Default()
	}
	if err != nil {
		return failBackend(err)
	}
	defer b.Close()

	if verbose {
		fmt.Fprintf(os.Stderr, "backend: cpu=%v cuda=%v supports_symbol=%v\n",
			b.SupportsCPU(), b.SupportsCUDA(), b.HasFunction("gosr_supports"))
	}

	r, err := b.NewResolver(strings.ToLower(algo), scale)
	if err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		switch {
		case errors.Is(err, gosr.ErrInvalidScale), errors.Is(err, gosr.ErrInvalidAlgorithm):
			return exitUsage
		default:
			return exitOther
		}
	}
	defer r.Close()

	if err := r.LoadModel(modelPath); err != nil {
		return failModel(err)
	}

	pngBytes, err := r.ResolveFile(inPath)
	if err != nil {
		return failImage(err)
	}

	if outPath == "" {
		ext := filepath.Ext(inPath)
		base := strings.TrimSuffix(inPath, ext)
		outPath = fmt.Sprintf("%s_%sx%d.png", base, algo, scale)
	}
	if err := os.WriteFile(outPath, pngBytes, 0o644); err != nil {
		fmt.Fprintln(os.Stderr, "error: cannot write output:", err)
		return exitOther
	}
	fmt.Fprintf(os.Stderr, "ok: %s -> %s (%d PNG bytes, %s x%d)\n",
		inPath, outPath, len(pngBytes), algo, scale)
	return exitOK
}

func usage(msg string) {
	fmt.Fprintln(os.Stderr, "error:", msg)
	flag.Usage()
}

func failBackend(err error) int {
	fmt.Fprintln(os.Stderr, "error: native backend unavailable:", err)
	switch {
	case errors.Is(err, gosr.ErrBackendFunctionMissing):
		fmt.Fprintln(os.Stderr, "hint: a library function the bindings need is "+
			"missing; rebuild the native backend from native/.")
	case errors.Is(err, gosr.ErrBackendABIMismatch):
		fmt.Fprintln(os.Stderr, "hint: backend and bindings versions differ; rebuild both.")
	default:
		fmt.Fprintln(os.Stderr, "hint: build native/ (make native) or set GOSR_NATIVE_LIB.")
	}
	return exitBackend
}

func failModel(err error) int {
	fmt.Fprintln(os.Stderr, "error:", err)
	switch {
	case errors.Is(err, gosr.ErrModelNotFound):
		fmt.Fprintln(os.Stderr, "hint: check the -model path.")
		return exitModelNotFound
	case errors.Is(err, gosr.ErrModelCorrupt):
		fmt.Fprintln(os.Stderr, "hint: the file exists but is not a valid model "+
			"for the chosen algorithm.")
		return exitModelCorrupt
	default:
		return exitOther
	}
}

func failImage(err error) int {
	fmt.Fprintln(os.Stderr, "error:", err)
	if errors.Is(err, gosr.ErrImageCorrupt) {
		fmt.Fprintln(os.Stderr, "hint: provide a valid PNG/JPEG input.")
		return exitImageCorrupt
	}
	return exitOther
}
