package bytecode

import (
	"fmt"
	"sort"
)

func (vm *VM) applyPipeline(program Program, receiver Value, method PipelineMethod, args []Value) (Value, error) {
	if method == PipelineIter {
		if len(args) != 0 {
			return NullValue(), fmt.Errorf("iter expects no arguments")
		}
		return bytecodeIterator(receiver)
	}
	if method == PipelineFilter || method == PipelineMap {
		if len(args) != 1 || args[0].Kind != ValueFunction {
			return NullValue(), fmt.Errorf("%s expects one Function", pipelineMethodName(method))
		}
		iterator, err := bytecodeIterator(receiver)
		if err != nil {
			return NullValue(), err
		}
		iterator.Iter.Stages = append(iterator.Iter.Stages, PipelineStage{Method: method, Function: args[0].Index})
		return iterator, nil
	}
	if method == PipelineSkip || method == PipelineTake {
		if len(args) != 1 || args[0].Kind != ValueInt || args[0].Int < 0 {
			return NullValue(), fmt.Errorf("%s expects one non-negative Int", pipelineMethodName(method))
		}
		iterator, err := bytecodeIterator(receiver)
		if err != nil {
			return NullValue(), err
		}
		iterator.Iter.Stages = append(iterator.Iter.Stages, PipelineStage{Method: method, Count: args[0].Int})
		return iterator, nil
	}
	iterator, err := bytecodeIterator(receiver)
	if err != nil {
		return NullValue(), err
	}
	switch method {
	case PipelineCollect, PipelineSort:
		if len(args) != 0 {
			return NullValue(), fmt.Errorf("%s expects no arguments", pipelineMethodName(method))
		}
		items, err := vm.collectPipeline(program, iterator.Iter)
		if err != nil {
			return NullValue(), err
		}
		if method == PipelineSort {
			for index := 1; index < len(items); index++ {
				if _, compareErr := applyBinary(OpLess, items[index], items[0]); compareErr != nil {
					return NullValue(), compareErr
				}
			}
			sort.SliceStable(items, func(left int, right int) bool {
				less, _ := applyBinary(OpLess, items[left], items[right])
				return less.Bool
			})
		}
		return ListValue(items), nil
	case PipelineFold:
		if len(args) != 2 || args[1].Kind != ValueFunction {
			return NullValue(), fmt.Errorf("fold expects an initial value and Function[U,T,U]")
		}
		result := cloneValue(args[0])
		for {
			value, ok, err := vm.nextPipeline(program, iterator.Iter)
			if err != nil {
				return NullValue(), err
			}
			if !ok {
				return result, nil
			}
			result, err = vm.call(program, args[1].Index, []Value{result, value})
			if err != nil {
				return NullValue(), err
			}
		}
	case PipelineAny, PipelineAll:
		if len(args) != 1 || args[0].Kind != ValueFunction {
			return NullValue(), fmt.Errorf("%s expects Function[T,Bool]", pipelineMethodName(method))
		}
		for {
			value, ok, err := vm.nextPipeline(program, iterator.Iter)
			if err != nil {
				return NullValue(), err
			}
			if !ok {
				return BoolValue(method == PipelineAll), nil
			}
			matched, err := vm.call(program, args[0].Index, []Value{value})
			if err != nil {
				return NullValue(), err
			}
			if matched.Kind != ValueBool {
				return NullValue(), fmt.Errorf("%s callback must return Bool", pipelineMethodName(method))
			}
			if method == PipelineAny && matched.Bool {
				return BoolValue(true), nil
			}
			if method == PipelineAll && !matched.Bool {
				return BoolValue(false), nil
			}
		}
	case PipelineForEach:
		if len(args) != 1 || args[0].Kind != ValueFunction {
			return NullValue(), fmt.Errorf("for_each expects Function[T,U]")
		}
		for {
			value, ok, err := vm.nextPipeline(program, iterator.Iter)
			if err != nil {
				return NullValue(), err
			}
			if !ok {
				return NullValue(), nil
			}
			if _, err := vm.call(program, args[0].Index, []Value{value}); err != nil {
				return NullValue(), err
			}
		}
	default:
		return NullValue(), fmt.Errorf("pipeline method %d is not supported", method)
	}
}

func bytecodeIterator(value Value) (Value, error) {
	if value.Kind == ValueIterator {
		return cloneValue(value), nil
	}
	var items []Value
	switch value.Kind {
	case ValueList:
		items = value.List
	case ValueString:
		for _, current := range []rune(value.String) {
			items = append(items, StringValue(string(current)))
		}
	case ValueInt:
		if value.Int < 0 {
			return NullValue(), fmt.Errorf("iterator count cannot be negative")
		}
		items = make([]Value, int(value.Int))
		for index := range items {
			items[index] = IntValue(int64(index))
		}
	default:
		return NullValue(), fmt.Errorf("pipeline expects List, String, Iterator, or range-compatible Int")
	}
	return IteratorValue(&IteratorData{Items: items}), nil
}

func (vm *VM) nextPipeline(program Program, iterator *IteratorData) (Value, bool, error) {
	if iterator.Exhausted {
		return NullValue(), false, nil
	}
	for _, stage := range iterator.Stages {
		if stage.Method == PipelineTake && stage.Seen >= stage.Count {
			iterator.Exhausted = true
			return NullValue(), false, nil
		}
	}
	for iterator.Position < len(iterator.Items) {
		current := cloneValue(iterator.Items[iterator.Position])
		iterator.Position++
		accepted := true
		for stageIndex := range iterator.Stages {
			stage := &iterator.Stages[stageIndex]
			switch stage.Method {
			case PipelineFilter:
				result, err := vm.call(program, stage.Function, []Value{current})
				if err != nil {
					return NullValue(), false, err
				}
				if result.Kind != ValueBool {
					return NullValue(), false, fmt.Errorf("filter callback must return Bool")
				}
				accepted = result.Bool
			case PipelineMap:
				result, err := vm.call(program, stage.Function, []Value{current})
				if err != nil {
					return NullValue(), false, err
				}
				current = result
			case PipelineSkip:
				if stage.Seen < stage.Count {
					stage.Seen++
					accepted = false
				}
			case PipelineTake:
				if stage.Seen >= stage.Count {
					iterator.Exhausted = true
					return NullValue(), false, nil
				}
				stage.Seen++
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

func (vm *VM) collectPipeline(program Program, iterator *IteratorData) ([]Value, error) {
	var result []Value
	for {
		value, ok, err := vm.nextPipeline(program, iterator)
		if err != nil {
			return nil, err
		}
		if !ok {
			return result, nil
		}
		result = append(result, value)
	}
}

func pipelineMethodName(method PipelineMethod) string {
	names := map[PipelineMethod]string{
		PipelineIter: "iter", PipelineFilter: "filter", PipelineMap: "map",
		PipelineSkip: "skip", PipelineTake: "limit", PipelineCollect: "collect",
		PipelineSort: "sort", PipelineFold: "fold", PipelineAny: "any",
		PipelineAll: "all", PipelineForEach: "for_each",
	}
	return names[method]
}
