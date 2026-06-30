package runtime

import (
	"encoding/json"
	"fmt"
	"io"
	"sort"
	"strconv"
	"strings"
	"unicode/utf8"
)

type JSONData struct {
	Value any
}

func JSONValue(value any) Value {
	return Value{Kind: ValueJSON, Data: JSONData{Value: value}}
}

func parseJSONValue(source string) (Value, error) {
	decoder := json.NewDecoder(strings.NewReader(source))
	decoder.UseNumber()

	var decoded any
	if err := decoder.Decode(&decoded); err != nil {
		return NullValue(), jsonSourceError(source, err)
	}

	var trailing any
	if err := decoder.Decode(&trailing); err != io.EOF {
		if err == nil {
			return NullValue(), Error{Message: "invalid JSON: expected one top-level value"}
		}
		return NullValue(), jsonSourceError(source, err)
	}
	return JSONValue(decoded), nil
}

func jsonSourceError(source string, err error) error {
	offset := int64(1)
	if syntaxErr, ok := err.(*json.SyntaxError); ok {
		offset = syntaxErr.Offset
	}
	line, column := jsonLineColumn(source, offset)
	return Error{Message: fmt.Sprintf("invalid JSON at %d:%d: %s", line, column, err)}
}

func jsonLineColumn(source string, offset int64) (int, int) {
	line := 1
	column := 1
	for index, current := range []byte(source) {
		if int64(index+1) >= offset {
			break
		}
		if current == '\n' {
			line++
			column = 1
		} else {
			column++
		}
	}
	return line, column
}

func stringifyJSONValue(value Value) (string, error) {
	if value.Kind != ValueJSON {
		return "", Error{Message: fmt.Sprintf("json_stringify expects JSON, got %s", value.Kind)}
	}
	encoded, err := json.Marshal(value.Data.(JSONData).Value)
	if err != nil {
		return "", Error{Message: fmt.Sprintf("cannot serialize JSON: %s", err)}
	}
	return string(encoded), nil
}

func runtimeValueToJSON(value Value) (any, error) {
	switch value.Kind {
	case ValueNull:
		return nil, nil
	case ValueJSON:
		return value.Data.(JSONData).Value, nil
	case ValueString, ValueChar:
		return value.Data.(string), nil
	case ValueInt, ValueFloat, ValueBool:
		return value.Data, nil
	case ValueList:
		items := value.Data.([]Value)
		converted := make([]any, 0, len(items))
		for _, item := range items {
			jsonItem, err := runtimeValueToJSON(item)
			if err != nil {
				return nil, err
			}
			converted = append(converted, jsonItem)
		}
		return converted, nil
	case ValueMap:
		converted := map[string]any{}
		for key, item := range value.Data.(map[string]Value) {
			jsonItem, err := runtimeValueToJSON(item)
			if err != nil {
				return nil, err
			}
			converted[key] = jsonItem
		}
		return converted, nil
	case ValueTable:
		converted := map[string]any{}
		table := value.Data.(TableData)
		for _, key := range table.Order {
			if key.Kind != ValueString {
				return nil, Error{Message: "JSON serialization requires String table keys"}
			}
			jsonItem, err := runtimeValueToJSON(table.Entries[key])
			if err != nil {
				return nil, err
			}
			converted[key.Repr] = jsonItem
		}
		return converted, nil
	case ValueOption:
		option := value.Data.(OptionData)
		if !option.Some {
			return nil, nil
		}
		return runtimeValueToJSON(option.Value)
	case ValueEnum:
		return value.Data.(EnumData).Variant, nil
	case ValueObject:
		object := value.Data.(ObjectData)
		if !object.Struct {
			return nil, Error{Message: fmt.Sprintf("JSON serialization expects a struct alias, got %s", object.Type)}
		}
		converted := map[string]any{}
		for field, item := range object.Fields {
			if strings.HasPrefix(field, "__") {
				continue
			}
			name := field
			if tagged, ok := object.JSONTags[field]; ok {
				name = tagged
			}
			jsonItem, err := runtimeValueToJSON(item)
			if err != nil {
				return nil, Error{Message: fmt.Sprintf("cannot serialize %s.%s: %s", object.Type, field, err)}
			}
			converted[name] = jsonItem
		}
		return converted, nil
	default:
		return nil, Error{Message: fmt.Sprintf("cannot serialize %s as JSON", runtimeTypeName(value))}
	}
}

func runtimeValueJSONString(value Value) (string, error) {
	converted, err := runtimeValueToJSON(value)
	if err != nil {
		return "", err
	}
	encoded, err := json.Marshal(converted)
	if err != nil {
		return "", Error{Message: fmt.Sprintf("cannot serialize %s as JSON: %s", runtimeTypeName(value), err)}
	}
	return string(encoded), nil
}

