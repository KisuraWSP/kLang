# kLang Codebase User Guide

This guide is for people who need to understand, repair, migrate, or extend the
kLang codebase without relying on an AI assistant. It explains how the repository
is laid out, how a `.klang` program moves through the implementation, and where
to make changes for common language features.

Read this together with:

- `DATA-TYPES.md`: the source of truth for builtin values and type behavior.
- `LANGUAGE-SPEC.md`: the source of truth for language semantics.
- `SYNTAX.md`: the source of truth for concrete user syntax and examples.
- `README.md`: user-facing project overview.
- `BACKEND.md`: backend-facing design notes.
- `RUNTIME-SPEC.md`: runtime-facing design notes.

Do not treat this file as a replacement for the three language spec files above.
This file explains the Go codebase and maintenance workflow.

## Quick Start

From the repository root:

```sh
go test ./...
go run . check path/to/project-or-file
go run . run path/to/project-or-file
go run . package path/to/project-or-file --backend=Standalone
go run . package path/to/project-or-file --backend=JS
go run . package path/to/project-or-file --backend=WASM
go run . fmt path/to/file.klang --check
```

Useful local conventions:

- A folder project is loaded from `klang.project`.
- A loose `.klang` file must opt in with `load_as_script;`.
- Legacy folder projects with `first.klang` also need `load_as_script;` unless
  they have `klang.project`.
- Use `--raw-lang` to disable stdlib module resolution while preserving local
  workspace imports.
- Use `no_cache;` in a source file to disable program cache load/store.

## The Big Picture

kLang is implemented in Go. The Go code provides:

1. CLI commands in `main.go`.
2. File and project loading in `src/engine/file`.
3. Module and stdlib resolution in `src/engine/module_system`.
4. Lexing in `src/lexer`.
5. Parsing and AST construction in `src/parser`.
6. Semantic/type checking in `src/engine/type_checker`.
7. Runtime interpretation in `src/engine/runtime`.
8. Backend packaging and code generation in `src/engine/backend`.
9. Formatting in `src/formatter`.
10. Stdlib source code in `stdlib`.
11. Examples and tests in `examples`, `tests`, `stdlib-examples`, and Go
    `_test.go` files.

The usual `check` or `run` flow is:

```text
CLI args
  -> file.LoadProgram
  -> module_system.Resolver.ResolveProgram
  -> type_checker.CheckProgram
  -> parser.ParseLoadedProgram
  -> runtime.NewWithArgs(...).Run(parsed)      only for run
```

The package flow is:

```text
CLI args
  -> file.LoadProgram
  -> module_system.Resolver.ResolveProgram
  -> type_checker.CheckProgram
  -> parser.ParseLoadedProgram
  -> backend Check/Emit/Package                for JS
  -> source bundle + klang-build.json          for all backends
  -> WASM browser runtime bundle               for WASM
```

Important detail: the type checker runs before the final parsed program in the
CLI flow. It parses internally too, but the runtime/backend receive a fresh
`parser.ParseLoadedProgram` result after type checking passes.

## Repository Map

### Root Files

- `main.go`: CLI entry point and command orchestration.
- `main_test.go`: CLI-level tests for project creation, packaging, JS builds,
  formatting, docs, and test runner behavior.
- `go.mod`: Go module declaration.
- `DATA-TYPES.md`, `LANGUAGE-SPEC.md`, `SYNTAX.md`: required language docs.
- `README.md`, `BACKEND.md`, `RUNTIME-SPEC.md`: project and architecture docs.
- `default_program.klang`: default Klang source used by language/runtime work.
- `first.klang`, `js_test.klang`, `testing_project`, `loltest`: local/manual
  sample programs.

### `src/lexer`

The lexer turns source text into tokens.

Key files:

- `lexer.go`: tokenization logic.
- `token.go`: token types and keyword mapping.
- `*_test.go`: lexer-specific tests.

Touch this package when adding syntax that introduces a new keyword, operator,
delimiter, literal form, or comment/string behavior.

