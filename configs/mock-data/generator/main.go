// Command generator emits mock JSON files for the O-Cloud Edge platform
// demo. See configs/mock-data/generator/README.md for usage. One CLI flag
// selects a preset (small/multi/stress); --output picks the destination
// directory.
package main

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/example/ocloud-edge/configs/mock-data/generator/pkg/model"
	"github.com/example/ocloud-edge/configs/mock-data/generator/pkg/writer"
)

var (
	flagPreset string
	flagOutput string
)

func main() {
	rootCmd := &cobra.Command{
		Use:   "generator",
		Short: "Generate mock JSON datasets for the O-Cloud Edge demo",
		Long: `generator emits nine JSON files (meta / clusters / nodes / npus / slices /
workloads / pools / presets / events) into --output. Output validates against
configs/mock-data/schema.json. See configs/CLAUDE.md §3.3-§3.4 for the data
realism contract.`,
		RunE: runGen,
	}
	rootCmd.Flags().StringVar(&flagPreset, "preset", "small", "Preset to generate: small | multi | stress")
	rootCmd.Flags().StringVar(&flagOutput, "output", "../set-a-small/", "Output directory (will be created if missing)")

	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}

func runGen(cmd *cobra.Command, _ []string) error {
	var (
		ds  *model.Dataset
		err error
	)

	switch flagPreset {
	case "small":
		ds = buildSetASmall()
	case "multi":
		ds, err = buildSetBMultiSite()
	case "stress":
		ds, err = buildSetCStress()
	default:
		return fmt.Errorf("unknown preset %q (allowed: small|multi|stress)", flagPreset)
	}

	if err != nil {
		return err
	}
	if ds == nil {
		return fmt.Errorf("preset %q returned nil dataset", flagPreset)
	}

	if err := writer.Write(flagOutput, ds); err != nil {
		return fmt.Errorf("write dataset: %w", err)
	}

	fmt.Fprintf(cmd.OutOrStdout(),
		"wrote preset=%s to %s (clusters=%d nodes=%d npus=%d slices=%d workloads=%d presets=%d events=%d)\n",
		flagPreset, flagOutput,
		len(ds.Clusters), len(ds.Nodes), len(ds.NPUs),
		len(ds.Slices), len(ds.Workloads), len(ds.Presets), len(ds.Events),
	)
	return nil
}
