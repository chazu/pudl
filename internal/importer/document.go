package importer

import (
	"fmt"
	"path/filepath"
	"time"

	"github.com/chazu/pudl/internal/database"
	"github.com/chazu/pudl/internal/identity"
	"github.com/chazu/pudl/internal/inference"
	"github.com/chazu/pudl/internal/projection"
)

// documentImport is the already-staged state of a single-document import.
type documentImport struct {
	id          string
	contentHash string
	format      string
	origin      string
	timestamp   time.Time
	storedPath  string
	metadataDir string
	sizeBytes   int64
}

// importDocument catalogs a committed single-document source: decode it, assign
// its schema, derive identity and version, and write metadata plus the catalog
// row. On error the caller removes the committed raw file; the metadata file is
// removed here.
func (e *EnhancedImporter) importDocument(opts ImportOptions, doc documentImport) (*ImportResult, error) {
	data, recordCount, err := e.analyzeData(opts.SourcePath, doc.format)
	if err != nil {
		return nil, fmt.Errorf("failed to analyze data: %w", err)
	}

	var assigned schemaAssignment
	if opts.assignments != nil {
		assigned, err = opts.assignments.read(0)
	} else {
		assigned, err = e.assignSchema(data, opts, inference.InferenceHints{
			Origin: doc.origin,
			Format: doc.format,
		})
	}
	if err != nil {
		return nil, err
	}
	schema := assigned.Schema

	// Identity comes from the schema's identity fields; a record they cannot be
	// extracted from is identified by content hash (catchall identity).
	schemaIdentityFields := e.getSchemaIdentityFields(schema)
	identityValues, extractErr := identity.ExtractFieldValues(data, schemaIdentityFields)
	var idTally identityTally
	if extractErr != nil {
		identityValues = nil
		idTally.record(schema, extractErr)
	}
	// Namespaced by the family root, not the assigned leaf.
	resourceID := identity.ComputeResourceID(e.identityNamespace(schema), identityValues, doc.contentHash)
	identityJSON := ""
	if len(identityValues) > 0 {
		if canonical, err := identity.CanonicalIdentityJSON(identityValues); err == nil {
			identityJSON = canonical
		}
	}

	version, latestVersion := 0, 0
	metadata := ImportMetadata{
		ID: doc.id,
		SourceInfo: SourceInfo{
			Origin:       doc.origin,
			OriginalPath: opts.originPath(),
			Confidence:   "high",
		},
		ImportMetadata: ImportMeta{
			Timestamp:   doc.timestamp.Format(time.RFC3339),
			Format:      doc.format,
			SizeBytes:   doc.sizeBytes,
			RecordCount: recordCount,
		},
		SchemaInfo: SchemaInfo{
			CuePackage:       extractPackage(schema),
			CueDefinition:    schema,
			ValidationStatus: validationStatus(assigned),
			SchemaVersion:    "v1.0",
		},
		ResourceTracking: ResourceTracking{
			IdentityFields: schemaIdentityFields,
			TrackedFields:  []string{},
			ResourceID:     resourceID,
			ContentHash:    doc.contentHash,
			IdentityValues: identityValues,
			Version:        version,
		},
	}

	enrichAssignment(&metadata.SchemaInfo, nil, assigned)
	metadataPath := filepath.Join(doc.metadataDir, doc.id+".meta")
	var identityJSONPtr *string
	if identityJSON != "" {
		identityJSONPtr = &identityJSON
	}
	contentHash := doc.contentHash
	entry := database.CatalogEntry{
		ID:              doc.id,
		StoredPath:      doc.storedPath,
		MetadataPath:    metadataPath,
		ImportTimestamp: doc.timestamp,
		Format:          doc.format,
		Origin:          doc.origin,
		Schema:          schema,
		Confidence:      assigned.Confidence,
		RecordCount:     recordCount,
		SizeBytes:       doc.sizeBytes,
		ResourceID:      &resourceID,
		ContentHash:     &contentHash,
		IdentityJSON:    identityJSONPtr,
		Version:         &version,
	}

	tally := projection.NewTally()
	projected, projectedRes := projection.Prepare(e.projector, schema, doc.id, resourceID, len(identityValues) > 0, data)
	tally.Note(e.projector, schema, projectedRes)

	var existing *database.CatalogEntry
	err = e.catalogDB.WithCatalogTxContext(opts.Context, func(tx *database.CatalogTx) error {
		var err error
		existing, err = tx.FindByContentHash(doc.contentHash)
		if err != nil {
			return err
		}
		if existing != nil {
			return nil
		}
		latestVersion, err = tx.GetLatestVersion(resourceID)
		if err != nil {
			return fmt.Errorf("get latest version: %w", err)
		}
		version = latestVersion + 1
		metadata.ResourceTracking.Version = version
		if err := e.publishMetadata(metadata, metadataPath, opts); err != nil {
			return fmt.Errorf("save metadata: %w", err)
		}
		if err := tx.AddEntry(entry); err != nil {
			return err
		}
		return projection.Observe(tx, projected)
	})
	if err != nil {
		return nil, fmt.Errorf("commit document: %w", err)
	}
	if existing != nil {
		result := &ImportResult{ID: existing.ID, SourcePath: opts.originPath(), StoredPath: existing.StoredPath, MetadataPath: existing.MetadataPath, DetectedFormat: existing.Format, DetectedOrigin: existing.Origin, AssignedSchema: existing.Schema, SchemaConfidence: existing.Confidence, RecordCount: existing.RecordCount, SizeBytes: existing.SizeBytes, ImportTimestamp: existing.ImportTimestamp.Format(time.RFC3339), ContentHash: doc.contentHash, Skipped: true, SkipReason: "content already exists in catalog"}
		if existing.Version != nil {
			result.Version = *existing.Version
		}
		if existing.ResourceID != nil {
			result.ResourceID = *existing.ResourceID
		}
		return result, nil
	}

	result := &ImportResult{
		ID:               doc.id,
		SourcePath:       opts.originPath(),
		StoredPath:       doc.storedPath,
		MetadataPath:     metadataPath,
		DetectedFormat:   doc.format,
		DetectedOrigin:   doc.origin,
		AssignedSchema:   schema,
		SchemaConfidence: assigned.Confidence,
		RecordCount:      recordCount,
		SizeBytes:        doc.sizeBytes,
		ImportTimestamp:  doc.timestamp.Format(time.RFC3339),
		ValidationResult: assigned.Validation,
		Explanation:      assigned.Trace,
		ResourceID:       resourceID,
		ContentHash:      doc.contentHash,
		Version:          version,
		IsNewVersion:     latestVersion > 0,
	}
	idTally.apply(result)
	applyTally(result, tally)
	return result, nil
}

// validationStatus describes how a schema assignment was reached, for the
// import metadata.
func validationStatus(assigned schemaAssignment) string {
	switch {
	case assigned.Validation == nil:
		return "auto-assigned"
	case assigned.Validation.Valid:
		return "validated"
	default:
		return "fallback"
	}
}
