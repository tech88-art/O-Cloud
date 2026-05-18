// Command demo-backend serves the O-Cloud edge cloud platform demo backend.
//
// Phase 1 scaffold (P1-T-005) wires Cobra+Viper config, zap logging, Gin
// engine and mounts /healthz + /version. Resource endpoints come in T101+.
package main

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/spf13/cobra"
	"go.uber.org/zap"

	"github.com/example/ocloud-edge/backend/pkg/api"
	"github.com/example/ocloud-edge/backend/pkg/config"
	"github.com/example/ocloud-edge/backend/pkg/datasource"
	"github.com/example/ocloud-edge/backend/pkg/datasource/k8s"
	"github.com/example/ocloud-edge/backend/pkg/datasource/mock"
	"github.com/example/ocloud-edge/backend/pkg/datasource/prometheus"
)

const (
	shutdownGrace    = 10 * time.Second
	readHeaderTimout = 10 * time.Second
)

func main() {
	if err := newRootCmd().Execute(); err != nil {
		_, _ = fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func newRootCmd() *cobra.Command {
	var configFile string

	cmd := &cobra.Command{
		Use:           "demo-backend",
		Short:         "O-Cloud edge cloud platform demo backend",
		Long:          "Serves the REST+WebSocket API behind the demo frontend. See backend/CLAUDE.md.",
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runServer(cmd.Context(), configFile)
		},
	}
	cmd.Flags().StringVarP(&configFile, "config", "c", "", "path to config file (default: ./configs/config.yaml)")
	cmd.AddCommand(newVersionCmd())
	return cmd
}

func newVersionCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "version",
		Short: "Print version and exit",
		RunE: func(cmd *cobra.Command, args []string) error {
			fmt.Printf("demo-backend %s (commit=%s, built=%s)\n", api.CurrentVersion, api.BuildCommit, api.BuildTimestamp)
			return nil
		},
	}
}

func runServer(ctx context.Context, configFile string) error {
	cfg, err := config.Load(configFile)
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}

	logger, err := buildLogger(cfg.Logging.Level)
	if err != nil {
		return fmt.Errorf("build logger: %w", err)
	}
	defer func() {
		// Sync may return EINVAL when stdout is a terminal — ignore that.
		_ = logger.Sync()
	}()

	// Construct concrete sources here (in main) to avoid the import cycle
	// that would arise if datasource/factory.go directly referenced
	// datasource/mock (which imports datasource for the Source interface).
	sources := map[string]datasource.Source{}
	if mockCfg, ok := cfg.Datasources["mock"]; ok && mockCfg.Enabled {
		sources["mock"] = mock.NewSource(mockCfg.Path)
	}
	// P2-T-006: wire k8s.Source when datasources.k8s.enabled = true.
	// Construction fetches the kubeconfig (path-on-disk or in-cluster
	// SA), builds the clientset. Failure aborts startup — operators
	// shouldn't silently fall back to mock when they asked for real K8s.
	if k8sCfg, ok := cfg.Datasources["k8s"]; ok && k8sCfg.Enabled {
		k8sSrc, err := k8s.NewSource(k8s.Options{
			KubeconfigPath: k8sCfg.Kubeconfig,
		})
		if err != nil {
			return fmt.Errorf("build k8s source: %w", err)
		}
		sources["k8s"] = k8sSrc
		logger.Info("k8s source wired",
			zap.String("kubeconfig", k8sCfg.Kubeconfig))
	}
	// P2-T-007: wire prometheus.Source when enabled. URL is the only
	// required field; bearer token is optional.
	if pCfg, ok := cfg.Datasources["prometheus"]; ok && pCfg.Enabled {
		pSrc, err := prometheus.NewSource(prometheus.Options{
			URL: pCfg.URL,
		})
		if err != nil {
			return fmt.Errorf("build prometheus source: %w", err)
		}
		sources["prometheus"] = pSrc
		logger.Info("prometheus source wired", zap.String("url", pCfg.URL))
	}
	reg, err := datasource.Build(cfg, sources)
	if err != nil {
		return fmt.Errorf("build datasource registry: %w", err)
	}

	handler := api.NewHandler(reg, logger)
	// T205 follow-up: wire Grafana base URL from config (fallback in handler).
	if cfg.Grafana.BaseURL != "" {
		handler.GrafanaBaseURL = cfg.Grafana.BaseURL
	}
	router := api.NewRouter(handler, api.RouterOptions{EnableCORS: cfg.Server.EnableCORS})

	addr := fmt.Sprintf(":%d", cfg.Server.Port)
	srv := &http.Server{
		Addr:              addr,
		Handler:           router,
		ReadHeaderTimeout: readHeaderTimout,
	}

	rootCtx, cancel := signal.NotifyContext(ctx, os.Interrupt, syscall.SIGTERM)
	defer cancel()

	errCh := make(chan error, 1)
	go func() {
		logger.Info("listening",
			zap.String("addr", addr),
			zap.String("version", api.CurrentVersion))
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			errCh <- err
			return
		}
		errCh <- nil
	}()

	select {
	case <-rootCtx.Done():
		logger.Info("shutdown signal received")
		shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), shutdownGrace)
		defer shutdownCancel()
		if err := srv.Shutdown(shutdownCtx); err != nil {
			return fmt.Errorf("shutdown: %w", err)
		}
		return nil
	case err := <-errCh:
		return err
	}
}

func buildLogger(level string) (*zap.Logger, error) {
	zapCfg := zap.NewProductionConfig()
	if level == "" {
		level = config.DefaultLogLevel
	}
	if err := zapCfg.Level.UnmarshalText([]byte(level)); err != nil {
		return nil, fmt.Errorf("parse log level %q: %w", level, err)
	}
	return zapCfg.Build()
}
