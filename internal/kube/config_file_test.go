package kube_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mo-pankaj/kctl/internal/kube"
)

func TestLoadConfigReadsRiskRules(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.yaml")
	os.WriteFile(path, []byte("risk:\n  - server: https://api.zulu.example.com\n    level: protected\n"), 0o600)

	cfg, err := kube.LoadConfig(path)
	if err != nil {
		t.Fatalf("LoadConfig returned error: %v", err)
	}

	if len(cfg.Risk) != 1 || cfg.Risk[0].Level != kube.RiskProtected {
		t.Fatalf("parsed rules = %+v", cfg.Risk)
	}

	if kube.RiskFor("https://api.zulu.example.com", cfg.Risk) != kube.RiskProtected {
		t.Fatal("rule from config did not take effect")
	}
}

func TestMissingConfigIsNotAnError(t *testing.T) {
	// kctl must run without a config file; the only thing it configures is
	// which clusters demand a typed confirmation.
	cfg, err := kube.LoadConfig(filepath.Join(t.TempDir(), "absent.yaml"))
	if err != nil {
		t.Fatalf("a missing config should not be an error, got: %v", err)
	}

	if len(cfg.Risk) != 0 {
		t.Fatalf("expected no rules, got %+v", cfg.Risk)
	}
}

func TestMalformedConfigIsAnError(t *testing.T) {
	path := filepath.Join(t.TempDir(), "bad.yaml")
	os.WriteFile(path, []byte("risk: [this is not a list of rules\n"), 0o600)

	_, err := kube.LoadConfig(path)
	if err == nil {
		t.Fatal("a malformed config should be reported, not silently ignored")
	}
}

func TestDefaultConfigPathIsUnderDotConfig(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", "")

	got := kube.DefaultConfigPath()

	// Must not land in ~/Library/Application Support on macOS: a config file
	// nobody finds silently means no cluster is protected.
	if !strings.HasSuffix(got, filepath.Join(".config", "kctl", "config.yaml")) {
		t.Fatalf("DefaultConfigPath() = %q, want it under ~/.config/kctl", got)
	}
}

func TestDefaultConfigPathHonoursXDG(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", "/tmp/xdg-test")

	got := kube.DefaultConfigPath()

	if got != "/tmp/xdg-test/kctl/config.yaml" {
		t.Fatalf("DefaultConfigPath() = %q, want it to honour XDG_CONFIG_HOME", got)
	}
}
