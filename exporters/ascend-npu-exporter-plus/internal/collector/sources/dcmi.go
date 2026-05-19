package sources

import "context"

// DCMISource is the Phase 4+ implementation reading from Huawei's DCMI
// (Device Control Management Interface) shared library. Phase 3 stub:
// returns ErrSourceNotAvailable until libdcmi.so is wired.
type DCMISource struct{}

// NewDCMISource constructs a placeholder DCMISource. Phase 4+ will accept
// driver config (library path, device filter) here.
func NewDCMISource() *DCMISource { return &DCMISource{} }

// ReadNPUs returns ErrSourceNotAvailable until Phase 4+ wires libdcmi.so.
func (s *DCMISource) ReadNPUs(ctx context.Context) ([]NPUSample, error) {
	return nil, ErrSourceNotAvailable
}
