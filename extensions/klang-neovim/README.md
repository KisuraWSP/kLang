# Klang Neovim Plugin

Neovim language support for `.klang` files.

## Features

- Filetype detection for `*.klang`.
- Syntax highlighting for Klang declarations, builtins, types, comments, doc comments, strings, here strings, alias hooks, directives, traits, namespaces, ownership keywords, async/coroutines, atomics, FFI helpers, and operators.
- Filetype settings for `--` comments, `.klang` suffix lookup, and sane indentation defaults.
- Basic indentation for functions, namespaces, traits, impls, alias functions, hooks, blocks, cases, and `try/catch` style code.
- Optional Lua helper snippets available through `:KlangSnippet`.

## Local Installation

With native Neovim packages:

```sh
mkdir -p ~/.local/share/nvim/site/pack/klang/start
cp -R extensions/klang-neovim ~/.local/share/nvim/site/pack/klang/start/klang-neovim
```

With lazy.nvim:

```lua
{
  dir = "/absolute/path/to/kLang/extensions/klang-neovim",
  ft = "klang",
}
```

With packer.nvim:

```lua
use {
  "/absolute/path/to/kLang/extensions/klang-neovim",
  ft = "klang",
}
```

Then open any `.klang` file. If a buffer was opened before the plugin loaded, run:

```vim
:set filetype=klang
```

## Snippets

The plugin exposes a small command for inserting common Klang shapes:

```vim
:KlangSnippet function
:KlangSnippet alias
:KlangSnippet namespace
:KlangSnippet main
:KlangSnippet match
```

These snippets are intentionally plain text so they work without a snippet engine.
