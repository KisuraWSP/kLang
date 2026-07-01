# kLang Language Feature Priorities

Status: cleaned planning document  
Review date: 2026-07-01  
Authority: `DATA-TYPES.md`, `LANGUAGE-SPEC.md`, and `SYNTAX.md`

## 1. Purpose

This document replaces the unfiltered cleanup list with a smaller, testable
roadmap. It distinguishes between:

- important work that should remain on the roadmap;
- ideas that should be merged into an existing subsystem;
- features that already exist and should no longer appear as TODO items;
- work that should be deferred until its prerequisites exist; and
- ideas that should be removed because they duplicate the language, weaken
  safety, or create permanent complexity without enough value.

This is a roadmap cleanup, not authorization to delete implemented language
features. An existing feature should be removed only after an evidence-based
audit, a deprecation period, migration guidance, and compatibility tests.

## 2. Decision Summary

| Proposal | Decision | Reason |
|---|---|---|
| Revisit stdlib modules as the language matures | Keep | The stdlib must converge on stable language protocols and error conventions. |
| `go_to` keyword | Remove | Unstructured jumps complicate scopes, cleanup, transactions, ownership, and control-flow analysis. |
| `foreign` keyword | Keep, redesign | A typed, capability-aware interop declaration is important, but must not be a generic escape hatch. |
| Remove unnecessary language features | Reframe | Use a formal feature audit and deprecation process; do not remove features based only on apparent usage. |
| `default_program.klang` controls the toolchain | Keep, staged | Make it the shared policy layer only after the typed driver ABI, trusted boot artifact, isolation, capabilities, rescue mode, and parity gates in `DEFAULT-PROGRAM-FIRST-BOOT-GAP-ANALYSIS.md` exist. |
| Add `Null` | Already implemented | `Null` exists in the interpreter, JS output, bytecode VM, JSON, Tables, and zero-value behavior. |
| Add bounds checking | Already implemented | List, String, VM, and region-backed indexing already perform checks. |
| Disable bounds checks | Remove from safe language | Keep checks mandatory. A future explicitly unsafe optimization may be considered only after proof and profiling. |
| Make every program multithreaded | Remove | It adds overhead and nondeterminism and cannot automatically discover safe parallelism. |
| Add a “run faster” flag | Reframe | Optimization levels and profiling are useful; a generic parallel flag is not. |
| Universal `.to_string()` override | Reframe as `Display` protocol | Formatting needs a coherent protocol rather than a magical method on every value. |
| Struct `.cast()` | Already implemented under `cast_as` | Do not add a duplicate spelling. Improve `cast_as` diagnostics and coverage instead. |
| Color for arbitrary `print` output | Keep as terminal styling API | Error diagnostics are already red. User output should use explicit styles, not implicit coloring. |
| Pretty-print special values | Keep | Important for usability, debugging, REPL output, and diagnostics. |
| Mascot in CLI help | Implemented as bounded CLI polish | Kibi appears only for interactive help, completed execution, and fatal errors; automation remains clean. |
| Whole-language speedup | Keep, make measurable | Build benchmarks and optimize measured bottlenecks through IR/runtime passes. |
| `delete` keyword for memory | Remove | It conflicts with value semantics and safe ownership. Use regions, allocators, `defer`, and future resource scopes. |
| CSV builtin type | Reframe as stdlib module | CSV is a format/library concern, not a universal runtime value like JSON. |
| Alias-function members | Already substantially implemented | Struct aliases already expose constructor fields, methods, traits, extensions, and lifecycle hooks. |
| Better error explanations and fixes | Keep, active | The structured diagnostic foundation exists; producers still need incremental migration and better spans. |
| `@backend(...)` annotation | Merge into backend capability/interop design | Availability, implementation selection, and permissions must be one coherent system. |
| Pass CLI arguments into `Args` | Already implemented | `Args` is an immutable builtin `List[String]` populated from program arguments. |
| Understand and execute JavaScript | Keep, existing roadmap | Use `JS-INTEROP-AND-PORTABLE-VM-ROADMAP.md` as the authoritative plan. |
| Directory-based modules | Already implemented | Directory modules and folder projects have resolver and loader coverage. |
| Data-oriented language API | Reframe | Keep only concrete layout, iteration, SIMD, allocation, and profiling objectives. |
| `--new_std_lib` command | Remove | User code must not mutate the shipped stdlib. Use normal modules, packages, or plugins. |
| Hot/cold programs | Reframe as hot reload | “Cold program” has no useful language meaning; hot reload is valid development tooling. |
| VS Code LSP | Keep, high priority | Structured diagnostics and parser data now provide a viable foundation. |
| Always run the shipped `default_program.klang` first | Adopt after readiness gates | The normal toolchain may boot a verified embedded driver first; it must never trust a same-named current-directory file and must retain a host-kernel rescue path. |
| Default program as first workspace | Keep, isolated | Run it in a trusted boot workspace separated from user globals, imports, mutable state, handles, and capabilities. |
| Expose call-site information | Keep | Useful for diagnostics, logging, macros, testing, and tracing when immutable and source-aware. |
| Function aliases behave like callbacks | Remove | Function aliases are constructor/type declarations; callbacks are represented by `Function[...]` and lambdas. |
| Advent of Code examples | Keep, low priority | Good conformance and performance examples after core behavior stabilizes. |
| Richer pattern destructuring | Keep | Natural extension of existing destructuring and pattern matching. |
| Compile-time evaluation/macros | Keep, constrained | Existing `Parsable` keyword macros should be strengthened without becoming unrestricted compile-time execution. |
| Cancellation/context support | Keep | Required for robust async, channels, host operations, and long-running work. |
| Channels/message passing | Keep | Provides safer high-level concurrency alongside `Atomic` and `transaction`. |
| Resource-safety syntax | Keep | Needed to make File/OS/FFI handles and allocators reliably releasable. |
| Million-line project under ten seconds | Keep as benchmark goal | Define hardware, warm/cold cache, project shape, memory ceiling, and correctness criteria. |
| Web runtime parity | Keep | Continue through the portable VM and host ABI roadmap. |
| Execute any code “without issue” | Replace with conformance goal | No implementation can guarantee arbitrary code succeeds; it can guarantee deterministic acceptance or diagnostics. |