### `src/parser`

The parser turns tokens into AST nodes.

Key files:

- `ast.go`: AST type definitions. If a feature needs a new syntax tree shape,
  it usually starts here.
- `parser.go`: statement parsing and high-level dispatch.
- `expression.go`: expression parsing.
- `program.go`: `ParseLoadedProgram`, concurrent parsing of loaded files,
  entry-point discovery, and cross-file type alias discovery.
- `desugar.go`: parser-level lowering, currently used for destructuring.
- `type_alias.go`: type alias discovery and resolution helpers.

Parser output types are used by the checker, runtime, formatter, docs, and JS
backend. AST changes have a wide blast radius.

### `src/engine/file`

This package loads source files and project manifests.

Key files:

- `file.go`: `LoadProgram`, `DiscoverPrograms`, `klang.project` reading,
  source file loading, and legacy script/folder behavior.
- `args.go`: legacy CLI flag helpers.

Important concepts:

- `file.Program` is the loaded source workspace.
- `file.SourceFile` contains a path, lines, and an optional
  `ModuleFunctionFilter`.
- `klang.project` contains `name`, `entry`, and optional `sources`.
- `dist` folders are skipped when discovering source files.

Touch this package when changing project layout, manifest behavior, script opt-in
rules, or source discovery.

### `src/engine/module_system`

The resolver expands imports and stdlib dependencies.

Key file:

- `module_system.go`.

Responsibilities:

- Resolves local imports and stdlib imports.
- Detects import cycles.
- Honors `module(disabled : True);`.
- Honors `module_caller(call_entire_module : True);`.
- Applies selective stdlib function loading through `ModuleFunctionFilter`.
- Loads `global namespace` stdlib modules into the internal symbol surface.
- Supports inferred imports from qualified calls such as `list.Append(...)`.

Touch this package when import rules, namespace aliases, stdlib dependency
selection, or global namespace behavior changes.

### `src/engine/type_checker`

The type checker performs semantic validation before runtime/backend execution.

Key files:

- `type_checker.go`: symbol collection, type alias collection, function/global
  checking, expression typing, type compatibility, generics, alias structs,
  traits, enums, Option/Result safety, and warnings.
- `scope_checker.go`: lexical scoping and shadowing checks.
- `*_test.go`: feature-specific checker tests.

Important structures:

- `TypeChecker`: owns symbol tables and current namespace state.
- `functionSymbol`, `variableSymbol`, `traitSymbol`, `enumSymbol`: internal
  semantic records.
- `Report`: errors, warnings, and compile-time state records.

Common feature work here:

- Add a new type rule.
- Add or reject a new expression/statement form.
- Teach the checker about a builtin function.
- Improve diagnostics.
- Enforce mutability, scope, alias, trait, enum, or Option/Result safety rules.

### `src/engine/runtime`

The runtime interprets parsed kLang programs.

Key files:

- `runtime.go`: `Runtime`, `ValueKind`, `Run`, `RunTests`,
  `prepareProgram`, symbol collection, function calls, statement execution, and
  builtin dispatch.
- `value.go`: value constructors, table/set/list helpers, cloning/sharing, and
  value formatting support.
- `environment.go`: lexical/runtime environment and bindings.
- `memory.go`: memory accounting.
- `json.go`: JSON builtin parsing/serialization.
- `parsable.go`: `Parsable[T]`, source transforms, AST metadata, macro helpers,
  and message polling.
- `*_test.go`: runtime feature tests.

Important runtime concepts:

- `Value` is a tagged dynamic Go representation of every runtime value.
- `Environment` stores bindings and parent scope links.
- `ObjectData` represents alias-struct objects.
- `TableData` and `SetData` preserve insertion order and key identity.
- Copy-on-write behavior is implemented by sharing/cloning aggregate values at
  runtime boundaries.
- Runtime errors should include line/column when possible.

Touch this package when a feature must actually execute, a builtin needs runtime
behavior, or a value kind changes.

