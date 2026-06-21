# types + test stdlib example

This example combines the builtin `types` namespace with the `test` standard
library module. It uses silent assertions to verify the program while printing
only the actual values and metadata produced at runtime.

It covers:

- signed and unsigned child-width aliases
- direct `Parent.child(bits)` types and `.sizeof`
- float and complex aliases
- widening into larger child and parent numeric types
- user type aliases layered over `types` aliases
- parent protocol behavior such as `.times`
- runtime `Type` metadata
- every non-terminal `test` helper: `assert_true`, `assert_false`, `equal`,
  `not_equal`, `some`, `none`, `ok`, and `err`
- a typed `IntentionalFailure()` demonstration for `test.fail` that is not
  invoked during the successful run

Run it from the repository root:

```sh
go run . run stdlib-examples/types-test
```

The final `return 0` means every silent assertion passed.

`test.fail` always terminates the current run. Call `IntentionalFailure()` from
`Main` when you specifically want to see its failure diagnostic.
