package pool

import (
	"errors"
	"fmt"
	"sync"
	"time"

	"one-codingplan/internal/models"

	"gorm.io/gorm"
)

// ErrNoUpstreams is returned by Select when all upstreams are unavailable.
var ErrNoUpstreams = errors.New("pool: no available upstreams")

// UpstreamEntry is the public view of an upstream returned by Select.
type UpstreamEntry struct {
	ID            uint
	Name          string
	BaseURL       string
	APIKey        string
	ModelOverride string
}

// entry is the internal pool entry with availability state.
type entry struct {
	UpstreamEntry
	available bool
}

// Config holds pool configuration.
type Config struct {
	RateLimitBackoff time.Duration
}

// Pool holds the in-memory upstream pool.
type Pool struct {
	mu      sync.RWMutex
	entries []entry
	idx     int
	cfg     *Config
	stopCh  chan struct{}
	once    sync.Once
}

// New loads all enabled upstreams from db, decrypts their API keys, and returns
// a ready Pool. It does not start any background goroutine (that is Plan 02).
func New(db *gorm.DB, encKey []byte, cfg *Config) (*Pool, error) {
	var upstreams []models.Upstream
	if err := db.Where("enabled = ?", true).Find(&upstreams).Error; err != nil {
		return nil, fmt.Errorf("pool: load upstreams: %w", err)
	}
	entries := make([]entry, 0, len(upstreams))
	for _, u := range upstreams {
		apiKey, err := u.DecryptAPIKey(encKey)
		if err != nil {
			return nil, fmt.Errorf("pool: decrypt key for %s: %w", u.Name, err)
		}
		entries = append(entries, entry{
			UpstreamEntry: UpstreamEntry{
				ID:      u.ID,
				Name:    u.Name,
				BaseURL: u.BaseURL,
				APIKey:  apiKey,
			},
			available: true,
		})
	}
	return &Pool{
		entries: entries,
		cfg:     cfg,
		stopCh:  make(chan struct{}),
	}, nil
}

// Select returns the next available upstream using round-robin.
// keyID is accepted for future per-key filtering (D-17) but is ignored now.
// Returns ErrNoUpstreams if all upstreams are unavailable.
func (p *Pool) Select(keyID string) (*UpstreamEntry, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	n := len(p.entries)
	if n == 0 {
		return nil, ErrNoUpstreams
	}
	for i := 0; i < n; i++ {
		p.idx = (p.idx + 1) % n
		if p.entries[p.idx].available {
			e := p.entries[p.idx].UpstreamEntry
			return &e, nil
		}
	}
	return nil, ErrNoUpstreams
}

// Mark sets the availability of the upstream with the given id.
func (p *Pool) Mark(id uint, available bool) {
	p.mu.Lock()
	defer p.mu.Unlock()
	for i := range p.entries {
		if p.entries[i].ID == id {
			p.entries[i].available = available
			return
		}
	}
}

// Stop signals the pool to shut down any background goroutines.
// It is idempotent.
func (p *Pool) Stop() {
	p.once.Do(func() {
		close(p.stopCh)
	})
}

// Backoff returns the configured rate-limit backoff duration.
func (p *Pool) Backoff() time.Duration {
	return p.cfg.RateLimitBackoff
}

// SetModelOverride sets the ModelOverride field on the entry with the given name.
// ModelOverride is not stored in the database — it comes from config and is applied
// after pool construction.
func (p *Pool) SetModelOverride(name, override string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	for i := range p.entries {
		if p.entries[i].Name == name {
			p.entries[i].ModelOverride = override
			return
		}
	}
}

// StartProbeLoop starts the background probe goroutine.
// It is called by main.go after pool construction.
func (p *Pool) StartProbeLoop() {
	go p.runProbeLoop()
}

// ProbeAll immediately probes all unavailable upstreams.
// It is exported for use in tests.
func (p *Pool) ProbeAll() {
	p.probeAll()
}

