package main

import (
	"flag"
	"fmt"
	"log"

	"one-codingplan/internal/config"
	"one-codingplan/internal/database"
	"one-codingplan/internal/server"
)

func main() {
	configPath := flag.String("config", "./config.yaml", "path to config file")
	flag.Parse()

	cfg, err := config.Load(*configPath)
	if err != nil {
		log.Fatalf("load config: %v", err)
	}

	db, err := database.Open(cfg.Database.Path)
	if err != nil {
		log.Fatalf("open db: %v", err)
	}

	if err := database.Migrate(db); err != nil {
		log.Fatalf("migrate: %v", err)
	}

	if err := database.SyncUpstreams(db, cfg.Upstreams); err != nil {
		log.Fatalf("sync upstreams: %v", err)
	}

	srv := server.New(db, cfg)
	r := srv.Engine()

	addr := fmt.Sprintf(":%d", cfg.Server.Port)
	log.Printf("ocp starting on %s", addr)
	if err := r.Run(addr); err != nil {
		log.Fatalf("server: %v", err)
	}
}
