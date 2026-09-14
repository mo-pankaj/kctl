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

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// The factory rebuilds a clientset and informer for whatever context the
	// user switches to; the session owns teardown of the previous one.
	factory := func(ctx context.Context, contextName, namespace string) (core.PodReader, core.NamespaceLister, error) {
		cfg, err := store.RESTConfig(contextName)
		if err != nil {
			err = fmt.Errorf("error-building-rest-config :%w", err)
			return nil, nil, err
		}

		c, err := kube.NewClientset(cfg)
		if err != nil {
			err = fmt.Errorf("error-building-clientset :%w", err)
			return nil, nil, err
		}

		src := kube.NewPodSource(logger, c, namespace)

		err = src.Start(ctx)
		if err != nil {
			err = fmt.Errorf("error-starting-pod-source :%w", err)
			return nil, nil, err
		}

		return src, kube.NewNamespaceStore(logger, c), nil
	}

	session := ui.NewSession(logger, factory)
	defer session.Close()

	// The session owns every source, including the first, so a later switch
	// tears this one down instead of leaving it running for the whole session.
	active, err := session.Switch(ctx, current.Name, current.Namespace)
	if err != nil {
		fmt.Fprintf(os.Stderr, "kctl: cannot reach cluster %s: %v\n", current.Server, err)
		return 1
	}

	model := ui.New(logger, store, active.Pods, core.Selector{Namespace: active.Namespace}).
		WithSession(session)

	program := tea.NewProgram(model)

	_, err = program.Run()
	if err != nil {
		logger.Error("error-running-program", zap.Error(err))
		fmt.Fprintf(os.Stderr, "kctl: %v\n", err)
		return 1
	}

	return code
}