## 3. Work That Is Already Complete

These items should be removed from active TODO lists. Future work may improve
them, but should be written as a specific deficiency rather than “add feature.”

### 3.1 Null and null safety

Current foundation:

- `Null` is a runtime and bytecode value.
- JSON distinguishes JSON null.
- Lists and Tables can hold `Null`.
- `Option[T]` remains the preferred typed absence mechanism.
- The checker has null/Option safety analysis.

Do not add a second null spelling or make every type implicitly nullable.
Improvements should target diagnostics, backend parity, or a documented
`Null`/`None`/JavaScript `undefined` conversion policy.

### 3.2 Bounds checking

Current foundation:

- List and String reads are checked.
- Bytecode indexing is checked.
- Region-backed values enforce capacity.
- JS-generated collection access preserves kLang checks.

Bounds checks remain part of safe kLang semantics. Optimizers may eliminate a
check only when an analysis proves it redundant. The source language should not
offer a general “turn safety off” flag.

### 3.3 Program arguments

`Args` already receives user arguments as an immutable `List[String]`. Remaining
work belongs to CLI parsing libraries:

- named options;
- short and long flags;
- typed conversion;
- help generation;
- validation errors; and
- separation of toolchain flags from program arguments.

This should be a stdlib `args` facade built on the existing builtin value.

### 3.4 Structural casts

Struct aliases already support:

```klang
local Table fields = user.cast_as(Table);
local JSON document = user.cast_as(JSON);
local String encoded = user.cast_as(String);
local PublicUser view = user.cast_as(PublicUser);
```

Do not add `.cast()` as a synonym. Improve `cast_as` with precise field labels,
missing-field suggestions, generic substitutions, and backend conformance.

### 3.5 Directory modules

Directory module resolution and folder projects already have implementation and
tests. Remaining work should be scoped to package identity, versioning,
visibility, cyclic import diagnostics, or dependency distribution.

### 3.6 Alias-function members

Struct-style alias functions already provide constructor fields through `this`,
methods, standalone extensions, traits, implementations, JSON tags, operators,
and lifecycle hooks. The proposed `let member test = this` syntax would overlap
with this model and should not be added.

