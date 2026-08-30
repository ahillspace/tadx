// Package identity resolves exact Tableau resource selectors.
package identity

import (
	"fmt"
	"sort"
	"strings"
)

// LUID is an authoritative Tableau locally unique identifier.
type LUID string

// Selector identifies a resource by authoritative LUID or exact human labels.
// When LUID is set, Resolve ignores Name and ProjectPath.
type Selector struct {
	LUID        LUID
	Name        string
	ProjectPath string
}

// Candidate is the common identity projection returned by resource adapters.
type Candidate struct {
	LUID        LUID
	Name        string
	ProjectPath string
}

// ResolutionErrorKind classifies a deterministic selector failure.
type ResolutionErrorKind string

const (
	ResolutionInvalidSelector ResolutionErrorKind = "invalid_selector"
	ResolutionNotFound        ResolutionErrorKind = "not_found"
	ResolutionAmbiguous       ResolutionErrorKind = "ambiguous"
)

// ResolutionError reports a selector failure and authoritative matching IDs.
type ResolutionError struct {
	Kind       ResolutionErrorKind
	Selector   Selector
	MatchLUIDs []LUID
}

func (e *ResolutionError) Error() string {
	description := describeSelector(e.Selector)
	switch e.Kind {
	case ResolutionInvalidSelector:
		return "resource selector must include a LUID, name, or project path"
	case ResolutionNotFound:
		return "no resource matches " + description
	case ResolutionAmbiguous:
		ids := make([]string, len(e.MatchLUIDs))
		for i, id := range e.MatchLUIDs {
			ids[i] = string(id)
		}
		return fmt.Sprintf("resource selector %s is ambiguous; matches LUIDs [%s]", description, strings.Join(ids, ", "))
	default:
		return "resource selector resolution failed"
	}
}

// Resolve returns exactly one candidate by authoritative LUID or literal labels.
func Resolve(selector Selector, candidates []Candidate) (Candidate, error) {
	if selector.LUID == "" && selector.Name == "" && selector.ProjectPath == "" {
		return Candidate{}, &ResolutionError{Kind: ResolutionInvalidSelector, Selector: selector}
	}

	matchesByLUID := make(map[LUID]Candidate)
	for _, candidate := range candidates {
		if candidate.LUID == "" || !matches(selector, candidate) {
			continue
		}
		current, exists := matchesByLUID[candidate.LUID]
		if !exists || candidateLess(candidate, current) {
			matchesByLUID[candidate.LUID] = candidate
		}
	}

	ids := make([]LUID, 0, len(matchesByLUID))
	for id := range matchesByLUID {
		ids = append(ids, id)
	}
	sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
	switch len(ids) {
	case 0:
		return Candidate{}, &ResolutionError{Kind: ResolutionNotFound, Selector: selector}
	case 1:
		return matchesByLUID[ids[0]], nil
	default:
		return Candidate{}, &ResolutionError{Kind: ResolutionAmbiguous, Selector: selector, MatchLUIDs: ids}
	}
}

func matches(selector Selector, candidate Candidate) bool {
	if selector.LUID != "" {
		return selector.LUID == candidate.LUID
	}
	if selector.Name != "" && selector.Name != candidate.Name {
		return false
	}
	return selector.ProjectPath == "" || selector.ProjectPath == candidate.ProjectPath
}

func candidateLess(a, b Candidate) bool {
	if a.Name != b.Name {
		return a.Name < b.Name
	}
	return a.ProjectPath < b.ProjectPath
}

func describeSelector(selector Selector) string {
	if selector.LUID != "" {
		return fmt.Sprintf("LUID %q", selector.LUID)
	}
	if selector.Name != "" && selector.ProjectPath != "" {
		return fmt.Sprintf("name %q in project %q", selector.Name, selector.ProjectPath)
	}
	if selector.Name != "" {
		return fmt.Sprintf("name %q", selector.Name)
	}
	return fmt.Sprintf("project path %q", selector.ProjectPath)
}