### `src/engine/context`

This package turns lower-level errors into source-aware diagnostics.

Key file:

- `context.go`.

Responsibilities:

- Tracks source files in a `Context`.
- Converts module, parse, type, runtime, and backend errors into
  `ErrorContext`.
- Adds source lines, columns, rules, hints, and humanized messages.
- Provides expected/found type tree formatting for type errors.

Touch this package when improving error messages, source spans, hints, or
diagnostic consistency.

### `src/engine/program_cache`

This package stores source-fingerprint cache entries for faster repeat checks
and runs.

Key file:

- `cache.go`.

Important behavior:

- Cache is keyed by entry point, raw-lang mode, and resolved source
  fingerprints.
- `no_cache;` disables cache load/store for a workspace.
- Warnings may be restored from cache.

Touch this package when cache invalidation, fingerprinting, or cached metadata
changes.

### `src/engine/backend`

This package defines backend contracts.

Key file:

- `backend.go`.

The interface is:

```go
type Backend interface {
    Name() string
    Check(Request) []Diagnostic
    Emit(Request) (Output, error)
    Package(Output, string) error
}
```

Current backend meaning:

- `Standalone`: source bundle for the Go interpreter/runtime.
- `WASM`: browser bundle that runs the Go interpreter in WebAssembly.
- `JS`: experimental native JS code generation through IR.

### `src/engine/backend/js`

The JS backend lowers supported kLang syntax into `src/engine/ir`, then emits
JavaScript plus source maps.

Key files:

- `js.go`: backend implementation and AST-to-IR lowerer.
- `collection_runtime.go`: generated JS helpers for collection/table/string
  semantics.
- `source_map.go`: source map generation.
- `js_test.go`: backend tests.

Important limitations:

- JS is a typed-core backend, not the full interpreter.
- Unsupported runtime-heavy features must produce backend diagnostics, not
  silently fall back to interpretation.
- Struct aliases, Lists, Maps, Tables, strings, source maps, and richer
  diagnostics are supported in the subset described by the language docs.

Touch this package when adding JS support for an existing language feature.

### `src/engine/ir`

Backend-neutral intermediate representation for code generation.

Key file:

- `ir.go`.

This is currently shaped mainly by JS backend needs. Extend it carefully: every
new IR node usually requires lowering and emitting support.

### `src/formatter`

The formatter normalizes parse-valid Klang source.

Key file:

- `formatter.go`.

Important behavior:

- Parses first; invalid source is not formatted.
- Preserves comments and here strings.
- Uses four-space indentation.
- Works line/token based rather than full pretty-printing from AST.

Touch this package when syntax changes affect formatting or when adding canonical
style rules.

### `stdlib`

The standard library is written in Klang. It is loaded by the module resolver
unless `--raw-lang` is used.

Important modules:

- Core collections: `array`, `list`, `table`.
- Data and safety: `json`, `option`, `result`, `test`, `types`, `typeinfo`.
- Text/math/io: `strings`, `unicode`, `mathg`, `fmt`, `io`, `path`,
  `calender`.
- Runtime/meta: `runtime`, `language`, `metasystem`, `reflect`, `args`,
  `repl`.
- Domain helpers: `dsa`, `html`, `hash`, `encoding`, `ffi`, `raylib`,
  `js-wasm/*`.

Stdlib style notes:

- Prefer focused, composable APIs over new runtime types unless there is a
  strong reason.
- Use existing language features: `Option`, `Result`, `Table`, `JSON`,
  alias-structs, `#extend`, type aliases, and Parsable helpers where they make
  the API clearer.
- Keep `global namespace` modules small. Anything in a global namespace may be
  loaded into many programs and can affect package manifests and backend checks.
- If a module is disabled with `module(disabled : True);`, imports will fail.
- For misspelled historical filenames such as `calender.klang`, keep
  compatibility wrappers when introducing corrected namespaces.

### `examples`, `tests`, and `stdlib-examples`