If a concrete member feature is still missing, describe the required visibility,
mutability, initialization, and layout behavior as an extension to the existing
struct-alias model.

## 4. Removed Ideas

### 4.1 Do not add `go_to`

`go_to` would allow execution to bypass:

- lexical variable initialization;
- `defer` and future resource cleanup;
- transaction boundaries;
- loop invariants;
- ownership and borrow rules;
- compiler control-flow assumptions; and
- structured stack traces.

If users need to leave nested control flow, add labelled `break` and `continue`
instead:

```klang
search: while condition {
    for item in items {
        if Found(item) {
            break search;
        }
    }
}
```

This retains a structured control-flow graph and has a clear cleanup boundary.

### 4.2 Do not make every program implicitly multithreaded

Parallel execution is not automatically faster. Small programs become slower,
output ordering becomes nondeterministic, mutable state becomes harder to
reason about, and debugging becomes less reproducible.

kLang should instead provide:

- explicit `spawn`/`join` for independent CPU work;
- `Channel[T]` for ownership-oriented communication;
- `Atomic[T]` for individual shared cells;
- `transaction` for multi-cell atomic state changes;
- async/await for lazy cooperative work; and
- parallel compiler and loader internals where determinism is preserved.

Optimization flags may choose compiler passes, but must not silently alter
program concurrency semantics.

### 4.3 Do not add a general bounds-check disabling feature

Unchecked indexing would undermine the safety contract across Standalone, JS,
WASM, and future FFI. Use range analysis to remove proven checks internally.

A future `unsafe` capability may expose narrowly scoped operations only if:

- unsafe code is visually explicit;
- safe callers cannot inherit unsafety accidentally;
- the operation is unavailable in sandboxed backends where necessary;
- diagnostics identify the violated proof obligation; and
- benchmarks prove the operation is worth its permanent complexity.

### 4.4 Do not add `delete` for ordinary memory

Values may be copied, shared through copy-on-write storage, captured, stored in
containers, or managed by an allocator. A user-directed `delete value` cannot
reliably prove that no aliases remain.

Use:

- lexical lifetime and garbage collection for ordinary values;
- regions and arenas for bulk lifetime;
- `defer` for cleanup calls;
- explicit collection operations such as `table_delete`; and
- a future resource scope for external handles.

### 4.5 Do not make CSV a builtin runtime type

JSON is a general tree value used throughout metaprogramming and interop. CSV is
a tabular serialization format with dialect choices: separator, quoting,
headers, comments, newlines, encoding, and type inference.

Implement CSV as `stdlib/csv.klang` plus a native streaming parser only if
profiling requires it. Its public API should return ordinary typed values:

```klang
local Result[List[List[String]], Atom] rows = csv.Parse(source);
local Result[List[Table], Atom] records = csv.ParseRecords(source);
```

Here strings remain an excellent way to provide inline CSV source.

### 4.6 Do not let a command mutate the shipped stdlib

Remove the proposed `--new_std_lib` flag. It would create non-reproducible local
toolchains and namespace/security problems.

User extensions should use:

- project-local modules;
- a future package manager;
- explicitly installed Codex/toolchain plugins; or
- standalone extension declarations owned by the project.

### 4.7 Do not boot an unverified project-local driver

The normal toolchain may eventually boot the shipped, versioned
`default_program.klang` driver before dispatching a command. That is toolchain
policy execution, not an implicit prelude added to every user program.

The boot design must:

- load a verified embedded artifact rather than `./default_program.klang`;
- isolate the trusted driver workspace from the user workspace;
- grant only declared host capabilities;
- preserve deterministic caching and reproducible builds;
- report driver and user frames distinctly;
- retain a minimal `--no-driver` rescue kernel; and
- leave the ordinary user `Main() : Int` ABI unchanged.

The complete prerequisites and migration gates are defined in
`DEFAULT-PROGRAM-FIRST-BOOT-GAP-ANALYSIS.md`.

### 4.8 Keep callbacks and alias functions separate

An alias function constructs or describes a type. A callback is a value of
`Function[...Args, Return]`, including a named function or lambda. Making alias
functions act “more like callbacks” would blur type construction and invocation.

