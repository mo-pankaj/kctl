package kube_test

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
	"testing"

	"k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/envtest"
)

// envCfg is the REST config for the test API server, or nil when envtest
// binaries are unavailable.
var envCfg *rest.Config

// TestMain starts a real kube-apiserver for the apply tests.
//
// A real server is required rather than client-go's fake clientset, which does
// not implement server-side apply: field ownership, dry run and 409 conflicts —
// the entire behaviour these tests exist to pin down — simply do not happen
// against the fake. Testing the one part of kctl that can damage a cluster
// against a stub would be worse than not testing it, because it would read as
// coverage.
func TestMain(m *testing.M) {
	if os.Getenv("KUBEBUILDER_ASSETS") == "" {
		if path := discoverAssets(); path != "" {
			os.Setenv("KUBEBUILDER_ASSETS", path)
		}
	}

	if os.Getenv("KUBEBUILDER_ASSETS") == "" {
		fmt.Fprintln(os.Stderr, "envtest assets not found; apply tests will skip. "+
			"Install with: go install sigs.k8s.io/controller-runtime/tools/setup-envtest@latest && setup-envtest use 1.33.0")
		os.Exit(m.Run())
	}

	env := &envtest.Environment{}

	cfg, err := env.Start()
	if err != nil {
		fmt.Fprintf(os.Stderr, "envtest failed to start: %v\n", err)
		os.Exit(1)
	}

	envCfg = cfg
	code := m.Run()

	_ = env.Stop()
	os.Exit(code)
}

// discoverAssets asks setup-envtest where the binaries live, if it is installed.
func discoverAssets() (path string) {
	gopath, err := exec.Command("go", "env", "GOPATH").Output()
	if err != nil {
		return path
	}

	bin := strings.TrimSpace(string(gopath)) + "/bin/setup-envtest"
	if _, err := os.Stat(bin); err != nil {
		return path
	}

	out, err := exec.Command(bin, "use", "-p", "path", "-i").Output()
	if err != nil {
		return path
	}

	path = strings.TrimSpace(string(out))

	return path
}

// requireEnv skips a test when no API server is available.
func requireEnv(t *testing.T) *rest.Config {
	t.Helper()

	if envCfg == nil {
		t.Skip("envtest binaries unavailable; see TestMain")
	}

	return envCfg
}
