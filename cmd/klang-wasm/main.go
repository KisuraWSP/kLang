//go:build js && wasm

package main

import (
	"encoding/json"
	"fmt"
	"strings"
	"syscall/js"

	"kLang/src/engine/file"
	"kLang/src/engine/runtime"
	typechecker "kLang/src/engine/type_checker"
	"kLang/src/parser"
)

type response struct {
	OK     bool              `json:"ok"`
	Value  any               `json:"value,omitempty"`
	Output []string          `json:"output,omitempty"`
	Memory map[string]int    `json:"memory,omitempty"`
	Errors []browserError    `json:"errors,omitempty"`
	Meta   map[string]string `json:"meta,omitempty"`
}

type browserError struct {
	Stage   string `json:"stage"`
	File    string `json:"file,omitempty"`
	Line    int    `json:"line,omitempty"`
	Column  int    `json:"column,omitempty"`
	Message string `json:"message"`
}

func main() {
	js.Global().Set("klangRun", js.FuncOf(runSource))
	js.Global().Set("klangCheck", js.FuncOf(checkSource))
	js.Global().Set("klangRunProject", js.FuncOf(runProject))
	js.Global().Set("klangCheckProject", js.FuncOf(checkProject))
	select {}
}

func runSource(_ js.Value, args []js.Value) any {
	if len(args) == 0 {
		return jsonResponse(response{OK: false, Errors: []browserError{{Stage: "input", Message: "klangRun expects source text"}}})
	}
	program := singleSourceProgram(args[0].String())
	return jsonResponse(run(program, jsStrings(argAt(args, 1))))
}

func checkSource(_ js.Value, args []js.Value) any {
	if len(args) == 0 {
		return jsonResponse(response{OK: false, Errors: []browserError{{Stage: "input", Message: "klangCheck expects source text"}}})
	}
	program := singleSourceProgram(args[0].String())
	return jsonResponse(check(program))
}

func runProject(_ js.Value, args []js.Value) any {
	if len(args) == 0 {
		return jsonResponse(response{OK: false, Errors: []browserError{{Stage: "input", Message: "klangRunProject expects a project object"}}})
	}
	program, err := programFromJSProject(args[0])
	if err != nil {
		return jsonResponse(response{OK: false, Errors: []browserError{{Stage: "input", Message: err.Error()}}})
	}
	return jsonResponse(run(program, jsStrings(argAt(args, 1))))
}

func checkProject(_ js.Value, args []js.Value) any {
	if len(args) == 0 {
		return jsonResponse(response{OK: false, Errors: []browserError{{Stage: "input", Message: "klangCheckProject expects a project object"}}})
	}
	program, err := programFromJSProject(args[0])
	if err != nil {
		return jsonResponse(response{OK: false, Errors: []browserError{{Stage: "input", Message: err.Error()}}})
	}
	return jsonResponse(check(program))
}

func run(program file.Program, args []string) response {
	if checked := check(program); !checked.OK {
		return checked
	}
	parsed := parser.ParseLoadedProgram(program)
	result, err := runtime.NewWithArgs(args).Run(parsed)
	if err != nil {
		return response{OK: false, Errors: []browserError{{Stage: "runtime", File: program.EntryPoint, Message: err.Error()}}}
	}
	return response{
		OK:     true,
		Value:  encodeValue(result.Value),
		Output: result.Output,
		Memory: map[string]int{
			"stack_objects": result.Memory.StackObjects,
			"heap_objects":  result.Memory.HeapObjects,
			"stack_bytes":   result.Memory.StackBytes,
			"heap_bytes":    result.Memory.HeapBytes,
			"total_objects": result.Memory.TotalObjects,
			"total_bytes":   result.Memory.TotalBytes,
		},
		Meta: map[string]string{
			"project": program.Name,
			"entry":   program.EntryPoint,
		},
	}
}

func check(program file.Program) response {
	typeReport := typechecker.CheckProgram(program)
	if !typeReport.Passed() {
		errors := make([]browserError, 0, len(typeReport.Errors))
		for _, err := range typeReport.Errors {
			errors = append(errors, browserError{Stage: "type", File: err.File, Line: err.Line, Message: err.Message})
		}
		return response{OK: false, Errors: errors}
	}

	parsed := parser.ParseLoadedProgram(program)
	if !parsed.Passed() {
		var errors []browserError
		for _, source := range parsed.Sources {
			for _, err := range source.Errors {
				errors = append(errors, browserError{Stage: "parse", File: source.Path, Line: err.Line, Column: err.Column, Message: err.Message})
			}
		}
		return response{OK: false, Errors: errors}
	}

	return response{OK: true, Meta: map[string]string{"project": program.Name, "entry": program.EntryPoint}}
}

