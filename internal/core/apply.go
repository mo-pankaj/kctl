package core

// Manifest is one document from a manifest file, already parsed enough to name
// what it would touch.
type Manifest struct {
	Index      int
	APIVersion string
	Kind       string
	Name       string
	Namespace  string
	Raw        []byte
}

// Describe renders the manifest as "kind/name" for display.
func (m Manifest) Describe() (s string) {
	s = m.Kind + "/" + m.Name
	return s
}

// DiffResult is the change one manifest would make.
type DiffResult struct {
	Manifest Manifest
	Creation bool
	Unified  string
	Added    int
	Removed  int

	// Conflict means another field manager owns fields this manifest sets.
	// Detected during the dry run, so the user sees it before deciding, not
	// after a half-applied set.
	Conflict bool
	Managers []string
	Fields   []string

	Err error
}

// Changed reports whether applying this manifest would alter anything.
func (d DiffResult) Changed() (changed bool) {
	changed = d.Added > 0 || d.Removed > 0
	return changed
}

// ApplyResult is the outcome of applying one manifest.
type ApplyResult struct {
	Manifest Manifest
	Applied  bool
	Conflict bool
	Managers []string
	Err      error
}
