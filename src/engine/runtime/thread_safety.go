package runtime

import "fmt"

// cloneThreadTransferValue validates and snapshots a value crossing a spawn
// boundary. Aggregate values are deeply cloned, while Atomic deliberately
// preserves its synchronized identity.
func cloneThreadTransferValue(value Value) (Value, error) {
	return cloneThreadTransferValueSeen(value, map[*AtomicData]bool{})
}

func cloneThreadTransferValueSeen(value Value, seen map[*AtomicData]bool) (Value, error) {
	switch value.Kind {
	case ValueNull, ValueInt, ValueFloat, ValueString, ValueBool, ValueChar, ValueAtom, ValueEnum, ValueJSON, ValueComplex:
		return value, nil
	case ValueList:
		items := value.Data.([]Value)
		cloned := make([]Value, len(items))
		for index, item := range items {
			next, err := cloneThreadTransferValueSeen(item, seen)
			if err != nil {
				return NullValue(), err
			}
			cloned[index] = next
		}
		return Value{Kind: ValueList, Data: cloned}, nil
	case ValueSet:
		source := value.Data.(SetData)
		cloned := newSetData()
		for _, key := range source.Order {
			item, ok := source.Entries[key]
			if !ok {
				continue
			}
			next, err := cloneThreadTransferValueSeen(item, seen)
			if err != nil {
				return NullValue(), err
			}
			cloned.Order = append(cloned.Order, key)
			cloned.Entries[key] = next
		}
		return Value{Kind: ValueSet, Data: cloned}, nil
	case ValueMap:
		source := value.Data.(map[string]Value)
		cloned := make(map[string]Value, len(source))
		for key, item := range source {
			next, err := cloneThreadTransferValueSeen(item, seen)
			if err != nil {
				return NullValue(), err
			}
			cloned[key] = next
		}
		return Value{Kind: ValueMap, Data: cloned}, nil
	case ValueOption:
		source := value.Data.(OptionData)
		next, err := cloneThreadTransferValueSeen(source.Value, seen)
		if err != nil {
			return NullValue(), err
		}
		source.Value = next
		return Value{Kind: ValueOption, Data: source}, nil
	case ValueResult:
		source := value.Data.(ResultData)
		next, err := cloneThreadTransferValueSeen(source.Value, seen)
		if err != nil {
			return NullValue(), err
		}
		source.Value = next
		return Value{Kind: ValueResult, Data: source}, nil
	case ValueSIMD:
		source := value.Data.(SIMDData)
		lanes := make([]Value, len(source.Lanes))
		for index, lane := range source.Lanes {
			next, err := cloneThreadTransferValueSeen(lane, seen)
			if err != nil {
				return NullValue(), err
			}
			lanes[index] = next
		}
		return Value{Kind: ValueSIMD, Data: SIMDData{Lanes: lanes}}, nil
	case ValueAtomic:
		atomic := value.Data.(*AtomicData)
		if seen[atomic] {
			return NullValue(), Error{Message: "cyclic Atomic values cannot cross a thread boundary"}
		}
		seen[atomic] = true
		atomic.Mutex.Lock()
		_, err := cloneThreadTransferValueSeen(atomic.Value, seen)
		atomic.Mutex.Unlock()
		delete(seen, atomic)
		if err != nil {
			return NullValue(), err
		}
		return value, nil
	case ValueObject:
		source := value.Data.(ObjectData)
		if !source.Struct && source.Type != "File" && source.Type != "OS" {
			return NullValue(), Error{Message: fmt.Sprintf("%s is not thread-transfer-safe", source.Type)}
		}
		fields := make(map[string]Value, len(source.Fields))
		for key, item := range source.Fields {
			next, err := cloneThreadTransferValueSeen(item, seen)
			if err != nil {
				return NullValue(), err
			}
			fields[key] = next
		}
		return Value{Kind: ValueObject, Data: ObjectData{
			Type: source.Type, Struct: source.Struct, Fields: fields, JSONTags: cloneStringMap(source.JSONTags),
		}}, nil
	default:
		return NullValue(), Error{Message: fmt.Sprintf("%s values cannot cross a thread boundary", value.Kind)}
	}
}

func (runtime *Runtime) threadSafeGlobalValue(binding *Binding) (Value, error) {
	if !runtime.worker {
		return runtime.forceBindingValue(binding)
	}
	globalBinding, ok := runtime.global.Get(binding.Name)
	if !ok || globalBinding != binding {
		return runtime.forceBindingValue(binding)
	}
	snapshot := binding.Snapshot()
	if snapshot.Mutable {
		return NullValue(), Error{Message: fmt.Sprintf(
			"thread worker cannot access mutable global %q; store shared mutable state in an immutable Atomic[T] binding",
			binding.Name,
		)}
	}
	value, err := runtime.forceBindingValue(binding)
	if err != nil {
		return NullValue(), err
	}
	cloned, err := cloneThreadTransferValue(value)
	if err != nil {
		return NullValue(), Error{Message: fmt.Sprintf("thread worker cannot access global %q: %v", binding.Name, err)}
	}
	return cloned, nil
}

func (runtime *Runtime) ensureThreadAssignmentSafe(binding *Binding) error {
	if !runtime.worker {
		return nil
	}
	globalBinding, ok := runtime.global.Get(binding.Name)
	if ok && globalBinding == binding {
		return Error{Message: fmt.Sprintf(
			"thread worker cannot mutate global %q; use atomic_store or atomic_add on an immutable Atomic[T] binding",
			binding.Name,
		)}
	}
	return nil
}
