package runtime

import (
	"fmt"
	"sync"
)

type Binding struct {
	mu       sync.Mutex
	Mutable  bool
	Type     string
	Value    Value
	ObjectID int
	Moved    bool
}

type Environment struct {
	mu       sync.RWMutex
	parent   *Environment
	bindings map[string]*Binding
}

func NewEnvironment(parent *Environment) *Environment {
	return &Environment{
		parent:   parent,
		bindings: map[string]*Binding{},
	}
}

func (env *Environment) Define(name string, mutable bool, typeName string, value Value, objectID int) error {
	env.mu.Lock()
	defer env.mu.Unlock()
	if _, exists := env.bindings[name]; exists {
		return Error{Message: fmt.Sprintf("variable %q is already defined in this scope", name)}
	}
	env.bindings[name] = &Binding{
		Mutable:  mutable,
		Type:     typeName,
		Value:    value,
		ObjectID: objectID,
	}
	return nil
}

func (env *Environment) Get(name string) (*Binding, bool) {
	env.mu.RLock()
	if binding, ok := env.bindings[name]; ok {
		env.mu.RUnlock()
		return binding, true
	}
	env.mu.RUnlock()
	if env.parent != nil {
		return env.parent.Get(name)
	}
	return nil, false
}

func (binding *Binding) WithLock(fn func() error) error {
	binding.mu.Lock()
	defer binding.mu.Unlock()
	return fn()
}

func (binding *Binding) Snapshot() Binding {
	binding.mu.Lock()
	defer binding.mu.Unlock()
	return Binding{
		Mutable:  binding.Mutable,
		Type:     binding.Type,
		Value:    binding.Value,
		ObjectID: binding.ObjectID,
		Moved:    binding.Moved,
	}
}
