// Package analysis exports change units for an agent to explain, and imports
// and stores the explanations it produces.
package analysis

import (
	"errors"
	"fmt"
	"strings"
)

// SchemaVersion is the version of both the export and the analysis format.
const SchemaVersion = "1.0"

// Limits on imported data. An analysis is written by an agent, so the sizes
// are bounded rather than trusted.
const (
	MaxAnalysisBytes = 4 << 20 // 4 MiB of JSON
	MaxFieldRunes    = 4000
	MaxTitleRunes    = 120
	MaxSteps         = 2000
)

// Step is one learning step, attached to a change unit by its id.
type Step struct {
	ChangeID string `json:"change_id"`
	Title    string `json:"title"`
	What     string `json:"what"`
	How      string `json:"how"`
	Why      string `json:"why,omitempty"`
}

// Analysis is the document an agent produces.
type Analysis struct {
	SchemaVersion string      `json:"schema_version"`
	SnapshotID    string      `json:"snapshot_id"`
	Repository    *Repository `json:"repository,omitempty"`
	Generator     string      `json:"generator,omitempty"`
	GeneratedAt   string      `json:"generated_at,omitempty"`
	Steps         []Step      `json:"steps"`
}

// Repository echoes the exported repository identity.
type Repository struct {
	Name       string `json:"name,omitempty"`
	Base       string `json:"base,omitempty"`
	BaseCommit string `json:"base_commit,omitempty"`
}

// ErrInvalid marks a document that failed validation.
var ErrInvalid = errors.New("invalid analysis")

// Validate checks the document's own shape, independently of any snapshot.
//
// Go has no standard library JSON Schema validator and the project avoids
// dependencies, so this mirrors references/analysis-schema.json by hand. That
// file stays the contract shown to the agent; this function enforces it.
func Validate(doc *Analysis) []string {
	var problems []string
	if doc.SchemaVersion != SchemaVersion {
		problems = append(problems,
			fmt.Sprintf("schema_version: must be %q, got %q", SchemaVersion, doc.SchemaVersion))
	}
	if !strings.HasPrefix(doc.SnapshotID, "sha256:") || len(doc.SnapshotID) != len("sha256:")+64 {
		problems = append(problems, "snapshot_id: must be a sha256 digest copied from the export")
	}
	if len(doc.Steps) == 0 {
		problems = append(problems, "steps: must not be empty")
	}
	if len(doc.Steps) > MaxSteps {
		problems = append(problems, fmt.Sprintf("steps: at most %d steps", MaxSteps))
	}
	for index, step := range doc.Steps {
		problems = append(problems, validateStep(index, step)...)
	}
	return problems
}

func validateStep(index int, step Step) []string {
	var problems []string
	at := func(field string) string { return fmt.Sprintf("steps/%d/%s", index, field) }

	if !strings.HasPrefix(step.ChangeID, "sha256:") || len(step.ChangeID) != len("sha256:")+64 {
		problems = append(problems, at("change_id")+": must be a change id from the export")
	}
	for field, value := range map[string]string{"title": step.Title, "what": step.What, "how": step.How} {
		if strings.TrimSpace(value) == "" {
			problems = append(problems, at(field)+": required, must not be empty")
		}
	}
	limits := map[string]struct {
		value string
		max   int
	}{
		"title": {step.Title, MaxTitleRunes},
		"what":  {step.What, MaxFieldRunes},
		"how":   {step.How, MaxFieldRunes},
		"why":   {step.Why, MaxFieldRunes},
	}
	for field, limit := range limits {
		if len([]rune(limit.value)) > limit.max {
			problems = append(problems,
				fmt.Sprintf("%s: at most %d characters", at(field), limit.max))
		}
	}
	return problems
}
