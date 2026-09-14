package logging_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mo-pankaj/kctl/internal/logging"
)

func TestRedirectStderrCapturesASubprocess(t *testing.T) {
	sink := filepath.Join(t.TempDir(), "err.log")

	restore, err := logging.RedirectStderr(sink)
	if err != nil {
		t.Fatalf("RedirectStderr returned error: %v", err)
	}

	// A CHILD PROCESS writing to stderr is the real case. client-go's exec
	// authenticator sets cmd.Stderr = os.Stderr explicitly, so the child writes
	// through whatever descriptor os.Stderr currently names — which is why the
	// descriptor has to be moved, not just the Go variable reassigned.
	// (exec.Command with a nil Stderr would send the child to /dev/null and
	// prove nothing.)
	cmd := exec.Command("sh", "-c", "echo plugin-auth-failed >&2")
	cmd.Stderr = os.Stderr
	runErr := cmd.Run()

	restore()

	if runErr != nil {
		t.Fatalf("subprocess failed: %v", runErr)
	}

	data, err := os.ReadFile(sink)
	if err != nil {
		t.Fatalf("reading sink: %v", err)
	}

	if !strings.Contains(string(data), "plugin-auth-failed") {
		t.Fatalf("subprocess stderr did not reach the sink; got %q", data)
	}
}

func TestRestorePutsStderrBack(t *testing.T) {
	sink := filepath.Join(t.TempDir(), "err.log")
	before := os.Stderr.Fd()

	restore, err := logging.RedirectStderr(sink)
	if err != nil {
		t.Fatalf("RedirectStderr returned error: %v", err)
	}

	restore()

	if os.Stderr.Fd() != before {
		t.Fatalf("stderr fd = %d after restore, want %d", os.Stderr.Fd(), before)
	}

	// And it must still be writable, so a fatal error after quit is visible.
	_, err = os.Stderr.WriteString("")
	if err != nil {
		t.Fatalf("stderr unusable after restore: %v", err)
	}
}

func TestRedirectToAnUnwritablePathIsAnError(t *testing.T) {
	_, err := logging.RedirectStderr(filepath.Join(t.TempDir(), "nope", "err.log"))
	if err == nil {
		t.Fatal("expected an error for an unwritable sink path")
	}
}
