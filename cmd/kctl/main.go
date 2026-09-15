// Command kctl is a terminal UI for everyday kubectl work.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"runtime"
	"runtime/debug"

	tea "charm.land/bubbletea/v2"
	"go.uber.org/zap"

	"github.com/mo-pankaj/kctl/internal/core"
	"github.com/mo-pankaj/kctl/internal/kube"
	"github.com/mo-pankaj/kctl/internal/logging"
	"github.com/mo-pankaj/kctl/internal/ui"
)

// Overridden at build time:
//
//	go build -ldflags "-X main.version=v0.1.0 -X main.commit=$(git rev-parse --short HEAD)"
var (
	version = "dev"
	commit  = "none"
)

const usage = `kctl — a terminal UI for everyday kubectl work.

Usage:
  kctl [flags]

Flags:
  -context string   kube context to start in (default: the kubeconfig's current context)
  -n, -namespace    namespace to start in (default: the context's namespace, "all" for every namespace)
  -kubeconfig path  explicit kubeconfig path (default: $KUBECONFIG, then ~/.kube/config)
  -version          print version and exit
  -help             print this message

Keys:
  /  filter      s  sort        enter  logs      d  describe
  a  apply       c  contexts    :  command       q  quit

Config:
  ~/.config/kctl/config.yaml    marks clusters protected by API server URL
Logs:
  written to the user cache dir; the UI never writes to the terminal
`

func main() {
	code := run()
	os.Exit(code)
}

// run holds the real body so deferred cleanup executes before os.Exit.
func run() (code int) {
	var (
		flagContext    string
		flagNamespace  string
		flagKubeconfig string
		flagVersion    bool
	)

	fs := flag.NewFlagSet("kctl", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	fs.Usage = func() { fmt.Fprint(os.Stderr, usage) }
	fs.StringVar(&flagContext, "context", "", "kube context to start in")
	fs.StringVar(&flagNamespace, "namespace", "", "namespace to start in")
	fs.StringVar(&flagNamespace, "n", "", "namespace to start in (shorthand)")
	fs.StringVar(&flagKubeconfig, "kubeconfig", "", "explicit kubeconfig path")
	fs.BoolVar(&flagVersion, "version", false, "print version and exit")

	err := fs.Parse(os.Args[1:])
	if err != nil {
		// flag already reported it; -help lands here too and is not a failure.
		if errors.Is(err, flag.ErrHelp) {
			return code
		}

		return 2
	}

	if flagVersion {
		fmt.Printf("kctl %s (%s, %s/%s, %s)\n", version, commit, runtime.GOOS, runtime.GOARCH, runtime.Version())

		return code
	}

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

	store, err := kube.NewContextStore(logger, flagKubeconfig)
	if err != nil {
		fmt.Fprintf(os.Stderr, "kctl: %v\n", err)
		return 1
	}

	current := store.Current()

	if flagContext != "" {
		err = store.Use(context.Background(), flagContext)
		if err != nil {
			fmt.Fprintf(os.Stderr, "kctl: %v\n", err)

			return 1
		}

		current = store.Current()
	}

	startNamespace := current.Namespace
	if flagNamespace != "" {
		startNamespace = flagNamespace
		if startNamespace == "all" {
			startNamespace = ""
		}
	}

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
	active, err := session.Switch(ctx, current.Name, startNamespace)
	if err != nil {
		fmt.Fprintf(os.Stderr, "kctl: cannot reach cluster %s: %v\n", current.Server, err)
		return 1
	}

	model := ui.New(logger, store, active.Pods, core.Selector{Namespace: active.Namespace}).
		WithSession(session).
		WithNamespaces(active.Namespaces)

	// The apply flow is optional: kctl is fully usable read-only if the
	// applier cannot be built for this cluster.
	appCfg, err := kube.LoadConfig(kube.DefaultConfigPath())
	if err != nil {
		fmt.Fprintf(os.Stderr, "kctl: %v\n", err)
		return 1
	}

	restCfg, err := store.RESTConfig(current.Name)
	if err == nil {
		applier, aerr := kube.NewApplier(logger, restCfg)
		if aerr == nil {
			parse := func(path, ns string) ([]core.Manifest, error) {
				f, ferr := os.Open(path)
				if ferr != nil {
					return nil, fmt.Errorf("error-opening-manifest :%w", ferr)
				}
				defer func() { _ = f.Close() }()

				return kube.ParseManifests(f, ns)
			}

			protected := func(server string) bool {
				return kube.RiskFor(server, appCfg.Risk) == kube.RiskProtected
			}

			model = model.WithApplier(applier, parse, protected)
		} else {
			logger.Warn("apply-disabled", zap.Error(aerr))
		}
	}

	// Move fd 2 to the log file, but only now — every startup failure above
	// still has to reach the user's terminal. kubeconfig exec credential
	// plugins are subprocesses that write their failures to stderr, and klog
	// does the same in-process; either prints over the rendered screen.
	restoreStderr, err := logging.RedirectStderr(logPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "kctl: %v\n", err)

		return 1
	}
	defer restoreStderr()

	program := tea.NewProgram(model)

	_, err = program.Run()
	if err != nil {
		logger.Error("error-running-program", zap.Error(err))
		fmt.Fprintf(os.Stderr, "kctl: %v\n", err)
		return 1
	}

	return code
}
