# Binary Search Tree

Builds a real binary search tree for warehouse package IDs. The example
implements recursive insertion, lookup, in-order traversal, node counting,
height calculation, and minimum/maximum lookup. Duplicate IDs are ignored.

## Files

- `app.klang` contains the tree algorithms and warehouse example.
- `first.klang` is the project entry file.
- `klang.project` defines the runnable workspace.

## Try It

From the repository root:

```sh
go run . check examples/binarysearchtree
go run . run examples/binarysearchtree
```

Expected program output:

```text
WAREHOUSE PACKAGE INDEX
-----------------------
Inserted IDs: [42, 18, 68, 7, 27, 55, 73, 20, 30, 65, 27]
Sorted IDs: [7, 18, 20, 27, 30, 42, 55, 65, 68, 73]
Unique packages: 10
Tree height: 4
ID range: 7 to 73
Contains 27: True
Contains 99: False
```
