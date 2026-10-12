package config

import (
	"errors"
	"fmt"
	"net"
	"net/url"
	"os"
	"strconv"
	"strings"
)

const (
	PostgresHost     = "soundmate-pg"
	PostgresPort     = 5432
	PostgresName     = "soundmate"
	PostgresPassword = "soundmate"
	LavalinkHost     = "soundmate-ll"
	LavalinkPort     = 2333
	LavalinkPassword = "soundmate"
)

type Postgres struct {
	Host     string
	Port     int
	User     string
	Password string
	Database string
}

func (p Postgres) DSN() string {
	query := url.Values{}
	query.Set("sslmode", "disable")
	dsn := url.URL{
		Scheme:   "postgres",
		User:     url.UserPassword(p.User, p.Password),
		Host:     net.JoinHostPort(p.Host, strconv.Itoa(p.Port)),
		Path:     "/" + p.Database,
		RawQuery: query.Encode(),
	}
	return dsn.String()
}

type Lavalink struct {
	Host     string
	Port     int
	Password string
}

func (l Lavalink) Address() string {
	return net.JoinHostPort(l.Host, strconv.Itoa(l.Port))
}

type Config struct {
	Token    string
	Debug    bool
	Postgres Postgres
	Lavalink Lavalink
}

type Lookup func(key string) (string, bool)

func Load(envFile string) (Config, error) {
	values, err := ReadEnvFile(envFile)
	if err != nil {
		return Config{}, err
	}
	return Parse(func(key string) (string, bool) {
		if v, ok := os.LookupEnv(key); ok {
			return v, true
		}
		v, ok := values[key]
		return v, ok
	})
}

func Parse(lookup Lookup) (Config, error) {
	p := parser{lookup: lookup}
	cfg := Config{
		Token: p.required("BOT_TOKEN"),
		Debug: p.boolean("DEBUG", false),
		Postgres: Postgres{
			Host:     PostgresHost,
			Port:     PostgresPort,
			User:     PostgresName,
			Password: PostgresPassword,
			Database: PostgresName,
		},
		Lavalink: Lavalink{
			Host:     LavalinkHost,
			Port:     LavalinkPort,
			Password: LavalinkPassword,
		},
	}
	if len(p.errs) > 0 {
		return Config{}, fmt.Errorf("invalid configuration: %w", errors.Join(p.errs...))
	}
	return cfg, nil
}

type parser struct {
	lookup Lookup
	errs   []error
}

func (p *parser) value(key string) (string, bool) {
	v, ok := p.lookup(key)
	v = strings.TrimSpace(v)
	return v, ok && v != ""
}

func (p *parser) required(key string) string {
	v, ok := p.value(key)
	if !ok {
		p.errs = append(p.errs, fmt.Errorf("%s is required", key))
	}
	return v
}

func (p *parser) boolean(key string, fallback bool) bool {
	v, ok := p.value(key)
	if !ok {
		return fallback
	}
	b, err := strconv.ParseBool(v)
	if err != nil {
		p.errs = append(p.errs, fmt.Errorf("%s must be a boolean, got %q", key, v))
		return fallback
	}
	return b
}
