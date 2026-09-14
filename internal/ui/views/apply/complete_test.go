package apply_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mo-pankaj/kctl/internal/ui/views/apply"
)

func fixtureDir(t *testing.T) string {
	t.Helper()

	dir := t.TempDir()
	for _, name := range []string{"deploy-create.yaml", "deploy-update.yaml", "pod.yml", "notes.txt", "values.json"} {
		os.WriteFile(filepath.Join(dir, name), []byte("x"), 0o600)
	}

	os.Mkdir(filepath.Join(dir, "nested"), 0o755)
	os.WriteFile(filepath.Join(dir, ".hidden.yaml"), []byte("x"), 0o600)

	return dir
}

func TestCompleteOffersOnlyApplyableFiles(t *testing.T) {
	dir := fixtureDir(t)

	_, got := apply.Complete(dir + "/")

	joined := strings.Join(got, " ")

	for _, want := range []string{"deploy-create.yaml", "deploy-update.yaml", "pod.yml", "values.json", "nested/"} {
		if !strings.Contains(joined, want) {
			t.Fatalf("candidates %v missing %q", got, want)
		}
	}

	// notes.txt cannot be applied, so offering it would only waste a keystroke.
	if strings.Contains(joined, "notes.txt") {
		t.Fatalf("candidates %v should not include a non-manifest file", got)
	}
}

func TestCompleteFillsTheUnambiguousPart(t *testing.T) {
	dir := fixtureDir(t)

	// Two files share "deploy-", so that much is certain and no more.
	completed, candidates := apply.Complete(filepath.Join(dir, "dep"))

	if len(candidates) != 2 {
		t.Fatalf("candidates = %v, want the two deploy files", candidates)
	}

	if !strings.HasSuffix(completed, "deploy-") {
		t.Fatalf("completed = %q, want it to end at the common prefix %q", completed, "deploy-")
	}
}

func TestCompleteFinishesAUniqueMatch(t *testing.T) {
	dir := fixtureDir(t)

	completed, candidates := apply.Complete(filepath.Join(dir, "po"))

	if len(candidates) != 1 {
		t.Fatalf("candidates = %v, want exactly one", candidates)
	}

	if !strings.HasSuffix(completed, "pod.yml") {
		t.Fatalf("completed = %q, want the full filename", completed)
	}
}

func TestCompleteMarksDirectoriesSoTabCanDescend(t *testing.T) {
	dir := fixtureDir(t)

	completed, _ := apply.Complete(filepath.Join(dir, "nes"))

	if !strings.HasSuffix(completed, "nested"+string(filepath.Separator)) {
		t.Fatalf("completed = %q, want a trailing separator so the next tab descends", completed)
	}
}

func TestCompleteHidesDotfilesUntilAsked(t *testing.T) {
	dir := fixtureDir(t)

	_, plain := apply.Complete(dir + "/")
	if strings.Contains(strings.Join(plain, " "), ".hidden") {
		t.Fatalf("dotfiles should not be offered unprompted: %v", plain)
	}

	// Concatenated, not filepath.Join: Join cleans "dir/." back to "dir",
	// which would complete against the parent directory instead.
	_, dotted := apply.Complete(dir + string(filepath.Separator) + ".")
	if !strings.Contains(strings.Join(dotted, " "), ".hidden.yaml") {
		t.Fatalf("a leading dot should reveal dotfiles, got %v", dotted)
	}
}

func TestCompleteOnAMissingDirectoryIsInert(t *testing.T) {
	in := "/definitely/not/here/x"

	completed, candidates := apply.Complete(in)

	if completed != in || candidates != nil {
		t.Fatalf("Complete(%q) = %q, %v; want the input unchanged and no candidates", in, completed, candidates)
	}
}