- `examples`: user-facing language examples.
- `tests`: Klang source tests used by CLI tests and manual validation.
- `stdlib-examples`: focused examples for stdlib modules.
- Go `_test.go` files: unit/integration tests for implementation packages.

When adding a feature, prefer at least one Go test and one `.klang` example or
test if the feature is user-visible.

### `extensions`

Editor support for VS Code, Vim, Neovim, Emacs, and Sublime.

Touch these when adding syntax that should be highlighted or added as snippets.

## Command Flow In Detail

### `go run . check target`

1. `main.go` parses CLI args.
2. `file.LoadProgram(target)` loads the workspace.
3. `module_system.NewResolver("").ResolveProgram(program)` expands imports.
4. `type_checker.CheckProgram(resolvedProgram)` validates semantics.
5. `parser.ParseLoadedProgram(resolvedProgram)` parse-validates final loaded
   sources and discovers the entry point.
6. No runtime execution happens.

### `go run . run target`

Same as `check`, then:

1. `runtime.NewWithArgs(programArgs).Run(parsedProgram)` prepares runtime state.
2. Runtime defines builtin `Args`.
3. Runtime collects functions, globals, alias functions, enums, regions, and
   namespace aliases from all parsed sources.
4. Runtime resolves the entry function, defaulting to `Main` if no directive
   selected another entry point.
5. Runtime calls the entry function and prints output.

### `go run . package target --backend=JS`

1. Loads, resolves, type-checks, and parses exactly like `check`.
2. Creates a backend request.
3. `jsbackend.New().Check(request)` rejects unsupported features.
4. `Emit` lowers to IR and emits JS artifacts.
5. `Package` writes generated artifacts into the bundle directory.
6. `main.go` writes source copies and `klang-build.json`.

### `go run . package target --backend=WASM`

1. Loads, resolves, type-checks, and parses.
2. Writes a source bundle.
3. Builds/copies the Go interpreter runtime as `klang.wasm`.
4. Writes browser loader files such as `wasm_exec.js`, `klang_browser.js`, and
   `index.html`.

## How To Add A Language Feature

Use this checklist when implementing a new feature.

### 1. Update Or Read The Specs First

Before code changes, understand where the feature belongs:

- Syntax only: `SYNTAX.md`.
- Semantic rule: `LANGUAGE-SPEC.md`.
- Builtin type/value behavior: `DATA-TYPES.md`.
- Runtime behavior: `RUNTIME-SPEC.md`.
- Backend behavior: `BACKEND.md`.

If the feature is accepted, update docs in the same change or immediately after.

### 2. Lexer

Touch `src/lexer` if you need:

- New keywords.
- New token kinds.
- New operators.
- New literal syntax.
- New comment/string behavior.

Add lexer tests when tokenization can be ambiguous.

### 3. Parser And AST

Touch `src/parser/ast.go` when the feature needs a new node or fields on an
existing node.

Touch `src/parser/parser.go` or `expression.go` when the feature has syntax.

Touch `desugar.go` when syntax should lower into existing AST forms before
checking/runtime.

Add parser tests for accepted and rejected syntax.

### 4. Type Checker

Touch `src/engine/type_checker` to:

- Register new symbols.
- Enforce type compatibility.
- Add scoping rules.
- Check mutability.
- Validate generics/traits/aliases.
- Produce warnings.
- Improve diagnostics.

Checker tests should cover both success and failure paths.

### 5. Runtime

Touch `src/engine/runtime` if the feature executes or changes values.

Common places:

- `runtime.go`: statement/expression execution, builtin dispatch, function call
  behavior.
- `value.go`: new values, cloning, display, table/list/set behavior.
- `environment.go`: binding/scope behavior.
- `json.go`: JSON conversion.
- `parsable.go`: metaprogramming behavior.

Runtime tests should check observable execution, error behavior, and edge cases.

### 6. Module System

Touch `src/engine/module_system` if the feature changes:

