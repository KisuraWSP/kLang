package wasmbackend

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"kLang/src/engine/backend"
	jsbackend "kLang/src/engine/backend/js"
	"kLang/src/engine/bytecode"
)

type Compiler struct{}

func New() *Compiler {
	return &Compiler{}
}

func (compiler *Compiler) Name() string {
	return "WASM"
}

func (compiler *Compiler) Check(request backend.Request) []backend.Diagnostic {
	_, diagnostics := compiler.lower(request)
	return diagnostics
}

func (compiler *Compiler) Emit(request backend.Request) (backend.Output, error) {
	program, diagnostics := compiler.lower(request)
	if len(diagnostics) != 0 {
		return backend.Output{}, fmt.Errorf("WASM bytecode backend rejected %d unsupported construct(s)", len(diagnostics))
	}
	encoded, err := bytecode.Encode(program)
	if err != nil {
		return backend.Output{}, fmt.Errorf("encode WASM bytecode: %w", err)
	}
	return backend.Output{
		Entry: "program.kbc",
		Artifacts: []backend.Artifact{
			{Path: "program.kbc", Content: encoded},
		},
	}, nil
}

func (compiler *Compiler) Package(output backend.Output, bundleDir string) error {
	for _, artifact := range output.Artifacts {
		clean := filepath.Clean(artifact.Path)
		if clean == "." || filepath.IsAbs(clean) || strings.HasPrefix(clean, ".."+string(filepath.Separator)) {
			return fmt.Errorf("invalid WASM bytecode artifact path %q", artifact.Path)
		}
		target := filepath.Join(bundleDir, clean)
		if err := os.MkdirAll(filepath.Dir(target), 0755); err != nil {
			return err
		}
		if err := os.WriteFile(target, artifact.Content, 0644); err != nil {
			return err
		}
	}
	return nil
}

func (compiler *Compiler) lower(request backend.Request) (bytecode.Program, []backend.Diagnostic) {
	request.Backend = "WASM"
	program, diagnostics := jsbackend.LowerIR(request)
	for index := range diagnostics {
		diagnostics[index].Rule = "WASM_BYTECODE_UNSUPPORTED"
		diagnostics[index].Hint = "This project will use the browser interpreter fallback until the bytecode subset supports this construct."
	}
	if len(diagnostics) != 0 {
		return bytecode.Program{}, diagnostics
	}
	compiled, compileDiagnostics := bytecode.Compile(program)
	for _, diagnostic := range compileDiagnostics {
		diagnostics = append(diagnostics, backend.Diagnostic{
			File: diagnostic.File, Line: diagnostic.Line, Column: diagnostic.Column,
			EndColumn: diagnostic.Column + 1,
			Rule:      "WASM_BYTECODE_UNSUPPORTED",
			Message:   diagnostic.Message,
			Hint:      diagnostic.Hint,
		})
	}
	return compiled, diagnostics
}
