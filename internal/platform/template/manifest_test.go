package template

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestRepositoryManifestIsValid(t *testing.T) {
	manifest, err := Load(filepath.Join("..", "..", "..", ".template", "manifest.json"))
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if err := manifest.Validate(); err != nil {
		t.Fatalf("Validate() error = %v", err)
	}
}

func TestManifestRejectsInvalidCommit(t *testing.T) {
	manifest := validManifest()
	manifest.TemplateCommit = "not-a-commit"

	if err := manifest.Validate(); err == nil {
		t.Fatal("expected invalid template commit to fail validation")
	}
}

func TestManifestAcceptsEmptySourceCommit(t *testing.T) {
	manifest := validManifest()
	manifest.TemplateCommit = ""

	if err := manifest.Validate(); err != nil {
		t.Fatalf("Validate() error = %v", err)
	}
}

func TestRecordProvenancePreservesManifestFields(t *testing.T) {
	temporary := t.TempDir()
	path := filepath.Join(temporary, "manifest.json")
	original := map[string]any{
		"template_version": "0.1.0",
		"template_commit":  "",
		"generated_from":   "github.com/example/project",
		"generated_project": map[string]any{
			"project_name": "Example",
		},
	}
	contents, err := json.MarshalIndent(original, "", "  ")
	if err != nil {
		t.Fatalf("Marshal() error = %v", err)
	}
	if err := os.WriteFile(path, contents, 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	commit := "0123456789abcdef0123456789abcdef01234567"
	if err := RecordProvenance(path, "0.2.0", commit); err != nil {
		t.Fatalf("RecordProvenance() error = %v", err)
	}

	updated, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}
	var document map[string]any
	if err := json.Unmarshal(updated, &document); err != nil {
		t.Fatalf("Unmarshal() error = %v", err)
	}
	if document["template_version"] != "0.2.0" || document["template_commit"] != commit {
		t.Fatalf("unexpected provenance: %#v", document)
	}
	if document["generated_from"] != "github.com/example/project" {
		t.Fatalf("generated_from was not preserved: %#v", document)
	}
}

func validManifest() Manifest {
	return Manifest{
		TemplateID:       "template-go-api",
		TemplateSource:   ExpectedTemplateSource,
		TemplateVersion:  "0.1.0",
		Repository:       ExpectedTemplateSource,
		License:          ExpectedLicense,
		MinimumGoVersion: ExpectedGoVersion,
		Database:         ExpectedDatabase,
		MigrationTool:    ExpectedMigrationTool,
		Architecture:     ExpectedArchitecture,
		SupportedEnvironments: []string{
			"development", "test", "production",
		},
		DependencyVersions: map[string]string{
			"github.com/golang-migrate/migrate/v4": "v4.19.1",
			"github.com/jackc/pgx/v5":              "v5.8.0",
		},
		Compatibility: Compatibility{
			Go:         ExpectedGoVersion,
			PostgreSQL: ExpectedPostgreSQL,
		},
	}
}
