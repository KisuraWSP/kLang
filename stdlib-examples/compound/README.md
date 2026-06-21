# compound stdlib example

This example treats the standard library as a connected toolkit rather than a
collection of isolated functions. A JSON project description moves through a
small build-planning pipeline:

- `json` parses a here string, safely extracts typed values, validates input,
  serializes the final report, and verifies the serialized result
- `strings` normalizes project and task names, joins module/file lists, and
  formats the ordered status output
- `list` appends parsed values and demonstrates `first`, `last`, `contains`,
  `index`, `map`, `filter`, `fold`, `sort`, and persistent snapshots
- `dsa` uses a Queue for pending tasks, a Stack for completion order, and an
  OrderedMap for deterministic task status indexing
- `metasystem` creates and validates a WorkSpace, prepares a build plan,
  processes tool messages, and produces a project AST descriptor
- `Parsable` treats generated Klang source as validated data: it exposes AST
  rows and argument channels, creates a workspace consumable by `metasystem`,
  supports immutable reparsed edits, and rejects an invalid source transform
- a trait-restricted `source_note` keyword macro adds contextual syntax without
  changing the language lexer or the Go runtime

The less obvious part is that normal stdlib tools can process compiler-facing
metadata. The example maps Parsable AST rows with `list`, puts their labels in a
`dsa.Queue`, describes the generated source through `metasystem`, and embeds the
result in the final JSON report. The original source remains unchanged after a
successful rewrite, while malformed appended source is returned as `Err`.

Run it from the repository root:

```sh
go run . run stdlib-examples/compound
```

The assertions are silent. The printed output contains the transformed project
data, queue/stack transitions, workspace details, and final JSON report. A
return value of `0` means the complete pipeline passed.
