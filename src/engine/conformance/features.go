package conformance

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"sort"
)

type Backend string
type Status string
type FeatureID string

const (
	BackendStandalone Backend = "Standalone"
	BackendJS         Backend = "JS"
	BackendWASM       Backend = "WASM"
	BackendBytecode   Backend = "bytecode"

	StatusInterpreted  Status = "interpreted"
	StatusCompiled     Status = "compiled"
	StatusHostProvided Status = "host-provided"
	StatusFallback     Status = "fallback"
	StatusRejected     Status = "rejected"

	FeatureValuesPrimitives     FeatureID = "values.primitives"
	FeatureValuesList           FeatureID = "values.list"
	FeatureValuesSet            FeatureID = "values.set"
	FeatureValuesMap            FeatureID = "values.map"
	FeatureValuesTable          FeatureID = "values.table"
	FeatureValuesJSON           FeatureID = "values.json"
	FeatureAtomResult           FeatureID = "errors.atom-result"
	FeatureExceptions           FeatureID = "errors.exceptions"
	FeatureConditionals         FeatureID = "control.conditionals"
	FeatureLoops                FeatureID = "control.loops"
	FeatureDirectFunctions      FeatureID = "functions.direct"
	FeatureAsyncFunctions       FeatureID = "functions.async"
	FeatureStructAliases        FeatureID = "structs.alias"
	FeatureExtensions           FeatureID = "functions.extensions"
	FeatureIteratorPipelines    FeatureID = "iterators.pipeline"
	FeatureThreads              FeatureID = "concurrency.threads"
	FeatureAtomic               FeatureID = "concurrency.atomic"
	FeatureTransactions         FeatureID = "concurrency.transactions"
	FeatureFiles                FeatureID = "host.files"
	FeatureOS                   FeatureID = "host.os"
	FeatureJavaScriptInterop    FeatureID = "interop.javascript"
	FeatureModules              FeatureID = "modules.imports"
	FeatureStructuredDiagnostic FeatureID = "diagnostics.structured"
)

type Feature struct {
	ID          FeatureID          `json:"id"`
	Description string             `json:"description"`
	Support     map[Backend]Status `json:"support"`
}

type Matrix struct {
	SchemaVersion int       `json:"schema_version"`
	Backends      []Backend `json:"backends"`
	Statuses      []Status  `json:"statuses"`
	Features      []Feature `json:"features"`
}

//go:embed backend_features.json
var encodedMatrix []byte

func Load() (Matrix, error) {
	var matrix Matrix
	if err := json.Unmarshal(encodedMatrix, &matrix); err != nil {
		return Matrix{}, fmt.Errorf("decode backend feature matrix: %w", err)
	}
	if err := Validate(matrix); err != nil {
		return Matrix{}, err
	}
	return matrix, nil
}

func Validate(matrix Matrix) error {
	if matrix.SchemaVersion != 1 {
		return fmt.Errorf("unsupported feature matrix schema version %d", matrix.SchemaVersion)
	}
	expectedBackends := map[Backend]bool{
		BackendStandalone: true,
		BackendJS:         true,
		BackendWASM:       true,
		BackendBytecode:   true,
	}
	if len(matrix.Backends) != len(expectedBackends) {
		return fmt.Errorf("feature matrix must declare exactly %d backends", len(expectedBackends))
	}
	for _, backend := range matrix.Backends {
		if !expectedBackends[backend] {
			return fmt.Errorf("unknown feature matrix backend %q", backend)
		}
		delete(expectedBackends, backend)
	}
	validStatuses := map[Status]bool{
		StatusInterpreted: true, StatusCompiled: true, StatusHostProvided: true,
		StatusFallback: true, StatusRejected: true,
	}
	seen := map[FeatureID]bool{}
	for _, feature := range matrix.Features {
		if feature.ID == "" || feature.Description == "" {
			return fmt.Errorf("feature matrix entries require id and description")
		}
		if seen[feature.ID] {
			return fmt.Errorf("duplicate feature matrix id %q", feature.ID)
		}
		seen[feature.ID] = true
		if len(feature.Support) != len(matrix.Backends) {
			return fmt.Errorf("feature %q must declare every backend", feature.ID)
		}
		for _, backend := range matrix.Backends {
			status, ok := feature.Support[backend]
			if !ok {
				return fmt.Errorf("feature %q is missing backend %q", feature.ID, backend)
			}
			if !validStatuses[status] {
				return fmt.Errorf("feature %q has invalid status %q", feature.ID, status)
			}
		}
	}
	if len(seen) == 0 {
		return fmt.Errorf("feature matrix must not be empty")
	}
	return nil
}

func Lookup(id FeatureID, backend Backend) (Status, bool) {
	matrix, err := Load()
	if err != nil {
		return "", false
	}
	for _, feature := range matrix.Features {
		if feature.ID == id {
			status, ok := feature.Support[backend]
			return status, ok
		}
	}
	return "", false
}

func IDs() []FeatureID {
	matrix, err := Load()
	if err != nil {
		return nil
	}
	ids := make([]FeatureID, 0, len(matrix.Features))
	for _, feature := range matrix.Features {
		ids = append(ids, feature.ID)
	}
	sort.Slice(ids, func(left, right int) bool { return ids[left] < ids[right] })
	return ids
}
