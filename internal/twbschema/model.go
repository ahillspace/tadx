package twbschema

import (
	"slices"
	"strconv"
	"strings"
)

// contentModel matches a sequence of child element names. It is a Thompson
// NFA over the type's particles, so it needs no determinism from the schema
// and bounded repeats stay exact.
type contentModel struct {
	root   *particle
	states []nfaState
	start  int
	final  int
}

type nfaState struct {
	eps  []int
	edge *edge
}

// edge consumes one child element. A nil decl with wildcard set matches any
// element and skips its content, as xs:any processContents="skip" does.
type edge struct {
	name     string
	decl     *elementDecl
	wildcard bool
	next     int
}

func newContentModel(root *particle) *contentModel {
	m := &contentModel{root: root}
	m.start = m.add()
	m.final = m.build(root, m.start)
	return m
}

func (m *contentModel) add() int {
	m.states = append(m.states, nfaState{})
	return len(m.states) - 1
}

func (m *contentModel) epsilon(from, to int) { m.states[from].eps = append(m.states[from].eps, to) }

func (m *contentModel) build(p *particle, from int) int {
	return m.repeat(p.min, p.max, from, func(s int) int { return m.once(p, s) })
}

func (m *contentModel) once(p *particle, from int) int {
	switch p.kind {
	case "element", "any":
		to := m.add()
		m.states[from].edge = &edge{name: p.declName(), decl: p.decl, wildcard: p.kind == "any", next: to}
		return to
	case "sequence":
		s := from
		for _, kid := range p.kids {
			s = m.build(kid, s)
		}
		return s
	case "choice":
		join := m.add()
		for _, kid := range p.kids {
			branch := m.add()
			m.epsilon(from, branch)
			m.epsilon(m.build(kid, branch), join)
		}
		return join
	case "all":
		// Each member appears at most once, in any order.
		join := m.add()
		for _, order := range permutations(len(p.kids)) {
			s := m.add()
			m.epsilon(from, s)
			for _, i := range order {
				s = m.build(p.kids[i], s)
			}
			m.epsilon(s, join)
		}
		return join
	}
	panic("twbschema: unknown particle " + p.kind)
}

func (m *contentModel) repeat(min, max, from int, body func(int) int) int {
	s := from
	for i := 0; i < min; i++ {
		s = body(m.fresh(s))
	}
	if max == unbounded {
		loop := m.add()
		m.epsilon(s, loop)
		m.epsilon(body(m.fresh(loop)), loop)
		return loop
	}
	end := m.add()
	m.epsilon(s, end)
	for i := min; i < max; i++ {
		s = body(m.fresh(s))
		m.epsilon(s, end)
	}
	return end
}

// fresh returns a state with no edge reached from s, so a body can attach its
// own edge.
func (m *contentModel) fresh(s int) int {
	next := m.add()
	m.epsilon(s, next)
	return next
}

func permutations(n int) [][]int {
	if n == 0 {
		return [][]int{{}}
	}
	var out [][]int
	for _, rest := range permutations(n - 1) {
		for i := 0; i <= len(rest); i++ {
			order := append(append(append([]int{}, rest[:i]...), n-1), rest[i:]...)
			out = append(out, order)
		}
	}
	return out
}

// stateSet is the set of NFA states reachable after the children seen so far.
type stateSet []int

func (m *contentModel) begin() stateSet { return m.closure([]int{m.start}) }

func (m *contentModel) closure(seed []int) stateSet {
	seen := make(map[int]bool, len(seed))
	var out stateSet
	var visit func(int)
	visit = func(s int) {
		if seen[s] {
			return
		}
		seen[s] = true
		out = append(out, s)
		for _, next := range m.states[s].eps {
			visit(next)
		}
	}
	for _, s := range seed {
		visit(s)
	}
	return out
}

// step consumes one child. It returns the next set and the matched edge, or
// a nil edge when the child is not allowed here.
func (m *contentModel) step(set stateSet, name string) (stateSet, *edge) {
	var targets []int
	var matched *edge
	for _, s := range set {
		e := m.states[s].edge
		if e == nil || (!e.wildcard && e.name != name) {
			continue
		}
		if matched == nil || matched.wildcard && !e.wildcard {
			matched = e
		}
		targets = append(targets, e.next)
	}
	if matched == nil {
		return set, nil
	}
	return m.closure(targets), matched
}

func (m *contentModel) accepts(set stateSet) bool {
	for _, s := range set {
		if s == m.final {
			return true
		}
	}
	return false
}

// expected lists the elements allowed next, in schema order.
func (m *contentModel) expected(set stateSet) []string {
	var names []string
	seen := map[string]bool{}
	// States are created in schema order.
	ordered := slices.Clone(set)
	slices.Sort(ordered)
	for _, s := range ordered {
		e := m.states[s].edge
		if e == nil {
			continue
		}
		name := e.name
		if e.wildcard {
			name = "*"
		}
		if !seen[name] {
			seen[name] = true
			names = append(names, name)
		}
	}
	return names
}

