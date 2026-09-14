package logging_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mo-pankaj/kctl/internal/logging"
)

func TestNewCreatesParentDirAndWritesFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "nested", "dir", "kctl.log")

	logger, err := logging.New(path)
	if err != nil {
		t.Fatalf("New returned error: %v", err)
	}

	logger.Info("hello-from-test")

	err = logger.Sync()
	if err != nil {
		t.Fatalf("Sync returned error: %v", err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("log file not created at %s: %v", path, err)
	}

	if !strings.Contains(string(data), "hello-from-test") {
		t.Fatalf("log file missing entry, got: %s", data)
	}
}

func TestDefaultPathEndsWithKctlLog(t *testing.T) {
	got := logging.DefaultPath()

	if filepath.Base(got) != "kctl.log" {
		t.Fatalf("DefaultPath() = %q, want basename kctl.log", got)
	}

	if !filepath.IsAbs(got) {
		t.Fatalf("DefaultPath() = %q, want an absolute path", got)
	}
}
