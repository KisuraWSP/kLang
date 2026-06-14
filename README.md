# kLang

kLang is a prototype programming language written in Go. It is built as a small interpreted language with a lexer, parser, module resolver, type checker, runtime, standard library folder, example projects, and editor extensions.

The language is function-first, strongly checked, and experiments with ideas from Go, Rust, Lua, Python, Elm, and functional languages. It supports scoped variables, type inference, `Option`/`Result`, list comprehensions, traits, async/await, coroutines, multi-threaded interpreter workers, pattern matching, alias functions, atomic values, allocator-style values, and human-friendly diagnostics.

## Project Layout

- `main.go` contains the CLI entrypoint.
- `src/lexer` tokenizes `.klang` source files.
- `src/parser` builds the AST and expression trees.
- `src/engine/module_system` resolves local and stdlib imports.
- `src/engine/type_checker` performs semantic checks and type checks.
- `src/engine/runtime` executes parsed Klang programs.
- `stdlib` contains standard library modules written for the language.
- `examples` contains runnable Klang projects.
- `tests` contains language test fixtures.
- `extensions/klang-vscode` contains the VS Code language extension.
- `extensions/klang-sublime` contains the Sublime Text language package.
- `extensions/klang-vim` contains the Vim and Neovim language plugin.
- `extensions/klang-emacs` contains the Emacs major mode.

## How To Program In kLang

kLang files use the `.klang` extension. A folder project uses `first.klang` as the entry file, and can import sibling files or stdlib modules.

```lua
import "mathg";

val appName = "demo";
const intSize = Int.sizeof;

function Add(left : Int, right : Int) : Int {
    return left + right;
}

function Main() : Int {
    let maybeCount = Some(41);
    let mut total = Add(maybeCount.value, 1);

    if total == {
        case 42:
            print(appName, "answer", total);
        case:
            print("unexpected", total);
    }

    return total;
}
```

Common language rules:

- Variables are immutable by default. Use `mut` when a binding or parameter must change.
- `local` and `global` declarations use explicit types.
- `let`, `val`, `var`, and `const` infer types strictly from initializers.
- `let` is local immutable, `let mut` is local mutable, `val` is global immutable, and `var` is global mutable.
- `const` is strictly immutable and must be resolved before runtime.
- `Args` is a builtin immutable `List[String]` containing command-line arguments passed to the program.
- `spawn`, `join`, and `thread_status` run functions on child interpreter workers; use `Atomic[T]` for shared mutable counters/state.
- `Type.sizeof` returns the language runtime size for builtin types as an `Int`.
- Tables are the dynamic Lua-style container; most other values are statically checked.

For the full syntax tour, see `SYNTAX.md`.

## How To Use The CLI

Run commands from the repository root with `go run .`:

```sh
go run . --help
```

Create a new project:

```sh
go run . new examples/myproject
```

Create a new project with a custom entry point:

```sh
go run . new examples/myproject --entry=["Process","Int"]
```

Check a script or project without running it:

```sh
go run . check examples/helloworld
```

Run a script or project:

```sh
go run . run examples/helloworld
```

Program runs print OS, architecture, CPU count, Go runtime version, and elapsed execution time after the program finishes.

Pass program arguments. They are available inside kLang as `Args`:

```sh
go run . run examples/commandlinearena demo 100
```

Run without stdlib module resolution:

```sh
go run . check examples/helloworld --raw-lang
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

Then open `http://localhost:8080`. The WASM backend compiles the Go-based kLang interpreter/runtime to `klang.wasm`, copies Go's `wasm_exec.js`, and loads the resolved `.klang` sources through `klang_browser.js`. Browser code can call `KlangBrowser.runProject()` or `KlangBrowser.runSource(source, args)`.

Start the built-in browser server without manually preparing the bundle:

```sh
go run . serve examples/helloworld --port=8080
```

This creates a temporary WASM browser bundle, hosts it with kLang's built-in static web server, and prints the local URL. You can also keep the bundle and serve it in one step:

```sh
go run . package examples/helloworld --backend=WASM --serve --out dist
```

Show import cache/details:

```sh
go run . check examples/showcase --verbose
```

Print a source file with line labels:

```sh
go run . file examples/helloworld/first.klang
```

## How To Build

Build a local binary:

```sh
go build -o kLang .
```

Then run the binary directly:

```sh
./kLang check examples/helloworld
./kLang run examples/helloworld
```

## How To Run Tests

Run the Go unit test suite:

```sh
go test ./...
```

Run the language fixture tests:

```sh
go run . test tests
```

Run and execute every discovered example project:

```sh
go run . test examples --run
```

Check every example project without executing it:

```sh
go run . test examples
```

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
5. Explore `examples/showcase` and `examples/stresstest` for larger programs.

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

## Suggested Improvement

The next readability improvement would be adding a dedicated `docs/` folder with focused guides:

- `docs/getting-started.md`
- `docs/cli.md`
- `docs/language-tour.md`
- `docs/runtime.md`
- `docs/editor-extensions.md`

That would keep the root README short and friendly while giving bigger language topics room to breathe.
