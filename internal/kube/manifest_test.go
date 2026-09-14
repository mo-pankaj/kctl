package kube_test

import (
	"strings"
	"testing"

	"github.com/mo-pankaj/kctl/internal/kube"
)

const twoDocs = `
apiVersion: v1
kind: ConfigMap
metadata:
  name: first
---
apiVersion: apps/v1
kind: Deployment
metadata:
  name: second
  namespace: explicit-ns
`

func TestParseSplitsDocumentsAndResolvesNamespace(t *testing.T) {
	docs, err := kube.ParseManifestBytes([]byte(twoDocs), "from-context")
	if err != nil {
		t.Fatalf("ParseManifestBytes returned error: %v", err)
	}

	if len(docs) != 2 {
		t.Fatalf("parsed %d documents, want 2", len(docs))
	}

	if docs[0].Kind != "ConfigMap" || docs[0].Name != "first" {
		t.Fatalf("doc 0 = %+v", docs[0])
	}

	// No namespace in the document: the current one is used.
	if docs[0].Namespace != "from-context" {
		t.Fatalf("doc 0 namespace = %q, want the context's namespace", docs[0].Namespace)
	}

	// An explicit namespace always wins — this is what stops a manifest landing
	// somewhere the user did not intend.
	if docs[1].Namespace != "explicit-ns" {
		t.Fatalf("doc 1 namespace = %q, want %q", docs[1].Namespace, "explicit-ns")
	}
}

func TestParseFallsBackToDefaultNamespace(t *testing.T) {
	docs, err := kube.ParseManifestBytes([]byte("apiVersion: v1\nkind: ConfigMap\nmetadata:\n  name: x\n"), "")
	if err != nil {
		t.Fatalf("error: %v", err)
	}

	if docs[0].Namespace != "default" {
		t.Fatalf("namespace = %q, want %q", docs[0].Namespace, "default")
	}
}

func TestParseRejectsIncompleteDocumentsByIndex(t *testing.T) {
	tests := []struct {
		name string
		yaml string
		want string
	}{
		{
			name: "missing name",
			yaml: "apiVersion: v1\nkind: ConfigMap\nmetadata: {}\n",
			want: "missing metadata.name",
		},
		{
			name: "missing kind",
			yaml: "apiVersion: v1\nmetadata:\n  name: x\n",
			want: "missing kind",
		},
		{
			name: "missing apiVersion",
			yaml: "kind: ConfigMap\nmetadata:\n  name: x\n",
			want: "missing apiVersion",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := kube.ParseManifestBytes([]byte(tt.yaml), "ns")
			if err == nil {
				t.Fatal("expected an error, got nil")
			}

			if !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("error = %q, want it to mention %q", err, tt.want)
			}

			// The index matters: "doc 2 is broken" is actionable, "invalid yaml" is not.
			if !strings.Contains(err.Error(), "doc 0") {
				t.Fatalf("error = %q, want it to name the document index", err)
			}
		})
	}
}

func TestParseSkipsEmptyDocuments(t *testing.T) {
	y := "---\n# just a comment\n---\napiVersion: v1\nkind: ConfigMap\nmetadata:\n  name: only\n---\n"

	docs, err := kube.ParseManifestBytes([]byte(y), "ns")
	if err != nil {
		t.Fatalf("error: %v", err)
	}

	if len(docs) != 1 || docs[0].Name != "only" {
		t.Fatalf("parsed %d docs, want just the one real document: %+v", len(docs), docs)
	}
}

func TestDescribeRendersKindAndName(t *testing.T) {
	docs, _ := kube.ParseManifestBytes([]byte("apiVersion: v1\nkind: ConfigMap\nmetadata:\n  name: cfg\n"), "ns")

	if got := docs[0].Describe(); got != "ConfigMap/cfg" {
		t.Fatalf("Describe() = %q, want %q", got, "ConfigMap/cfg")
	}
}
