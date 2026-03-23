package eval

// scopeMode determines how variable assignment interacts with outer scopes.
type scopeMode int

const (
	// scopeStatement: assigning to an existing outer variable modifies it.
	// New variables are block-scoped.
	scopeStatement scopeMode = iota
	// scopeExpression: assigning to an existing outer variable shadows it.
	// All variables are local to this scope.
	scopeExpression
)

// scope is a linked scope chain for variable resolution.
type scope struct {
	parent *scope
	mode   scopeMode
	vars   map[string]any
}

func newScope(parent *scope, mode scopeMode) *scope {
	return &scope{
		parent: parent,
		mode:   mode,
		vars:   make(map[string]any),
	}
}

// get looks up a variable by walking the scope chain.
func (s *scope) get(name string) (any, bool) {
	for cur := s; cur != nil; cur = cur.parent {
		if v, ok := cur.vars[name]; ok {
			return v, true
		}
	}
	return nil, false
}

// set assigns a variable, respecting the scope mode:
//   - Expression mode: always writes locally (shadow).
//   - Statement mode: if variable exists in an ancestor, update the ancestor.
//     Otherwise, create locally.
func (s *scope) set(name string, value any) {
	if s.mode == scopeStatement {
		for cur := s.parent; cur != nil; cur = cur.parent {
			if _, ok := cur.vars[name]; ok {
				cur.vars[name] = value
				return
			}
		}
	}
	s.vars[name] = value
}

// has reports whether the variable exists in this scope (not ancestors).
func (s *scope) has(name string) bool {
	_, ok := s.vars[name]
	return ok
}
