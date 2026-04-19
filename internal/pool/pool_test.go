package pool_test

import (
	"sync"
	"testing"
	"time"

	"one-codingplan/internal/crypto"
	"one-codingplan/internal/database"
	"one-codingplan/internal/models"
	"one-codingplan/internal/pool"
)

// encKey is exactly 32 bytes for AES-256.
var testEncKey = []byte("test-encryption-key-32bytes!!XXX")

func TestForceRotate(t *testing.T) {
	p := newTestPool(t, "a", "b", "c")
	seen := []string{}
	for i := 0; i < 3; i++ {
		name, err := p.ForceRotate()
		if err != nil {
			t.Fatalf("ForceRotate: %v", err)
		}
		seen = append(seen, name)
	}
	// Should cycle through all three distinct names
	unique := map[string]bool{}
	for _, n := range seen {
		unique[n] = true
	}
	if len(unique) != 3 {
		t.Errorf("expected 3 distinct upstreams in 3 rotations, got %v", seen)
	}
}

func TestForceRotate_AllUnavailable(t *testing.T) {
	p := newTestPool(t, "a", "b")
	// Mark all unavailable
	ids := []uint{}
	seen := map[string]bool{}
	for len(seen) < 2 {
		e, err := p.Select(nil)
		if err != nil {
			t.Fatalf("Select: %v", err)
		}
		if !seen[e.Name] {
			seen[e.Name] = true
			ids = append(ids, e.ID)
		}
	}
	for _, id := range ids {
		p.Mark(id, false)
	}
	_, err := p.ForceRotate()
	if err != pool.ErrNoUpstreams {
		t.Errorf("expected ErrNoUpstreams, got %v", err)
	}
}

func TestSelectWithFilter_Unrestricted(t *testing.T) {
	p := newTestPool(t, "kimi", "glm", "qwen")
	seen := map[string]bool{}
	for i := 0; i < 6; i++ {
		e, err := p.Select(nil)
		if err != nil {
			t.Fatalf("Select: %v", err)
		}
		seen[e.Name] = true
	}
	if !seen["kimi"] || !seen["glm"] || !seen["qwen"] {
		t.Errorf("unrestricted select should return all upstreams, got %v", seen)
	}
}

func TestSelectWithFilter_Restricted(t *testing.T) {
	p := newTestPool(t, "kimi", "glm", "qwen")
	for i := 0; i < 5; i++ {
		e, err := p.Select([]string{"kimi"})
		if err != nil {
			t.Fatalf("Select: %v", err)
		}
		if e.Name != "kimi" {
			t.Errorf("expected kimi, got %s", e.Name)
		}
	}
}

func TestSelectWithFilter_NoMatch(t *testing.T) {
	p := newTestPool(t, "kimi", "glm", "qwen")
	_, err := p.Select([]string{"nonexistent"})
	if err != pool.ErrNoUpstreams {
		t.Errorf("expected ErrNoUpstreams, got %v", err)
	}
}

func TestList(t *testing.T) {
	p := newTestPool(t, "a", "b")
	list := p.List()
	if len(list) != 2 {
		t.Fatalf("expected 2 entries, got %d", len(list))
	}
	for _, info := range list {
		if info.Name == "" {
			t.Error("Name should not be empty")
		}
		if info.BaseURL == "" {
			t.Error("BaseURL should not be empty")
		}
		if !info.Available {
			t.Errorf("expected Available=true for %s", info.Name)
		}
	}
}

