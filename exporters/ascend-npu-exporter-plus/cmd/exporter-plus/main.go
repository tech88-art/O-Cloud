// Command exporter-plus is the ascend-npu-exporter-plus binary.
//
// Source wiring (the decoupling seam lives in package sources · ADR-0024
// §2 Decision G):
//   - registry.New() preloads exporter_build_info +
//     exporter_collect_duration_seconds (P3-T-006).
//   - --source picks the implementation via sources.Select: "simulator"
//     (default · demo profile · replays --simulator JSON) or "real"
//     (P13-T-102 · real profile · live npu-smi telemetry, no cgo). For
//     backward compatibility, a non-empty --simulator with the default
//     --source implies the simulator type.
//   - NPUCollector (+ SliceCollector when the source provides slices) are
//     registered against whatever Select returns — identical collector
//     code for demo and real.
//   - --enable-workload-correlation (default OFF · exporters/CLAUDE.md §8)
//     additionally registers WorkloadCollector. With --workload-sim-root
//     set it uses the simulator fake-fs (demo); empty selects the real
//     /proc reader (P13-T-102 · real profile, requires hostPID).
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
	var listen, simulator, sourceType, nodeName, npuSMIPath, workloadSimRoot string
	var enableWorkloadCorrelation bool
	flag.StringVar(&listen, "listen", ":9100", "address the /metrics HTTP server binds to")
	flag.StringVar(&sourceType, "source", "",
		"NPU sample source: simulator (default) | real. real reads live npu-smi telemetry (no cgo).")
	flag.StringVar(&simulator, "simulator", "", "path to simulator JSON (simulator source). Non-empty implies --source=simulator.")
	flag.StringVar(&nodeName, "node-name", "",
		"K8s Node name stamped on real NPU samples ($NODE_NAME in the DaemonSet). real source only.")
	flag.StringVar(&npuSMIPath, "npu-smi-path", "",
		"Absolute path to the npu-smi binary (real source). Empty = PATH lookup.")
	flag.BoolVar(&enableWorkloadCorrelation, "enable-workload-correlation", false,
		"Enable PID-level workload metrics (P3-T-102). Default OFF (opt-in for Phase 5 PD Router consumers).")
	flag.StringVar(&workloadSimRoot, "workload-sim-root", "",
		"Simulator cgroup fake-fs root for --enable-workload-correlation. Empty = real /proc reader (P13-T-102).")
	flag.Parse()

	reg := registry.New(version.Version, version.Commit, runtime.Version())

	// Resolve the source type. A bare --simulator (legacy invocation) maps
	// to the simulator type; otherwise --source decides (default simulator).
	st := sources.SourceType(sourceType)
	if st == "" {
		st = sources.SourceTypeSimulator
	}

	if st == sources.SourceTypeReal || simulator != "" {
		npuSrc, sliceSrc, err := sources.Select(sources.SelectConfig{
			Type:             st,
			SimulatorPath:    simulator,
			NodeName:         nodeName,
			NPUSMIBinaryPath: npuSMIPath,
		})
		if err != nil {
			log.Fatalf("source select (%s): %v", st, err)
		}
		reg.MustRegister(collector.NewNPUCollector(npuSrc))
		if sliceSrc != nil {
			reg.MustRegister(collector.NewSliceCollector(sliceSrc))
			log.Printf("source %q active (NPU + slice collectors registered)", st)
		} else {
			log.Printf("source %q active (NPU collector registered; slice series via DRA/backend, not this exporter)", st)
		}
	} else {
		log.Printf("no source configured (--source unset and --simulator empty); only meta metrics on /metrics")
	}

	if enableWorkloadCorrelation {
		cg, err := sources.NewCgroupSource(workloadSimRoot)
		if err != nil {
			log.Fatalf("cgroup source load: %v", err)
		}
		reg.MustRegister(collector.NewWorkloadCollector(cg))
		if workloadSimRoot != "" {
			log.Printf("workload correlation enabled (simulator cgroup fs: %q)", workloadSimRoot)
		} else {
			log.Printf("workload correlation enabled (real /proc reader)")
		}
	}

	srv := server.New(listen, reg)
	log.Printf("ascend-npu-exporter-plus %s (commit %s) listening on %s",
		version.Version, version.Commit, listen)
	if err := srv.ListenAndServe(); err != nil {
		log.Fatalf("server exited: %v", err)
	}
}
