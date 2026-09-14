// Package logging builds the file-backed logger kctl uses for diagnostics.
//
// kctl is a full-screen TUI: Bubble Tea owns stdout, so anything written there
// corrupts the display. Every log line goes to a file instead. The log is
// forensics, not error reporting — errors the user needs to see are surfaced in
// the UI.
package logging

import (
	"fmt"
	"os"
	"path/filepath"

	"go.uber.org/zap"
)

// New builds a logger writing to path, creating parent directories as needed.
func New(path string) (logger *zap.Logger, err error) {
	err = os.MkdirAll(filepath.Dir(path), 0o755)
	if err != nil {
		err = fmt.Errorf("error-creating-log-directory :%w", err)
		return logger, err
	}

	cfg := zap.NewProductionConfig()
	cfg.OutputPaths = []string{path}
	cfg.ErrorOutputPaths = []string{path}

	logger, err = cfg.Build()
	if err != nil {
		err = fmt.Errorf("error-building-logger :%w", err)
		return logger, err
	}

	return logger, err
}

// DefaultPath returns the standard log location, falling back to the temp
// directory when the user cache directory cannot be determined.
func DefaultPath() (path string) {
	dir, err := os.UserCacheDir()
	if err != nil {
		path = filepath.Join(os.TempDir(), "kctl", "kctl.log")
		return path
	}

	path = filepath.Join(dir, "kctl", "kctl.log")
	return path
}
