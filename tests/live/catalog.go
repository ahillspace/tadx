// Package live defines the selectable process-level Tableau qualification suite.
package live

import (
	"crypto/sha256"
	_ "embed"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"path"
	"regexp"
	"slices"
	"strings"
)

//go:embed catalog.json
var catalogBytes []byte

//go:embed exclusions.json
var exclusionsBytes []byte

type Spec struct {
	ID           string   `json:"id"`
	Title        string   `json:"title"`
	Group        string   `json:"group"`
	Capabilities []string `json:"capabilities"`
	Requires     []string `json:"requires"`
	Commands     []string `json:"commands"`
	Assertions   []string `json:"assertions"`
	Cleanup      string   `json:"cleanup"`
	Owners       []string `json:"owners"`
	Mode         string   `json:"mode"`
	Notes        string   `json:"notes,omitempty"`
	GoName       string   `json:"go_name"`
	Optional     bool     `json:"optional,omitzero"`
	Deadline     string   `json:"deadline"`
}

func Catalog() ([]Spec, error) {
	var out []Spec
	if err := json.Unmarshal(catalogBytes, &out); err != nil {
		return nil, err
	}
	seen := map[string]bool{}
	names := map[string]bool{}
	pattern := regexp.MustCompile(`^live\.[a-z][a-z0-9-]*\.[a-z][a-z0-9-]*$`)
	for _, s := range out {
		if !pattern.MatchString(s.ID) || seen[s.ID] || names[s.GoName] || s.GoName == "" || len(s.Capabilities) == 0 || len(s.Assertions) == 0 || len(s.Commands) == 0 || len(s.Owners) == 0 {
			return nil, fmt.Errorf("invalid or duplicate catalog definition %s", s.ID)
		}
		seen[s.ID] = true
		names[s.GoName] = true
	}
	return out, nil
}
func CatalogHash() string { h := sha256.Sum256(catalogBytes); return hex.EncodeToString(h[:]) }
func Exclusions() (map[string]string, error) {
	var rows []struct{ ID, Reason string }
	if err := json.Unmarshal(exclusionsBytes, &rows); err != nil {
		return nil, err
	}
	m := map[string]string{}
	for _, v := range rows {
		if v.ID == "" || v.Reason == "" || m[v.ID] != "" {
			return nil, errors.New("invalid exclusions ledger")
		}
		m[v.ID] = v.Reason
	}
	return m, nil
}

type Selectors struct{ IDs, Capabilities, Groups, ChangedPaths []string }
type Selection struct {
	ID      string   `json:"id"`
	Reasons []string `json:"reasons"`
}
type Plan struct {
	Version     int         `json:"schema_version"`
	CatalogHash string      `json:"catalog_hash"`
	Tests       []Selection `json:"tests"`
	NotSelected []string    `json:"not_selected"`
	Limits      []string    `json:"limits,omitempty"`
}

func Select(in Selectors) (Plan, error) {
	out := Plan{Version: 1, CatalogHash: CatalogHash()}
	if len(in.IDs)+len(in.Capabilities)+len(in.Groups)+len(in.ChangedPaths) == 0 {
		return out, errors.New("an explicit test, capability, group, selection file, or changed-path selector is required")
	}
	catalog, err := Catalog()
	if err != nil {
		return out, err
	}
	selected := map[string][]string{}
	choose := func(label, value string, match func(Spec) bool) error {
		found := false
		for _, spec := range catalog {
			if match(spec) {
				selected[spec.ID] = append(selected[spec.ID], label+": "+value)
				found = true
			}
		}
		if !found {
			return fmt.Errorf("unknown or unmapped %s %q", label, value)
		}
		return nil
	}
	for _, id := range in.IDs {
		if err = choose("id", id, func(s Spec) bool { return s.ID == id }); err != nil {
			return out, err
		}
	}
	for _, id := range in.Capabilities {
		if err = choose("capability", id, func(s Spec) bool { return slices.Contains(s.Capabilities, id) }); err != nil {
			return out, err
		}
	}
	for _, group := range in.Groups {
		if err = choose("group", group, func(s Spec) bool { return s.Group == group }); err != nil {
			return out, err
		}
	}
	for _, raw := range in.ChangedPaths {
		p := path.Clean(strings.ReplaceAll(raw, "\\", "/"))
		if p == ".." || strings.HasPrefix(p, "../") || strings.HasPrefix(p, "/") {
			return out, errors.New("changed paths must be repository-relative")
		}
		if strings.HasSuffix(p, ".md") || strings.HasPrefix(p, "docs/") || strings.HasPrefix(p, "site/") {
			continue
		}
		if strings.HasSuffix(p, "_test.go") && !strings.HasPrefix(p, "tests/live/") {
			continue
		}
		shared := strings.HasPrefix(p, "internal/app/") || strings.HasPrefix(p, "internal/auth/") || strings.HasPrefix(p, "internal/config/") || p == "internal/tableau/transport.go" || strings.HasPrefix(p, "internal/output/") || strings.HasPrefix(p, "internal/toon/") || strings.HasPrefix(p, "cmd/tadx/") || strings.HasPrefix(p, "internal/cli/root") || strings.HasPrefix(p, "internal/managedpolicy/") || strings.HasPrefix(p, "internal/cli/batch")
		if strings.HasPrefix(p, "tests/live/") {
			shared = true
		}
		if err = choose("changed path", p, func(s Spec) bool {
			if shared {
				return true
			}
			for _, owner := range s.Owners {
				if strings.HasPrefix(p, strings.TrimSuffix(owner, "/")+"/") || p == strings.TrimSuffix(owner, "/") {
					return true
				}
			}
			return false
		}); err != nil {
			return out, err
		}
	}
	for _, spec := range catalog {
		if reasons, ok := selected[spec.ID]; ok {
			out.Tests = append(out.Tests, Selection{spec.ID, reasons})
		} else {
			out.NotSelected = append(out.NotSelected, spec.ID)
		}
	}
	if len(in.ChangedPaths) > 0 {
		out.Limits = append(out.Limits, "Selection follows declared ownership and shared consumers; unselected contracts have no qualification claim.")
	}
	return out, nil
}
func (p Plan) Validate() error {
	if p.Version != 1 || p.CatalogHash != CatalogHash() {
		return errors.New("selection plan schema or catalog fingerprint is stale")
	}
	catalog, err := Catalog()
	if err != nil {
		return err
	}
	known := map[string]bool{}
	for _, spec := range catalog {
		known[spec.ID] = true
	}
	seen := map[string]bool{}
	for _, v := range p.Tests {
		if !known[v.ID] || seen[v.ID] || len(v.Reasons) == 0 {
			return errors.New("selection contains unknown, duplicate, or unexplained test IDs")
		}
		seen[v.ID] = true
	}
	return nil
}