func newTestPool(t *testing.T, names ...string) *pool.Pool {
	t.Helper()
	db, err := database.Open(":memory:")
	if err != nil {
		t.Fatalf("database.Open: %v", err)
	}
	if err := database.Migrate(db); err != nil {
		t.Fatalf("database.Migrate: %v", err)
	}
	for _, name := range names {
		enc, err := crypto.Encrypt(testEncKey, "sk-"+name)
		if err != nil {
			t.Fatalf("crypto.Encrypt(%s): %v", name, err)
		}
		u := models.Upstream{
			Name:      name,
			BaseURL:   "https://" + name + ".example.com",
			APIKeyEnc: enc,
			Enabled:   true,
		}
		if err := db.Create(&u).Error; err != nil {
			t.Fatalf("db.Create(%s): %v", name, err)
		}
	}
	p, err := pool.New(db, testEncKey, &pool.Config{RateLimitBackoff: 5 * time.Second})
	if err != nil {
		t.Fatalf("pool.New: %v", err)
	}
	t.Cleanup(func() { p.Stop() })
	return p
}

func TestSelect_RoundRobin(t *testing.T) {
	p := newTestPool(t, "a", "b")
	seen := map[string]int{}
	for i := 0; i < 10; i++ {
		e, err := p.Select(nil)
		if err != nil {
			t.Fatalf("Select: %v", err)
		}
		seen[e.Name]++
	}
	if seen["a"] != 5 || seen["b"] != 5 {
		t.Errorf("uneven distribution: %v", seen)
	}
}

func TestSelect_SkipsUnavailable(t *testing.T) {
	p := newTestPool(t, "a", "b")

	// Find a's ID by selecting until we get "a"
	var aID uint
	for i := 0; i < 4; i++ {
		e, err := p.Select(nil)
		if err != nil {
			t.Fatalf("Select: %v", err)
		}
		if e.Name == "a" {
			aID = e.ID
			break
		}
	}
	if aID == 0 {
		t.Fatal("could not find upstream a")
	}

	p.Mark(aID, false)
	for i := 0; i < 3; i++ {
		e, err := p.Select(nil)
		if err != nil {
			t.Fatalf("Select after mark: %v", err)
		}
		if e.Name != "b" {
			t.Errorf("expected b, got %s", e.Name)
		}
	}
}

func TestSelect_NoUpstreams(t *testing.T) {
	p := newTestPool(t, "a", "b")

	// Mark both unavailable by iterating to find their IDs
	var ids []uint
	seen := map[string]bool{}
	for len(ids) < 2 {
		e, err := p.Select(nil)
		if err != nil {
			break
		}
		if !seen[e.Name] {
			seen[e.Name] = true
			ids = append(ids, e.ID)
		}
	}
	for _, id := range ids {
		p.Mark(id, false)
	}

	_, err := p.Select(nil)
	if err != pool.ErrNoUpstreams {
		t.Errorf("expected ErrNoUpstreams, got %v", err)
	}
}

func TestMark_Available(t *testing.T) {
	p := newTestPool(t, "a", "b")

	// Find a's ID
	var aID uint
	seen := map[string]bool{}
	for len(seen) < 2 {
		e, err := p.Select(nil)
		if err != nil {
			t.Fatalf("Select: %v", err)
		}
		if e.Name == "a" && aID == 0 {
			aID = e.ID
		}
		seen[e.Name] = true
	}
	if aID == 0 {
		t.Fatal("could not find upstream a")
	}

	p.Mark(aID, false)
	// Should only see b now
	e, err := p.Select(nil)
	if err != nil {
		t.Fatalf("Select after marking a unavailable: %v", err)
	}
	if e.Name != "b" {
		t.Errorf("expected b after marking a unavailable, got %s", e.Name)
	}

	// Re-enable a
	p.Mark(aID, true)
	seenAfter := map[string]bool{}
	for i := 0; i < 4; i++ {
		e, err := p.Select(nil)
		if err != nil {
			t.Fatalf("Select after re-enabling a: %v", err)
		}
		seenAfter[e.Name] = true
	}
	if !seenAfter["a"] {
		t.Error("expected to see a after re-enabling, but did not")
	}
}

func TestSelect_Concurrent(t *testing.T) {
	p := newTestPool(t, "a", "b")
	var wg sync.WaitGroup
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 100; j++ {
				if _, err := p.Select(nil); err != nil {
					t.Errorf("concurrent Select: %v", err)
					return
				}
			}
		}()
	}
	wg.Wait()
}

