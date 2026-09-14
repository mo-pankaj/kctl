package kube

import (
	"fmt"
	"os"
	"path/filepath"

	"sigs.k8s.io/yaml"
)

// Config is kctl's own configuration, distinct from kubeconfig.
type Config struct {
	Risk []RiskRule `json:"risk"`
}

// DefaultConfigPath returns the standard config location: $XDG_CONFIG_HOME/kctl,
// falling back to ~/.config/kctl.
//
// Deliberately NOT os.UserConfigDir: on macOS that resolves to
// ~/Library/Application Support, which is not where anyone looks for a command
// line tool's config, and not where the design said this file lives. A config
// file nobody can find is the same as no config file — and here that silently
// means no cluster is treated as protected.
func DefaultConfigPath() (path string) {
	dir := os.Getenv("XDG_CONFIG_HOME")
	if dir == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			path = filepath.Join(os.TempDir(), "kctl", "config.yaml")

			return path
		}

		dir = filepath.Join(home, ".config")
	}

	path = filepath.Join(dir, "kctl", "config.yaml")

	return path
}

// LoadConfig reads the config file. A missing file is not an error: kctl is
// fully usable without one, and the only thing it configures is which clusters
// demand a typed confirmation.
func LoadConfig(path string) (cfg Config, err error) {
	raw, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		err = nil

		return cfg, err
	}

	if err != nil {
		err = fmt.Errorf("error-reading-config :%w", err)

		return cfg, err
	}

	err = yaml.Unmarshal(raw, &cfg)
	if err != nil {
		err = fmt.Errorf("error-parsing-config :%s: %w", path, err)

		return cfg, err
	}

	return cfg, err
}
