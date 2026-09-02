package template

import (
	"encoding/json"
	"fmt"
	"os"
	"regexp"
)

const (
	ExpectedTemplateSource = "ThatSoftwareCompany/template-go-api"
	ExpectedLicense        = "Apache-2.0"
	ExpectedGoVersion      = "1.26.0"
	ExpectedDatabase       = "PostgreSQL"
	ExpectedMigrationTool  = "golang-migrate"
	ExpectedArchitecture   = "modular-mvc"
	ExpectedPostgreSQL     = "16+"
)

var versionPattern = regexp.MustCompile(`^[0-9]+\.[0-9]+\.[0-9]+$`)
var commitPattern = regexp.MustCompile(`^[0-9a-f]{40}$`)

type Manifest struct {
	TemplateID            string            `json:"template_id"`
	TemplateSource        string            `json:"template_source"`
	TemplateVersion       string            `json:"template_version"`
	TemplateCommit        string            `json:"template_commit"`
	GeneratedFrom         string            `json:"generated_from"`
	Repository            string            `json:"repository"`
	License               string            `json:"license"`
	MinimumGoVersion      string            `json:"minimum_go_version"`
	Database              string            `json:"database"`
	MigrationTool         string            `json:"migration_tool"`
	Architecture          string            `json:"architecture"`
	SupportedEnvironments []string          `json:"supported_environments"`
	DependencyVersions    map[string]string `json:"dependency_versions"`
	Compatibility         Compatibility     `json:"compatibility"`
}

type Compatibility struct {
	Go         string `json:"go"`
	PostgreSQL string `json:"postgresql"`
}

func Load(path string) (Manifest, error) {
	contents, err := os.ReadFile(path)
	if err != nil {
		return Manifest{}, fmt.Errorf("read manifest: %w", err)
	}

	var manifest Manifest
	if err := json.Unmarshal(contents, &manifest); err != nil {
		return Manifest{}, fmt.Errorf("parse manifest: %w", err)
	}
	return manifest, nil
}

func RecordProvenance(path, version, commit string) error {
	if !versionPattern.MatchString(version) {
		return fmt.Errorf("template version must use major.minor.patch format")
	}
	if !commitPattern.MatchString(commit) {
		return fmt.Errorf("template commit must be a 40-character lowercase commit SHA")
	}

	contents, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("read manifest: %w", err)
	}

	updated, err := replaceStringField(contents, "template_version", version)
	if err != nil {
		return err
	}
	updated, err = replaceStringField(updated, "template_commit", commit)
	if err != nil {
		return err
	}
	if err := os.WriteFile(path, updated, 0o644); err != nil {
		return fmt.Errorf("write manifest: %w", err)
	}
	return nil
}

func replaceStringField(contents []byte, field, value string) ([]byte, error) {
	pattern := regexp.MustCompile(`(?m)^([[:space:]]*"` + regexp.QuoteMeta(field) + `"[[:space:]]*:[[:space:]]*")[^"]*(".*)$`)
	if len(pattern.FindAllIndex(contents, -1)) != 1 {
		return nil, fmt.Errorf("manifest field must appear exactly once: %s", field)
	}
	return pattern.ReplaceAll(contents, []byte("${1}"+value+"${2}")), nil
}

func (m Manifest) Validate() error {
	checks := map[string]bool{
		"template_id":              m.TemplateID != "",
		"template_source":          m.TemplateSource == ExpectedTemplateSource,
		"template_version":         versionPattern.MatchString(m.TemplateVersion),
		"repository":               m.Repository == ExpectedTemplateSource,
		"license":                  m.License == ExpectedLicense,
		"minimum_go_version":       m.MinimumGoVersion == ExpectedGoVersion,
		"database":                 m.Database == ExpectedDatabase,
		"migration_tool":           m.MigrationTool == ExpectedMigrationTool,
		"architecture":             m.Architecture == ExpectedArchitecture,
		"compatibility.go":         m.Compatibility.Go == ExpectedGoVersion,
		"compatibility.postgresql": m.Compatibility.PostgreSQL == ExpectedPostgreSQL,
	}

	for field, valid := range checks {
		if !valid {
			return fmt.Errorf("invalid manifest field: %s", field)
		}
	}

	if len(m.SupportedEnvironments) != 3 {
		return fmt.Errorf("supported_environments must contain development, test, and production")
	}
	environments := map[string]struct{}{}
	for _, environment := range m.SupportedEnvironments {
		environments[environment] = struct{}{}
	}
	for _, environment := range []string{"development", "test", "production"} {
		if _, ok := environments[environment]; !ok {
			return fmt.Errorf("supported_environments is missing %s", environment)
		}
	}

	for _, dependency := range []string{
		"github.com/golang-migrate/migrate/v4",
		"github.com/jackc/pgx/v5",
	} {
		if m.DependencyVersions[dependency] == "" {
			return fmt.Errorf("dependency_versions is missing %s", dependency)
		}
	}

	if m.TemplateCommit != "" && !commitPattern.MatchString(m.TemplateCommit) {
		return fmt.Errorf("template_commit must be empty or a 40-character lowercase commit SHA")
	}

	return nil
}