func TestNew_LoadsFromDB(t *testing.T) {
	db, err := database.Open(":memory:")
	if err != nil {
		t.Fatalf("database.Open: %v", err)
	}
	if err := database.Migrate(db); err != nil {
		t.Fatalf("database.Migrate: %v", err)
	}

	enc, _ := crypto.Encrypt(testEncKey, "sk-x")
	// 2 enabled, 1 disabled.
	// GORM omits boolean false on Create due to gorm:"default:true"; update after create to force false.
	db.Create(&models.Upstream{Name: "x", BaseURL: "https://x.example.com", APIKeyEnc: enc, Enabled: true})
	db.Create(&models.Upstream{Name: "y", BaseURL: "https://y.example.com", APIKeyEnc: enc, Enabled: true})
	zUp := models.Upstream{Name: "z", BaseURL: "https://z.example.com", APIKeyEnc: enc}
	db.Create(&zUp)
	db.Model(&zUp).Update("enabled", false)

	p, err := pool.New(db, testEncKey, &pool.Config{RateLimitBackoff: 5 * time.Second})
	if err != nil {
		t.Fatalf("pool.New: %v", err)
	}
	defer p.Stop()

	// List() must return all 3 upstreams including the disabled one.
	list := p.List()
	if len(list) != 3 {
		t.Fatalf("expected List() to return 3 entries, got %d", len(list))
	}

	// Find z in the list and verify its flags.
	var zInfo *pool.UpstreamInfo
	for i := range list {
		if list[i].Name == "z" {
			zInfo = &list[i]
		}
	}
	if zInfo == nil {
		t.Fatal("disabled upstream z not found in List()")
	}
	if zInfo.Enabled {
		t.Error("z.Enabled should be false")
	}
	if zInfo.Available {
		t.Error("z.Available should be false")
	}

	// Select() must still skip z.
	seen := map[string]bool{}
	for i := 0; i < 4; i++ {
		e, err := p.Select(nil)
		if err != nil {
			t.Fatalf("Select: %v", err)
		}
		seen[e.Name] = true
	}
	if seen["z"] {
		t.Error("disabled upstream z should not be returned by Select()")
	}
	if !seen["x"] || !seen["y"] {
		t.Errorf("expected both x and y from Select(), got %v", seen)
	}

	// SetEnabled(z, true) should make z selectable.
	p.SetEnabled("z", true)
	seenAfter := map[string]bool{}
	for i := 0; i < 6; i++ {
		e, err := p.Select(nil)
		if err != nil {
			t.Fatalf("Select after SetEnabled(z, true): %v", err)
		}
		seenAfter[e.Name] = true
	}
	if !seenAfter["z"] {
		t.Error("expected z to appear in Select() after SetEnabled(z, true)")
	}
}

func TestNew_DecryptsKeys(t *testing.T) {
	db, err := database.Open(":memory:")
	if err != nil {
		t.Fatalf("database.Open: %v", err)
	}
	if err := database.Migrate(db); err != nil {
		t.Fatalf("database.Migrate: %v", err)
	}

	enc, err := crypto.Encrypt(testEncKey, "sk-secret-key")
	if err != nil {
		t.Fatalf("crypto.Encrypt: %v", err)
	}
	db.Create(&models.Upstream{Name: "test", BaseURL: "https://test.example.com", APIKeyEnc: enc, Enabled: true})

	p, err := pool.New(db, testEncKey, &pool.Config{RateLimitBackoff: 5 * time.Second})
	if err != nil {
		t.Fatalf("pool.New: %v", err)
	}
	defer p.Stop()

	e, err := p.Select(nil)
	if err != nil {
		t.Fatalf("Select: %v", err)
	}
	if e.APIKey != "sk-secret-key" {
		t.Errorf("expected decrypted key sk-secret-key, got %q", e.APIKey)
	}
}