Improve callback ergonomics through inference, traits, effect metadata, and
closure diagnostics instead.

## 5. High-Priority Work

Each item below is designed to be implementable one prompt at a time.

### P1 — Finish structured diagnostics

Why:

The shared diagnostic model now exists, including stable codes, severity, spans,
labels, fixes, expected/found types, red console output, and structured runtime
frames. Many older producers still create message-only diagnostics.

Scope:

1. Give lexer tokens and all AST nodes complete UTF-8-aware source spans.
2. Replace remaining message parsing with producer-owned codes and fields.
3. Add secondary labels for declaration/use and expected/found locations.
4. Add explicit `ErrorType` propagation to suppress cascaded diagnostics.
5. Add `--diagnostic-format=json` for editors and CI.
6. Add golden tests for parser, type, module, runtime, JS, and WASM output.
7. Keep Atom propagation separate from rich diagnostic metadata.

Done when:

- diagnostic rule, code, span, and hint never depend on parsing prose;
- one root mistake does not produce a large cascade;
- CLI and JSON renderings come from the same structured value; and
- editor consumers receive stable machine-readable fields.

Suggested first prompt:

> Add complete source spans to lexer tokens and parser expressions/statements,
> preserving compatibility with Position. Migrate unknown-name diagnostics to
> use those spans without searching source text.

### P2 — Backend capabilities and typed `foreign`

Why:

FFI, JavaScript, OS access, backend-specific stdlib code, and WASM restrictions
all need one model. Adding unrelated `foreign` and `@backend` mechanisms would
create contradictory rules.

Proposed direction:

```klang
@target("JS")
foreign function console_log(value : String) : Null;

@target("Standalone")
@requires("process.spawn")
foreign function run_process(command : String) : Result[Table, Atom];
```

Rules:

- `foreign` declares a host-provided function with no kLang body.
- Every parameter and return value must have a defined ABI conversion.
- `@target` controls availability, not runtime branching.
- `@requires` declares capabilities such as `fs.read`, `process.spawn`, or
  `js.execute`.
- Calls are rejected during checking when the selected backend cannot provide
  the declaration.
- Foreign calls must declare retry safety, thread transfer, blocking behavior,
  and possible Atom failures.
- Arbitrary Go symbol access is never exposed.

Dependencies:

- backend capability descriptors;
- project/CLI capability declarations;
- stable host ABI;
- opaque handle ownership rules; and
- structured backend diagnostics.

Done when:

- unsupported foreign calls fail before execution;
- manifests cannot silently grant capabilities;
- the same declaration is checked consistently by every backend; and
- host errors become stable Atom results plus structured diagnostics.

Suggested first prompt:

> Design an ADR for `foreign`, `@target`, and `@requires`. Reconcile it with
> the existing backend interface and JS interop roadmap. Do not modify the
> parser until the ABI, capability, and effect rules are approved.

### P3 — Display, pretty printing, and terminal styles

Why:

User-defined values need readable output without conflating debug formatting,
serialization, and terminal control.

Design:

- `Display` produces user-facing text.
- `Debug` produces unambiguous developer text.
- `String` casts remain deterministic conversion/serialization operations.
- `JSON` serialization remains independent from display.
- `print` uses `Display` when implemented and a safe builtin fallback otherwise.
- `debug` and diagnostics use `Debug`.
- recursive values have depth, item-count, and cycle limits.

Illustrative surface:

```klang
trait Display {
    function display() : String;
}

impl Display for User {
    function display() : String {
        return this.name + " (#" + this.id as String + ")";
    }
}
```

Terminal color belongs in a library:

```klang
print(terminal.red("failed"));
print(terminal.bold(user));
```

The library must:

- honor `NO_COLOR`;
- detect non-interactive output;
- avoid corrupting redirected files;
- support nested styles safely; and
- separate ANSI styling from the underlying value.

Done when:

- user output, debug output, JSON, and casts have distinct contracts;
- alias structs can implement display without changing equality or storage;
- Lists, Maps, Sets, Tables, Results, and nested structs pretty-print with
  deterministic limits; and
- terminal styling is explicit and portable.

Suggested first prompt:

