//go:build localembed

package local

import (
	"fmt"
	"sync"

	ort "github.com/yalue/onnxruntime_go"
)

// mE5 (Xenova quantized export) takes three INT64 inputs and produces the
// token-level last_hidden_state.
var (
	ortInputNames  = []string{"input_ids", "attention_mask", "token_type_ids"}
	ortOutputNames = []string{"last_hidden_state"}
)

// ortInitOnce guards process-wide ONNX Runtime environment initialization.
var ortInitOnce sync.Once
var ortInitErr error

// ortSession is the real onnxSession implementation backed by ONNX Runtime.
type ortSession struct {
	sess *ort.DynamicAdvancedSession
}

// newORTSession initializes the ONNX Runtime environment (once per process) and
// creates a session for the given model. threads caps intra-op parallelism so
// background indexing does not peg the machine (<= 0 leaves the runtime default).
func newORTSession(dylibPath, modelPath string, threads int) (onnxSession, error) {
	ortInitOnce.Do(func() {
		ort.SetSharedLibraryPath(dylibPath)
		if !ort.IsInitialized() {
			ortInitErr = ort.InitializeEnvironment()
		}
	})
	if ortInitErr != nil {
		return nil, fmt.Errorf("local: init ONNX Runtime: %w", ortInitErr)
	}

	opts, err := ort.NewSessionOptions()
	if err != nil {
		return nil, fmt.Errorf("local: session options: %w", err)
	}
	defer opts.Destroy()
	if threads > 0 {
		if err := opts.SetIntraOpNumThreads(threads); err != nil {
			return nil, fmt.Errorf("local: set intra-op threads: %w", err)
		}
	}

	sess, err := ort.NewDynamicAdvancedSession(modelPath, ortInputNames, ortOutputNames, opts)
	if err != nil {
		return nil, fmt.Errorf("local: create session: %w", err)
	}
	return &ortSession{sess: sess}, nil
}

// run builds padded 2-D tensors for the batch, executes the model, and
// mean-pools each sequence's token embeddings over its attention mask. It
// returns one (un-normalized) pooled vector per input row; L2 normalization is
// applied by the caller.
func (s *ortSession) run(inputIDs, attnMask, typeIDs [][]int64) ([][]float32, error) {
	n := len(inputIDs)
	if n == 0 {
		return [][]float32{}, nil
	}
	maxLen := len(inputIDs[0])

	flatIDs := flatten(inputIDs, n, maxLen)
	flatMask := flatten(attnMask, n, maxLen)
	flatType := flatten(typeIDs, n, maxLen)

	shape := ort.NewShape(int64(n), int64(maxLen))

	tIDs, err := ort.NewTensor(shape, flatIDs)
	if err != nil {
		return nil, fmt.Errorf("local: input_ids tensor: %w", err)
	}
	defer tIDs.Destroy()
	tMask, err := ort.NewTensor(shape, flatMask)
	if err != nil {
		return nil, fmt.Errorf("local: attention_mask tensor: %w", err)
	}
	defer tMask.Destroy()
	tType, err := ort.NewTensor(shape, flatType)
	if err != nil {
		return nil, fmt.Errorf("local: token_type_ids tensor: %w", err)
	}
	defer tType.Destroy()

	outputs := []ort.Value{nil}
	if err := s.sess.Run([]ort.Value{tIDs, tMask, tType}, outputs); err != nil {
		return nil, fmt.Errorf("local: run: %w", err)
	}
	out, ok := outputs[0].(*ort.Tensor[float32])
	if !ok {
		return nil, fmt.Errorf("local: unexpected output tensor type %T", outputs[0])
	}
	defer out.Destroy()

	data := out.GetData()
	total := n * maxLen
	if total == 0 {
		return nil, fmt.Errorf("local: empty output")
	}
	dim := len(data) / total
	if dim*total != len(data) {
		return nil, fmt.Errorf("local: output size %d not divisible by n*maxLen=%d", len(data), total)
	}

	// Reshape per sequence into [maxLen][dim] and mean-pool over the mask.
	pooled := make([][]float32, n)
	for i := 0; i < n; i++ {
		hidden := make([][]float32, maxLen)
		for j := 0; j < maxLen; j++ {
			base := (i*maxLen + j) * dim
			hidden[j] = data[base : base+dim]
		}
		pooled[i] = meanPool(hidden, attnMask[i])
	}
	return pooled, nil
}

func (s *ortSession) close() error {
	if s.sess != nil {
		s.sess.Destroy()
		s.sess = nil
	}
	return nil
}

// flatten concatenates n rows of length maxLen into a single row-major buffer.
func flatten(rows [][]int64, n, maxLen int) []int64 {
	flat := make([]int64, n*maxLen)
	for i := 0; i < n; i++ {
		copy(flat[i*maxLen:], rows[i])
	}
	return flat
}