func singleSourceProgram(source string) file.Program {
	path := "browser.klang"
	return file.Program{
		Name:       "browser",
		Root:       ".",
		EntryPoint: path,
		Files: []file.SourceFile{
			{Path: path, Lines: sourceLines(source)},
		},
	}
}

func programFromJSProject(project js.Value) (file.Program, error) {
	if project.IsUndefined() || project.IsNull() {
		return file.Program{}, fmt.Errorf("project is null or undefined")
	}

	name := stringProperty(project, "name", "browser-project")
	entry := stringProperty(project, "entry", "")
	filesValue := project.Get("files")
	if filesValue.IsUndefined() || filesValue.IsNull() {
		return file.Program{}, fmt.Errorf("project.files is required")
	}

	keys := js.Global().Get("Object").Call("keys", filesValue)
	files := make([]file.SourceFile, 0, keys.Length())
	for index := 0; index < keys.Length(); index++ {
		path := keys.Index(index).String()
		source := filesValue.Get(path).String()
		files = append(files, file.SourceFile{Path: path, Lines: sourceLines(source)})
		if entry == "" {
			entry = path
		}
	}
	if len(files) == 0 {
		return file.Program{}, fmt.Errorf("project.files must contain at least one Klang source")
	}
	return file.Program{Name: name, Root: ".", EntryPoint: entry, Files: files}, nil
}

func stringProperty(value js.Value, name string, fallback string) string {
	property := value.Get(name)
	if property.IsUndefined() || property.IsNull() {
		return fallback
	}
	text := strings.TrimSpace(property.String())
	if text == "" {
		return fallback
	}
	return text
}

func sourceLines(source string) []string {
	source = strings.ReplaceAll(source, "\r\n", "\n")
	source = strings.ReplaceAll(source, "\r", "\n")
	source = strings.TrimSuffix(source, "\n")
	if source == "" {
		return nil
	}
	return strings.Split(source, "\n")
}

func jsStrings(value js.Value) []string {
	if value.IsUndefined() || value.IsNull() {
		return nil
	}
	items := make([]string, 0, value.Length())
	for index := 0; index < value.Length(); index++ {
		items = append(items, value.Index(index).String())
	}
	return items
}

func argAt(args []js.Value, index int) js.Value {
	if index >= len(args) {
		return js.Undefined()
	}
	return args[index]
}

func jsonResponse(value response) string {
	encoded, err := json.Marshal(value)
	if err != nil {
		fallback, _ := json.Marshal(response{OK: false, Errors: []browserError{{Stage: "bridge", Message: err.Error()}}})
		return string(fallback)
	}
	return string(encoded)
}

func encodeValue(value runtime.Value) any {
	encoded := map[string]any{"kind": string(value.Kind)}
	switch value.Kind {
	case runtime.ValueNull:
		encoded["value"] = nil
	case runtime.ValueInt, runtime.ValueFloat, runtime.ValueString, runtime.ValueBool, runtime.ValueChar:
		encoded["value"] = value.Data
	case runtime.ValueList:
		items := value.Data.([]runtime.Value)
		values := make([]any, 0, len(items))
		for _, item := range items {
			values = append(values, encodeValue(item))
		}
		encoded["value"] = values
	case runtime.ValueTable, runtime.ValueMap:
		items := value.Data.(map[string]runtime.Value)
		values := map[string]any{}
		for key, item := range items {
			values[key] = encodeValue(item)
		}
		encoded["value"] = values
	case runtime.ValueOption:
		option := value.Data.(runtime.OptionData)
		encoded["some"] = option.Some
		encoded["value"] = encodeValue(option.Value)
	case runtime.ValueResult:
		result := value.Data.(runtime.ResultData)
		encoded["ok"] = result.Ok
		encoded["value"] = encodeValue(result.Value)
	case runtime.ValueComplex:
		complexValue := value.Data.(runtime.ComplexData)
		encoded["real"] = complexValue.Real
		encoded["imaginary"] = complexValue.Imag
	case runtime.ValueSIMD:
		simd := value.Data.(runtime.SIMDData)
		values := make([]any, 0, len(simd.Lanes))
		for _, lane := range simd.Lanes {
			values = append(values, encodeValue(lane))
		}
		encoded["value"] = values
	case runtime.ValueObject:
		object := value.Data.(runtime.ObjectData)
		fields := map[string]any{}
		for key, item := range object.Fields {
			fields[key] = encodeValue(item)
		}
		encoded["type"] = object.Type
		encoded["fields"] = fields
	default:
		encoded["value"] = fmt.Sprintf("%v", value.Data)
	}
	return encoded
}
