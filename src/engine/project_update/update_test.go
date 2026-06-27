package projectupdate

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"kLang/src/engine/file"
)

func TestUpdateAddsLanguageVersionAndBackup(t *testing.T) {
	root := t.TempDir()
	manifestPath := writeFile(t, root, file.KlangProjectFile, `# Keep this comment.
name = "demo"
entry = "first.klang"
sources = ["first.klang"]
`)
	writeFile(t, root, file.KlangEntryPoint, "function Main() : Int { return 0; }\n")

	report, err := Update(root)
	if err != nil {
		t.Fatalf("Update returned an error: %v", err)
	}
	if !report.Changed || report.FromVersion != 0 || report.ToVersion != file.CurrentLanguageVersion {
		t.Fatalf("unexpected report: %#v", report)
	}
	if report.Backup == "" {
		t.Fatal("expected an update backup")
	}
	updated, err := os.ReadFile(manifestPath)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(updated), "# Keep this comment.") ||
		!strings.Contains(string(updated), "language_version = 1") {
		t.Fatalf("manifest was not migrated safely:\n%s", updated)
	}
}

func TestUpdateIsIdempotent(t *testing.T) {
	root := t.TempDir()
	writeFile(t, root, file.KlangProjectFile, `name = "demo"
entry = "first.klang"
language_version = 1
sources = ["first.klang"]
`)
	writeFile(t, root, file.KlangEntryPoint, "function Main() : Int { return 0; }\n")

	report, err := Update(root)
	if err != nil {
		t.Fatalf("Update returned an error: %v", err)
	}
	if report.Changed || report.Backup != "" {
		t.Fatalf("current project should not change: %#v", report)
	}
}

func TestUpdateCreatesManifestForLegacyProject(t *testing.T) {
	root := t.TempDir()
	writeFile(t, root, file.KlangEntryPoint, "function Main() : Int { return 0; }\n")
	writeFile(t, root, "app.klang", "namespace App {}\n")

	report, err := Update(root)
	if err != nil {
		t.Fatalf("Update returned an error: %v", err)
	}
	if !report.Created || !report.Changed {
		t.Fatalf("expected legacy project migration: %#v", report)
	}
	manifest, err := os.ReadFile(filepath.Join(root, file.KlangProjectFile))
	if err != nil {
		t.Fatal(err)
	}
	for _, expected := range []string{
		`entry = "first.klang"`,
		"language_version = 1",
		`sources = ["first.klang", "app.klang"]`,
	} {
		if !strings.Contains(string(manifest), expected) {
			t.Fatalf("generated manifest missing %q:\n%s", expected, manifest)
		}
	}
}

func TestUpdateRejectsFutureLanguageVersion(t *testing.T) {
	root := t.TempDir()
	writeFile(t, root, file.KlangProjectFile, `name = "future"
entry = "first.klang"
language_version = 99
`)
	writeFile(t, root, file.KlangEntryPoint, "function Main() : Int { return 0; }\n")

	if _, err := Update(root); err == nil || !strings.Contains(err.Error(), "newer") {
		t.Fatalf("expected future-version error, got %v", err)
	}
}

func writeFile(t *testing.T, root string, name string, contents string) string {
	t.Helper()
	path := filepath.Join(root, name)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(contents), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}
