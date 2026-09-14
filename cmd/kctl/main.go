// Command kctl is a terminal UI for everyday kubectl work.
package main

import (
	"fmt"
	"os"
	"runtime/debug"

	tea "github.com/charmbracelet/bubbletea"
	"go.uber.org/zap"

	"github.com/mo-pankaj/kctl/internal/logging"
	"github.com/mo-pankaj/kctl/internal/ui"
)

func main() {
	code := run()
	os.Exit(code)
}

// run holds the real body so deferred cleanup executes before os.Exit.
func run() (code int) {
	defer func() {
		r := recover()
		if r != nil {
			fmt.Fprintf(os.Stderr, "kctl panic: %v\n\n%s\n", r, debug.Stack())
			code = 2
		}
	}()

	logPath := logging.DefaultPath()

	logger, err := logging.New(logPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "kctl: cannot open log file %s: %v\n", logPath, err)
		return 1
	}
	defer func() { _ = logger.Sync() }()

	program := tea.NewProgram(ui.New(logger), tea.WithAltScreen())

	_, err = program.Run()
	if err != nil {
		logger.Error("error-running-program", zap.Error(err))
		fmt.Fprintf(os.Stderr, "kctl: %v\n", err)
		return 1
	}

	return code
}
