package runtime

import "fmt"

type Binding struct {
	Mutable  bool
	Type     string
	Value    Value
	ObjectID int
	Moved    bool
}

type Environment struct {
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
	if binding, ok := env.bindings[name]; ok {
		return binding, true
	}
	if env.parent != nil {
		return env.parent.Get(name)
	}
	return nil, false
}
