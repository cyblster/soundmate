package config

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"strings"
)

func ReadEnvFile(path string) (map[string]string, error) {
	if path == "" {
		return map[string]string{}, nil
	}
	f, err := os.Open(path)
	if errors.Is(err, fs.ErrNotExist) {
		return map[string]string{}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("open env file: %w", err)
	}
	defer f.Close()
	return ParseEnv(f)
}

func ParseEnv(r io.Reader) (map[string]string, error) {
	values := map[string]string{}
	scanner := bufio.NewScanner(r)
	line := 0
	for scanner.Scan() {
		line++
		text := strings.TrimSpace(scanner.Text())
		if text == "" || strings.HasPrefix(text, "#") {
			continue
		}
		text = strings.TrimPrefix(text, "export ")
		key, value, found := strings.Cut(text, "=")
		key = strings.TrimSpace(key)
		if !found || key == "" {
			return nil, fmt.Errorf("env file line %d: expected KEY=VALUE", line)
		}
		values[key] = unquote(strings.TrimSpace(value))
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("read env file: %w", err)
	}
	return values, nil
}

func unquote(value string) string {
	if len(value) >= 2 {
		first, last := value[0], value[len(value)-1]
		if (first == '"' || first == '\'') && first == last {
			return value[1 : len(value)-1]
		}
	}
	if i := strings.Index(value, " #"); i >= 0 {
		return strings.TrimSpace(value[:i])
	}
	return value
}
