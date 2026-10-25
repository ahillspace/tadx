// Package fixtures contains the public native fixture generator used by setup.
package fixtures

import _ "embed"

// Generator builds deterministic native packages without site data or credentials.
//
//go:embed generate.py
var Generator string