func jsonDataToRuntime(value any) (Value, error) {
	switch current := value.(type) {
	case nil:
		return NullValue(), nil
	case bool:
		return BoolValue(current), nil
	case string:
		return StringValue(current), nil
	case json.Number:
		if integer, err := strconv.ParseInt(string(current), 10, 0); err == nil {
			return IntValue(int(integer)), nil
		}
		number, err := strconv.ParseFloat(string(current), 64)
		if err != nil {
			return NullValue(), Error{Message: fmt.Sprintf("invalid JSON number %q", current)}
		}
		return FloatValue(number), nil
	case []any:
		items := make([]Value, 0, len(current))
		for _, item := range current {
			decoded, err := jsonDataToRuntime(item)
			if err != nil {
				return NullValue(), err
			}
			items = append(items, decoded)
		}
		return Value{Kind: ValueList, Data: items}, nil
	case map[string]any:
		keys := make([]string, 0, len(current))
		for key := range current {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		entries := make([]TableEntryData, 0, len(keys))
		for _, key := range keys {
			decoded, err := jsonDataToRuntime(current[key])
			if err != nil {
				return NullValue(), Error{Message: fmt.Sprintf("cannot decode JSON field %q: %s", key, err)}
			}
			entries = append(entries, TableEntryData{Key: StringValue(key), Value: decoded})
		}
		return TableValueFromEntries(entries), nil
	default:
		return NullValue(), Error{Message: fmt.Sprintf("unsupported decoded JSON value %T", value)}
	}
}

func jsonValueKind(value Value) (string, error) {
	if value.Kind != ValueJSON {
		return "", Error{Message: fmt.Sprintf("expected JSON, got %s", value.Kind)}
	}
	switch value.Data.(JSONData).Value.(type) {
	case nil:
		return "null", nil
	case bool:
		return "bool", nil
	case string:
		return "string", nil
	case json.Number:
		return "number", nil
	case []any:
		return "array", nil
	case map[string]any:
		return "object", nil
	default:
		return "", Error{Message: "JSON contains an unsupported internal value"}
	}
}

func jsonValueLength(value Value) (int, error) {
	if value.Kind != ValueJSON {
		return 0, Error{Message: fmt.Sprintf("len expects JSON, got %s", value.Kind)}
	}
	switch current := value.Data.(JSONData).Value.(type) {
	case []any:
		return len(current), nil
	case map[string]any:
		return len(current), nil
	case string:
		return utf8.RuneCountInString(current), nil
	default:
		kind, _ := jsonValueKind(value)
		return 0, Error{Message: fmt.Sprintf("len does not support JSON %s", kind)}
	}
}

func jsonLookup(value Value, key Value) (Value, bool, error) {
	if value.Kind != ValueJSON {
		return NullValue(), false, Error{Message: fmt.Sprintf("expected JSON, got %s", value.Kind)}
	}
	switch current := value.Data.(JSONData).Value.(type) {
	case map[string]any:
		if key.Kind != ValueString {
			return NullValue(), false, Error{Message: fmt.Sprintf("JSON object index expects String, got %s", key.Kind)}
		}
		child, ok := current[key.Data.(string)]
		return JSONValue(child), ok, nil
	case []any:
		if key.Kind != ValueInt {
			return NullValue(), false, Error{Message: fmt.Sprintf("JSON array index expects Int, got %s", key.Kind)}
		}
		index := key.Data.(int)
		if index < 0 || index >= len(current) {
			return NullValue(), false, nil
		}
		return JSONValue(current[index]), true, nil
	default:
		kind, _ := jsonValueKind(value)
		return NullValue(), false, Error{Message: fmt.Sprintf("JSON %s is not indexable", kind)}
	}
}

func jsonStringValue(value Value) (Value, bool) {
	if value.Kind != ValueJSON {
		return NullValue(), false
	}
	current, ok := value.Data.(JSONData).Value.(string)
	return StringValue(current), ok
}

func jsonIntValue(value Value) (Value, bool) {
	if value.Kind != ValueJSON {
		return NullValue(), false
	}
	number, ok := value.Data.(JSONData).Value.(json.Number)
	if !ok {
		return NullValue(), false
	}
	parsed, err := strconv.ParseInt(string(number), 10, 0)
	if err != nil {
		return NullValue(), false
	}
	return IntValue(int(parsed)), true
}

func jsonFloatValue(value Value) (Value, bool) {
	if value.Kind != ValueJSON {
		return NullValue(), false
	}
	number, ok := value.Data.(JSONData).Value.(json.Number)
	if !ok {
		return NullValue(), false
	}
	parsed, err := number.Float64()
	if err != nil {
		return NullValue(), false
	}
	return FloatValue(parsed), true
}

func jsonBoolValue(value Value) (Value, bool) {
	if value.Kind != ValueJSON {
		return NullValue(), false
	}
	current, ok := value.Data.(JSONData).Value.(bool)
	return BoolValue(current), ok
}

func jsonIsNull(value Value) bool {
	return value.Kind == ValueJSON && value.Data.(JSONData).Value == nil
}
