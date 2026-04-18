package main

import (
	"fmt"
	"log/slog"
	"os"

	"github.com/spf13/cobra"

	"one-codingplan/internal/config"
	"one-codingplan/internal/database"
	"one-codingplan/internal/logging"
	"one-codingplan/internal/pool"
	"one-codingplan/internal/server"
)

var serveConfigPath string

var serveCmd = &cobra.Command{
	Use:   "serve",
	Short: "Start the ocp proxy server",
	RunE: func(cmd *cobra.Command, args []string) error {
		cfg, err := config.Load(serveConfigPath)
		if err != nil {
			return fmt.Errorf("load config: %w", err)
		}

		encKey := []byte(os.Getenv("OCP_ENCRYPTION_KEY"))
		if len(encKey) != 16 && len(encKey) != 24 && len(encKey) != 32 {
			return fmt.Errorf("OCP_ENCRYPTION_KEY must be set to a 16, 24, or 32 byte string (got %d bytes)", len(encKey))
		}

		db, err := database.Open(cfg.Database.Path)
		if err != nil {
			return fmt.Errorf("open db: %w", err)
		}

		if err := database.Migrate(db); err != nil {
			return fmt.Errorf("migrate: %w", err)
		}

		logging.Setup(cfg.Logging)

		poolCfg := &pool.Config{
			RateLimitBackoff: cfg.PoolBackoff(),
		}
		p, err := pool.New(db, encKey, poolCfg)
		if err != nil {
			return fmt.Errorf("init pool: %w", err)
		}
		p.StartProbeLoop()
		defer p.Stop()

		srv := server.New(db, cfg, p, encKey)
		r := srv.Engine()

		addr := fmt.Sprintf(":%d", cfg.Server.Port)
		slog.Info("ocp starting", "addr", addr)
		return r.Run(addr)
	},
}

func init() {
	serveCmd.Flags().StringVar(&serveConfigPath, "config", "./config.yaml", "path to config file")
	rootCmd.AddCommand(serveCmd)
}
