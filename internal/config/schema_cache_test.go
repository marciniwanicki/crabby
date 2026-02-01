package config

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestSchemaCache_SetAndGet(t *testing.T) {
	// Create temp directory for test
	tmpDir, err := os.MkdirTemp("", "schema_cache_test")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(tmpDir) })

	cache := &SchemaCache{cacheDir: tmpDir}

	schema := &CachedSchema{
		Command:  "test-cmd",
		Schema:   map[string]any{"name": "test-cmd", "description": "A test command"},
		HelpText: "Test help text",
	}

	// Set
	if err := cache.Set(schema); err != nil {
		t.Fatalf("failed to set schema: %v", err)
	}

	// Get
	retrieved, ok := cache.Get("test-cmd")
	if !ok {
		t.Fatal("expected to find cached schema")
	}

	if retrieved.Command != schema.Command {
		t.Errorf("expected command %s, got %s", schema.Command, retrieved.Command)
	}

	if retrieved.HelpText != schema.HelpText {
		t.Errorf("expected help text %s, got %s", schema.HelpText, retrieved.HelpText)
	}
}

func TestSchemaCache_GetNonExistent(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "schema_cache_test")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(tmpDir) })

	cache := &SchemaCache{cacheDir: tmpDir}

	_, ok := cache.Get("nonexistent")
	if ok {
		t.Error("expected not to find non-existent schema")
	}
}

func TestSchemaCache_Delete(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "schema_cache_test")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(tmpDir) })

	cache := &SchemaCache{cacheDir: tmpDir}

	schema := &CachedSchema{
		Command: "to-delete",
		Schema:  map[string]any{},
	}

	_ = cache.Set(schema)

	if err := cache.Delete("to-delete"); err != nil {
		t.Fatalf("failed to delete: %v", err)
	}

	_, ok := cache.Get("to-delete")
	if ok {
		t.Error("expected schema to be deleted")
	}
}

func TestSchemaCache_List(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "schema_cache_test")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(tmpDir) })

	cache := &SchemaCache{cacheDir: tmpDir}

	_ = cache.Set(&CachedSchema{Command: "cmd1", Schema: map[string]any{}})
	_ = cache.Set(&CachedSchema{Command: "cmd2", Schema: map[string]any{}})

	list, err := cache.List()
	if err != nil {
		t.Fatalf("failed to list: %v", err)
	}

	if len(list) != 2 {
		t.Errorf("expected 2 items, got %d", len(list))
	}
}

func TestSchemaCache_Clear(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "schema_cache_test")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(tmpDir) })

	cache := &SchemaCache{cacheDir: tmpDir}

	_ = cache.Set(&CachedSchema{Command: "cmd1", Schema: map[string]any{}})
	_ = cache.Set(&CachedSchema{Command: "cmd2", Schema: map[string]any{}})

	if err := cache.Clear(); err != nil {
		t.Fatalf("failed to clear: %v", err)
	}

	list, _ := cache.List()
	if len(list) != 0 {
		t.Errorf("expected 0 items after clear, got %d", len(list))
	}
}

func TestSchemaCache_Expiration(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "schema_cache_test")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(tmpDir) })

	cache := &SchemaCache{cacheDir: tmpDir}

	// Create an expired schema (manually set old date)
	schema := &CachedSchema{
		Command:     "expired-cmd",
		Schema:      map[string]any{},
		GeneratedAt: time.Now().Add(-8 * 24 * time.Hour), // 8 days ago
	}

	// Write directly to bypass Set which updates GeneratedAt
	data := []byte(`{"command":"expired-cmd","schema":{},"help_text":"","generated_at":"2020-01-01T00:00:00Z"}`)
	path := filepath.Join(tmpDir, "expired-cmd.json")
	//nolint:gosec // G306: test file, permissions are fine
	_ = os.WriteFile(path, data, 0600)

	_, ok := cache.Get("expired-cmd")
	if ok {
		t.Error("expected expired schema to not be returned")
	}

	// Non-expired should work
	_ = cache.Set(&CachedSchema{Command: "fresh-cmd", Schema: map[string]any{}})
	_, ok = cache.Get("fresh-cmd")
	if !ok {
		t.Error("expected fresh schema to be returned")
	}

	_ = schema // Use the variable to avoid unused warning
}

func TestSanitizeFilename(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"simple", "simple"},
		{"with-dash", "with-dash"},
		{"with_underscore", "with_underscore"},
		{"with/slash", "with_slash"},
		{"with space", "with_space"},
		{"with.dot", "with_dot"},
		{"MixedCase123", "MixedCase123"},
	}

	for _, tc := range tests {
		t.Run(tc.input, func(t *testing.T) {
			result := sanitizeFilename(tc.input)
			if result != tc.expected {
				t.Errorf("sanitizeFilename(%s) = %s, want %s", tc.input, result, tc.expected)
			}
		})
	}
}
