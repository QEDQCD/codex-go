package store

import (
	"database/sql"
	"os"
	"path/filepath"
	"testing"
	"time"

	_ "modernc.org/sqlite"
)

func setupStore(t *testing.T) (*Store, string) {
	t.Helper()
	tmpDir := t.TempDir()
	os.Setenv("HOME", tmpDir)
	os.Setenv("USERPROFILE", tmpDir)
	s, err := Open()
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { s.Close() })
	return s, tmpDir
}

func TestStoreCRUD(t *testing.T) {
	s, _ := setupStore(t)

	now := time.Now()
	sess := &Session{
		ID:           "test-session-1",
		Name:         "test",
		WorkDir:      "/tmp/test",
		Status:       "idle",
		CodexPID:     0,
		CreatedAt:    now,
		LastActiveAt: now,
		HistoryPath:  "/some/path.jsonl",
	}
	if err := s.InsertSession(sess); err != nil {
		t.Fatalf("insert: %v", err)
	}

	got, err := s.GetSession("test-session-1")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got.Name != "test" {
		t.Errorf("expected name test, got %s", got.Name)
	}
	if got.WorkDir != "/tmp/test" {
		t.Errorf("expected /tmp/test, got %s", got.WorkDir)
	}
}

func TestUpdateSessionStatus(t *testing.T) {
	s, _ := setupStore(t)

	s.InsertSession(&Session{
		ID:      "test-update-1",
		Name:    "test",
		WorkDir: "/tmp",
		Status:  "idle",
	})
	if err := s.UpdateSessionStatus("test-update-1", "active", 12345); err != nil {
		t.Fatalf("update: %v", err)
	}
	got, _ := s.GetSession("test-update-1")
	if got.Status != "active" {
		t.Errorf("expected active, got %s", got.Status)
	}
	if got.CodexPID != 12345 {
		t.Errorf("expected pid 12345, got %d", got.CodexPID)
	}
}

func TestListSessions(t *testing.T) {
	s, _ := setupStore(t)

	for i, id := range []string{"a", "b", "c"} {
		s.InsertSession(&Session{ID: id, Name: id, Status: "idle", LastActiveAt: time.Now().Add(time.Duration(i) * time.Hour)})
	}
	list, err := s.ListSessions()
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(list) != 3 {
		t.Errorf("expected 3, got %d", len(list))
	}
	// Should be ordered by last_active_at DESC
	if list[0].ID != "c" {
		t.Errorf("expected c first, got %s", list[0].ID)
	}
}

func TestGetActiveSession(t *testing.T) {
	s, _ := setupStore(t)

	s.InsertSession(&Session{ID: "idle-1", Status: "idle"})
	s.InsertSession(&Session{ID: "active-1", Status: "active"})
	s.InsertSession(&Session{ID: "idle-2", Status: "idle"})

	got, err := s.GetActiveSession()
	if err != nil {
		t.Fatalf("get active: %v", err)
	}
	if got.ID != "active-1" {
		t.Errorf("expected active-1, got %s", got.ID)
	}
}

func TestGetActiveSession_None(t *testing.T) {
	s, _ := setupStore(t)
	s.InsertSession(&Session{ID: "idle-1", Status: "idle"})
	_, err := s.GetActiveSession()
	if err == nil {
		t.Error("expected error when no active session")
	}
}

func TestDeleteSession(t *testing.T) {
	s, _ := setupStore(t)

	s.InsertSession(&Session{ID: "to-delete", Status: "idle"})
	if err := s.DeleteSession("to-delete"); err != nil {
		t.Fatalf("delete: %v", err)
	}
	_, err := s.GetSession("to-delete")
	if err == nil {
		t.Error("expected error after delete")
	}
}

func TestMigrateAddsCodexPIDToExistingSchema(t *testing.T) {
	tmpDir := t.TempDir()
	os.Setenv("HOME", tmpDir)
	os.Setenv("USERPROFILE", tmpDir)
	dbDir := filepath.Join(tmpDir, ".codex-go")
	if err := os.MkdirAll(dbDir, 0755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	db, err := sql.Open("sqlite", filepath.Join(dbDir, "codex-go.db"))
	if err != nil {
		t.Fatalf("open seed db: %v", err)
	}
	_, err = db.Exec(`CREATE TABLE sessions (
		seq INTEGER PRIMARY KEY AUTOINCREMENT,
		id TEXT UNIQUE NOT NULL,
		name TEXT NOT NULL DEFAULT '',
		work_dir TEXT NOT NULL DEFAULT '',
		model TEXT NOT NULL DEFAULT '',
		status TEXT NOT NULL DEFAULT 'idle',
		created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
		last_active_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
		history_path TEXT NOT NULL DEFAULT '',
		message_count INTEGER NOT NULL DEFAULT 0,
		git_branch TEXT NOT NULL DEFAULT ''
	)`)
	if err != nil {
		t.Fatalf("create seed schema: %v", err)
	}
	db.Close()

	s, err := Open()
	if err != nil {
		t.Fatalf("open migrated store: %v", err)
	}
	defer s.Close()

	if err := s.InsertSession(&Session{ID: "migrated", Status: "active", CodexPID: 123}); err != nil {
		t.Fatalf("insert after migration: %v", err)
	}
	got, err := s.GetSession("migrated")
	if err != nil {
		t.Fatalf("get after migration: %v", err)
	}
	if got.CodexPID != 123 {
		t.Fatalf("expected codex pid 123, got %d", got.CodexPID)
	}
}

func TestSyncFromDiscoveryDoesNotPanic(t *testing.T) {
	s, _ := setupStore(t)
	discovered := []struct {
		ID           string
		Name         string
		WorkDir      string
		Model        string
		Modified     string
		MessageCount int
		GitBranch    string
		FilePath     string
	}{
		{
			ID:           "discovered-1",
			Name:         "Discovered",
			WorkDir:      "/tmp/project",
			Model:        "gpt-5.5",
			Modified:     time.Now().Format(time.RFC3339),
			MessageCount: 3,
			GitBranch:    "main",
			FilePath:     "/tmp/session.jsonl",
		},
	}

	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("SyncFromDiscovery panicked: %v", r)
		}
	}()
	if err := s.SyncFromDiscovery(discovered); err != nil {
		t.Fatalf("sync discovery: %v", err)
	}
}
