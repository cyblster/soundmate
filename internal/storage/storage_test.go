package storage

import (
	"strconv"
	"strings"
	"testing"
	"testing/fstest"
)

func TestParseAndFormatID(t *testing.T) {
	for _, id := range []string{"1", "123456789012345678", "9223372036854775807"} {
		v, err := ParseID(id)
		if err != nil {
			t.Fatalf("ParseID(%q) error: %v", id, err)
		}
		if FormatID(v) != id {
			t.Errorf("round trip of %q produced %q", id, FormatID(v))
		}
	}
	for _, id := range []string{"", "0", "-5", "abc", "9223372036854775808"} {
		if _, err := ParseID(id); err == nil {
			t.Errorf("ParseID(%q) must fail", id)
		}
	}
}

func TestParseIDs(t *testing.T) {
	ids, err := parseIDs("1", "2", "3")
	if err != nil || len(ids) != 3 || ids[2] != 3 {
		t.Fatalf("parseIDs() = %v, %v", ids, err)
	}
	if _, err := parseIDs("1", "bad"); err == nil {
		t.Fatal("parseIDs() must fail on invalid input")
	}
}

func TestEmbeddedMigrations(t *testing.T) {
	migrations, err := Migrations()
	if err != nil {
		t.Fatalf("Migrations() error: %v", err)
	}
	if len(migrations) < 2 {
		t.Fatalf("expected at least 2 migrations, got %d", len(migrations))
	}
	for i, m := range migrations {
		if strings.TrimSpace(m.SQL) == "" {
			t.Errorf("migration %s is empty", m.Version)
		}
		if i > 0 && migrations[i-1].Version >= m.Version {
			t.Errorf("migrations are not ordered: %s before %s", migrations[i-1].Version, m.Version)
		}
	}
	if !strings.Contains(migrations[0].SQL, "CREATE TABLE IF NOT EXISTS guild") {
		t.Error("first migration must create the guild table idempotently")
	}
}

func TestLoadMigrationsSortsAndFilters(t *testing.T) {
	fsys := fstest.MapFS{
		"m/0002_second.sql": {Data: []byte("SELECT 2;")},
		"m/0001_first.sql":  {Data: []byte("SELECT 1;")},
		"m/README.md":       {Data: []byte("ignored")},
	}
	migrations, err := loadMigrations(fsys, "m")
	if err != nil {
		t.Fatalf("loadMigrations() error: %v", err)
	}
	if len(migrations) != 2 || migrations[0].Version != "0001" || migrations[1].SQL != "SELECT 2;" {
		t.Fatalf("unexpected migrations: %+v", migrations)
	}
}

func TestLoadMigrationsRejectsDuplicates(t *testing.T) {
	fsys := fstest.MapFS{
		"m/0001_a.sql": {Data: []byte("SELECT 1;")},
		"m/0001_b.sql": {Data: []byte("SELECT 2;")},
	}
	if _, err := loadMigrations(fsys, "m"); err == nil {
		t.Fatal("duplicate versions must be rejected")
	}
}

func TestHistoryRetentionMatchesHistorySize(t *testing.T) {
	migrations, err := Migrations()
	if err != nil {
		t.Fatal(err)
	}
	var retention string
	for _, m := range migrations {
		if strings.Contains(m.SQL, "CREATE TRIGGER history_keep_latest") {
			retention = m.SQL
		}
	}
	if retention == "" {
		t.Fatal("history retention trigger migration not found")
	}
	if !strings.Contains(retention, "OFFSET "+strconv.Itoa(HistorySize-1)+" LIMIT 1") {
		t.Errorf("trigger must keep the latest %d rows", HistorySize)
	}
	if !strings.Contains(retention, "rank > "+strconv.Itoa(HistorySize)) {
		t.Errorf("existing history must be trimmed to %d rows", HistorySize)
	}
}
