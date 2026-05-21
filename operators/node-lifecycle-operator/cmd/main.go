/*
Copyright 2026.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0
*/

// Command node-lifecycle-operator is the scaffold-only Phase 9 binary
// per ADR-0003 v2 + CLAUDE.md §14.2 scaffold pattern.
//
// DESIGN.md deferred to controller-body task per CLAUDE.md §14.2
// scaffold pattern. Phase 10 lands the full controller + reconciler +
// helm chart + DESIGN.md when the body work begins.
//
// Phase 9 P9-T-105 ships api/v1alpha1 types + scheme registration + 3
// round-trip tests + sample YAML + this minimal binary that prints a
// banner and exits cleanly (does NOT register a reconciler).
package main

import (
	"fmt"
	"os"
)

func main() {
	fmt.Println("node-lifecycle-operator · Phase 9 P9-T-105 scaffold · controller body deferred to Phase 10 per ADR-0003 v2 + CLAUDE.md §14.2")
	os.Exit(0)
}
