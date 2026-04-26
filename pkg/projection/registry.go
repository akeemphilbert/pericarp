package projection

import (
	"fmt"
	"sort"
	"sync"
)

type ResourceType struct {
	Name     string
	Parent   string
	Abstract bool
	Table    string
}

type Option func(*ResourceType)

func AsAbstract() Option {
	return func(rt *ResourceType) {
		rt.Abstract = true
	}
}

func WithParent(parent string) Option {
	return func(rt *ResourceType) {
		rt.Parent = parent
	}
}

func WithTable(table string) Option {
	return func(rt *ResourceType) {
		rt.Table = table
	}
}

type Registry struct {
	mu    sync.RWMutex
	types map[string]*ResourceType
}

func NewRegistry() *Registry {
	return &Registry{
		types: make(map[string]*ResourceType),
	}
}

func (r *Registry) Register(name string, opts ...Option) error {
	rt := &ResourceType{Name: name}
	for _, o := range opts {
		o(rt)
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	if _, exists := r.types[name]; exists {
		return fmt.Errorf("%w: %s", ErrDuplicateType, name)
	}

	if rt.Abstract && rt.Parent != "" {
		return fmt.Errorf("%w: %s", ErrNestedAbstract, name)
	}

	if rt.Abstract && rt.Table == "" {
		return fmt.Errorf("%w: %s", ErrAbstractRequiresTable, name)
	}

	if rt.Parent != "" {
		parent, ok := r.types[rt.Parent]
		if !ok {
			return fmt.Errorf("%w: %s", ErrParentNotRegistered, rt.Parent)
		}
		if !parent.Abstract {
			return fmt.Errorf("%w: %s", ErrParentNotAbstract, rt.Parent)
		}
		if rt.Table != "" {
			return fmt.Errorf("%w: %s", ErrCannotOverrideTable, name)
		}
	}

	r.types[name] = rt
	return nil
}

func (r *Registry) MustRegister(name string, opts ...Option) {
	if err := r.Register(name, opts...); err != nil {
		panic(err)
	}
}

func (r *Registry) IsAbstract(name string) bool {
	r.mu.RLock()
	defer r.mu.RUnlock()
	rt, ok := r.types[name]
	return ok && rt.Abstract
}

func (r *Registry) IsRegistered(name string) bool {
	r.mu.RLock()
	defer r.mu.RUnlock()
	_, ok := r.types[name]
	return ok
}

// GetConcreteTypes returns all concrete type names for the given type.
// For an abstract type, it returns all registered concrete children.
// For a concrete type, it returns a slice containing just that type.
func (r *Registry) GetConcreteTypes(name string) ([]string, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	rt, ok := r.types[name]
	if !ok {
		return nil, fmt.Errorf("%w: %s", ErrTypeNotRegistered, name)
	}

	if !rt.Abstract {
		return []string{name}, nil
	}

	var concrete []string
	for _, t := range r.types {
		if t.Parent == name && !t.Abstract {
			concrete = append(concrete, t.Name)
		}
	}
	sort.Strings(concrete)
	return concrete, nil
}

// GetTable returns the projection table name for the given type.
// Concrete subtypes inherit their parent's table name.
func (r *Registry) GetTable(name string) (string, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	rt, ok := r.types[name]
	if !ok {
		return "", fmt.Errorf("%w: %s", ErrTypeNotRegistered, name)
	}

	if rt.Table != "" {
		return rt.Table, nil
	}

	if rt.Parent != "" {
		parent, ok := r.types[rt.Parent]
		if ok && parent.Table != "" {
			return parent.Table, nil
		}
	}

	return "", fmt.Errorf("%w: no table resolved for %s", ErrTypeNotRegistered, name)
}

func (r *Registry) GetParent(name string) (string, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	rt, ok := r.types[name]
	if !ok {
		return "", fmt.Errorf("%w: %s", ErrTypeNotRegistered, name)
	}
	return rt.Parent, nil
}

func (r *Registry) IsSubtypeOf(child, parent string) bool {
	r.mu.RLock()
	defer r.mu.RUnlock()

	rt, ok := r.types[child]
	if !ok {
		return false
	}
	return rt.Parent == parent
}
