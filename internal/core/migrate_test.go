package core

import (
	"context"
	"database/sql"
	"path/filepath"
	"testing"

	_ "modernc.org/sqlite"
)

func TestMigratesExistingDatabase(t *testing.T) {
	path := filepath.Join(t.TempDir(), "old.db")
	old, err := sql.Open("sqlite", "file:"+path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := old.Exec(schema); err != nil {
		t.Fatal(err)
	}
	old.Exec(`INSERT INTO users (name, token_hash, created_at) VALUES ('Old', 'h', 0)`)
	old.Exec(`INSERT INTO groups (code, name, created_by, created_at) VALUES ('abc123', 'legacy', 1, 0)`)
	old.Close()

	s, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	g, err := s.GroupByCode(context.Background(), "abc123")
	if err != nil || g.Kind != "friends" {
		t.Fatalf("legacy group after migration = %+v %v", g, err)
	}
	s.Close()
	if s2, err := Open(path); err != nil {
		t.Fatalf("reopen: %v", err)
	} else {
		s2.Close()
	}
}
