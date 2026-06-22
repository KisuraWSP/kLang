# loops and large calculations

This project demonstrates several loop forms while doing more than trivial
counter increments:

- a `range` loop sums 100,001 squares and checks the result against the closed
  formula
- a `while` loop calculates a modular polynomial checksum over 250,000 values
- nested loops calculate an energy value for a 180 by 180 numerical grid
- nested `for`/`while` iteration searches for the longest Collatz sequence
  below 5,000
- a fixed-count loop approximates the square root of two with Newton's method
- a `while` loop traverses the numeric interval from 0 to 100 billion using
  ten-billion-value checkpoints

Run the safe demonstration from the repository root:

```sh
go run . run stdlib-examples/loops
```

The source also contains `PrintEveryValue`, which literally prints every
integer from zero through a requested inclusive limit. It is opt-in so the
normal example remains useful:

```sh
go run . run stdlib-examples/loops print-all 1000
```

You *could* pass `100000000000`, but that would produce 100,000,000,001 lines
and likely more than a terabyte of text. The default checkpoint demonstration
reaches the same upper bound in eleven prints and separately calculates the
approximate sum of the complete range using `Float`.

All integer stress calculations deliberately use bounds or modular arithmetic
that keep intermediate values inside kLang's 64-bit `Int` range.