> Design and implement the `Display` protocol for alias structs and builtin
> fallback values. Keep JSON and cast semantics unchanged. Add recursion and
> cycle limits plus cross-backend tests.

### P4 — Channels and structured concurrency

Why:

`spawn`, `join`, `Atomic`, and `transaction` cover low-level parallelism. A
message-passing layer provides ownership-oriented coordination without requiring
shared mutation.

Proposed types:

```klang
Channel[T]
Sender[T]
Receiver[T]
```

Required semantics:

- typed values use the existing thread-transfer rules;
- channels may be bounded or rendezvous-based;
- send/receive define blocking behavior;
- closing is idempotent or produces a precise error by specification;
- receive after close returns an explicit `Option[T]` or `Result[T,Atom]`;
- sender/receiver handles have documented thread-transfer behavior;
- blocked operations can be cancelled;
- no implicit global scheduler is introduced; and
- WASM/browser support is capability-reported rather than silently emulated.

Build this in separate prompts:

1. channel type and transfer contract;
2. bounded native runtime implementation;
3. close and error semantics;
4. cancellation-aware send/receive;
5. selection over multiple operations, only if justified;
6. backend capability reporting.

Done when:

- producer/consumer programs require no shared mutable globals;
- blocked goroutines cannot leak after cancellation;
- race tests and deterministic close tests pass; and
- unsupported backends report a source-located diagnostic.

Suggested first prompt:

> Write the Channel[T] semantic ADR, including ownership, transfer, buffering,
> close, blocking, cancellation, and backend rules. Do not implement `select`
> in the first version.

### P5 — Cancellation and execution context

Why:

Threads, channels, async work, subprocesses, network operations, and hot reload
need cooperative cancellation and deadlines.

Do not reuse the existing compiler/runtime metadata `Context` name. Prefer
`CancelContext`, `TaskContext`, or another unambiguous type.

Required operations:

```klang
local TaskContext root = task_context();
local TaskContext timed = root.with_timeout(5_000);
timed.cancel();
local Bool stopped = timed.cancelled;
```

Rules:

- cancellation is monotonic and thread-safe;
- children observe parent cancellation;
- deadlines use a monotonic host clock;
- cancellation is not an exception by default;
- blocking operations return `:cancelled` or `:timeout`;
- cancellation handlers cannot run more than once;
- transaction bodies cannot cancel external work because they may retry; and
- context handles have explicit transfer and lifetime rules.

Done when:

- channel, thread join, and supported host calls can terminate cooperatively;
- timeout tests do not depend on fragile wall-clock timing;
- cancellation does not leak workers; and
- Atom results remain stable across hosts.

Suggested first prompt:

> Design TaskContext cancellation semantics and implement a deterministic
> in-memory runtime model with parent/child cancellation tests. Do not connect
> it to OS operations in the first prompt.

### P6 — Resource safety

Why:

File descriptors, subprocesses, JS handles, sockets, and future FFI resources
need deterministic cleanup. Raw `delete` is not safe.

Proposed direction:

```klang
using local FileHandle handle = file.open("data.txt")! {
    local String content = handle.read_all()!;
}
```

Semantics:

- the initializer must produce a type implementing `Disposable` or `Close`;
- cleanup runs on normal completion, return, break, continue, and Atom throw;
- cleanup order is reverse declaration order;
- cleanup behavior during panic/runtime failure is specified;
- values cannot escape the resource scope unless the type explicitly supports
  ownership transfer;
- `defer` remains available for general actions;
- region and arena cleanup can use the same protocol; and
- transaction bodies cannot acquire external resources.

Prerequisites:

- effect/escape analysis for resource values;
- structured control-flow cleanup lowering;
- backend-specific cleanup support; and
- rich diagnostics with declaration and escape labels.

Done when:

- every exit path runs cleanup exactly once;
- use-after-close is rejected or diagnosed;
- resource handles cannot cross unsupported thread/host boundaries; and
- generated JS and interpreter behavior agree.

Suggested first prompt:

> Write the resource-scope ADR. Compare `using`, existing `defer`, and allocator
> hooks, then specify cleanup and ownership transfer for every control-flow exit.

### P7 — Performance engineering and optimization passes

Why:

“Make everything faster” is not actionable. Optimization must preserve language
semantics and target measured costs.