// declFor finds a declaration for a child the model rejected, so its content
// is still checked.
func (m *contentModel) declFor(name string) *elementDecl {
	for _, s := range m.states {
		if s.edge != nil && !s.edge.wildcard && s.edge.name == name {
			return s.edge.decl
		}
	}
	return nil
}

func (p *particle) declName() string {
	if p.decl == nil {
		return ""
	}
	return p.decl.name
}

// order renders the model the way Tableau's load error does, with groups
// flattened: (view, style, panes, mark-layout?, rows, cols, ...).
func (m *contentModel) order() string {
	root := simplify(m.root)
	if root.kind != "sequence" {
		return "(" + render(root) + ")"
	}
	return "(" + strings.Join(renderKids(root), ", ") + ")"
}

// simplify folds single-member groups into their member, the way a group
// reference reads in Tableau's error, when the folded occurrence is exact.
func simplify(p *particle) *particle {
	if p.kind == "element" || p.kind == "any" {
		return p
	}
	kids := make([]*particle, 0, len(p.kids))
	for _, kid := range p.kids {
		kid = simplify(kid)
		if kid.kind == "sequence" && p.kind == "sequence" && kid.min == 1 && kid.max == 1 {
			kids = append(kids, kid.kids...)
			continue
		}
		kids = append(kids, kid)
	}
	folded := &particle{kind: p.kind, min: p.min, max: p.max, kids: kids}
	if len(kids) != 1 || !foldable(folded, kids[0]) {
		return folded
	}
	only := *kids[0]
	only.min = p.min * only.min
	if p.max == unbounded || only.max == unbounded {
		only.max = unbounded
	} else {
		only.max = p.max * only.max
	}
	return &only
}

// foldable reports whether outer{a,b} of inner{c,d} is exactly {a*c,b*d}.
func foldable(outer, inner *particle) bool {
	once := func(p *particle) bool { return p.min == 1 && p.max == 1 }
	simple := func(p *particle) bool { return p.min <= 1 && (p.max == 1 || p.max == unbounded) }
	return once(outer) || once(inner) || simple(outer) && simple(inner)
}

func renderKids(p *particle) []string {
	out := make([]string, 0, len(p.kids))
	for _, kid := range p.kids {
		out = append(out, render(kid))
	}
	return out
}

func render(p *particle) string {
	switch p.kind {
	case "element":
		return p.decl.name + suffix(p.min, p.max)
	case "any":
		return "any" + suffix(p.min, p.max)
	}
	sep := ", "
	switch p.kind {
	case "choice":
		sep = " | "
	case "all":
		sep = " & "
	}
	return "(" + strings.Join(renderKids(p), sep) + ")" + suffix(p.min, p.max)
}

func suffix(min, max int) string {
	switch {
	case min == 1 && max == 1:
		return ""
	case min == 0 && max == 1:
		return "?"
	case min == 0 && max == unbounded:
		return "*"
	case min == 1 && max == unbounded:
		return "+"
	case max == unbounded:
		return "{" + strconv.Itoa(min) + ",}"
	}
	return "{" + strconv.Itoa(min) + "," + strconv.Itoa(max) + "}"
}

// required lists the elements that can start a shortest completion of the
// content, in schema order: what is missing, without the optional elements
// that could also come next.
func (m *contentModel) required(set stateSet) []string {
	dist := m.distanceToFinal()
	best := -1
	for _, s := range set {
		if e := m.states[s].edge; e != nil && dist[e.next] >= 0 && (best < 0 || dist[e.next] < best) {
			best = dist[e.next]
		}
	}
	var names []string
	ordered := slices.Clone(set)
	slices.Sort(ordered)
	for _, s := range ordered {
		e := m.states[s].edge
		if e == nil || dist[e.next] != best {
			continue
		}
		name := e.name
		if e.wildcard {
			name = "*"
		}
		if !slices.Contains(names, name) {
			names = append(names, name)
		}
	}
	return names
}

// distanceToFinal counts the fewest elements from each state to acceptance,
// or -1 when acceptance is unreachable. Epsilon moves are free.
func (m *contentModel) distanceToFinal() []int {
	reverse := make([][]struct{ from, cost int }, len(m.states))
	for s, state := range m.states {
		for _, next := range state.eps {
			reverse[next] = append(reverse[next], struct{ from, cost int }{s, 0})
		}
		if state.edge != nil {
			reverse[state.edge.next] = append(reverse[state.edge.next], struct{ from, cost int }{s, 1})
		}
	}
	dist := make([]int, len(m.states))
	for i := range dist {
		dist[i] = -1
	}
	dist[m.final] = 0
	queue := []int{m.final}
	for len(queue) > 0 {
		s := queue[0]
		queue = queue[1:]
		for _, in := range reverse[s] {
			d := dist[s] + in.cost
			if dist[in.from] >= 0 && dist[in.from] <= d {
				continue
			}
			dist[in.from] = d
			if in.cost == 0 {
				queue = append([]int{in.from}, queue...)
			} else {
				queue = append(queue, in.from)
			}
		}
	}
	return dist
}
