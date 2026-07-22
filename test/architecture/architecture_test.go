package architecture_test

import (
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func repoRoot(t *testing.T) string {
	t.Helper()

	_, currentFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("resolve current file")
	}

	return filepath.Join(filepath.Dir(currentFile), "..", "..")
}

func listGoFiles(t *testing.T, root string, relativeDir string) []string {
	t.Helper()

	dir := filepath.Join(root, relativeDir)
	var files []string

	err := filepath.WalkDir(dir, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() {
			return nil
		}
		if !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}

		files = append(files, path)
		return nil
	})
	if err != nil {
		t.Fatalf("walk %s: %v", relativeDir, err)
	}

	return files
}

func parsedImports(t *testing.T, filePath string) []string {
	t.Helper()

	fileSet := token.NewFileSet()
	parsed, err := parser.ParseFile(fileSet, filePath, nil, parser.ImportsOnly)
	if err != nil {
		t.Fatalf("parse imports %s: %v", filePath, err)
	}

	imports := make([]string, 0, len(parsed.Imports))
	for _, imported := range parsed.Imports {
		imports = append(imports, strings.Trim(imported.Path.Value, `"`))
	}

	return imports
}

func packageName(t *testing.T, filePath string) string {
	t.Helper()

	fileSet := token.NewFileSet()
	parsed, err := parser.ParseFile(fileSet, filePath, nil, parser.PackageClauseOnly)
	if err != nil {
		t.Fatalf("parse package name %s: %v", filePath, err)
	}

	return parsed.Name.Name
}

func TestTemplateRequiredPathsExist(t *testing.T) {
	root := repoRoot(t)

	requiredPaths := []string{
		"configs/development.yaml",
		"configs/production.yaml",
		"docs/architecture.md",
		"docs/openapi3.yaml",
		"internal/api/http/v1",
		"internal/api/http/v1/docs",
		"internal/api/http/v1/health",
		"internal/api/http/v1/transport",
		"internal/auth",
		"internal/bootstrap",
		"internal/config",
		"internal/domain/errors",
		"internal/middleware",
		"internal/platform",
		"test/contract",
		"test/e2e",
		"test/integration",
	}

	for _, relativePath := range requiredPaths {
		if _, err := os.Stat(filepath.Join(root, relativePath)); err != nil {
			t.Fatalf("required architecture path is missing: %s", relativePath)
		}
	}
}

func TestTemplateDoesNotContainReferenceBusinessLayers(t *testing.T) {
	root := repoRoot(t)

	forbiddenPaths := []string{
		"internal/ports",
		"internal/repo",
		"internal/services",
		"internal/usecase",
		"migrations",
	}

	for _, relativePath := range forbiddenPaths {
		if _, err := os.Stat(filepath.Join(root, relativePath)); err == nil {
			t.Fatalf("template must not contain reference business path: %s", relativePath)
		} else if !os.IsNotExist(err) {
			t.Fatalf("stat %s: %v", relativePath, err)
		}
	}
}

func TestTemplateDoesNotContainReferenceFeature(t *testing.T) {
	root := repoRoot(t)
	forbiddenMarker := "to" + "do"

	err := filepath.WalkDir(root, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() {
			switch entry.Name() {
			case ".git", "tmp", "vendor":
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, ".go") && !strings.HasSuffix(path, ".md") && !strings.HasSuffix(path, ".yaml") && !strings.HasSuffix(path, ".yml") {
			return nil
		}

		payload, readErr := os.ReadFile(path)
		if readErr != nil {
			return readErr
		}
		if strings.Contains(strings.ToLower(string(payload)), forbiddenMarker) {
			t.Fatalf("template must not contain reference business feature text: %s", path)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walk repo: %v", err)
	}
}

func TestTemplateLayerPackageNames(t *testing.T) {
	root := repoRoot(t)

	expectedPackages := map[string]string{
		"internal/api/http/v1":           "http",
		"internal/api/http/v1/docs":      "docs",
		"internal/api/http/v1/health":    "http",
		"internal/api/http/v1/transport": "http",
		"internal/auth":                  "auth",
		"internal/bootstrap":             "bootstrap",
		"internal/config":                "config",
		"internal/domain/errors":         "errors",
		"internal/middleware":            "middleware",
		"internal/platform":              "platform",
	}

	for relativeDir, expectedPackage := range expectedPackages {
		files := listGoFiles(t, root, relativeDir)
		if len(files) == 0 {
			t.Fatalf("expected go files in %s", relativeDir)
		}

		for _, filePath := range files {
			effectiveExpectedPackage := expectedPackage
			relativeFilePath, err := filepath.Rel(root, filePath)
			if err != nil {
				t.Fatalf("relative file path %s: %v", filePath, err)
			}
			matchedDir := relativeDir
			for expectedDir, expectedDirPackage := range expectedPackages {
				if strings.HasPrefix(relativeFilePath, expectedDir+string(filepath.Separator)) && len(expectedDir) > len(matchedDir) {
					effectiveExpectedPackage = expectedDirPackage
					matchedDir = expectedDir
				}
			}

			if actualPackage := packageName(t, filePath); actualPackage != effectiveExpectedPackage {
				t.Fatalf("unexpected package name in %s: got %s want %s", filePath, actualPackage, effectiveExpectedPackage)
			}
		}
	}
}

func TestTemplateDependencyBoundaries(t *testing.T) {
	root := repoRoot(t)

	type dependencyRule struct {
		relativeDir      string
		forbiddenImports []string
	}

	rules := []dependencyRule{
		{
			relativeDir: "internal/domain",
			forbiddenImports: []string{
				"/internal/api/http",
				"/internal/middleware",
				"/internal/repo/postgres",
				"database/sql",
				"github.com/gofiber/",
				"github.com/jackc/pgx",
			},
		},
		{
			relativeDir: "internal/api/http",
			forbiddenImports: []string{
				"/internal/repo/postgres",
			},
		},
	}

	for _, rule := range rules {
		for _, filePath := range listGoFiles(t, root, rule.relativeDir) {
			for _, importedPath := range parsedImports(t, filePath) {
				for _, forbidden := range rule.forbiddenImports {
					if strings.Contains(importedPath, forbidden) {
						t.Fatalf("forbidden dependency %s found in %s", importedPath, filePath)
					}
				}
			}
		}
	}
}

func TestBootstrapAppDoesNotRegisterBusinessModules(t *testing.T) {
	root := repoRoot(t)
	appFile := filepath.Join(root, "internal/bootstrap/app.go")
	payload, err := os.ReadFile(appFile)
	if err != nil {
		t.Fatalf("read app.go: %v", err)
	}

	forbidden := []string{
		"UseCaseModules",
		"RepositoryModules",
	}

	for _, item := range forbidden {
		if strings.Contains(string(payload), item) {
			t.Fatalf("template app.go must not register %s", item)
		}
	}
}
