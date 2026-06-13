# Klang Vim Plugin

Vim and Neovim language support for `.klang` files.

## Features

- Filetype detection for `*.klang`.
- Syntax highlighting for declarations, builtin types, comments, strings, here strings, hooks, directives, traits, aliases, pattern matching, async/coroutines, atomics, ownership keywords, and operators.
- Basic indentation for `{ ... }`, `do ... end`, `case`, `try/catch`, namespaces, functions, traits, impls, and alias functions.
- Filetype settings for `--` comments and common Klang buffers.
- Insert-mode template mappings for common declarations.

## Local Installation

With Vim packages:

```sh
mkdir -p ~/.vim/pack/klang/start
cp -R extensions/klang-vim ~/.vim/pack/klang/start/klang-vim
```

With Neovim packages:

```sh
mkdir -p ~/.local/share/nvim/site/pack/klang/start
cp -R extensions/klang-vim ~/.local/share/nvim/site/pack/klang/start/klang-vim
```

Then open a `.klang` file. If detection does not run automatically, use:

```vim
:set filetype=klang
```

## Template Mappings

These mappings are available in insert mode:

- `<leader>kf` inserts a function template.
- `<leader>ka` inserts an alias function template.
- `<leader>ki` inserts an `if` block.
- `<leader>km` inserts a `Main` function.

