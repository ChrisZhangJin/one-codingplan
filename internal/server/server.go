package server

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"one-codingplan/internal/config"
	"one-codingplan/internal/pool"
)

type Server struct {
	db   *gorm.DB
	cfg  *config.Config
	pool *pool.Pool
}

func New(db *gorm.DB, cfg *config.Config, p *pool.Pool) *Server {
	return &Server{db: db, cfg: cfg, pool: p}
}

func (s *Server) Engine() *gin.Engine {
	r := gin.New()
	r.Use(gin.Logger())
	r.Use(gin.Recovery())
	r.GET("/health", s.handleHealth)
	v1 := r.Group("/v1")
	v1.Use(s.authMiddleware)
	v1.POST("/chat/completions", s.handleRelay)
	return r
}

func (s *Server) handleHealth(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{"status": "ok"})
}
