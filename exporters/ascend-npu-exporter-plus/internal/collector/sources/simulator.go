package sources

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"math/rand"
	"os"
	"sync"
	"time"
)

// simulatorFile is the on-disk format. Each entry is a single NPU with
// seed values (utilization%, memory used bytes, HBM bw bytes/s). The
// simulator emits the seed +/- a sine-wave perturbation each ReadNPUs
// call so dashboards show movement.
type simulatorFile struct {
	NPUs []simulatorNPU `json:"npus"`
}

type simulatorNPU struct {
	ID                  string  `json:"id"`
	NodeName            string  `json:"nodeName"`
	Model               string  `json:"model"`
	UtilizationSeed     float64 `json:"utilizationSeed"`     // 0-100
	MemoryUsedSeedBytes uint64  `json:"memoryUsedSeedBytes"`
	MemoryTotalBytes    uint64  `json:"memoryTotalBytes"`
	HBMBandwidthSeedBPS uint64  `json:"hbmBandwidthSeedBytesPerSecond"`
	Healthy             bool    `json:"healthy"`
}

// SimulatorSource replays an on-disk JSON snapshot with sine-wave
// perturbation. Closes the spirit of known-issue #9 (dashboards see
// movement, not flat lines).
type SimulatorSource struct {
	path  string
	mu    sync.Mutex
	npus  []simulatorNPU
	start time.Time
	rng   *rand.Rand
}

// NewSimulatorSource loads NPUs from a JSON file on disk. Returns an
// error if the file is missing or malformed.
func NewSimulatorSource(path string) (*SimulatorSource, error) {
	s := &SimulatorSource{
		path:  path,
		start: time.Now(),
		rng:   rand.New(rand.NewSource(time.Now().UnixNano())),
	}
	if err := s.load(); err != nil {
		return nil, err
	}
	return s, nil
}

func (s *SimulatorSource) load() error {
	data, err := os.ReadFile(s.path)
	if err != nil {
		return fmt.Errorf("read simulator file %q: %w", s.path, err)
	}
	var f simulatorFile
	if err := json.Unmarshal(data, &f); err != nil {
		return fmt.Errorf("parse simulator file %q: %w", s.path, err)
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.npus = f.NPUs
	return nil
}

// ReadNPUs returns one perturbed NPUSample per loaded entry. Sine-wave
// perturbation stays within +/- 10% of each seed.
func (s *SimulatorSource) ReadNPUs(ctx context.Context) ([]NPUSample, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	out := make([]NPUSample, 0, len(s.npus))
	elapsed := time.Since(s.start).Seconds()
	for i, n := range s.npus {
		// Phase shift per NPU index so dashboards see uncorrelated movement.
		phase := elapsed*0.5 + float64(i)*math.Pi/3
		amplitude := 0.10 // +/- 10%
		factor := 1.0 + amplitude*math.Sin(phase)
		utilization := clamp(n.UtilizationSeed*factor, 0, 100)
		memUsed := uint64(float64(n.MemoryUsedSeedBytes) * factor)
		if memUsed > n.MemoryTotalBytes {
			memUsed = n.MemoryTotalBytes
		}
		bw := uint64(float64(n.HBMBandwidthSeedBPS) * factor)
		out = append(out, NPUSample{
			ID:                         n.ID,
			NodeName:                   n.NodeName,
			Model:                      n.Model,
			AICore:                     utilization,
			MemoryUsedBytes:            memUsed,
			MemoryTotalBytes:           n.MemoryTotalBytes,
			HBMBandwidthBytesPerSecond: bw,
			Healthy:                    n.Healthy,
		})
	}
	return out, nil
}

func clamp(v, lo, hi float64) float64 {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}
