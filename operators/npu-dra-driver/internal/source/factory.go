/*
Copyright 2026.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package source

import (
	"fmt"
	"time"
)

// SourceType enumerates the source backends cmd/main.go's selectSource
// function recognises. Phase 7 ships two (mock-json + real-ascend);
// Phase 10+ may add more (kubelet-cri-resource-manager-bridge, e.g.).
type SourceType string

const (
	// SourceTypeMockJSON is the Phase 4-6 default — reads
	// configs/mock-data/set-a-small/* JSON. Preserved in Phase 7 as
	// the helm chart default (sourceType: mock-json).
	SourceTypeMockJSON SourceType = "mock-json"

	// SourceTypeRealAscend selects the lab-conditional realascend body.
	// Phase 7 W1 ships only the stub (ErrNotImplemented); Phase 7 T101
	// lab body lights up real silicon support. Operators must
	// explicitly opt in via chart values (sourceType: real-ascend).
	SourceTypeRealAscend SourceType = "real-ascend"
)

// ParseSourceType normalises a CLI / chart value into the enum + returns
// a friendly error for unknown values. Empty string defaults to
// SourceTypeMockJSON (Phase 4-6 behavior preserved when --source-type is
// absent — used by Phase 4-5 publisher_test.go callers that construct
// MockJSONSource directly without going through this path).
func ParseSourceType(s string) (SourceType, error) {
	switch SourceType(s) {
	case "":
		return SourceTypeMockJSON, nil
	case SourceTypeMockJSON:
		return SourceTypeMockJSON, nil
	case SourceTypeRealAscend:
		return SourceTypeRealAscend, nil
	default:
		return "", fmt.Errorf("source: unknown type %q (want %q | %q)",
			s, SourceTypeMockJSON, SourceTypeRealAscend)
	}
}

// FactoryConfig captures the runtime parameters cmd/main.go's
// selectSource function forwards to the selected backend's constructor.
// Field meanings are backend-specific but the struct is shared (avoids
// parallel type proliferation).
//
// Dispatch lives in cmd/main.go (selectSource) rather than this package
// to avoid an internal/source → internal/source/{mockjson,realascend} →
// internal/source import cycle. This package owns the enum + config
// type; cmd/main.go owns the switch + import of subpackages.
type FactoryConfig struct {
	// MockJSONPath is the JSON file path forwarded to MockJSONSource.
	// Required when SourceType=mock-json. Maps to chart values
	// `mockJSONPath` (Phase 4 default
	// "/etc/npu-dra-driver/mock/npus.json").
	MockJSONPath string

	// MockJSONWatchPollInterval is the file-mtime poll interval for
	// MockJSONSource.Watch. Default 5s when zero. Tests override; chart
	// values do not surface this knob.
	MockJSONWatchPollInterval time.Duration

	// MockJSONSliceAICoreCapacityFallback is the per-device aiCore
	// capacity used when the JSON entry lacks aiCoreTotal. Default
	// "32" when empty.
	MockJSONSliceAICoreCapacityFallback string

	// RealAscendMode forwarded to RealAscendSource (Phase 7 T101 will
	// use this to switch between "exec" (npu-smi via os/exec) and
	// future "library" (libdcmi shared object) modes).
	RealAscendMode string
}
