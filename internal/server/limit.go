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

// ResetPerDayCounters clears all per-day rate limit counters.
// Exported for use in tests to avoid inter-test interference.
func ResetPerDayCounters() {
	perDayCounters.Range(func(k, _ any) bool {
		perDayCounters.Delete(k)
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

func incrementCounter(counters *sync.Map, keyID string, currentWindow int) {
	val, _ := counters.LoadOrStore(keyID, &rateCounter{})
	rc := val.(*rateCounter)
	rc.mu.Lock()
	defer rc.mu.Unlock()
	if rc.windowID != currentWindow {
		rc.count = 0
		rc.windowID = currentWindow
	}
	rc.count++
}

func currentDayCount(keyID string) int {
	val, ok := perDayCounters.Load(keyID)
	if !ok {
		return 0
	}
	rc := val.(*rateCounter)
	rc.mu.Lock()
	defer rc.mu.Unlock()
	if rc.windowID != time.Now().UTC().YearDay() {
		return 0
	}
	return rc.count
}

// InjectDayCount sets a perDayCounters entry for the given keyID with today's windowID
// and the given count. Only for use in tests.
func InjectDayCount(keyID string, count int) {
	rc := &rateCounter{count: count, windowID: time.Now().UTC().YearDay()}
	perDayCounters.Store(keyID, rc)
}

// InjectDayCountStale sets a perDayCounters entry for the given keyID with a windowID
// from yesterday. Only for use in tests.
func InjectDayCountStale(keyID string) {
	rc := &rateCounter{count: 99, windowID: time.Now().UTC().YearDay() - 1}
	perDayCounters.Store(keyID, rc)
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

	// Per-day counter — always increment for Today display; enforce limit if set (D-08)
	dayWindow := time.Now().UTC().YearDay()
	if key.RateLimitPerDay > 0 {
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
	} else {
		incrementCounter(&perDayCounters, key.ID, dayWindow)
	}

	c.Next()
}
