package logging

import (
	"fmt"
	"os"
	"syscall"
)

// RedirectStderr points file descriptor 2 at the given file and returns a
// function that puts it back.
//
// This exists because kctl is not the only thing that writes to the terminal.
// client-go runs kubeconfig exec credential plugins as CHILD PROCESSES, and
// those inherit fd 2 — so an OIDC or EKS helper failing to authenticate prints
// its error straight over the rendered UI and leaves the screen unreadable.
// klog does the same from inside the process. Reassigning the os.Stderr
// variable is not enough: a subprocess inherits the descriptor, not the Go
// variable, so the descriptor itself has to be moved.
func RedirectStderr(path string) (restore func(), err error) {
	restore = func() {}

	file, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		err = fmt.Errorf("error-opening-stderr-sink :%w", err)

		return restore, err
	}

	saved, err := syscall.Dup(int(os.Stderr.Fd()))
	if err != nil {
		_ = file.Close()
		err = fmt.Errorf("error-saving-stderr :%w", err)

		return restore, err
	}

	err = syscall.Dup2(int(file.Fd()), int(os.Stderr.Fd()))
	if err != nil {
		_ = syscall.Close(saved)
		_ = file.Close()
		err = fmt.Errorf("error-redirecting-stderr :%w", err)

		return restore, err
	}

	restore = func() {
		_ = syscall.Dup2(saved, int(os.Stderr.Fd()))
		_ = syscall.Close(saved)
		_ = file.Close()
	}

	return restore, err
}
