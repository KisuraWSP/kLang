# Coroutines

Models an order-fulfillment scheduler with three `Coroutine[String]` jobs:
order validation, inventory reservation, and shipment creation.

Klang coroutines are currently one-shot. The wrapped function runs on the first
`resume`, which returns `Some(value)`. Further resumes return `None`, allowing a
scheduler to detect that the job has completed.

## Files

- `app.klang` defines the jobs and coroutine scheduler.
- `first.klang` is the project entry file.
- `klang.project` defines the runnable workspace.

## Try It

From the repository root:

```sh
go run . check examples/coroutines
go run . run examples/coroutines
```

Expected program output:

```text
ORDER FULFILLMENT COROUTINES
----------------------------
Starting: validate order
Completed: Order ORD-2048 validated
Starting: reserve inventory
Completed: 3 items reserved in warehouse A
Starting: create shipment
Completed: Shipment SHP-901 scheduled for pickup

Jobs completed: 3
All coroutines exhausted: True
```
