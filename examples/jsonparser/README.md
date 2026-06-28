# JSON Parser

Reads a realistic order from `order.json`, safely parses it into Klang's
immutable `JSON` type, extracts typed values, iterates over the order items,
and prints a calculated order summary.

## Files

- `order.json` contains the input data.
- `app.klang` reads, parses, and reports the order.
- `first.klang` is the project entry file.
- `klang.project` defines the runnable workspace.

## Try It

From the repository root:

```sh
go run . check examples/jsonparser
go run . run examples/jsonparser
```

Expected program output:

```text
Reading examples/jsonparser/order.json
ORDER SUMMARY
-------------
Order: ORD-2026-1042
Customer: Maya Chen (maya.chen@example.com)
Status: processing

Items:
- 1 x Mechanical Keyboard @ USD 129.99 = USD 129.99
- 2 x USB-C Cable @ USD 14.5 = USD 29
- 1 x Laptop Stand @ USD 48 = USD 48

Units: 4
Subtotal: USD 206.99
Ship to: Seattle, USA
```
