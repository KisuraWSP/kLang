package runtime

import (
	"fmt"
	"sort"
)

const (
	iteratorStageFilter = "filter"
	iteratorStageMap    = "map"
	iteratorStageSkip   = "skip"
	iteratorStageTake   = "take"
)

func (runtime *Runtime) iteratorFromValue(value Value) (Value, error) {
	if value.Kind == ValueIterator {
		return Value{Kind: ValueIterator, Data: cloneIteratorData(value.Data.(*IteratorData))}, nil
	}
	var items []Value
	switch value.Kind {
	case ValueList:
		items = value.Data.([]Value)
	case ValueSet:
		items = setValues(value.Data.(SetData))
	case ValueString:
		for _, current := range []rune(value.Data.(string)) {
			items = append(items, CharValue(string(current)))
		}
	case ValueInt:
		count := value.Data.(int)
		if count < 0 {
			return NullValue(), Error{Message: "iterator count cannot be negative"}
		}
		items = make([]Value, count)
		for index := range items {
			items[index] = IntValue(index)
		}
	case ValueTable:
		items = tableEntries(value.Data.(TableData))
	case ValueMap:
		fields := value.Data.(map[string]Value)
		keys := make([]string, 0, len(fields))
		for key := range fields {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		for _, key := range keys {
			items = append(items, TableValue(map[string]Value{
				"key":   StringValue(key),
				"value": cloneValue(fields[key]),
			}))
		}
	default:
		return NullValue(), Error{Message: fmt.Sprintf("iter expects List, Set, String, Table, Iterator, or range-compatible Int, got %s", value.Kind)}
	}
	return Value{Kind: ValueIterator, Data: &IteratorData{Items: items}}, nil
}

func cloneIteratorData(source *IteratorData) *IteratorData {
	return &IteratorData{
		Items:     source.Items,
		Index:     source.Index,
		Stages:    append([]IteratorStage(nil), source.Stages...),
		Exhausted: source.Exhausted,
	}
}

func (runtime *Runtime) appendIteratorStage(receiver Value, stage IteratorStage) (Value, error) {
	iterator, err := runtime.iteratorFromValue(receiver)
	if err != nil {
		return NullValue(), err
	}
	data := iterator.Data.(*IteratorData)
	data.Stages = append(data.Stages, stage)
	return iterator, nil
}

func (runtime *Runtime) nextIterator(iterator *IteratorData) (Value, bool, error) {
	if iterator.Exhausted {
		return NullValue(), false, nil
	}
	for _, stage := range iterator.Stages {
		if stage.Kind == iteratorStageTake && stage.Seen >= stage.Count {
			iterator.Exhausted = true
			return NullValue(), false, nil
		}
	}
	for iterator.Index < len(iterator.Items) {
		current := cloneValue(iterator.Items[iterator.Index])
		iterator.Index++
		accepted := true
		for stageIndex := range iterator.Stages {
			stage := &iterator.Stages[stageIndex]
			switch stage.Kind {
			case iteratorStageFilter:
				result, err := runtime.callFunction(stage.Function, []Value{current})
				if err != nil {
					return NullValue(), false, err
				}
				if result.Kind != ValueBool {
					return NullValue(), false, Error{Message: fmt.Sprintf("filter callback must return Bool, got %s", runtimeTypeName(result))}
				}
				if !result.Data.(bool) {
					accepted = false
				}
			case iteratorStageMap:
				result, err := runtime.callFunction(stage.Function, []Value{current})
				if err != nil {
					return NullValue(), false, err
				}
				current = result
			case iteratorStageSkip:
				if stage.Seen < stage.Count {
					stage.Seen++
					accepted = false
				}
			case iteratorStageTake:
				if stage.Seen >= stage.Count {
					iterator.Exhausted = true
					return NullValue(), false, nil
				}
				stage.Seen++
			default:
				return NullValue(), false, Error{Message: fmt.Sprintf("unknown iterator stage %q", stage.Kind)}
			}
			if !accepted {
				break
			}
		}
		if accepted {
			return cloneValue(current), true, nil
		}
	}
	iterator.Exhausted = true
	return NullValue(), false, nil
}

func (runtime *Runtime) collectIterator(iterator *IteratorData) ([]Value, error) {
	var result []Value
	for {
		value, ok, err := runtime.nextIterator(iterator)
		if err != nil {
			return nil, err
		}
		if !ok {
			return result, nil
		}
		result = append(result, value)
	}
}

func (runtime *Runtime) sortIterator(iterator *IteratorData) ([]Value, error) {
	items, err := runtime.collectIterator(iterator)
	if err != nil {
		return nil, err
	}
	for index := 1; index < len(items); index++ {
		if _, compareErr := compareOrdered(items[index], items[0], func(result int) bool { return result < 0 }); compareErr != nil {
			return nil, compareErr
		}
	}
	sort.SliceStable(items, func(left int, right int) bool {
		less, _ := compareOrdered(items[left], items[right], func(result int) bool { return result < 0 })
		return less.Data.(bool)
	})
	return items, nil
}
