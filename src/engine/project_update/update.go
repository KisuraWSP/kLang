package projectupdate

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"kLang/src/engine/file"
)

type Report struct {
	Root        string
	Manifest    string
	Backup      string
	FromVersion int
	ToVersion   int
	Created     bool
	Changed     bool
	Migrations  []string
}

func Update(path string) (Report, error) {
	root, manifestPath, err := projectPaths(path)
	if err != nil {
		return Report{}, err
	}
	report := Report{
		Root:       root,
		Manifest:   manifestPath,
		ToVersion:  file.CurrentLanguageVersion,
		Migrations: []string{"validate the strict Main() : Int entrypoint ABI"},
	}

	if !file.FileExists(manifestPath) {
		if !file.FileExists(filepath.Join(root, file.KlangEntryPoint)) {
			return Report{}, fmt.Errorf("project must contain %s or legacy %s", file.KlangProjectFile, file.KlangEntryPoint)
		}
		sourcePaths, err := discoverSources(root)
		if err != nil {
			return Report{}, err
		}
		manifest := buildManifest(filepath.Base(root), sourcePaths)
		if err := os.WriteFile(manifestPath, []byte(manifest), 0o644); err != nil {
			return Report{}, fmt.Errorf("create %s: %w", manifestPath, err)
		}
		report.Created = true
		report.Changed = true
		report.Migrations = append([]string{"create klang.project for the legacy folder"}, report.Migrations...)
		return report, nil
	}

	manifest, err := file.ReadProjectManifest(manifestPath)
	if err != nil {
		return Report{}, err
	}
	report.FromVersion = manifest.LanguageVersion
	if manifest.LanguageVersion > file.CurrentLanguageVersion {
		return Report{}, fmt.Errorf(
			"project language_version %d is newer than this kLang version (%d)",
			manifest.LanguageVersion,
			file.CurrentLanguageVersion,
		)
	}
	if manifest.LanguageVersion == file.CurrentLanguageVersion {
		return report, nil
	}

	original, err := os.ReadFile(manifestPath)
	if err != nil {
		return Report{}, fmt.Errorf("read %s: %w", manifestPath, err)
	}
	updated := setLanguageVersion(string(original), file.CurrentLanguageVersion)
	backupPath, err := nextBackupPath(manifestPath)
	if err != nil {
		return Report{}, err
	}
	if err := os.WriteFile(backupPath, original, 0o644); err != nil {
		return Report{}, fmt.Errorf("back up %s: %w", manifestPath, err)
	}
	if err := os.WriteFile(manifestPath, []byte(updated), 0o644); err != nil {
		return Report{}, fmt.Errorf("update %s: %w", manifestPath, err)
	}

	report.Backup = backupPath
	report.Changed = true
	report.Migrations = append([]string{"record language_version in klang.project"}, report.Migrations...)
	return report, nil
}

func projectPaths(path string) (string, string, error) {
	cleanPath := filepath.Clean(path)
	info, err := os.Stat(cleanPath)
	if err != nil {
		return "", "", err
	}
	if info.IsDir() {
		return cleanPath, filepath.Join(cleanPath, file.KlangProjectFile), nil
	}
	if filepath.Base(cleanPath) != file.KlangProjectFile {
		return "", "", fmt.Errorf("update expects a project folder or %s", file.KlangProjectFile)
	}
	return filepath.Dir(cleanPath), cleanPath, nil
}

func discoverSources(root string) ([]string, error) {
	var paths []string
	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() {
			if path != root && entry.Name() == file.KlangDistDir {
				return filepath.SkipDir
			}
			return nil
		}
		if !file.IsSourcePath(entry.Name()) {
			return nil
		}
		relativePath, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		paths = append(paths, filepath.ToSlash(relativePath))
		return nil
	})
	if err != nil {
		return nil, err
	}
	sort.Slice(paths, func(left, right int) bool {
		if paths[left] == file.KlangEntryPoint {
			return true
		}
		if paths[right] == file.KlangEntryPoint {
			return false
		}
		return paths[left] < paths[right]
	})
	return paths, nil
}

func buildManifest(name string, sources []string) string {
	quotedSources := make([]string, 0, len(sources))
	for _, source := range sources {
		quotedSources = append(quotedSources, fmt.Sprintf("%q", source))
	}
	return fmt.Sprintf(
		"name = %q\nentry = %q\nlanguage_version = %d\nsources = [%s]\n",
		name,
		file.KlangEntryPoint,
		file.CurrentLanguageVersion,
		strings.Join(quotedSources, ", "),
	)
}

func setLanguageVersion(source string, version int) string {
	lines := strings.Split(strings.TrimSuffix(source, "\n"), "\n")
	for index, line := range lines {
		key, _, ok := strings.Cut(line, "=")
		if ok && strings.TrimSpace(key) == "language_version" {
			lines[index] = fmt.Sprintf("language_version = %d", version)
			return strings.Join(lines, "\n") + "\n"
		}
	}

	insertAt := 0
	for index, line := range lines {
		key, _, ok := strings.Cut(line, "=")
		if ok && strings.TrimSpace(key) == "entry" {
			insertAt = index + 1
			break
		}
	}
	lines = append(lines, "")
	copy(lines[insertAt+1:], lines[insertAt:])
	lines[insertAt] = fmt.Sprintf("language_version = %d", version)
	return strings.Join(lines, "\n") + "\n"
}

func nextBackupPath(manifestPath string) (string, error) {
	candidate := manifestPath + ".bak"
	for suffix := 0; ; suffix++ {
		if suffix > 0 {
			candidate = fmt.Sprintf("%s.bak.%d", manifestPath, suffix)
		}
		_, err := os.Stat(candidate)
		if os.IsNotExist(err) {
			return candidate, nil
		}
		if err != nil {
			return "", err
		}
	}
}
