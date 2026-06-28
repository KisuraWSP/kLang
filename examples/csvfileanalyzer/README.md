# CSV Sales Analyzer

Reads a sales CSV file, parses quoted fields, validates its schema and numeric
columns, builds typed records, and produces regional and product analytics.

The bundled data includes a quoted product name containing a comma:
`"Monitor, 27 inch"`.

## Features

- Native `File.read_lines()` with typed `Result` error handling.
- Quote-aware parsing for commas and escaped `""` quote characters.
- Header validation for `date,region,product,units,unit_price`.
- Row and column diagnostics with numeric and positive-value validation.
- Struct-style `Sale`, `Dataset`, `GroupTotal`, and `Analysis` aliases.
- Alias methods for revenue, row counts, and date ranges.
- Compile-time aliases for record and summary lists.
- Standard-library `list.append`, `list.contains`, and `list.fold`.
- Cent-based money rounding and two-decimal currency formatting.
- Optional nested JSON serialization with alias field tags.
- Command-line selection of alternate CSV files through `args` and `option`.

Each CSV record must occupy one physical line. The parser supports quoted
commas and doubled quotes but intentionally does not combine multiline fields.

## Project Files

- `model.klang` defines typed records and JSON field mappings.
- `csv.klang` parses and validates CSV input.
- `analyzer.klang` calculates totals and grouped summaries.
- `app.klang` handles files, CLI arguments, reports, and JSON output.
- `data/sales.csv` contains the valid sales fixture.
- `data/malformed.csv` demonstrates an unterminated quote diagnostic.
- `first.klang` provides the required `Main() : Int` entry point.
- `klang.project` defines the five-source workspace.

## Command Line

Check and run the default data:

```sh
go run . check examples/csvfileanalyzer
go run . run examples/csvfileanalyzer
```

Pass another CSV file as the first program argument:

```sh
go run . run examples/csvfileanalyzer path/to/sales.csv
```

Add `json` to print the typed `Analysis` as JSON:

```sh
go run . run examples/csvfileanalyzer examples/csvfileanalyzer/data/sales.csv json
```

Run the intentional parse failure:

```sh
go run . run examples/csvfileanalyzer examples/csvfileanalyzer/data/malformed.csv
```

```text
CSV parse error: line 2: unterminated quoted field
```

## Expected Output

```text
CSV SALES ANALYZER
==================
File: examples/csvfileanalyzer/data/sales.csv
Period: 2026-06-01 to 2026-06-08
Rows: 8
Units sold: 36
Revenue: USD 3529.93
Top region: West (USD 1412.49)
Top product: Webcam (15 units)

Regional breakdown:
- North: 16 units, USD 1381.46
- West: 10 units, USD 1412.49
- East: 10 units, USD 735.98
```
