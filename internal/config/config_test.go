package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func lookupFrom(values map[string]string) Lookup {
	return func(key string) (string, bool) {
		v, ok := values[key]
		return v, ok
	}
}

func validEnv() map[string]string {
	return map[string]string{"BOT_TOKEN": "token"}
}

func TestParseDefaults(t *testing.T) {
	cfg, err := Parse(lookupFrom(validEnv()))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.Token != "token" || cfg.Debug {
		t.Errorf("unexpected defaults: %+v", cfg)
	}
	want := Postgres{Host: PostgresHost, Port: PostgresPort, User: PostgresName, Password: PostgresPassword, Database: PostgresName}
	if cfg.Postgres != want {
		t.Errorf("postgres = %+v, want %+v", cfg.Postgres, want)
	}
	if cfg.Lavalink.Address() != "soundmate-ll:2333" || cfg.Lavalink.Password != LavalinkPassword {
		t.Errorf("unexpected lavalink defaults: %+v", cfg.Lavalink)
	}
}

func TestParseOverrides(t *testing.T) {
	env := validEnv()
	env["DEBUG"] = "1"
	cfg, err := Parse(lookupFrom(env))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !cfg.Debug {
		t.Errorf("unexpected config: %+v", cfg)
	}
}

func TestParseCollectsAllErrors(t *testing.T) {
	_, err := Parse(lookupFrom(map[string]string{
		"DEBUG":     "maybe",
		"BOT_TOKEN": "   ",
	}))
	if err == nil {
		t.Fatal("expected an error")
	}
	for _, want := range []string{"BOT_TOKEN is required", "DEBUG must be a boolean"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error %q does not mention %q", err, want)
		}
	}
}

func TestPostgresDSNEscapesCredentials(t *testing.T) {
	p := Postgres{Host: "db", Port: 5432, User: "us@r", Password: "p@ss:w/rd", Database: "music"}
	want := "postgres://us%40r:p%40ss%3Aw%2Frd@db:5432/music?sslmode=disable"
	if got := p.DSN(); got != want {
		t.Errorf("DSN() = %q, want %q", got, want)
	}
}

func TestParseEnv(t *testing.T) {
	input := `
# comment
BOT_TOKEN=abc
export USER_NAME = music
DOUBLE="quoted # value"
SINGLE='single'
DEBUG=true # inline comment
EMPTY=
`
	values, err := ParseEnv(strings.NewReader(input))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := map[string]string{
		"BOT_TOKEN": "abc",
		"USER_NAME": "music",
		"DOUBLE":    "quoted # value",
		"SINGLE":    "single",
		"DEBUG":     "true",
		"EMPTY":     "",
	}
	for k, v := range want {
		if values[k] != v {
			t.Errorf("%s = %q, want %q", k, values[k], v)
		}
	}
}

func TestParseEnvRejectsMalformedLine(t *testing.T) {
	if _, err := ParseEnv(strings.NewReader("VALID=1\nbroken line\n")); err == nil {
		t.Fatal("expected an error for a line without '='")
	}
}

func TestLoadPrefersProcessEnvironment(t *testing.T) {
	path := filepath.Join(t.TempDir(), ".env")
	content := "BOT_TOKEN=file\nDEBUG=true\n"
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("BOT_TOKEN", "process")
	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.Token != "process" {
		t.Errorf("Token = %q, want value from process environment", cfg.Token)
	}
	if !cfg.Debug {
		t.Errorf("Debug = %v, want value from env file", cfg.Debug)
	}
}

func TestReadEnvFileMissingIsNotAnError(t *testing.T) {
	values, err := ReadEnvFile(filepath.Join(t.TempDir(), "missing.env"))
	if err != nil || len(values) != 0 {
		t.Fatalf("ReadEnvFile() = %v, %v; want empty map and nil error", values, err)
	}
}
