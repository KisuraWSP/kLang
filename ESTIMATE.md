> These are just estimates to gauge in the future so we can make the language powerful enough for other users
```
Small but usable stable runtime:      70k-100k total LOC
Serious stable runtime:              100k-180k total LOC
Very powerful mature runtime:        180k-300k+ total LOC
```

```
Parser / AST / resolver / module system:       8k-15k
Type checker / generics / alias structs:       15k-30k
Borrow checker / lifetime analysis:            12k-30k
Interpreter runtime / values / execution:      20k-40k
Host operations / file IO / capabilities:       8k-18k
WASM runtime / browser bridge:                  8k-20k
Context / diagnostics / stack traces:           6k-15k
Metadata / reflection / Type system:            8k-18k
Stdlib written in kLang:                       10k-30k
Tests:                                         30k-80k
Docs / examples:                               10k-30k
```