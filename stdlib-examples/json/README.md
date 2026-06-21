# json stdlib example

This example is a comprehensive tour of the runtime-backed `json` standard
library module. It covers:

- here-string parsing, validation, all JSON kinds, and safe object/array access
- typed `Option` extraction for String, Int, Float, and Bool values
- native Table/List/scalar serialization, deserialization, and fallback decode
- deterministic round trips, null values, and unsafe Table-key errors
- alias-struct serialization, `json:"..."` field tags, and tag metadata
- primitive compatibility encoders and array/object builder functions
- checked list/map encoders, custom encoder hooks, and formatter helpers
- compatibility decoder functions and continuation state

Every section prints its intermediate values and uses assertions to verify the
documented behavior.

Run it from the repository root:

```sh
go run . run stdlib-examples/json
```
