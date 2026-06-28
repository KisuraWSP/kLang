# Tape+ Toy Language

Tape+ is a small, file-backed superset of Brainfuck implemented entirely in
Klang. It includes a tokenizer, optimizing bytecode compiler, jump table,
virtual machine, runtime diagnostics, command-line arguments, JSON reporting,
and bundled example programs.

Classic Brainfuck programs remain valid. Tape+ adds comments, numeric output,
newlines, cell doubling, cell clearing, and VM tracing.

## Language Reference

| Command | Behavior |
| --- | --- |
| `>` | Move the tape pointer right, growing the tape when needed. |
| `<` | Move left. Moving before cell zero is a runtime error. |
| `+` | Increment the current 8-bit cell, wrapping after 255. |
| `-` | Decrement the current 8-bit cell, wrapping before zero. |
| `.` | Append the current cell as an ASCII character. |
| `,` | Read one ASCII character from the CLI input string; EOF stores zero. |
| `[` | Enter a loop while the current cell is nonzero. |
| `]` | Return to the matching `[` while the current cell is nonzero. |
| `0` | Tape+ extension: clear the current cell. |
| `:` | Tape+ extension: append the current cell as a decimal number. |
| `;` | Tape+ extension: append a newline. |
| `*` | Tape+ extension: double the current cell with 8-bit wrapping. |
| `?` | Tape+ extension: record the instruction, pointer, and cell in the trace. |
| `# ... #` | Tape+ extension: ignore a block comment. |

Characters outside commands and comments are ignored, matching Brainfuck's
traditional source behavior.

## Runtime Model

- Cells are integers from 0 through 255.
- The tape begins with one zeroed cell and grows to the right.
- Repeated `>`, `<`, `+`, and `-` commands are compressed into typed bytecode.
- Brackets are validated before execution and linked through a `Map[Int, Int]`
  bytecode jump table while diagnostics retain source-command offsets.
- Printable ASCII, tab, carriage return, and newline are accepted as input.
- Unsupported input characters become zero.
- Execution stops with an error after 100,000 steps.
- A successful run reports output, steps, allocated cells, final pointer, and
  optional trace records.

## Klang Features

The implementation deliberately uses the deeper parts of Klang rather than
representing every compiler value as a dynamic `Table`:

- `OpCode` is an exhaustive typed `enum` used by VM pattern matching.
- `Instruction`, `TapeProgram`, `VMConfig`, and `VMState` are struct-style
  alias functions with statically checked fields.
- Alias-owned methods expose operation counts, optimization statistics,
  configuration validation, and trace state.
- Compile-time type aliases define `command_stream`, `bytecode`, `jump_table`,
  and `memory_tape`.
- A standalone `#extend Char` method recognizes Tape+ commands through
  `Set[Char]`.
- The lexer consumes source through `Iterator[Char]` and `Option[Char]`.
- The compiler uses persistent `list.append`, `list.last`, and `list.pop`
  operations from the standard library.
- The CLI uses the `args` and `option` modules rather than indexing `Args`
  unsafely.
- File loading, compilation, and execution use typed `Result` values with
  exhaustive `Ok`/`Err` matching.
- Optional state output uses `json.serialize` and alias JSON field tags.

## Project Files

- `model.klang` defines opcodes, typed bytecode, VM state, and extensions.
- `lexer.klang` uses an iterator to filter commands and comments.
- `parser.klang` compresses commands, validates brackets, and links jumps.
- `vm.klang` executes typed opcodes through exhaustive enum matching.
- `app.klang` uses stdlib args/options, reads files, and renders reports.
- `first.klang` provides the required `Main() : Int` entry point.
- `klang.project` defines the six-source workspace.
- `programs/hello.tape` is a classic Brainfuck Hello World.
- `programs/features.tape` demonstrates every Tape+ output/state extension.
- `programs/echo.tape` reads one command-line input character.
- `programs/broken.tape` intentionally demonstrates bracket diagnostics.

## Command Line

Check the interpreter:

```sh
go run . check examples/toylang
```

Run the default Hello World program:

```sh
go run . run examples/toylang
```

Run a specific source file:

```sh
go run . run examples/toylang examples/toylang/programs/features.tape
```

The first program argument is the `.tape` path. The optional second argument is
the input consumed by `,`:

```sh
go run . run examples/toylang examples/toylang/programs/echo.tape K
```

That program prints the byte value and character:

```text
Program output:
75
K
Execution stats:
- steps: 4
- cells used: 1
- final pointer: 0
```

Add `json` to the arguments to serialize the final typed `VMState`. For a
program that consumes input, place it after the input argument:

```sh
go run . run examples/toylang examples/toylang/programs/echo.tape K json
```

For Hello World, the second argument is unused and can select JSON directly:

```sh
go run . run examples/toylang examples/toylang/programs/hello.tape json
```

```text
State JSON:
{"cells_used":7,"executed_steps":906,"final_pointer":6,"output":"Hello World!\n","trace":""}
```

Use `disassemble` to inspect optimized enum bytecode:

```sh
go run . run examples/toylang examples/toylang/programs/features.tape disassemble
```

The listing includes the bytecode index, `OpCode` name, repeat count, and
original source-command offset:

```text
Bytecode:
0 Increment x 5 @ source 0
1 EmitNumber x 1 @ source 5
2 EmitNewline x 1 @ source 6
3 Double x 1 @ source 7
```

Run the intentional compiler failure:

```sh
go run . run examples/toylang examples/toylang/programs/broken.tape
```

```text
Tape+ compile error: unmatched '[' at source command 3
```

## Default Output

```text
TAPE+ LANGUAGE
==============
Source: examples/toylang/programs/hello.tape
Source commands: 106
Bytecode instructions: 59
Operations optimized: 47

Program output:
Hello World!

Execution stats:
- steps: 906
- cells used: 7
- final pointer: 6
```