Benchmark suites:

- lexer/parser/type checking on cold and warm caches;
- module resolution on many-file projects;
- interpreter dispatch;
- bytecode VM dispatch;
- function calls and tail calls;
- List/Map/Table allocation and mutation;
- iterator pipelines;
- Atomic/transaction contention;
- generated JS startup and throughput;
- WASM package size and execution; and
- memory usage and allocation count.

Optimization pipeline:

1. Record reproducible baselines and hardware metadata.
2. Profile before every optimization.
3. Add backend-neutral typed IR passes:
   - constant folding;
   - dead branch elimination;
   - unreachable code removal;
   - copy elision;
   - redundant bounds-check elimination;
   - simple inlining;
   - tail-call marking; and
   - fused iterator lowering.
4. Add runtime improvements:
   - explicit VM frames;
   - compact bytecode;
   - interned symbols;
   - allocation reuse;
   - incremental program cache; and
   - deterministic parallel frontend work.
5. Require semantic differential tests for every pass.

Optimization levels may eventually be:

- `-O0`: easiest debugging;
- `-O1`: safe low-cost transformations;
- `-O2`: normal release optimization; and
- `-Os`: output-size oriented.

They must never change bounds safety, integer behavior, output ordering, race
rules, or exception semantics.

Done when:

- benchmark results are stored and comparable;
- each pass can be independently enabled for testing;
- differential tests prove optimized and unoptimized results agree; and
- performance targets include a memory ceiling and correctness checks.

Suggested first prompt:

> Add a reproducible benchmark harness for frontend, interpreter, VM, and
> collections. Record baselines without implementing optimizations.

### P8 — Language Server Protocol

Why:

An LSP delivers immediate value: diagnostics, navigation, completion, hover,
rename, formatting, and code actions. The new structured diagnostic model is
designed for this.

Delivery order:

1. stdio JSON-RPC server and document synchronization;
2. publish parse/type diagnostics;
3. hover using resolved type/runtime metadata;
4. go-to-definition for variables, functions, aliases, types, and modules;
5. document/workspace symbols;
6. completion from lexical scope and imports;
7. formatting;
8. references and safe rename;
9. code actions from diagnostic fixes;
10. VS Code client packaging.

Architecture:

- reuse parser/type checker libraries rather than shelling out to the CLI;
- use incremental document snapshots;
- cancel stale checks;
- never mutate source during analysis;
- preserve stable diagnostic codes;
- map UTF-8 source columns correctly to LSP UTF-16 positions; and
- keep the server editor-neutral.

Done when:

- VS Code is only one thin client;
- diagnostics update after edits without saving;
- definition/rename respect namespaces and modules; and
- stale analysis cannot overwrite newer results.

Suggested first prompt:

> Implement a minimal editor-neutral kLang LSP server over stdio with initialize,
> shutdown, document open/change/close, and parse diagnostics only.

## 6. Medium-Priority Work

### P9 — Richer destructuring

Extend existing destructuring in this order:

1. function parameters;
2. `for_each` bindings;
3. match-case nested records;
4. rest patterns for Lists;
5. renamed/defaulted object fields;
6. refutable versus irrefutable pattern diagnostics.

Rules:

- declarations and parameter patterns must be irrefutable;
- match cases may be refutable;
- each source expression is evaluated once;
- bindings inherit immutability unless explicitly marked;
- duplicate names are rejected;
- missing fields and wrong item types receive labelled diagnostics; and
- lowering preserves source spans.

Suggested first prompt:

> Add destructuring function parameters using the existing pattern AST and
> lowering. Require irrefutable patterns and single evaluation.

### P10 — Constrained compile-time evaluation and macros

Existing `Parsable` keyword macros already inspect context and return parsed
source expansions. Improve this system rather than adding a parallel macro
language.

Required controls:

- deterministic evaluation;
- no filesystem, environment, network, clock, random, threads, or host process
  access unless explicitly declared and sandboxed;
- instruction, recursion, memory, and expansion-depth limits;
- hygienic generated names or an explicit unhygienic escape;
- source maps linking generated code to invocation and definition;
- cache keys containing macro implementation and inputs;
- cycle detection;
- typed expansion contracts; and
- stable diagnostics for generated source.

