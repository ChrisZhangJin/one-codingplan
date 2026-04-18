package server

import (
	"net/http"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"one-codingplan/internal/models"
)

type rateCounter struct {
	mu       sync.Mutex
	count    int
	windowID int
}

var perMinuteCounters sync.Map // keyID string -> *rateCounter
var perDayCounters sync.Map    // keyID string -> *rateCounter

// ResetPerMinuteCounters clears all per-minute rate limit counters.
// Exported for use in tests to avoid inter-test interference.
func ResetPerMinuteCounters() {
	perMinuteCounters.Range(func(k, _ any) bool {
		perMinuteCounters.Delete(k)
		return true
	})
}

func checkRate(counters *sync.Map, keyID string, limit int, currentWindow int) bool {
	val, _ := counters.LoadOrStore(keyID, &rateCounter{})
	rc := val.(*rateCounter)
	rc.mu.Lock()
	defer rc.mu.Unlock()
	if rc.windowID != currentWindow {
		rc.count = 0
		rc.windowID = currentWindow
	}
	if rc.count >= limit {
		return false
	}
	rc.count++
	return true
}

func (s *Server) limitMiddleware(c *gin.Context) {
	key := c.MustGet("accessKey").(models.AccessKey)

	// Token budget check (KEY-04, D-09)
	if key.TokenBudget > 0 {
		var totalInput, totalOutput int64
		s.db.Model(&models.UsageRecord{}).
			Select("COALESCE(SUM(input_tokens),0), COALESCE(SUM(output_tokens),0)").
			Where("key_id = ?", key.ID).
			Row().Scan(&totalInput, &totalOutput)
		if totalInput+totalOutput >= key.TokenBudget {
			c.AbortWithStatusJSON(http.StatusTooManyRequests, gin.H{
				"error": gin.H{
					"message": "token budget exceeded",
					"type":    "requests",
					"code":    "rate_limit_exceeded",
				},
			})
			return
		}
	}

	// Per-minute rate limit (D-07)
	if key.RateLimitPerMinute > 0 {
		now := time.Now().UTC()
		minuteWindow := now.Hour()*60 + now.Minute()
		if !checkRate(&perMinuteCounters, key.ID, key.RateLimitPerMinute, minuteWindow) {
			c.AbortWithStatusJSON(http.StatusTooManyRequests, gin.H{
				"error": gin.H{
					"message": "per-minute rate limit exceeded",
					"type":    "requests",
					"code":    "rate_limit_exceeded",
				},
			})
			return
		}
	}

	// Per-day rate limit (D-08)
	if key.RateLimitPerDay > 0 {
		dayWindow := time.Now().UTC().YearDay()
		if !checkRate(&perDayCounters, key.ID, key.RateLimitPerDay, dayWindow) {
			c.AbortWithStatusJSON(http.StatusTooManyRequests, gin.H{
				"error": gin.H{
					"message": "per-day rate limit exceeded",
					"type":    "requests",
					"code":    "rate_limit_exceeded",
				},
			})
			return
		}
	}

	c.Next()
}
