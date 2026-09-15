package kube

import (
	"bytes"
	"errors"
	"fmt"
	"io"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/util/yaml"

	"github.com/mo-pankaj/kctl/internal/core"
)

// maxManifestBytes bounds a single document so a stray binary file cannot be
// decoded into memory unchecked.
const maxManifestBytes = 4 << 20

// ParseManifests splits a multi-document YAML stream and validates each document
// enough to name what it would touch.
//
// defaultNamespace is applied to any document that does not set one. Namespace
// resolution is explicit rather than left to the server because an implicit
// namespace is how a manifest lands in the wrong place, and the confirm screen
// has to be able to show where each document is going.
func ParseManifests(r io.Reader, defaultNamespace string) (docs []core.Manifest, err error) {
	decoder := yaml.NewYAMLOrJSONDecoder(r, 4096)

	for index := 0; ; index++ {
		var raw map[string]interface{}

		err = decoder.Decode(&raw)
		// errors.Is, not ==: a wrapped EOF would otherwise fall through to the
		// error branch and report a parse failure at the end of every file.
		if errors.Is(err, io.EOF) {
			err = nil

			return docs, err
		}

		if err != nil {
			err = fmt.Errorf("error-decoding-document :doc %d: %w", index, err)

			return docs, err
		}

		// A document that is only comments or blank decodes to nothing.
		if len(raw) == 0 {
			continue
		}

		obj := &unstructured.Unstructured{Object: raw}

		doc, err := toManifest(obj, index, defaultNamespace)
		if err != nil {
			return docs, err
		}

		docs = append(docs, doc)
	}
}

func toManifest(obj *unstructured.Unstructured, index int, defaultNamespace string) (doc core.Manifest, err error) {
	if obj.GetAPIVersion() == "" {
		err = fmt.Errorf("error-invalid-document :doc %d: missing apiVersion", index)

		return doc, err
	}

	if obj.GetKind() == "" {
		err = fmt.Errorf("error-invalid-document :doc %d: missing kind", index)

		return doc, err
	}

	if obj.GetName() == "" {
		err = fmt.Errorf("error-invalid-document :doc %d: missing metadata.name", index)

		return doc, err
	}

	namespace := obj.GetNamespace()
	if namespace == "" {
		namespace = defaultNamespace
	}

	if namespace == "" {
		namespace = "default"
	}

	obj.SetNamespace(namespace)

	encoded, err := obj.MarshalJSON()
	if err != nil {
		err = fmt.Errorf("error-encoding-document :doc %d: %w", index, err)

		return doc, err
	}

	if len(encoded) > maxManifestBytes {
		err = fmt.Errorf("error-document-too-large :doc %d: %d bytes", index, len(encoded))

		return doc, err
	}

	doc = core.Manifest{
		Index:      index,
		APIVersion: obj.GetAPIVersion(),
		Kind:       obj.GetKind(),
		Name:       obj.GetName(),
		Namespace:  namespace,
		Raw:        encoded,
	}

	return doc, err
}

// ParseManifestBytes is the byte-slice convenience form.
func ParseManifestBytes(b []byte, defaultNamespace string) (docs []core.Manifest, err error) {
	docs, err = ParseManifests(bytes.NewReader(b), defaultNamespace)

	return docs, err
}
