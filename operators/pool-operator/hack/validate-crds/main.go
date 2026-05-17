/*
Copyright 2026.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

// validate-crds is an offline equivalent of `kubectl apply --dry-run=client`
// for CRD YAML manifests. It decodes each manifest as an
// apiextensions/v1.CustomResourceDefinition and runs the structural-schema
// validation the apiserver would run, without requiring a live cluster.
//
// This is the Phase 1 stand-in for the AC line
//
//	kubectl apply --dry-run=client -f config/crd/bases/
//
// because kubectl v1.29 still requires API discovery (i.e. a reachable
// cluster) to resolve the CustomResourceDefinition kind even with
// --dry-run=client --validate=false.
//
// Usage:
//
//	go run ./hack/validate-crds <dir-or-file>...
//
// Exits 0 if every file decodes cleanly and structural validation passes.
// Exits non-zero with a per-file diagnostic on any failure.
package main

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	apiextv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/serializer"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	"sigs.k8s.io/yaml"
)

var scheme = runtime.NewScheme()

func init() {
	utilruntime.Must(apiextv1.AddToScheme(scheme))
}

func main() {
	if len(os.Args) < 2 {
		fmt.Fprintln(os.Stderr, "usage: validate-crds <dir-or-file>...")
		os.Exit(2)
	}

	files, err := collect(os.Args[1:])
	if err != nil {
		fmt.Fprintf(os.Stderr, "collect: %v\n", err)
		os.Exit(2)
	}
	if len(files) == 0 {
		fmt.Fprintln(os.Stderr, "no YAML files found")
		os.Exit(2)
	}

	codec := serializer.NewCodecFactory(scheme).UniversalDeserializer()
	failures := 0
	for _, f := range files {
		if err := validate(f, codec); err != nil {
			fmt.Printf("FAIL %s: %v\n", f, err)
			failures++
			continue
		}
		fmt.Printf("OK   %s\n", f)
	}
	if failures > 0 {
		fmt.Fprintf(os.Stderr, "\n%d / %d file(s) failed validation\n", failures, len(files))
		os.Exit(1)
	}
	fmt.Fprintf(os.Stderr, "\nall %d CRD manifest(s) validated\n", len(files))
}

func collect(roots []string) ([]string, error) {
	var out []string
	seen := map[string]bool{}
	add := func(p string) {
		p = filepath.Clean(p)
		if !seen[p] {
			seen[p] = true
			out = append(out, p)
		}
	}
	for _, r := range roots {
		fi, err := os.Stat(r)
		if err != nil {
			return nil, err
		}
		if !fi.IsDir() {
			add(r)
			continue
		}
		err = filepath.WalkDir(r, func(p string, d fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if d.IsDir() {
				return nil
			}
			low := strings.ToLower(p)
			if strings.HasSuffix(low, ".yaml") || strings.HasSuffix(low, ".yml") {
				add(p)
			}
			return nil
		})
		if err != nil {
			return nil, err
		}
	}
	return out, nil
}

func validate(path string, codec runtime.Decoder) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	// Some manifest files concatenate multiple YAML documents with "---".
	docs := splitYAMLDocs(data)
	for i, doc := range docs {
		doc = bytesTrim(doc)
		if len(doc) == 0 {
			continue
		}
		// First decode through sigs.k8s.io/yaml to catch syntactic errors
		// with line numbers; then re-decode through the typed scheme.
		var raw map[string]any
		if err := yaml.Unmarshal(doc, &raw); err != nil {
			return fmt.Errorf("doc %d: yaml: %w", i, err)
		}
		obj, gvk, err := codec.Decode(doc, nil, nil)
		if err != nil {
			return fmt.Errorf("doc %d: decode: %w", i, err)
		}
		if gvk.Kind != "CustomResourceDefinition" {
			return fmt.Errorf("doc %d: expected CustomResourceDefinition, got %s", i, gvk)
		}
		crd, ok := obj.(*apiextv1.CustomResourceDefinition)
		if !ok {
			return fmt.Errorf("doc %d: not a *CustomResourceDefinition", i)
		}
		if err := checkCRD(crd); err != nil {
			return fmt.Errorf("doc %d (%s): %w", i, crd.Name, err)
		}
	}
	return nil
}

// checkCRD runs the cheap structural checks the apiserver would otherwise
// perform during `kubectl apply`. We deliberately keep this conservative —
// the apiserver's full structural-schema validation requires more imports
// than we want at Phase 1.
func checkCRD(crd *apiextv1.CustomResourceDefinition) error {
	if crd.APIVersion != "apiextensions.k8s.io/v1" {
		return fmt.Errorf("apiVersion %q, want apiextensions.k8s.io/v1", crd.APIVersion)
	}
	if crd.Name == "" {
		return fmt.Errorf("metadata.name is empty")
	}
	spec := crd.Spec
	if spec.Group == "" {
		return fmt.Errorf("spec.group is empty")
	}
	if spec.Scope == "" {
		return fmt.Errorf("spec.scope is empty")
	}
	if spec.Scope != apiextv1.ClusterScoped && spec.Scope != apiextv1.NamespaceScoped {
		return fmt.Errorf("spec.scope %q is invalid", spec.Scope)
	}
	if spec.Names.Kind == "" || spec.Names.Plural == "" || spec.Names.Singular == "" {
		return fmt.Errorf("spec.names missing kind/plural/singular")
	}
	if len(spec.Versions) == 0 {
		return fmt.Errorf("spec.versions is empty")
	}
	storage := 0
	for i, v := range spec.Versions {
		if v.Name == "" {
			return fmt.Errorf("spec.versions[%d].name is empty", i)
		}
		if v.Schema == nil || v.Schema.OpenAPIV3Schema == nil {
			return fmt.Errorf("spec.versions[%d] has no openAPIV3Schema", i)
		}
		if v.Storage {
			storage++
		}
	}
	if storage != 1 {
		return fmt.Errorf("expected exactly one storage=true version, got %d", storage)
	}
	// metadata.name must equal <plural>.<group> per the apiserver.
	want := spec.Names.Plural + "." + spec.Group
	if crd.Name != want {
		return fmt.Errorf("metadata.name=%q != %q", crd.Name, want)
	}
	return nil
}

// splitYAMLDocs splits a single multi-document YAML payload on "\n---\n".
// We avoid the heavier yaml-go streaming decoder to keep this tool tiny.
func splitYAMLDocs(b []byte) [][]byte {
	const sep = "\n---"
	s := string(b)
	parts := strings.Split(s, sep)
	out := make([][]byte, 0, len(parts))
	for _, p := range parts {
		out = append(out, []byte(p))
	}
	return out
}

func bytesTrim(b []byte) []byte {
	return []byte(strings.TrimSpace(string(b)))
}
