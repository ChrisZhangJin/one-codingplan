package main

import (
	"flag"
	"fmt"
	"log"

	"github.com/gin-gonic/gin"
	"one-codingplan/internal/config"
	"one-codingplan/internal/database"
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

	r := gin.New()
	r.Use(gin.Logger())
	r.Use(gin.Recovery())
	r.GET("/health", func(c *gin.Context) {
		c.JSON(200, gin.H{"status": "ok"})
	})

	addr := fmt.Sprintf(":%d", cfg.Server.Port)
	log.Printf("starting on %s", addr)
	if err := r.Run(addr); err != nil {
		log.Fatalf("server: %v", err)
	}
}
