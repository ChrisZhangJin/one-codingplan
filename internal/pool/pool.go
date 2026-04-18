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
	enabled   bool
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

// New loads all upstreams from db, decrypts their API keys, and returns
// a ready Pool. It does not start any background goroutine (that is Plan 02).
func New(db *gorm.DB, encKey []byte, cfg *Config) (*Pool, error) {
	var upstreams []models.Upstream
	if err := db.Find(&upstreams).Error; err != nil {
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
				ID:            u.ID,
				Name:          u.Name,
				BaseURL:       u.BaseURL,
				APIKey:        apiKey,
				ModelOverride: u.ModelOverride,
			},
			available: u.Enabled,
			enabled:   u.Enabled,
		})
	}
	return &Pool{
		entries: entries,
		cfg:     cfg,
		stopCh:  make(chan struct{}),
	}, nil
}

// NewForTest creates a Pool from a slice of UpstreamEntry values without a database.
// All entries are marked available. For use in tests only.
func NewForTest(entries []UpstreamEntry) *Pool {
	es := make([]entry, len(entries))
	for i, e := range entries {
		es[i] = entry{UpstreamEntry: e, available: true}
	}
	return &Pool{
		entries: es,
		cfg:     &Config{RateLimitBackoff: 5 * time.Second},
		stopCh:  make(chan struct{}),
	}
}

// UpstreamInfo is the public view of an upstream returned by List (no API key).
type UpstreamInfo struct {
	ID            uint   `json:"id"`
	Name          string `json:"name"`
	BaseURL       string `json:"base_url"`
	Enabled       bool   `json:"enabled"`
	Available     bool   `json:"available"`
	ModelOverride string `json:"model_override,omitempty"`
	MaskedKey     string `json:"masked_key,omitempty"`
	Position      bool   `json:"position"`
}

// Select returns the next available upstream using round-robin.
// If allowedUpstreams is non-empty, only upstreams in that list are considered.
// Returns ErrNoUpstreams if no matching available upstream exists.
func (p *Pool) Select(allowedUpstreams []string) (*UpstreamEntry, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	n := len(p.entries)
	if n == 0 {
		return nil, ErrNoUpstreams
	}
	allowed := make(map[string]bool, len(allowedUpstreams))
	for _, name := range allowedUpstreams {
		allowed[name] = true
	}
	unrestricted := len(allowed) == 0
	for i := 0; i < n; i++ {
		p.idx = (p.idx + 1) % n
		e := &p.entries[p.idx]
		if e.available && (unrestricted || allowed[e.Name]) {
			out := e.UpstreamEntry
			return &out, nil
		}
	}
	return nil, ErrNoUpstreams
}

// ForceRotate advances the round-robin cursor to the next available upstream
// and returns its name. Used by POST /api/upstreams/rotate.
func (p *Pool) ForceRotate() (string, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	n := len(p.entries)
	if n == 0 {
		return "", ErrNoUpstreams
	}
	for i := 0; i < n; i++ {
		p.idx = (p.idx + 1) % n
		if p.entries[p.idx].available {
			return p.entries[p.idx].Name, nil
		}
	}
	return "", ErrNoUpstreams
}

// List returns status info for all pool entries. API keys are never included.
func (p *Pool) List() []UpstreamInfo {
	p.mu.RLock()
	defer p.mu.RUnlock()
	result := make([]UpstreamInfo, len(p.entries))
	for i, e := range p.entries {
		result[i] = UpstreamInfo{
			ID:            e.ID,
			Name:          e.Name,
			BaseURL:       e.BaseURL,
			Enabled:       e.enabled,
			Available:     e.available,
			ModelOverride: e.ModelOverride,
			Position:      i == p.idx,
		}
	}
	return result
}

// UpdateEntry updates the editable fields of the pool entry with the given ID.
// If apiKey is empty, the existing key is preserved.
func (p *Pool) UpdateEntry(id uint, name, baseURL, apiKey, modelOverride string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	for i := range p.entries {
		if p.entries[i].ID == id {
			p.entries[i].Name = name
			p.entries[i].BaseURL = baseURL
			if apiKey != "" {
				p.entries[i].APIKey = apiKey
			}
			p.entries[i].ModelOverride = modelOverride
			return
		}
	}
}

// AddEntry adds a new upstream entry to the pool.
// The entry is marked available and enabled.
func (p *Pool) AddEntry(id uint, name, baseURL, apiKey, modelOverride string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.entries = append(p.entries, entry{
		UpstreamEntry: UpstreamEntry{
			ID:            id,
			Name:          name,
			BaseURL:       baseURL,
			APIKey:        apiKey,
			ModelOverride: modelOverride,
		},
		available: true,
		enabled:   true,
	})
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

// SetEnabled updates the enabled and available state of the upstream with the given name.
// Disabling marks the entry unavailable so it is skipped by Select.
// Enabling marks it available again so it re-enters the rotation.
func (p *Pool) SetEnabled(name string, enabled bool) {
	p.mu.Lock()
	defer p.mu.Unlock()
	for i := range p.entries {
		if p.entries[i].Name == name {
			p.entries[i].enabled = enabled
			p.entries[i].available = enabled
			return
		}
	}
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

