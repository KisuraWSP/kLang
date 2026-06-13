# Klang Sublime Text Package

Sublime Text language support for `.klang` files.

## Features

- Syntax highlighting for comments, strings, chars, numbers, declarations, builtin types, ownership keywords, pattern matching, async/coroutines, aliases, traits, hooks, operators, and function calls.
- Line comments with `--`.
- Menu entry under `View > Syntax > Klang`.
- Snippets for common Klang declarations, control flow, inferred variables, aliases, lambdas, async code, Option/Result values, and `Type.sizeof`.

## Local Installation

Copy this folder into your Sublime Text `Packages` directory as `klang-sublime`.

On macOS that is usually:

```sh
~/Library/Application Support/Sublime Text/Packages/klang-sublime
```

Then open a `.klang` file and choose `View > Syntax > Klang` if Sublime does not auto-detect it.