- Imports.
- Namespace aliases.
- Stdlib resolution.
- Global namespace loading.
- Module directives.
- Selective stdlib function loading.

Be careful: resolver changes often affect package manifest file counts and many
CLI tests.

### 7. Formatter

Touch `src/formatter` if new syntax should format predictably.

Formatter input must already parse. Add tests for comments, here strings,
nesting, and spacing if the syntax interacts with them.

### 8. Backends

For JS support:

1. Add or extend IR in `src/engine/ir`.
2. Lower parser AST to IR in `src/engine/backend/js/js.go`.
3. Emit JS for the IR node.
4. Add runtime helper support in `collection_runtime.go` if needed.
5. Add source-map support if generated structure changes.
6. Add backend diagnostics for unsupported cases.

For Standalone/WASM packaging:

- Most source-level language features work through the interpreter.
- WASM packaging may need browser runtime files or loader changes.

### 9. Stdlib

If a feature is expressible in Klang, prefer implementing it in `stdlib` rather
than Go. Use Go only for behavior that needs host/runtime support or must be
builtin.

Stdlib changes should be checked against:

- Selective import behavior.
- JS backend support if the module may be imported by JS projects.
- Global namespace impact.
- Examples in `stdlib-examples`.

### 10. Tests And Examples

Run:

```sh
go test ./...
```

Then add targeted tests. Good places:

- `src/lexer/*_test.go`: token tests.
- `src/parser/*_test.go`: AST/syntax tests.
- `src/engine/type_checker/*_test.go`: semantic tests.
- `src/engine/runtime/*_test.go`: execution tests.
- `src/engine/backend/js/*_test.go`: JS backend tests.
- `main_test.go`: CLI behavior.
- `tests` or `stdlib-examples`: user-facing Klang samples.

## Feature Playbooks

### Add A New Builtin Function

1. Document it in `DATA-TYPES.md` or `LANGUAGE-SPEC.md`.
2. Teach the type checker the signature if calls need static checking.
3. Implement runtime dispatch in `src/engine/runtime`.
4. Add runtime tests for success and failure.
5. Add JS backend support or a clear backend diagnostic if JS cannot support it.
6. Add a stdlib wrapper if users should call it through a module.

### Add A New Statement

1. Add lexer keyword/token if needed.
2. Add AST node in `parser/ast.go`.
3. Parse it in `parser/parser.go`.
4. Add type checker handling.
5. Add runtime execution.
6. Add formatter behavior.
7. Add backend support or diagnostics.
8. Add tests at each layer.

### Add A New Expression

1. Add token support if needed.
2. Add AST expression type/fields.
3. Parse it in `expression.go`.
4. Type-check it.
5. Evaluate it in runtime.
6. Lower it for JS if supported.
7. Format it.
8. Test precedence and nesting carefully.

### Add A New Builtin Type

1. Add the type to `DATA-TYPES.md`.
2. Add a `ValueKind` and data structure in `runtime.go` if runtime-backed.
3. Add constructors/cloning/display in `value.go`.
4. Add parser/type syntax if it has custom generic or literal forms.
5. Teach the type checker about assignability, casts, selectors, and builtins.
6. Add JSON/serialization rules if applicable.
7. Add stdlib facade functions.
8. Add JS backend support or diagnostics.
9. Add tests for construction, assignment, mutation, printing, and errors.

### Add Or Change Stdlib Modules

1. Prefer normal `namespace module_name { ... }`.
2. Use `global namespace` only for compiler/runtime-wide builtins or aliases.
3. Keep APIs focused and composable.
4. Use `Option` and `Result` for absence/failure.
5. Use `Table` for descriptor records.
6. Use alias structs for stable structured values with methods.
7. Use JSON tags on alias fields when serialization names matter.
8. Consider JS backend limitations if examples package to JS.
9. Add a `stdlib-examples` folder when the module is important.

### Add JS Backend Support For An Existing Feature

