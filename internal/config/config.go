package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
)

const (
	// EnvSession lets an agent supply the session without touching disk.
	EnvSession = "WANDERLOG_SESSION"
	EnvBaseURL = "WANDERLOG_BASE_URL"
)

type Config struct {
	Session string `json:"session"`
	Email   string `json:"email,omitempty"`
	UserID  int    `json:"userId,omitempty"`
}

func Dir() (string, error) {
	if d := os.Getenv("WANDERLOG_CONFIG_DIR"); d != "" {
		return d, nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".config", "wanderlog"), nil
}

func Path() (string, error) {
	d, err := Dir()
	if err != nil {
		return "", err
	}
	return filepath.Join(d, "credentials.json"), nil
}

func Load() (*Config, error) {
	if s := os.Getenv(EnvSession); s != "" {
		return &Config{Session: s}, nil
	}
	p, err := Path()
	if err != nil {
		return nil, err
	}
	b, err := os.ReadFile(p)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return &Config{}, nil
		}
		return nil, err
	}
	cfg := &Config{}
	if err := json.Unmarshal(b, cfg); err != nil {
		return nil, fmt.Errorf("parsing %s: %w", p, err)
	}
	return cfg, nil
}

// Save writes credentials with owner-only permissions. The session cookie is a
// bearer credential valid for about a year, so it is treated like a password.
func (c *Config) Save() error {
	d, err := Dir()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(d, 0o700); err != nil {
		return err
	}
	p, err := Path()
	if err != nil {
		return err
	}
	b, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return err
	}
	if err := os.WriteFile(p, append(b, '\n'), 0o600); err != nil {
		return err
	}
	// os.WriteFile's perm arg is a no-op for ACLs on Windows; the file inherits
	// the user profile directory's ACL, which is already owner-scoped.
	if runtime.GOOS != "windows" {
		_ = os.Chmod(p, 0o600)
	}
	return nil
}

func Clear() error {
	p, err := Path()
	if err != nil {
		return err
	}
	if err := os.Remove(p); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	return nil
}
