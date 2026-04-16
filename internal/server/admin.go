package server

import (
	"crypto/subtle"
	"encoding/json"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"one-codingplan/internal/models"
)

func (s *Server) adminMiddleware(c *gin.Context) {
	auth := c.GetHeader("Authorization")
	token, ok := cutPrefix(auth, "Bearer ")
	if !ok || token == "" || subtle.ConstantTimeCompare([]byte(token), []byte(s.cfg.Server.AdminKey)) != 1 {
		c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}
	c.Next()
}

// Request/response types

type createKeyRequest struct {
	Name               string     `json:"name"`
	TokenBudget        *int64     `json:"token_budget"`
	AllowedUpstreams   []string   `json:"allowed_upstreams"`
	ExpiresAt          *time.Time `json:"expires_at"`
	RateLimitPerMinute *int       `json:"rate_limit_per_minute"`
	RateLimitPerDay    *int       `json:"rate_limit_per_day"`
}

type patchKeyRequest struct {
	Name               *string    `json:"name"`
	TokenBudget        *int64     `json:"token_budget"`
	AllowedUpstreams   []string   `json:"allowed_upstreams"`
	ExpiresAt          *time.Time `json:"expires_at"`
	RateLimitPerMinute *int       `json:"rate_limit_per_minute"`
	RateLimitPerDay    *int       `json:"rate_limit_per_day"`
}

type keyResponse struct {
	ID                 string     `json:"id"`
	Name               string     `json:"name"`
	Token              string     `json:"token"`
	Enabled            bool       `json:"enabled"`
	TokenBudget        int64      `json:"token_budget"`
	AllowedUpstreams   []string   `json:"allowed_upstreams"`
	ExpiresAt          *time.Time `json:"expires_at,omitempty"`
	RateLimitPerMinute int        `json:"rate_limit_per_minute"`
	RateLimitPerDay    int        `json:"rate_limit_per_day"`
	UsageTotalInput    int64      `json:"usage_total_input"`
	UsageTotalOutput   int64      `json:"usage_total_output"`
	CreatedAt          time.Time  `json:"created_at"`
	UpdatedAt          time.Time  `json:"updated_at"`
}

// maskToken masks a token for display. Shows first 7 and last 3 chars with *** in between.
func maskToken(token string) string {
	if len(token) <= 10 {
		return "***"
	}
	return token[:7] + "***" + token[len(token)-3:]
}

// usageTotals queries aggregated token counts for a key.
func (s *Server) usageTotals(keyID string) (int64, int64) {
	var input, output int64
	s.db.Raw(
		"SELECT COALESCE(SUM(input_tokens),0), COALESCE(SUM(output_tokens),0) FROM usage_records WHERE key_id = ?",
		keyID,
	).Row().Scan(&input, &output)
	return input, output
}

// parseAllowedUpstreams decodes the JSON-encoded AllowedUpstreams field.
func parseAllowedUpstreams(raw string) []string {
	if raw == "" {
		return []string{}
	}
	var result []string
	if err := json.Unmarshal([]byte(raw), &result); err != nil {
		return []string{}
	}
	return result
}

// toKeyResponse converts an AccessKey model to a keyResponse, masking or exposing token.
func (s *Server) toKeyResponse(key models.AccessKey, exposeToken bool) keyResponse {
	tok := key.Token
	if !exposeToken {
		tok = maskToken(key.Token)
	}
	input, output := s.usageTotals(key.ID)
	return keyResponse{
		ID:                 key.ID,
		Name:               key.Name,
		Token:              tok,
		Enabled:            key.Enabled,
		TokenBudget:        key.TokenBudget,
		AllowedUpstreams:   parseAllowedUpstreams(key.AllowedUpstreams),
		ExpiresAt:          key.ExpiresAt,
		RateLimitPerMinute: key.RateLimitPerMinute,
		RateLimitPerDay:    key.RateLimitPerDay,
		UsageTotalInput:    input,
		UsageTotalOutput:   output,
		CreatedAt:          key.CreatedAt,
		UpdatedAt:          key.UpdatedAt,
	}
}