1. Confirm the type checker already accepts the feature.
2. Add IR representation if existing IR cannot express it.
3. Extend JS lowerer collection if the feature introduces symbols.
4. Extend statement/expression lowering.
5. Extend emit helpers.
6. Add source map positions for new emitted constructs.
7. Add `js_test.go` coverage.
8. Ensure unsupported subcases produce diagnostics with rule/hint/source span.

## Diagnostics Guide

Diagnostics should be source-aware whenever possible.

Use `src/engine/context` to convert package-specific errors into `ErrorContext`.
Good diagnostics include:

- Phase: source, module, parse, type, runtime, backend, JS, or WASM.
- File.
- Line.
- Column and end column when possible.
- Rule.
- Human message.
- Hint.
- Source excerpt.

Type checker errors are plain strings internally, then improved by
`context.HumanTypeMessage`, `typeErrorRule`, `typeErrorHint`, and span helpers.
If you introduce new recurring type errors, teach `context.go` how to explain
them.

Backend diagnostics use `backend.Diagnostic` and should set:

- `Rule`, for example `js-backend/unsupported-feature`.
- `Message`, describing exactly what is unsupported.
- `Hint`, explaining what to use instead.
- Source position from the original AST node.

## Cache Notes

The program cache can hide repeated work. When debugging resolver or checker
changes:

- Add `no_cache;` to the source under test, or
- Run with changed source fingerprints, or
- Inspect `src/engine/program_cache`.

The CLI prints cache hit/miss details with `--verbose`.

## Stdlib Resolution Notes

Stdlib imports are selective by default. This means:

```klang
import "list";
list.Append(values, item);
```

should load only the functions needed for `list.Append` and dependencies, not
every function in `stdlib/list.klang`.

Use:

```klang
module_caller(call_entire_module : True);
```

when a source intentionally wants entire imported stdlib modules.

Global namespace modules are special. They can be loaded without an explicit
import to populate internal compiler/runtime symbols. Keep these small and
backend-safe.

## Runtime Value Notes

The runtime stores all values as:

```go
type Value struct {
    Kind ValueKind
    Data any
}
```

When adding value behavior, check:

- Construction helpers.
- `cloneValue` and `shareValue`.
- Copy-on-write behavior for aggregates.
- `valueString` display formatting.
- Equality/comparison.
- Selector/index behavior.
- JSON serialization.
- Type checker compatibility.
- JS backend equivalent behavior.

Tables and Sets use normalized primitive keys. Lists, Tables, Sets, Maps, and
other aggregate values must preserve observable copy isolation.

## Parser And Type Alias Notes

Type aliases are discovered before full parsing in `parser/program.go`, so all
loaded files can share aliases. If a new type syntax interacts with aliases,
check:

- `parser/type_alias.go`.
- `parser.ParseLoadedProgram`.
- `typechecker.collectTypeAliases`.
- Cast target rules in checker/runtime.

## Entry Point Notes

CLI project creation now always scaffolds `first.klang` with a stable
`Main() : Int` wrapper that returns `App.Start()`. The old `new --entry` flag is
deprecated, accepted for compatibility, and ignored.

`#set_entry_point_to_here` still arms the next function as the runtime entry
point when a source file intentionally needs a custom entry.

Entry discovery happens in `parser/program.go` by scanning parsed statements and
namespace bodies. If no entry is found, runtime defaults to `Main`.

## Testing Strategy

Minimum test command:

```sh
go test ./...
```

Useful targeted commands:

```sh
go test ./src/lexer
go test ./src/parser
go test ./src/engine/type_checker
go test ./src/engine/runtime
go test ./src/engine/backend/js
go test .
```

When a feature touches many layers, test in this order:

1. Lexer tests.
2. Parser tests.
3. Type checker tests.
4. Runtime tests.
5. Formatter tests.
6. Backend tests.
7. CLI tests.
8. Manual `go run . check/run/package` against examples.

## Common Failure Modes

### Parser Accepts Syntax But Type Checker Fails Everywhere

Likely cause: new AST node or expression is not handled by type checker
statement/expression dispatch.

