// Command exporter-plus is the ascend-npu-exporter-plus binary.
//
// Phase 3 (P3-T-006) scaffold: builds a Prometheus Registry pre-loaded with
// exporter_build_info + exporter_collect_duration_seconds, and serves
// /metrics + /healthz over HTTP. Collector implementations (NPU/slice/PID)
// come in P3-T-007/T101/T102.
package main

import (
	"flag"
	"log"
	"runtime"

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
	// TODO(P3-T-007+): plumb simulator path into NPU/slice/workload collectors
	// and call reg.MustRegister on each.
	_ = simulator

	srv := server.New(listen, reg)
	log.Printf("ascend-npu-exporter-plus %s (commit %s) listening on %s",
		version.Version, version.Commit, listen)
	if err := srv.ListenAndServe(); err != nil {
		log.Fatalf("server exited: %v", err)
	}
}