func (s *Server) handleCreateKey(c *gin.Context) {
	var req createKeyRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	var allowedJSON string
	if len(req.AllowedUpstreams) > 0 {
		bs, _ := json.Marshal(req.AllowedUpstreams)
		allowedJSON = string(bs)
	}

	var budget int64
	if req.TokenBudget != nil {
		budget = *req.TokenBudget
	}
	var rpm, rpd int
	if req.RateLimitPerMinute != nil {
		rpm = *req.RateLimitPerMinute
	}
	if req.RateLimitPerDay != nil {
		rpd = *req.RateLimitPerDay
	}

	key := models.AccessKey{
		ID:                 uuid.New().String(),
		Token:              "ocp-" + uuid.New().String(),
		Enabled:            true,
		Name:               req.Name,
		TokenBudget:        budget,
		AllowedUpstreams:   allowedJSON,
		ExpiresAt:          req.ExpiresAt,
		RateLimitPerMinute: rpm,
		RateLimitPerDay:    rpd,
	}

	if err := s.db.Create(&key).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to create key"})
		return
	}

	c.JSON(http.StatusCreated, s.toKeyResponse(key, true))
}

func (s *Server) handleListKeys(c *gin.Context) {
	var keys []models.AccessKey
	if err := s.db.Find(&keys).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to list keys"})
		return
	}

	resp := make([]keyResponse, len(keys))
	for i, k := range keys {
		resp[i] = s.toKeyResponse(k, false)
	}
	c.JSON(http.StatusOK, resp)
}

func (s *Server) handleGetKey(c *gin.Context) {
	id := c.Param("id")
	var key models.AccessKey
	if err := s.db.First(&key, "id = ?", id).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "key not found"})
		return
	}
	c.JSON(http.StatusOK, s.toKeyResponse(key, false))
}

func (s *Server) handleUpdateKey(c *gin.Context) {
	id := c.Param("id")
	var key models.AccessKey
	if err := s.db.First(&key, "id = ?", id).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "key not found"})
		return
	}

	var req patchKeyRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	updates := map[string]any{}
	if req.Name != nil {
		updates["name"] = *req.Name
	}
	if req.TokenBudget != nil {
		updates["token_budget"] = *req.TokenBudget
	}
	if req.AllowedUpstreams != nil {
		if len(req.AllowedUpstreams) == 0 {
			updates["allowed_upstreams"] = ""
		} else {
			bs, _ := json.Marshal(req.AllowedUpstreams)
			updates["allowed_upstreams"] = string(bs)
		}
	}
	if req.ExpiresAt != nil {
		updates["expires_at"] = req.ExpiresAt
	}
	if req.RateLimitPerMinute != nil {
		updates["rate_limit_per_minute"] = *req.RateLimitPerMinute
	}
	if req.RateLimitPerDay != nil {
		updates["rate_limit_per_day"] = *req.RateLimitPerDay
	}

	if len(updates) > 0 {
		if err := s.db.Model(&key).Updates(updates).Error; err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to update key"})
			return
		}
	}

	// Reload to get fresh state
	s.db.First(&key, "id = ?", id)
	c.JSON(http.StatusOK, s.toKeyResponse(key, false))
}

func (s *Server) handleBlockKey(c *gin.Context) {
	id := c.Param("id")
	var key models.AccessKey
	if err := s.db.First(&key, "id = ?", id).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "key not found"})
		return
	}
	if err := s.db.Model(&key).Update("enabled", false).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to block key"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "key blocked"})
}

func (s *Server) handleUnblockKey(c *gin.Context) {
	id := c.Param("id")
	var key models.AccessKey
	if err := s.db.First(&key, "id = ?", id).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "key not found"})
		return
	}
	if err := s.db.Model(&key).Update("enabled", true).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to unblock key"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "key unblocked"})
}

func (s *Server) handleDeleteKey(c *gin.Context) {
	id := c.Param("id")
	result := s.db.Delete(&models.AccessKey{}, "id = ?", id)
	if result.Error != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to delete key"})
		return
	}
	if result.RowsAffected == 0 {
		c.JSON(http.StatusNotFound, gin.H{"error": "key not found"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "key deleted"})
}

func (s *Server) handleRotateUpstream(c *gin.Context) {
	c.JSON(http.StatusNotImplemented, gin.H{"error": "not implemented"})
}

func (s *Server) handleListUpstreams(c *gin.Context) {
	c.JSON(http.StatusNotImplemented, gin.H{"error": "not implemented"})
}
