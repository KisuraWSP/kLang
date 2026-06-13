# Klang Emacs Mode

Emacs major mode support for `.klang` files.

## Features

- Automatic `klang-mode` for `*.klang`.
- Syntax highlighting for comments, strings, here strings, declarations, builtin types, traits, aliases, hooks, directives, pattern matching, async/coroutines, atomics, ownership keywords, constants, and function calls.
- Basic indentation for braces, `do/end`, `case`, `else`, and `catch`.
- `comment-dwim` support for `--` line comments.
- Skeleton commands for common Klang declarations.

## Local Installation

Add this folder to your Emacs `load-path` and require the mode:

```elisp
(add-to-list 'load-path "/path/to/kLang/extensions/klang-emacs")
(require 'klang-mode)
```

Then open a `.klang` file, or run:

```elisp
M-x klang-mode
```

## Skeleton Commands

- `M-x klang-insert-function`
- `M-x klang-insert-alias-function`
- `M-x klang-insert-main`

