// Command kctl is a terminal UI for everyday kubectl work.
package main

import (
	"context"
	"fmt"
	"os"
	"runtime/debug"

	tea "charm.land/bubbletea/v2"
	"go.uber.org/zap"

	"github.com/mo-pankaj/kctl/internal/core"
	"github.com/mo-pankaj/kctl/internal/kube"
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

	store, err := kube.NewContextStore(logger, "")
	if err != nil {
		fmt.Fprintf(os.Stderr, "kctl: %v\n", err)
		return 1
	}

	current := store.Current()

	restCfg, err := store.RESTConfig(current.Name)
	if err != nil {
		fmt.Fprintf(os.Stderr, "kctl: %v\n", err)
		return 1
	}

	client, err := kube.NewClientset(restCfg)
	if err != nil {
		fmt.Fprintf(os.Stderr, "kctl: %v\n", err)
		return 1
	}

	source := kube.NewPodSource(logger, client, current.Namespace)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	err = source.Start(ctx)
	if err != nil {
		fmt.Fprintf(os.Stderr, "kctl: cannot reach cluster %s: %v\n", current.Server, err)
		return 1
	}

	sel := core.Selector{Namespace: current.Namespace}

	program := tea.NewProgram(ui.New(logger, store, source, sel))

	_, err = program.Run()
	if err != nil {
		logger.Error("error-running-program", zap.Error(err))
		fmt.Fprintf(os.Stderr, "kctl: %v\n", err)
		return 1
	}

	return code
}