Where to look:

- `src/engine/type_checker/type_checker.go`
- `src/engine/type_checker/scope_checker.go`

### Runtime Fails But Type Checker Passed

Likely cause: runtime evaluator does not implement a checked AST path, builtin,
selector, or value conversion.

Where to look:

- `src/engine/runtime/runtime.go`
- `src/engine/runtime/value.go`
- Runtime tests for similar features.

### JS Package Fails On Stdlib Code

Likely cause: a stdlib module loaded into a JS project uses a feature unsupported
by the JS typed core.

Where to look:

- `src/engine/backend/js/js.go`
- `stdlib` module imported by the program.
- Global namespace modules, especially if loaded implicitly.

Fix options:

- Make the stdlib function JS-core-friendly.
- Move runtime-heavy helpers into an opt-in module.
- Teach JS backend the feature.
- Emit a better backend diagnostic.

### Package Manifest File Count Changed

Likely cause: module resolver behavior changed, a global namespace gained
functions, or selective stdlib loading changed.

Where to look:

- `src/engine/module_system/module_system.go`
- `main_test.go`
- `ModuleFunctionFilter` behavior.

### Formatter Breaks Here Strings Or Comments

Likely cause: `formatter.go` line/token logic does not understand a new syntax
boundary.

Where to look:

- `startsHereString`
- `formatCodeLine`
- `formatTokens`
- formatter tests.

### Import Works In Runtime But Not In JS

Likely cause: resolver loads the right files, but JS lowerer rejects the imported
module feature.

Where to look:

- Resolver verbose output from `go run . package ... --backend=JS --verbose`.
- `jsbackend.lowerProgram`.
- Backend diagnostics.

## Manual Debugging Tips

Run with verbose output:

```sh
go run . run examples/showcase --verbose
go run . check tests/test21 --verbose
go run . package examples/helloworld --backend=JS --verbose
```

Successful `run` output ends with CLI-owned runtime metrics. `time` measures the
entrypoint runtime execution, and `lines` counts the resolved source lines used
by that run before deriving source lines per second.

Temporarily use `report value;` in Klang source to print runtime type, value,
and stack frames.

Use `debug_state()` in Klang source to inspect runtime binding events.

Use `--raw-lang` to determine whether a bug comes from user code or stdlib
resolution.

Use `module_caller(call_entire_module : True);` when checking whether selective
stdlib loading is hiding a dependency bug.

## Safe Change Rules

- Keep spec docs and implementation aligned.
- Prefer small feature slices that pass tests.
- Do not silently accept unsupported backend features.
- Do not add runtime-heavy functions to global stdlib modules unless necessary.
- Preserve compatibility wrappers when renaming public stdlib namespaces.
- Add source-aware diagnostics for new failure modes.
- Add tests close to the layer where the behavior lives.
- Run `go test ./...` before considering a change done.

## If You Are Lost

Start with these questions:

1. Is this source loading, importing, parsing, checking, running, formatting, or
   code generation?
2. Does the failure happen with `--raw-lang`?
3. Does `go run . check ...` pass but `go run . run ...` fail?
4. Does Standalone work but JS fail?
5. Did a global namespace or stdlib module start loading unexpectedly?
6. Is the behavior defined in `DATA-TYPES.md`, `LANGUAGE-SPEC.md`, or
   `SYNTAX.md`?

Then jump to the matching package:

- Loading/project issue: `src/engine/file`.
- Import/module issue: `src/engine/module_system`.
- Syntax issue: `src/lexer` and `src/parser`.
- Semantic issue: `src/engine/type_checker`.
- Execution issue: `src/engine/runtime`.
- Diagnostic issue: `src/engine/context`.
- Formatting issue: `src/formatter`.
- JS issue: `src/engine/backend/js`.
- Stdlib API issue: `stdlib`.

This repository is easiest to maintain when every feature has a clear path
through these layers and every unsupported path fails loudly with a useful
diagnostic.
