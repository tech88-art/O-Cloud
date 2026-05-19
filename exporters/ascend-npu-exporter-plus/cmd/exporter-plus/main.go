// Command exporter-plus is the ascend-npu-exporter-plus binary.
//
// Phase 3 wiring:
//   - registry.New() preloads exporter_build_info +
//     exporter_collect_duration_seconds (P3-T-006).
//   - When --simulator is non-empty (P3-T-007 / T101), build a
//     SimulatorSource and register NPUCollector + SliceCollector.
//   - When --enable-workload-correlation is set (P3-T-102), additionally
//     register WorkloadCollector against --workload-sim-root (defaults
//     to ./testdata/simulator-cgroup-fs for dev). Default OFF per
//     exporters/CLAUDE.md sec 8 to avoid /proc scan cost on idle hosts.
//   - When --simulator is empty, the DCMI / npu-smi sources are stubs
//     (Phase 4+); only meta metrics appear on /metrics.
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
	var listen, simulator, workloadSimRoot string
	var enableWorkloadCorrelation bool
	flag.StringVar(&listen, "listen", ":9100", "address the /metrics HTTP server binds to")
	flag.StringVar(&simulator, "simulator", "", "path to simulator JSON; empty = DCMI (Phase 4+)")
	flag.BoolVar(&enableWorkloadCorrelation, "enable-workload-correlation", false,
		"Enable PID-level workload metrics (P3-T-102). Default OFF (opt-in for Phase 5 PD Router consumers).")
	flag.StringVar(&workloadSimRoot, "workload-sim-root", "",
		"Path to the simulator cgroup fake-fs root used when --enable-workload-correlation is set. Empty = real /proc (Phase 4+ stub).")
	flag.Parse()

	reg := registry.New(version.Version, version.Commit, runtime.Version())

	if simulator != "" {
		src, err := sources.NewSimulatorSource(simulator)
		if err != nil {
			log.Fatalf("simulator source load: %v", err)
		}
		reg.MustRegister(collector.NewNPUCollector(src))
		reg.MustRegister(collector.NewSliceCollector(src))
		log.Printf("simulator source active: %s (NPU + slice collectors registered)", simulator)
	} else {
		log.Printf("no source configured (--simulator unset); Phase 4+ DCMI source not yet implemented")
	}

	if enableWorkloadCorrelation {
		cg, err := sources.NewCgroupSource(workloadSimRoot)
		if err != nil {
			log.Fatalf("cgroup source load: %v", err)
		}
		reg.MustRegister(collector.NewWorkloadCollector(cg))
		log.Printf("workload correlation enabled (cgroup sim root: %q)", workloadSimRoot)
	}

	srv := server.New(listen, reg)
	log.Printf("ascend-npu-exporter-plus %s (commit %s) listening on %s",
		version.Version, version.Commit, listen)
	if err := srv.ListenAndServe(); err != nil {
		log.Fatalf("server exited: %v", err)
	}
}
