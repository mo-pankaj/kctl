package apply

import (
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// manifestExts are the extensions worth completing to. Completing to arbitrary
// files would mostly offer things that cannot be applied.
var manifestExts = map[string]bool{".yaml": true, ".yml": true, ".json": true}

// Complete extends a partially typed path.
//
// It returns the longest unambiguous completion plus the candidates, so the
// caller can fill in what is certain and show the rest. Directories complete
// with a trailing separator so the next Tab descends into them.
func Complete(input string) (completed string, candidates []string) {
	completed = input

	expanded := expandHome(input)

	dir, prefix := filepath.Split(expanded)
	if dir == "" {
		dir = "."
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		return completed, candidates
	}

	for _, e := range entries {
		name := e.Name()

		if !strings.HasPrefix(name, prefix) {
			continue
		}

		// Hidden files only when explicitly reached for.
		if strings.HasPrefix(name, ".") && !strings.HasPrefix(prefix, ".") {
			continue
		}

		if e.IsDir() {
			candidates = append(candidates, name+string(filepath.Separator))

			continue
		}

		if manifestExts[strings.ToLower(filepath.Ext(name))] {
			candidates = append(candidates, name)
		}
	}

	if len(candidates) == 0 {
		return completed, candidates
	}

	sort.Strings(candidates)

	// Keep the directory the user typed, rather than the expanded form, so a
	// leading ~ survives completion.
	typedDir, _ := filepath.Split(input)
	completed = typedDir + commonPrefix(candidates)

	return completed, candidates
}

// commonPrefix returns the longest prefix shared by every candidate.
func commonPrefix(names []string) (prefix string) {
	if len(names) == 0 {
		return prefix
	}

	prefix = names[0]

	for _, n := range names[1:] {
		for !strings.HasPrefix(n, prefix) {
			prefix = prefix[:len(prefix)-1]
			if prefix == "" {
				return prefix
			}
		}
	}

	return prefix
}
