# Klang Language Support

VS Code language support for `.klang` files.

## Features

- Syntax highlighting for Klang comments, strings, chars, numbers, keywords, types, variables, functions, namespaces, imports, operators, and function calls.
- Auto-closing brackets and quotes.
- Line comments with `--`.
- Basic indentation and folding markers.
- Snippets for common Klang declarations and control flow.

## Local Development

Open this folder in VS Code:

```sh
code vscode/klang-vscode
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
