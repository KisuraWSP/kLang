package conformance

import (
	"reflect"
	"sort"
	"testing"
)

func TestBackendFeatureMatrixIsCompleteAndStable(t *testing.T) {
	matrix, err := Load()
	if err != nil {
		t.Fatalf("load backend feature matrix: %v", err)
	}

	expectedIDs := []FeatureID{
		FeatureAsyncFunctions,
		FeatureAtomResult,
		FeatureAtomic,
		FeatureConditionals,
		FeatureDirectFunctions,
		FeatureExceptions,
		FeatureExtensions,
		FeatureFiles,
		FeatureIteratorPipelines,
		FeatureJavaScriptInterop,
		FeatureLoops,
		FeatureModules,
		FeatureOS,
		FeatureStructAliases,
		FeatureStructuredDiagnostic,
		FeatureThreads,
		FeatureTransactions,
		FeatureValuesJSON,
		FeatureValuesList,
		FeatureValuesMap,
		FeatureValuesPrimitives,
		FeatureValuesSet,
		FeatureValuesTable,
	}
	sort.Slice(expectedIDs, func(left, right int) bool { return expectedIDs[left] < expectedIDs[right] })
	if actual := IDs(); !reflect.DeepEqual(actual, expectedIDs) {
		t.Fatalf("feature constants and matrix differ:\nwant %#v\ngot  %#v", expectedIDs, actual)
	}

	for _, feature := range matrix.Features {
		wasm := feature.Support[BackendWASM]
		bytecode := feature.Support[BackendBytecode]
		if wasm == StatusCompiled && bytecode != StatusCompiled {
			t.Fatalf("feature %s claims compiled WASM without bytecode support", feature.ID)
		}
		if wasm == StatusFallback && bytecode != StatusRejected {
			t.Fatalf("feature %s claims WASM fallback without bytecode rejection", feature.ID)
		}
	}
}

func TestRejectedFeaturesAreExplicit(t *testing.T) {
	matrix, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	for _, feature := range matrix.Features {
		for _, backend := range matrix.Backends {
			status := feature.Support[backend]
			if status == "" {
				t.Fatalf("feature %s silently omits backend %s", feature.ID, backend)
			}
		}
	}
}
