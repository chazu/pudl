package validator

import (
	"fmt"
	"strings"

	"cuelang.org/go/cue"
)

// Review statuses for `_pudl.review.status`.
const (
	// ReviewDraft is the default: nobody has blessed the schema as a complete
	// representation of the resource.
	ReviewDraft = "draft"
	// ReviewValidated means a person or agent reviewed the type schema and
	// blessed it as complete, so its required fields are always collected.
	ReviewValidated = "validated"
)

// SchemaReview is `_pudl.review: {status: "draft" | "validated", by?: string,
// at?: string}`. It is review metadata on the schema, unrelated to the
// record-level validation_status.
type SchemaReview struct {
	Status string `json:"status"`
	By     string `json:"by,omitempty"`
	At     string `json:"at,omitempty"`
}

// IsValidated reports whether the schema has been reviewed and blessed.
func (m SchemaMetadata) IsValidated() bool {
	return m.Review.Status == ReviewValidated
}

// Describe renders the review for listings: "draft", or "validated by X at Y".
func (r SchemaReview) Describe() string {
	status := r.Status
	if status == "" {
		status = ReviewDraft
	}
	parts := []string{status}
	if r.By != "" {
		parts = append(parts, "by "+r.By)
	}
	if r.At != "" {
		parts = append(parts, "at "+r.At)
	}
	return strings.Join(parts, " ")
}

// decodeReview strictly decodes `_pudl.review`. An absent block means draft.
// A present but invalid block (unknown status or key, non-concrete or
// non-string values) leaves the schema draft and records ReviewError, which
// CheckModuleIntegrity reports as a schema load error.
func decodeReview(pudl cue.Value, meta *SchemaMetadata) {
	meta.Review = SchemaReview{Status: ReviewDraft}
	v := pudl.LookupPath(cue.ParsePath("review"))
	if !v.Exists() {
		return
	}
	review, err := parseReview(v)
	if err != nil {
		meta.ReviewError = fmt.Sprintf("review: %v", err)
		return
	}
	meta.Review = review
}

func parseReview(v cue.Value) (SchemaReview, error) {
	if err := v.Validate(cue.Concrete(true)); err != nil {
		return SchemaReview{}, err
	}
	iter, err := v.Fields()
	if err != nil {
		return SchemaReview{}, fmt.Errorf("must be a struct {status, by?, at?}")
	}
	var r SchemaReview
	for iter.Next() {
		key := iter.Selector().Unquoted()
		var s string
		if err := iter.Value().Decode(&s); err != nil {
			return SchemaReview{}, fmt.Errorf("%s must be a string", key)
		}
		switch key {
		case "status":
			r.Status = s
		case "by":
			r.By = s
		case "at":
			r.At = s
		default:
			return SchemaReview{}, fmt.Errorf("unknown field %q (allowed: status, by, at)", key)
		}
	}
	switch r.Status {
	case ReviewDraft, ReviewValidated:
		return r, nil
	case "":
		return SchemaReview{}, fmt.Errorf("status is required (%q or %q)", ReviewDraft, ReviewValidated)
	default:
		return SchemaReview{}, fmt.Errorf("invalid status %q (want %q or %q)", r.Status, ReviewDraft, ReviewValidated)
	}
}