Useful first applications:

- constant validation;
- repetitive declarations;
- typed DSL generation;
- specialized function generation; and
- compile-time metadata tables.

Suggested first prompt:

> Add expansion-depth, instruction, and memory limits to existing Parsable
> keyword macros, with source-related diagnostics for generated code.

### P11 — Immutable call-site metadata

Expose call-site information through a builtin immutable value without allowing
stack mutation:

```klang
CallSite {
    function : String,
    module : String,
    file : String,
    line : Int,
    column : Int
}
```

Uses:

- logging;
- assertions and test helpers;
- macro invocation context;
- tracing;
- captured exception reports; and
- deprecation diagnostics.

Rules:

- compiler optimizations preserve logical kLang call sites;
- paths can be redacted or packaged-portable;
- source locations use the shared diagnostic span model;
- JS source maps and WASM source tables map back to kLang;
- retrieving a call site is read-only and does not expose mutable frames; and
- production stripping, if supported, is explicit.

Suggested first prompt:

> Add a CallSite builtin value backed by the runtime's structured call frames.
> Expose `call_site()` for the current caller and preserve packaged source paths.

### P12 — Hot reload as development tooling

Hot reload should be explicit:

```text
kLang run . --watch
```

It is not a new program category. Delivery:

1. watch resolved source/module files;
2. debounce changes;
3. parse and type-check a new workspace snapshot;
4. retain the previous running version if checking fails;
5. restart the process safely;
6. later support state transfer only through an explicit versioned protocol.

Do not patch function bodies in place initially. Full restart is simpler,
predictable, and compatible with resource cleanup.

Done when:

- only resolved files trigger reload;
- invalid edits show diagnostics without killing the last valid build;
- workers and resources from the previous run are terminated; and
- watch behavior is deterministic and testable.

Suggested first prompt:

> Add `kLang run --watch` using full-process restart semantics. Watch resolved
> source files, debounce edits, and retain the last valid program on check errors.

### P13 — Data-oriented APIs with measurable semantics

Do not use “data oriented” as a blanket design instruction. Target concrete
capabilities:

- contiguous typed region arrays;
- structure-of-arrays helpers;
- stable memory-layout metadata;
- SIMD-friendly iteration;
- allocation-free iterator terminals where possible;
- explicit alignment;
- bulk copy/transform operations; and
- profiler-visible allocation and cache behavior.

Every API must define:

- layout;
- ownership;
- mutability;
- bounds;
- alignment;
- thread transfer;
- backend support; and
- fallback semantics.

Suggested first prompt:

> Audit region arrays and SIMD for layout, bounds, alignment, allocation, and
> backend parity. Produce benchmarks and an ADR before adding new syntax.

### P14 — Standard-library convergence audit

Why:

The stdlib should express reusable policy while Go runtime builtins provide only
the host primitives that cannot reasonably be written in kLang. Revisiting it
without an audit would repeatedly rename or rewrite modules.

Audit every implemented module except intentionally unavailable Raylib and
`js-wasm` work:

1. Record public functions, receiver methods, generic constraints, and effects.
2. Verify every fallible facade returns `Result[...,Atom]`.
3. Identify duplicated helpers and choose one canonical spelling.
4. Preserve old spellings through explicit deprecation wrappers.
5. Replace eager collection helpers with fused iterators where signatures allow.
6. Record backend availability through the future capability model.
7. Separate pure kLang policy from native File/OS/JSON/crypto primitives.
8. Add one focused test file and runnable example per public module.
9. Ensure formatter, checker, documentation generator, and package backends can
   process the complete stdlib.
10. Publish a compatibility table rather than relying on source inspection.

Do not:

- expose Go implementation details as public APIs;
- add native builtins merely to solve one example or puzzle;
- mutate the installed stdlib from user commands; or
- revive Raylib/JS-WASM wrappers before their interop foundations exist.

Done when:

- public naming and error behavior are consistent;
- deprecated APIs identify their replacement;
- every module has deterministic tests;
- backend restrictions are explicit; and
- the stdlib can serve as a conformance workload for the language.

Suggested first prompt:

