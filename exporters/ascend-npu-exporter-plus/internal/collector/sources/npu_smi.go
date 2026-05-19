package sources

import "context"

// NPUSMISource is the Phase 4+ implementation reading from Huawei's
// npu-smi CLI tool (stdout parsing). Phase 3 stub: returns
// ErrSourceNotAvailable. Phase 4+ wires npu-smi CLI parsing.
type NPUSMISource struct{}

// NewNPUSMISource constructs a placeholder NPUSMISource. Phase 4+ will
// accept invocation config (binary path, refresh interval) here.
func NewNPUSMISource() *NPUSMISource { return &NPUSMISource{} }

// ReadNPUs returns ErrSourceNotAvailable until Phase 4+ wires npu-smi CLI
// parsing.
func (s *NPUSMISource) ReadNPUs(ctx context.Context) ([]NPUSample, error) {
	return nil, ErrSourceNotAvailable
}
