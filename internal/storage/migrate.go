package storage

import (
	"embed"
	"fmt"
	"io/fs"
	"path"
	"slices"
	"strings"
)

//go:embed migrations/*.sql
var migrationFiles embed.FS

type Migration struct {
	Version string
	SQL     string
}

func Migrations() ([]Migration, error) {
	return loadMigrations(migrationFiles, "migrations")
}

func loadMigrations(fsys fs.FS, dir string) ([]Migration, error) {
	entries, err := fs.ReadDir(fsys, dir)
	if err != nil {
		return nil, fmt.Errorf("read migrations: %w", err)
	}
	var result []Migration
	seen := map[string]bool{}
	for _, e := range entries {
		if e.IsDir() || path.Ext(e.Name()) != ".sql" {
			continue
		}
		version, _, _ := strings.Cut(strings.TrimSuffix(e.Name(), ".sql"), "_")
		if version == "" || seen[version] {
			return nil, fmt.Errorf("migration %s has an empty or duplicate version", e.Name())
		}
		seen[version] = true
		data, err := fs.ReadFile(fsys, path.Join(dir, e.Name()))
		if err != nil {
			return nil, fmt.Errorf("read migration %s: %w", e.Name(), err)
		}
		result = append(result, Migration{Version: version, SQL: string(data)})
	}
	slices.SortFunc(result, func(a, b Migration) int { return strings.Compare(a.Version, b.Version) })
	return result, nil
}
