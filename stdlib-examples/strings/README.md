# strings stdlib example

This example is a comprehensive, assertion-backed tour of the `strings`
standard library module. It covers:

- length, emptiness, equality, comparison, concatenation, and cloning
- substring search, prefix/suffix checks, indexes, and overlapping counts
- joining, repeating, replacing, splitting, and slicing
- prefix/suffix trimming and empty-value defaults
- left/right padding, including multi-character fill strings
- conversion to `List[String]` and Unicode code-point behavior

The module uses Go-inspired names, but preserves the behavior implemented by
kLang. In particular, `strings.Count("aaaa", "aa")` counts overlapping matches
and returns `3`.

Run it from the repository root:

```sh
go run . run stdlib-examples/strings
```

