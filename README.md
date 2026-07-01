# kLang

<!-- ![Logo](logo.png) -->
<img src="logo.png" width="30%">

![written in Go](https://img.shields.io/badge/written%20in-Go-blue) ![version alpha-june](https://img.shields.io/badge/version-alpha%3A%20june-navy)

[User Guide](USER_GUIDE.md)

kLang is an experimental programming language implemented in Go. The project includes a lexer, parser, module resolver, type checker, interpreter runtime, standard library modules, test fixtures, example projects, editor extensions, and early packaging support for standalone and browser/WASM workflows.

The language is function-first and strongly checked, with a design that borrows practical ideas from Go, Rust, Swift, Lua, Python, Elm, and functional languages. The current focus is building a small but expressive language core with clear source-aware diagnostics and predictable runtime behavior.

> language is currently under construction be prepared to face breaking changes

> Temporary note: the examples in examples folder are incomplete and experimental as langauges api and features will change

> Breaking change: kLang projects now require a `klang.project` TOML manifest. Existing `.klang` scripts or legacy `first.klang` folder projects will not run unless their entry source explicitly opts into script loading with `load_as_script;`. This prepares the executable to run `default_program.klang` by default in the future while keeping intentional scripts explicit.

> Breaking change: every runnable script or project must define exactly one top-level `function Main() : Int`, unless exactly one alternate `function Name() : Int` is immediately preceded by `#set_entry_point_to_here`. Entry functions cannot accept parameters, use generics, be async, or return another type.

## Language Snapshot

kLang currently experiments with:

- A simpler `.grua` subset frontend that runs through the same kLang toolchain and runtime.
- Immutable-by-default bindings, explicit `mut`, and strict type checking.
- Explicit typed declarations with `local` and `global`.
- Inferred declarations with `let`, `val`, `var`, and `const`.
- Builtin `Option[T]` and `Result[T, E]` values.
- Builtin `Set[T]` for deterministic deduplication and membership checks.
- Runtime-backed string formatting through `format`, `printf`, and the stdlib `fmt` module.
- Generic constraints through `restrict[...]`, named groups such as `numeric`/`hashable`/`iterable`, and trait-bound type parameters.
- Compile-time and runtime state tracking for variables, parameters, named returns, and return values.
- Function-first programming, first-class functions, lambdas, and inline candidates.
- Pass-by-value function calls by default, with explicit `ref` parameters for mutable pass-by-reference calls.
- Multiple return values, named returns, destructuring, and discard bindings with `_`.
- Pattern matching with `if value == { case ... }`.
- Typed ordinal `enum` declarations inspired by Go `const`/`iota`.
- Shared builtin protocols such as `.count`, `.uppercase()`, `.lowercase()`, and integer `.times(...)`.
- Copy-on-write aggregate storage for `List`, `Map`, `Table`, and `SIMD`.
- Lua-style `Table` as the only dynamically typed container.
- Alias functions for constructor-like custom data types, binary operator overloads, and standalone extension methods for builtin and alias-struct receivers.
- Async functions, awaitable values, iterators, coroutines, and interpreter worker threads.
- `Atomic[T]` for race-safe shared values.
- Builtin allocator and reference-style values such as `Box`, `Ref`, `RefMut`, and `RefCell`.
- Source-aware `Context` and `ErrorContext` diagnostics across parsing, checking, runtime, packaging, and WASM backend work.
- First-class CLI testing through `kLang test`, `Test...` functions, `assert`, return-value checks, and optional golden-output files.

For the full design surface, read `LANGUAGE-SPEC.md`, `DATA-TYPES.md`, and `SYNTAX.md`.

## Project Layout

- `main.go` contains the CLI entrypoint.
- `src/lexer` tokenizes `.klang` source files.
- `src/grua` validates and lowers `.grua` subset source into kLang.
- `src/parser` builds the AST and expression trees.
- `src/engine/module_system` resolves local and stdlib imports.
- `src/engine/program_cache` persists safe startup cache entries for unchanged scripts and projects.
- `src/engine/type_checker` performs semantic checks and type checks.
- `src/engine/runtime` executes parsed kLang programs.
- `src/engine/context` carries workspace context and source-aware diagnostics.
- `stdlib` contains standard library modules written for the language.
- `examples` contains experimental kLang projects.
- `tests` contains language test fixtures.
- `extensions/klang-vscode` contains the VS Code language extension.
- `extensions/klang-sublime` contains the Sublime Text language package.
- `extensions/klang-vim` contains the Vim and Neovim language plugin.
- `extensions/klang-emacs` contains the Emacs major mode.

## Quick Start

Run commands from the repository root.

```sh
go run . --help
```

Create a new project:

```sh
go run . new examples/myproject
```

Check a project without running it:

```sh
go run . check examples/helloworld
```

Run a project:

```sh
go run . run examples/helloworld
```

Run a standalone Grua subset program:

```sh
go run . run examples/grua/hello.grua
```

Grua uses inferred variables, Table aggregates, `::Hint` function parameters,
`switch` pattern matching, one `for` loop family, and only the `basic`, `file`,
`io`, and `repl` stdlib modules. These resolve exclusively from `stdlib/grua`
as `.grua` source; Grua never falls through to the richer `.klang` modules.

Run without stdlib module resolution while still allowing local workspace imports:

```sh
go run . check examples/helloworld --raw-lang
```

Build a local binary:

```sh
./build.sh
./build/native/$(go env GOOS)-$(go env GOARCH)/kLang check examples/helloworld
./build/native/$(go env GOOS)-$(go env GOARCH)/kLang run examples/helloworld
```

The portable build script runs on macOS, Linux, WSL, MSYS2, and Git Bash.
Use `./build.sh --all` to cross-compile the Linux, macOS, and Windows release
matrix plus the browser WASM runtime, `./build.sh --target windows/amd64` for
one target, or `./build.sh --test` to test before building. Run
`./build.sh --help` for artifact paths and all options.

## A Small Program

kLang files use the `.klang` extension. Projects are described by `klang.project`, a TOML manifest:

```toml
name = "demo"
entry = "first.klang"
language_version = 1
sources = ["first.klang", "app.klang"]
```

Run `kLang update <project-folder>` after upgrading kLang. It applies deterministic
manifest migrations, keeps a backup of an existing manifest, and reports source
changes that need manual attention through the normal compiler diagnostics.

Loose `.klang` files are treated as scripts only when they opt in:

```lua
load_as_script;
```

```lua
val appName = "demo";
const intSize = Int.sizeof;

enum NetworkState {
    Idle;
    Connecting = 10;
    Connected;
    Failed;
}

function Add(left : Int, right : Int) : Int {
    return left + right;
}

function RememberIndex(index : Int) : Int {
    return index;
}

function Main() : Int {
    let maybeCount = Some(41);
    let mut total = Add(maybeCount.value, 1);
    local NetworkState state = NetworkState.Connected;

    if state == {
        case NetworkState.Idle:
            print(appName, "idle");
        case NetworkState.Connected:
            print(appName, "connected", total);
        case:
            print(appName, "other state", state.name);
    }

    local Int itemCount = [10, 20, 30].count;
    local String loud = "hallo".uppercase();
    local Int lastIndex = 5.times(RememberIndex);

    print(loud, itemCount, lastIndex, intSize);
    return total;
}
```

Common rules:

- Variables are immutable by default.
- Use `mut` when a variable or function parameter must change.
- `local` and `global` declarations use explicit types.
- `let`, `val`, `var`, and `const` infer types from initializers.
- `let` is local immutable, `let mut` is local mutable, `val` is global immutable, and `var` is global mutable.
- `const` is strictly immutable and must have an initializer.
- `Args` is a builtin immutable `List[String]` containing command-line arguments.
- Builtin type names expose `.sizeof`.
- Collection-like values expose `.count`.
- Tables are dynamic. Most other values are statically checked.

## CLI Commands

Create a project with the default `Main()` entry wrapper:

```sh
go run . new examples/myproject
```

New projects always generate `first.klang` with a stable `Main() : Int` entry
function that calls `App.Start()`. This exact zero-argument `Int` signature is
mandatory. The old `--entry` flag is deprecated and ignored. A program may
instead choose exactly one custom zero-argument `Int` entry function by placing
`#set_entry_point_to_here` immediately before it.

Pass program arguments. They are available inside kLang as `Args`:

```sh
go run . run examples/commandlinearena demo 100
```

After a successful run, the CLI prints the returned value, elapsed runtime, the
number of resolved source lines processed, and source lines processed per
second.

Show import cache/details:

```sh
go run . check examples/showcase --verbose
```

Successful `check` and `run` commands write a best-effort `.klang-cache` folder beside the program root. On the next unchanged startup, kLang can reuse the resolved and type-checked source set and skip that work. Source edits, entry changes, and `--raw-lang` changes invalidate the cache automatically.

Print a source file with line labels:

```sh
go run . file examples/helloworld/first.klang
```

Generate static HTML documentation for one file or a folder project:

```sh
go run . doc '--sourcefile=[examples/helloworld]' --out=helloworld-docs.html
```

The generated documentation includes declaration cards and source-code chapters for each loaded `.klang` file.

Package a checked project into a compact source bundle:

```sh
go run . package examples/helloworld --backend=Standalone
```

The package command writes a `klang-build.json` manifest plus source files under `dist/<project>-<backend>`. Valid backends are `Standalone`, `JS`, and `WASM`. Standalone packages the Go interpreter, JS emits native JavaScript for its typed subset, and WASM emits bytecode for its typed core subset with a browser interpreter fallback.

Compile a typed-core program to JavaScript:

```bash
go run . package main.klang --backend=JS
node dist/main-js/program.js
```

The JS subset covers primitive, `List[T]`, `Map[K,V]`, and `Table` values, variables, expressions, ordinary functions, namespaces, namespace aliases, imported modules, single returns, `if`/`unless`, `while`, integer range loops, assignment, loop control, assertions, throws, and `print`. Selective imports emit only referenced functions and their helper dependencies. Unsupported runtime-heavy features fail during the JS backend check with source-positioned diagnostics.

JS-generated Strings support mixed concatenation, Unicode-aware `len` and `.count`, bounds-checked indexing, `.uppercase()`, `.lowercase()`, and primitive-to-String casts. The generated runtime helpers preserve Klang's code-point and display-format semantics instead of JavaScript's raw UTF-16 behavior.

JS-generated Lists support literals, empty values, `len`/`.count`, bounds-checked reads, indexed growth and compound mutation, list comprehensions, and `for index := range(len(values))`. Nested List values retain kLang's value isolation across assignments and function boundaries.

JS-generated Maps enforce declared key/value types across literals, reads, mutation, compound assignment, returns, count, and copying. Tables retain insertion order and primitive key identity while supporting selector reads, fallbacks, deletion, keys/values/entries, sequence counts, JSON String-key rules, and value isolation.

JSON serialization works directly with Tables, String-keyed Maps, Lists, primitives, Null/Option values, enums, and tagged alias structs. `json_encode`/`json_decode` and `json.serialize`/`json.deserialize` provide safe Result-based native-value round trips; generated JavaScript implements the same Table/List conversion surface.

JS-generated struct aliases support constructors, typed fields, trailing defaults, field access, `#extend` methods, and value isolation. Struct values also support `cast_as(Table)`, `cast_as(JSON)`, `cast_as(String)`, and field-matched conversion to another struct alias with target defaults. `JSON(value)` and `json_stringify(value)` serialize nested structs and Lists with declared JSON field tags and deterministic key ordering. Runtime-heavy alias hooks still produce source-positioned JS backend diagnostics.

Native JS packages include `program.js.map` with Source Map v3 mappings, portable `src/...` references, and embedded source content. `npm start` enables Node source-map stacks; direct `node program.js` execution still prints built-in kLang function frames, source excerpts, and carets for uncaught runtime errors. Backend rejections retain JS-specific rule identifiers and source spans.

Build a browser-ready WASM bundle:

```sh
go run . package examples/helloworld --backend=WASM
cd examples/helloworld/dist/helloworld-wasm
python3 -m http.server 8080
```

Then open `http://localhost:8080`. For supported programs the WASM backend lowers typed-core IR into versioned `program.kbc` bytecode and executes it with the stack VM inside `klang.wasm`. Bytecode version 3 covers primitives, recursive Lists, named callback references, fused pipelines, checked indexing and direct mutation, value isolation, `len`/`.count`, range and `for_each` loops, locals, arithmetic/comparison, short-circuit booleans, `if`, `while`, direct functions, assertions, `print`, and single returns. Packages outside that subset omit `program.kbc` and use the bundled source interpreter, with the selected path recorded in `klang-build.json`. Browser code can call `KlangBrowser.runProject()` or `KlangBrowser.runSource(source, args)`.

Lists, arrays, typed Maps, Tables, Sets, Strings, Int counts, and Iterators support lazy chains such as `values.filter(Keep).map(Convert).limit(20).sort()`. Intermediate stages build a fused plan without running callbacks or creating temporary Lists. Terminals including `collect`, `sort`, `fold`, `first`, `any`, `all`, `for_each`, and Iterator `.count` execute the plan. The interpreter, generated JavaScript, and bytecode version 3 share these semantics; `sort` is the intentional materialization barrier. The lazy `limit` stage is separate from existing eager stdlib methods named `take`.

Start the built-in browser server without manually preparing the bundle:

```sh
go run . serve examples/helloworld --port=8080
```

You can also keep the bundle and serve it in one step:

```sh
go run . package examples/helloworld --backend=WASM --serve --out dist
```

## Standard Library API

Stdlib modules use lowercase import names, UpperCamelCase namespace functions, and lower_snake_case receiver methods. Older callable spellings remain available as deprecated compatibility shims with direct replacement suggestions. Import `api` for reflective module discovery, Parsable AST inspection, polling, source transforms, and migration metadata.

## HTML And Browser-Oriented Code

The stdlib `html` module renders escaped text, attributes, fragments, documents, and named HTML tags as strings.

```lua
import "html";

local String page = html.Document(
    "Hello",
    html.Main([html.Class("page")], html.H1([], html.Text("Hello from kLang")))
);
```

## Testing

Run the Go unit test suite:

```sh
go test ./...
```

Run the language fixture tests:

```sh
go run . test tests
```

Write first-class kLang tests as functions whose names start with `Test`. A test can use `assert`, return `Bool`, return an `Int` status code where `0` means success, or return nothing for assertion-only tests.

```lua
function TestAddition() {
    assert 1 + 1 == 2;
}

function TestBoolStyle() : Bool {
    return "klang".count == 5;
}

function TestStatusStyle() : Int {
    return 0;
}
```

Run a single test file:

```sh
go run . test tests/math_test.klang
```

Golden-output tests compare all output printed by the discovered `Test...` functions against a sibling `.golden` file. For `math_test.klang`, use `math_test.golden`; for folder projects, `project-name.golden` or `test.golden` in the project root are also recognized.

Check every example project without executing it:

```sh
go run . test examples
```

Run and execute every discovered example project:

```sh
go run . test examples --run
```

The examples are useful for exploring the language, but they should be treated as experimental while the language APIs and features are still changing.

## Root Markdown Files

- `README.md` is this project overview and getting-started guide.
- `AGENTS.md` contains contributor and agent guidance for working on the language.
- `DATA-TYPES.md` lists builtin language data types.
- `LANGUAGE-SPEC.md` describes the high-level language rules and design.
- `SYNTAX.md` shows syntax examples for variables, functions, modules, runtime features, and newer language constructs.

## Recommended Reading Path

1. Start with this `README.md`.
2. Read `LANGUAGE-SPEC.md` for the design rules.
3. Read `DATA-TYPES.md` for the builtin type surface.
4. Use `SYNTAX.md` as the practical cookbook while writing `.klang` files.
5. Explore `tests` for known-good language fixtures.
6. Explore `examples` as experimental projects that may lag behind the newest rules.

## Editor Support

VS Code:

```sh
code extensions/klang-vscode
```

Sublime Text:

Copy `extensions/klang-sublime` into your Sublime Text `Packages` directory.

Vim or Neovim:

```sh
mkdir -p ~/.vim/pack/klang/start
cp -R extensions/klang-vim ~/.vim/pack/klang/start/klang-vim
```

Emacs:

```elisp
(add-to-list 'load-path "/path/to/kLang/extensions/klang-emacs")
(require 'klang-mode)
```

The editor packages include syntax highlighting and templates/snippets for current kLang syntax.
