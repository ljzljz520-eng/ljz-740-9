package gosr

import (
	"bytes"
	"errors"
	"fmt"
	"image"
	"image/png"
	"os"
	"path/filepath"
	"sync"

	"github.com/solo-manager/gosr/internal/nativelib"
)

// Algorithms supported by the OpenCV dnn_superres backend.
const (
	EDSR   = "edsr"
	ESPCN  = "espcn"
	FSRCNN = "fsrcnn"
	LapSRN = "lapsrn"
)

// validScales lists the scale factors each model family ships weights for.
var validScales = map[string]map[int]bool{
	EDSR:   {2: true, 3: true, 4: true},
	ESPCN:  {2: true, 3: true, 4: true},
	FSRCNN: {2: true, 3: true, 4: true},
	LapSRN: {2: true, 4: true, 8: true},
}

// SuperResolver performs local super-resolution against one loaded model.
type SuperResolver struct {
	mu        sync.Mutex
	backend   *Backend
	algorithm string
	scale     int
	handle    nativelib.Handle
	modelPath string
	closed    bool
}

// New creates a super-resolution session on the default backend.
//
// algorithm is one of EDSR/ESPCN/FSRCNN/LapSRN; scale is the upscaling
// factor (2/3/4/8, depending on the algorithm).
func New(algorithm string, scale int) (*SuperResolver, error) {
	b, err := Default()
	if err != nil {
		return nil, err
	}
	return b.NewResolver(algorithm, scale)
}

// NewResolver creates a session on an explicitly opened backend.
func (b *Backend) NewResolver(algorithm string, scale int) (*SuperResolver, error) {
	if b == nil || b.lib == nil {
		return nil, errors.New("gosr: backend is closed")
	}
	if !validScales[algorithm][scaleKey(scale)] {
		if _, ok := validScales[algorithm]; !ok {
			return nil, fmt.Errorf("%w: %q (want edsr/espcn/fsrcnn/lapsrn)",
				ErrInvalidAlgorithm, algorithm)
		}
		return nil, fmt.Errorf("%w: %s does not support x%d",
			ErrInvalidScale, algorithm, scale)
	}
	h, err := b.lib.Create(algorithm, scale)
	if err != nil {
		return nil, translateNative(err)
	}
	return &SuperResolver{
		backend:   b,
		algorithm: algorithm,
		scale:     scale,
		handle:    h,
	}, nil
}

func scaleKey(scale int) int { return scale }

// Algorithm and Scale accessors.
func (r *SuperResolver) Algorithm() string { return r.algorithm }
func (r *SuperResolver) Scale() int        { return r.scale }

// LoadModel loads model weights from disk. It must be called before Upsample.
func (r *SuperResolver) LoadModel(modelPath string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if err := r.checkOpenLocked(); err != nil {
		return err
	}
	// Surface the required "model does not exist" class in Go as well, so
	// callers get the same error regardless of backend behavior.
	if fi, err := os.Stat(modelPath); err != nil {
		if os.IsNotExist(err) {
			return fmt.Errorf("%w: %s", ErrModelNotFound, modelPath)
		}
		return fmt.Errorf("%w: cannot stat %s: %v", ErrModelNotFound, modelPath, err)
	} else if fi.IsDir() {
		return fmt.Errorf("%w: %s is a directory", ErrModelNotFound, modelPath)
	}
	if err := r.backend.lib.LoadModel(r.handle, modelPath); err != nil {
		return translateNative(err)
	}
	r.modelPath = modelPath
	return nil
}

// SetScale changes the upscaling factor. If a model is already loaded it is
// reloaded (the scale is baked into the weights); otherwise the new scale is
// ready for the next LoadModel.
func (r *SuperResolver) SetScale(scale int) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if err := r.checkOpenLocked(); err != nil {
		return err
	}
	if !validScales[r.algorithm][scale] {
		return fmt.Errorf("%w: %s does not support x%d",
			ErrInvalidScale, r.algorithm, scale)
	}
	if scale == r.scale {
		return nil
	}

	newHandle, err := r.backend.lib.Create(r.algorithm, scale)
	if err != nil {
		return translateNative(err)
	}
	reloaded := ""
	if r.modelPath != "" {
		if err := r.backend.lib.LoadModel(newHandle, r.modelPath); err != nil {
			r.backend.lib.Destroy(newHandle)
			return translateNative(err)
		}
		reloaded = r.modelPath
	}

	old := r.handle
	r.handle = newHandle
	r.scale = scale
	r.backend.lib.Destroy(old)
	if reloaded != "" {
		r.modelPath = reloaded
	}
	return nil
}

// UpsampleImage encodes img as PNG, sends it through the backend and returns
// the super-resolved PNG bytes.
func (r *SuperResolver) UpsampleImage(img image.Image) ([]byte, error) {
	if img == nil {
		return nil, fmt.Errorf("%w: nil image", ErrImageCorrupt)
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		return nil, fmt.Errorf("%w: cannot re-encode input: %v", ErrImageCorrupt, err)
	}
	return r.UpsampleBytes(buf.Bytes())
}

// UpsampleBytes sends encoded image bytes (PNG/JPEG/...) to the backend and
// returns the super-resolved PNG.
func (r *SuperResolver) UpsampleBytes(input []byte) ([]byte, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if err := r.checkOpenLocked(); err != nil {
		return nil, err
	}
	if len(input) == 0 {
		return nil, fmt.Errorf("%w: empty input", ErrImageCorrupt)
	}
	if r.modelPath == "" {
		return nil, errors.New("gosr: call LoadModel before Upsample")
	}
	out, err := r.backend.lib.Upsample(r.handle, input)
	if err != nil {
		return nil, translateNative(err)
	}
	return out, nil
}

// Close releases the native session; subsequent calls fail with ErrClosed.
func (r *SuperResolver) Close() error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.closed {
		return nil
	}
	r.closed = true
	r.backend.lib.Destroy(r.handle)
	r.handle = nil
	return nil
}

func (r *SuperResolver) checkOpenLocked() error {
	if r.closed || r.backend == nil || r.backend.lib == nil || r.handle == nil {
		return ErrClosed
	}
	return nil
}

// LoadModelFile is a convenience: open the default backend, create a session,
// and load the model in one call. The caller must Close the resolver.
func LoadModelFile(algorithm string, scale int, modelPath string) (*SuperResolver, error) {
	if _, err := os.Stat(modelPath); err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("%w: %s", ErrModelNotFound, modelPath)
		}
		return nil, fmt.Errorf("%w: cannot stat %s: %v",
			ErrModelNotFound, modelPath, err)
	}
	r, err := New(algorithm, scale)
	if err != nil {
		return nil, err
	}
	if err := r.LoadModel(modelPath); err != nil {
		_ = r.Close()
		return nil, err
	}
	return r, nil
}

// ResolveFile super-resolves an image file and returns PNG bytes.
func (r *SuperResolver) ResolveFile(imagePath string) ([]byte, error) {
	data, err := os.ReadFile(imagePath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("%w: input image not found: %s",
				ErrImageCorrupt, imagePath)
		}
		return nil, fmt.Errorf("%w: cannot read %s: %v",
			ErrImageCorrupt, filepath.Base(imagePath), err)
	}
	return r.UpsampleBytes(data)
}