> Generate a stdlib API and effect inventory, excluding Raylib and js-wasm.
> Classify duplicate, deprecated, pure, host-native, fallible, and
> backend-restricted functions without changing public behavior.

## 7. Deferred Long-Term Work

### P15 — JavaScript execution and portable VM

This remains important, but its detailed authoritative plan is already in
`JS-INTEROP-AND-PORTABLE-VM-ROADMAP.md`. Do not duplicate that roadmap here.

Immediate next action:

> Implement P0.2 from `JS-INTEROP-AND-PORTABLE-VM-ROADMAP.md`: approve the JS
> interop syntax ADR before parser work.

The `foreign` and capability work in P2 must remain compatible with that plan.

### P16 — Default-program first boot

Long-term goal:

kLang should express shared command policy through its shipped
`default_program.klang`. Native, Node, and browser/WASM launchers remain small
host kernels and boot the same verified driver for normal toolchain
invocations.

Required prerequisites:

- a versioned typed driver protocol;
- an embedded, verified boot artifact;
- a reusable toolchain service extracted from root `main.go`;
- isolated driver and user workspaces;
- capabilities and sandboxing;
- structured diagnostics and events;
- cancellation, limits, and resource cleanup;
- deterministic caching and recovery;
- a portable VM value and host-call model; and
- cross-host parity tests.

Rules:

- never resolve the privileged driver from the current directory;
- driver permissions are explicit, minimal, and reviewable;
- driver failure cannot corrupt the Go implementation or stdlib;
- the host kernel provides a `--no-driver` rescue path;
- workspaces remain isolated unless a typed API connects them; and
- ordinary `Main()` programs remain unaffected and do not recursively boot the
  driver.

This is not full self-hosting. Full self-hosting would require the compiler,
runtime, package system, and bootstrap process to be specified independently.

The authoritative implementation sequence, DP0 through DP24, is in
`DEFAULT-PROGRAM-FIRST-BOOT-GAP-ANALYSIS.md`.

Suggested first prompt:

> Implement DP0 from `DEFAULT-PROGRAM-FIRST-BOOT-GAP-ANALYSIS.md`. Freeze the
> current CLI behavior in machine-readable parity fixtures before changing
> startup.

### P17 — Examples and conformance corpus

Advent of Code examples are useful after correctness and performance stabilize.
Treat them as executable conformance programs:

- include representative parsing, graph, dynamic programming, and simulation
  problems;
- avoid puzzle-specific native builtins;
- record Standalone/JS/WASM support;
- assert known answers without publishing private puzzle inputs;
- benchmark parsing and execution separately; and
- use examples to reveal missing generic collection APIs.

CSV examples should use the future stdlib module, not a builtin type.

## 8. Revised Milestone

Target date: 2027-01-01

“Powerful enough to understand user code” is replaced by measurable exit
criteria:

- the core feature/backend matrix is test-enforced;
- no known broken loop/function/struct/type behavior lacks a regression test;
- all unsupported backend features produce source-located diagnostics;
- the LSP publishes live parser and type diagnostics;
- the runtime passes race tests for threads, Atomic, transactions, and channels
  implemented by that date;
- formatter and checker are stable on the full stdlib;
- cold and warm performance baselines are published with hardware metadata;
- a substantial multi-file conformance project checks and starts within the
  agreed performance budget;
- the browser runtime executes its declared supported subset without semantic
  drift; and
- crashes or host panics become structured diagnostics rather than corrupting
  the process.

The million-line/ten-second target remains an aspirational benchmark until a
representative generated project, hardware profile, cache state, and memory
limit are defined.

## 9. Recommended Prompt Order

Use one prompt per item:

1. P1 complete source spans.
2. P1 diagnostic producer migration and cascade suppression.
3. P2 `foreign`/target/capability ADR.
4. P3 `Display` protocol.
5. P14 stdlib API/effect inventory.
6. P4 Channel semantic ADR.
7. P5 cancellation core.
8. P6 resource-scope ADR.
9. P7 benchmark harness.
10. P8 minimal LSP.
11. P9 destructuring parameters.
12. P10 macro limits.
13. P11 CallSite.
14. P12 restart-based watch mode.
15. Continue the JS/portable VM roadmap.

This order deliberately builds correctness and observability before adding more
runtime power.
