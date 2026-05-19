// Command exporter-plus is the ascend-npu-exporter-plus binary.
//
// Phase 3 wiring:
//   - registry.New() preloads exporter_build_info +
//     exporter_collect_duration_seconds (P3-T-006).
//   - When --simulator is non-empty (P3-T-007), build a SimulatorSource
//     and register an NPUCollector that emits per-device gauges.
//   - When --simulator is empty, the DCMI / npu-smi sources are stubs
//     (Phase 4+); only meta metrics will appear on /metrics until then.
package main

import (
	"flag"
	"log"
	"runtime"

	"github.com/example/ocloud-edge/exporters/ascend-npu-exporter-plus/internal/collector"
	"github.com/example/ocloud-edge/exporters/ascend-npu-exporter-plus/internal/collector/sources"
	"github.com/example/ocloud-edge/exporters/ascend-npu-exporter-plus/internal/registry"
	"github.com/example/ocloud-edge/exporters/ascend-npu-exporter-plus/internal/server"
	"github.com/example/ocloud-edge/exporters/ascend-npu-exporter-plus/internal/version"
)

func main() {
	var listen, simulator string
	flag.StringVar(&listen, "listen", ":9100", "address the /metrics HTTP server binds to")
	flag.StringVar(&simulator, "simulator", "", "path to simulator JSON; empty = DCMI (Phase 4+)")
	flag.Parse()

	reg := registry.New(version.Version, version.Commit, runtime.Version())

	if simulator != "" {
		src, err := sources.NewSimulatorSource(simulator)
		if err != nil {
			log.Fatalf("simulator source load: %v", err)
		}
		reg.MustRegister(collector.NewNPUCollector(src))
		log.Printf("simulator source active: %s", simulator)
	} else {
		log.Printf("no source configured (--simulator unset); Phase 4+ DCMI source not yet implemented")
	}

	srv := server.New(listen, reg)
	log.Printf("ascend-npu-exporter-plus %s (commit %s) listening on %s",
		version.Version, version.Commit, listen)
	if err := srv.ListenAndServe(); err != nil {
		log.Fatalf("server exited: %v", err)
	}
}
