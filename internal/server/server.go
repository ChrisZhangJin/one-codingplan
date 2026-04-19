package server

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"one-codingplan/internal/config"
	"one-codingplan/internal/models"
	"one-codingplan/internal/pool"
)

type Server struct {
	db      *gorm.DB
	cfg     *config.Config
	pool    *pool.Pool
	encKey  []byte
	usageCh chan models.UsageRecord
}

func New(db *gorm.DB, cfg *config.Config, p *pool.Pool, encKey []byte) *Server {
	s := &Server{db: db, cfg: cfg, pool: p, encKey: encKey, usageCh: make(chan models.UsageRecord, 512)}
	go s.usageWriter()
	return s
}

// usageWriter drains usageCh and writes records serially, eliminating SQLite write lock contention.
func (s *Server) usageWriter() {
	for rec := range s.usageCh {
		s.db.Create(&rec) //nolint:errcheck
	}
}

func (s *Server) Engine() *gin.Engine {
	r := gin.New()
	r.Use(gin.Logger())
	r.Use(gin.Recovery())
	r.GET("/health", s.handleHealth)
	v1 := r.Group("/v1")
	v1.Use(s.authMiddleware)
	v1.Use(s.limitMiddleware)
	v1.POST("/chat/completions", s.handleRelay)
	v1.POST("/messages", s.handleAnthropicRelay)
	v1.POST("/responses", s.handleResponsesRelay)

	api := r.Group("/api")
	api.Use(s.adminMiddleware)
	api.POST("/keys", s.handleCreateKey)
	api.GET("/keys", s.handleListKeys)
	api.GET("/keys/:id", s.handleGetKey)
	api.PATCH("/keys/:id", s.handleUpdateKey)
	api.DELETE("/keys/:id", s.handleDeleteKey)
	api.POST("/keys/:id/block", s.handleBlockKey)
	api.POST("/keys/:id/unblock", s.handleUnblockKey)
	api.GET("/usage", s.handleUsageStats)
	api.POST("/upstreams", s.handleCreateUpstream)
	api.POST("/upstreams/rotate", s.handleRotateUpstream)
	api.GET("/upstreams", s.handleListUpstreams)
	api.PATCH("/upstreams/:id", s.handleUpdateUpstream)
	api.POST("/upstreams/:id/toggle", s.handleToggleUpstream)
	r.NoRoute(spaHandler())
	return r
}

func (s *Server) handleHealth(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{"status": "ok"})
}
