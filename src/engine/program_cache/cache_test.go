package programcache

import (
	"os"
	"path/filepath"
	"testing"

	"kLang/src/engine/file"
)

func TestStoreAndLoadProgramCache(t *testing.T) {
	root := t.TempDir()
	sourcePath := filepath.Join(root, "main.klang")
	writeSource(t, sourcePath, "function Main() : Int { return 7; }\n")

	program := file.Program{
		Name:       "main",
		Root:       root,
		EntryPoint: sourcePath,
		Files: []file.SourceFile{{
			Path:                 sourcePath,
			Lines:                []string{"function Main() : Int { return 7; }"},
			ModuleFunctionFilter: map[string]bool{"copy": true},
		}},
	}
	warnings := []Warning{{File: sourcePath, Line: 1, Message: "unused local variable"}}
	if err := Store(program, false, warnings); err != nil {
		t.Fatalf("store cache failed: %v", err)
	}

	loaded, entry, ok := Load(program, false)
	if !ok {
		t.Fatal("expected cache hit")
	}
	if loaded.Name != program.Name || loaded.EntryPoint != program.EntryPoint || len(loaded.Files) != 1 {
		t.Fatalf("unexpected cached program: %#v", loaded)
	}
	if !loaded.Files[0].ModuleFunctionFilter["copy"] {
		t.Fatalf("expected module function filter to round trip: %#v", loaded.Files[0].ModuleFunctionFilter)
	}
	if len(entry.Warnings) != 1 || entry.Warnings[0].Message != warnings[0].Message {
		t.Fatalf("expected warnings to round trip: %#v", entry.Warnings)
	}
}

func TestLoadMissesWhenSourceChanges(t *testing.T) {
	root := t.TempDir()
	sourcePath := filepath.Join(root, "main.klang")
	writeSource(t, sourcePath, "function Main() : Int { return 7; }\n")
	program := file.Program{
		Name:       "main",
		Root:       root,
		EntryPoint: sourcePath,
		Files: []file.SourceFile{{
			Path:  sourcePath,
			Lines: []string{"function Main() : Int { return 7; }"},
		}},
	}
	if err := Store(program, false, nil); err != nil {
		t.Fatalf("store cache failed: %v", err)
	}

	writeSource(t, sourcePath, "function Main() : Int { return 8; }\n")
	if _, _, ok := Load(program, false); ok {
		t.Fatal("expected cache miss after source change")
	}
}

func TestLoadMissesWhenProjectAddsSourceFile(t *testing.T) {
	root := t.TempDir()
	firstPath := filepath.Join(root, "first.klang")
	extraPath := filepath.Join(root, "extra.klang")
	writeSource(t, firstPath, "function Main() : Int { return 1; }\n")
	program := file.Program{
		Name:       "project",
		Root:       root,
		EntryPoint: firstPath,
		Files: []file.SourceFile{{
			Path:  firstPath,
			Lines: []string{"function Main() : Int { return 1; }"},
		}},
	}
	if err := Store(program, false, nil); err != nil {
		t.Fatalf("store cache failed: %v", err)
	}

	writeSource(t, extraPath, "function Extra() : Int { return 2; }\n")
	program.Files = append(program.Files, file.SourceFile{
		Path:  extraPath,
		Lines: []string{"function Extra() : Int { return 2; }"},
	})
	if _, _, ok := Load(program, false); ok {
		t.Fatal("expected cache miss after adding a project source file")
	}
}

func TestRawLangUsesSeparateCacheEntry(t *testing.T) {
	root := t.TempDir()
	sourcePath := filepath.Join(root, "main.klang")
	writeSource(t, sourcePath, "function Main() : Int { return 1; }\n")
	program := file.Program{
		Name:       "main",
		Root:       root,
		EntryPoint: sourcePath,
		Files: []file.SourceFile{{
			Path:  sourcePath,
			Lines: []string{"function Main() : Int { return 1; }"},
		}},
	}
	if err := Store(program, false, nil); err != nil {
		t.Fatalf("store cache failed: %v", err)
	}

	if _, _, ok := Load(program, true); ok {
		t.Fatal("expected raw-lang cache lookup to miss normal cache entry")
	}
}

func TestNoCacheDirectiveSkipsStoreAndLoad(t *testing.T) {
	root := t.TempDir()
	sourcePath := filepath.Join(root, "main.klang")
	writeSource(t, sourcePath, "no_cache;\nfunction Main() : Int { return 1; }\n")
	program := file.Program{
		Name:       "main",
		Root:       root,
		EntryPoint: sourcePath,
		Files: []file.SourceFile{{
			Path:  sourcePath,
			Lines: []string{"no_cache;", "function Main() : Int { return 1; }"},
		}},
	}

	if !HasNoCache(program) {
		t.Fatal("expected no_cache directive to be detected")
	}
	if err := Store(program, false, nil); err != nil {
		t.Fatalf("store with no_cache failed: %v", err)
	}
	if cachePath, ok := Path(program, false); ok {
		if _, err := os.Stat(cachePath); !os.IsNotExist(err) {
			t.Fatalf("expected no cache file at %s, stat err=%v", cachePath, err)
		}
	}
	if _, _, ok := Load(program, false); ok {
		t.Fatal("expected no_cache program to miss cache")
	}
}

func TestCachedNoCacheEntryIsRejected(t *testing.T) {
	root := t.TempDir()
	sourcePath := filepath.Join(root, "main.klang")
	writeSource(t, sourcePath, "function Main() : Int { return 1; }\n")
	program := file.Program{
		Name:       "main",
		Root:       root,
		EntryPoint: sourcePath,
		Files: []file.SourceFile{{
			Path:  sourcePath,
			Lines: []string{"function Main() : Int { return 1; }"},
		}},
	}
	if err := Store(program, false, nil); err != nil {
		t.Fatalf("store cache failed: %v", err)
	}

	writeSource(t, sourcePath, "no_cache;\nfunction Main() : Int { return 1; }\n")
	program.Files[0].Lines = []string{"no_cache;", "function Main() : Int { return 1; }"}
	if _, _, ok := Load(program, false); ok {
		t.Fatal("expected cache miss after no_cache directive is added")
	}
}

func writeSource(t *testing.T, path string, source string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(source), 0644); err != nil {
		t.Fatalf("write source failed: %v", err)
	}
}
