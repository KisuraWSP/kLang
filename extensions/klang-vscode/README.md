# Klang Language Support

VS Code language support for `.klang` files.

## Features

- Syntax highlighting for Klang comments, strings, chars, numbers, keywords, builtin types, inferred variables, functions, namespaces, imports, traits, aliases, hooks, pattern matching, ownership keywords, async/lazy functions, `.sizeof`, operators, and function calls.
- Auto-closing brackets and quotes.
- Line comments with `--`.
- Basic indentation and folding markers.
- Snippets for common Klang declarations, `let`/`var`/`val`/`const`, `Type.sizeof`, control flow, pattern matching, aliases, traits, lambdas, async functions, Option/Result values, list comprehensions, regions, and try/catch.

## Local Development

Open this folder in VS Code:

```sh
code extensions/klang-vscode
```

Then press `F5` to launch an Extension Development Host. Open a `.klang` file there to test highlighting.

## Packaging

If you have `vsce` installed:

```sh
vsce package
```

That creates a `.vsix` file you can install with:

```sh
code --install-extension klang-vscode-0.1.0.vsix
```
