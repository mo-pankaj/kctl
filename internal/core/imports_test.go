package core_test

import (
	"go/build"
	"strings"
	"testing"
)

func TestCoreHasNoForbiddenImports(t *testing.T) {
	forbidden := []string{
		"k8s.io/",
		"charm.land/",
		"github.com/charmbracelet/",
		"go.uber.org/zap",
	}

	pkg, err := build.Import("github.com/mo-pankaj/kctl/internal/core", "", 0)
	if err != nil {
		t.Fatalf("error importing core package: %v", err)
	}

	for _, imported := range pkg.Imports {
		for _, bad := range forbidden {
			if strings.HasPrefix(imported, bad) {
				t.Errorf("internal/core must not import %q (matched forbidden prefix %q)", imported, bad)
			}
		}
	}
}
