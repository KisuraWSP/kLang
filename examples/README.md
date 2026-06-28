# Klang Examples

These projects are small, focused Klang programs. Each folder is a complete workspace with a `first.klang` entry file.

## Common Commands

Check every example:

```sh
go run . test examples
```

Run one example:

```sh
go run . run examples/helloworld
```

Package one example for the browser WASM runtime:

```sh
go run . package examples/helloworld --backend=WASM
```

Serve one example through the built-in browser runtime server:

```sh
go run . serve examples/helloworld --port=8080
```

## Projects

| Example | Purpose |
| --- | --- |
| [Audio Player](./audioplayer/) | Models a small audio-player style workflow with typed state and playback controls. |
| [Binary Search Tree](./binarysearchtree/) | Builds and queries a recursive package-ID tree with sorted traversal and duplicate handling. |
| [Command Line Arena](./commandlinearena/) | Runs a multi-file turn-based duel with fighter state, damage rules, power surges, and a winner. |
| [Coroutines](./coroutines/) | Schedules one-shot order-fulfillment jobs with `resume` and Option-based completion checks. |
| [CSV File Analyzer](./csvfileanalyzer/) | Parses quoted sales CSV files into typed records with validation, grouped analytics, CLI files, and JSON output. |
| [FizzBuzz](./fizzbuzz/) | Classic FizzBuzz control-flow example using loops, conditionals, and printing. |
| [Functions](./functions/) | Collects function examples, including typed parameters, return values, and reusable helpers. |
| [Game of Life](./gameoflife/) | Runs a small cellular-automata style simulation over list/grid-like data. |
| [Hello World](./helloworld/) | Minimal project showing the standard first.klang entry point importing app.klang. |
| [Interactive Planner](./interactiveplanner/) | Interactive command-line planning example that uses input-oriented flows and typed records. |
| [JSON Parser](./jsonparser/) | Reads an order JSON file, safely parses typed fields, and prints a calculated order summary. |
| [Lambda Functions](./lambdafunctions/) | Shows lambda and first-class function usage in a compact project. |
| [Markdown to HTML Converter](./markdowntohtmlconvertor/) | Converts markdown-like text into HTML-like output using strings and table/list helpers. |
| [Matrix Multiplier](./matrixmultiplier/) | Demonstrates numeric loops and matrix multiplication style list processing. |
| [Restricted Generics](./restrict/) | Shows restricted generic syntax and strict checking of allowed generic types. |
| [Showcase](./showcase/) | Large multi-file showcase covering many current language features in one project. |
| [SIMD](./simd/) | Demonstrates SIMD values and vector-style numeric operations. |
| [Snake](./snake/) | Simple game-logic example modeling snake state and board updates. |
| [Static Page Server](./staticpageserver/) | Shows a static page server style project with an index.html asset beside Klang source. |
| [Stress Test](./stresstest/) | Stress-style example that imports stdlib helpers and exercises repeated runtime operations. |
| [Tetris](./tetris/) | Simple Tetris-style game-logic example with board and piece state. |
| [Threads](./threads/) | Demonstrates spawn, join, Thread[T], and atomic-style concurrency helpers. |
| [Toy Language](./toylang/) | Implements Tape+, a typed Brainfuck superset with optimized bytecode, enum dispatch, CLI input, JSON state, diagnostics, and tracing. |
| [Variables](./variables/) | Focused tour of variable declarations, inference, mutability, and value operations. |
| [Web App](./webapp/) | Web-app themed example that models frontend/runtime data flow in Klang source. |
| [Web Server](./webserver/) | Simple webserver-themed project that loads an index.html-style page. |
| [Xenith Shell](./xenithshell/) | Command-shell themed example using command input, branching, and typed command handling. |
