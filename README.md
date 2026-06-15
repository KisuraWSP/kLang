# kLang

kLang is an experimental programming language implemented in Go. The project includes a lexer, parser, module resolver, type checker, interpreter runtime, standard library modules, test fixtures, example projects, editor extensions, and early packaging support for standalone and browser/WASM workflows.

The language is function-first and strongly checked, with a design that borrows practical ideas from Go, Rust, Swift, Lua, Python, Elm, and functional languages. The current focus is building a small but expressive language core with clear source-aware diagnostics and predictable runtime behavior.

> Temporary note: the examples in examples folder are incomplete and experimental as langauges api and features will change

## Language Snapshot

kLang currently experiments with:

- Immutable-by-default bindings, explicit `mut`, and strict type checking.
- Explicit typed declarations with `local` and `global`.
- Inferred declarations with `let`, `val`, `var`, and `const`.
- Builtin `Option[T]` and `Result[T, E]` values.
- Function-first programming, first-class functions, lambdas, and inline candidates.
- Multiple return values, named returns, destructuring, and discard bindings with `_`.
- Pattern matching with `if value == { case ... }`.
- Typed ordinal `enum` declarations inspired by Go `const`/`iota`.
- Shared builtin protocols such as `.count`, `.uppercase()`, `.lowercase()`, and integer `.times(...)`.
- Copy-on-write aggregate storage for `List`, `Map`, `Table`, and `SIMD`.
- Lua-style `Table` as the only dynamically typed container.
- Alias functions for constructor-like custom data types and extension methods.
- Async functions, awaitable values, iterators, coroutines, and interpreter worker threads.
- `Atomic[T]` for race-safe shared values.
- Builtin allocator and reference-style values such as `Box`, `Ref`, `RefMut`, and `RefCell`.
- Source-aware `Context` and `ErrorContext` diagnostics across parsing, checking, runtime, packaging, and WASM backend work.

For the full design surface, read `LANGUAGE-SPEC.md`, `DATA-TYPES.md`, and `SYNTAX.md`.

## Project Layout

- `main.go` contains the CLI entrypoint.
- `src/lexer` tokenizes `.klang` source files.
- `src/parser` builds the AST and expression trees.
- `src/engine/module_system` resolves local and stdlib imports.
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

Check a script or project without running it:

```sh
go run . check examples/helloworld
```

Run a script or project:

```sh
go run . run examples/helloworld
```

Run without stdlib module resolution while still allowing local workspace imports:

```sh
go run . check examples/helloworld --raw-lang
```

Build a local binary:

```sh
go build -o kLang .
./kLang check examples/helloworld
./kLang run examples/helloworld
```

## A Small Program

kLang files use the `.klang` extension. Folder projects use `first.klang` as the entry file.

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

Create a project with a custom entry point:

```sh
go run . new examples/myproject --entry=["Process","Int"]
```

Pass program arguments. They are available inside kLang as `Args`:

```sh
go run . run examples/commandlinearena demo 100
```

Show import cache/details:

```sh
go run . check examples/showcase --verbose
```

Print a source file with line labels:

```sh
go run . file examples/helloworld/first.klang
```

Package a checked project into a compact source bundle:

```sh
go run . package examples/helloworld --backend=Standalone
```

The package command writes a `klang-build.json` manifest plus source files under `dist/<project>-<backend>`. Valid backends are `Standalone`, `JS`, and `WASM`.

Build a browser-ready WASM bundle:

```sh
go run . package examples/helloworld --backend=WASM
cd examples/helloworld/dist/helloworld-wasm
python3 -m http.server 8080
```

Then open `http://localhost:8080`. The WASM backend compiles the Go-based kLang interpreter/runtime to `klang.wasm`, copies Go's `wasm_exec.js`, and loads resolved `.klang` sources through `klang_browser.js`. Browser code can call `KlangBrowser.runProject()` or `KlangBrowser.runSource(source, args)`.

Start the built-in browser server without manually preparing the bundle:

```sh
go run . serve examples/helloworld --port=8080
```

You can also keep the bundle and serve it in one step:

```sh
go run . package examples/helloworld --backend=WASM --serve --out dist
```

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
