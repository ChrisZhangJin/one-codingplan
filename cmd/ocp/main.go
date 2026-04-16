package main

import (
	"flag"
	"fmt"
	"log"
	"os"

	"one-codingplan/internal/config"
	"one-codingplan/internal/database"
	"one-codingplan/internal/pool"
	"one-codingplan/internal/server"
)

func main() {
	configPath := flag.String("config", "./config.yaml", "path to config file")
	flag.Parse()

	cfg, err := config.Load(*configPath)
	if err != nil {
		log.Fatalf("load config: %v", err)
	}

	encKey := []byte(os.Getenv("OCP_ENCRYPTION_KEY"))
	if len(encKey) != 16 && len(encKey) != 24 && len(encKey) != 32 {
		log.Fatalf("OCP_ENCRYPTION_KEY must be set to a 16, 24, or 32 byte string (got %d bytes)", len(encKey))
	}

	db, err := database.Open(cfg.Database.Path)
	if err != nil {
		log.Fatalf("open db: %v", err)
	}

	if err := database.Migrate(db); err != nil {
		log.Fatalf("migrate: %v", err)
	}

	if err := database.SyncUpstreams(db, cfg.Upstreams, encKey); err != nil {
		log.Fatalf("sync upstreams: %v", err)
	}

	poolCfg := &pool.Config{
		RateLimitBackoff: cfg.PoolBackoff(),
	}
	p, err := pool.New(db, encKey, poolCfg)
	if err != nil {
		log.Fatalf("init pool: %v", err)
	}
	for _, u := range cfg.Upstreams {
		if u.ModelOverride != "" {
			p.SetModelOverride(u.Name, u.ModelOverride)
		}
	}
	p.StartProbeLoop()
	defer p.Stop()

	srv := server.New(db, cfg, p)
	r := srv.Engine()

	addr := fmt.Sprintf(":%d", cfg.Server.Port)
	log.Printf("ocp starting on %s", addr)
	if err := r.Run(addr); err != nil {
		log.Fatalf("server: %v", err)
	}
}
