package importer

import (
	"fmt"
	"strings"
	"time"

	"pudl/internal/database"
)

// assignSchema assigns a schema to data using basic rule-based logic
// This is a simplified version that will be replaced with Zygomys rule engine later
func (i *Importer) assignSchema(data interface{}, origin, format string) (string, float64) {
	// Convert data to map for analysis if possible
	var dataMap map[string]interface{}

	switch d := data.(type) {
	case map[string]interface{}:
		dataMap = d
	case []interface{}:
		// If it's an array, try to get the first element
		if len(d) > 0 {
			if firstItem, itemOk := d[0].(map[string]interface{}); itemOk {
				dataMap = firstItem
			}
		}
	}

	// If we couldn't extract a map, use catchall
	if dataMap == nil {
		return "unknown.#CatchAll", 0.1
	}

	// AWS EC2 Instance detection
	if i.hasFields(dataMap, []string{"InstanceId", "State", "InstanceType"}) {
		confidence := 0.9
		if instanceId, exists := dataMap["InstanceId"].(string); exists {
			if strings.HasPrefix(instanceId, "i-") && len(instanceId) >= 10 {
				confidence = 0.95
			}
		}
		return "aws.#EC2Instance", confidence
	}

	// AWS S3 Bucket detection
	if i.hasFields(dataMap, []string{"Name", "CreationDate"}) && 
		strings.Contains(strings.ToLower(origin), "s3") {
		return "aws.#S3Bucket", 0.9
	}

	// Kubernetes resource detection with enhanced schema mapping
	if i.hasFields(dataMap, []string{"kind", "apiVersion", "metadata"}) {
		if kind, exists := dataMap["kind"].(string); exists {
			// Map specific Kubernetes resources to their PUDL schemas
			switch kind {
			case "Pod":
				return "k8s.#Pod", 0.95
			case "Service":
				return "k8s.#Service", 0.95
			case "Deployment":
				return "k8s.#Deployment", 0.95
			case "StatefulSet":
				return "k8s.#StatefulSet", 0.95
			case "DaemonSet":
				return "k8s.#DaemonSet", 0.95
			case "ReplicaSet":
				return "k8s.#ReplicaSet", 0.95
			case "ConfigMap":
				return "k8s.#ConfigMap", 0.95
			case "Secret":
				return "k8s.#Secret", 0.95
			case "PersistentVolume":
				return "k8s.#PersistentVolume", 0.95
			case "PersistentVolumeClaim":
				return "k8s.#PersistentVolumeClaim", 0.95
			case "Ingress":
				return "k8s.#Ingress", 0.95
			case "NetworkPolicy":
				return "k8s.#NetworkPolicy", 0.95
			case "Role":
				return "k8s.#Role", 0.95
			case "ClusterRole":
				return "k8s.#ClusterRole", 0.95
			case "RoleBinding":
				return "k8s.#RoleBinding", 0.95
			case "ClusterRoleBinding":
				return "k8s.#ClusterRoleBinding", 0.95
			case "ServiceAccount":
				return "k8s.#ServiceAccount", 0.95
			case "Job":
				return "k8s.#Job", 0.95
			case "CronJob":
				return "k8s.#CronJob", 0.95
			case "HorizontalPodAutoscaler":
				return "k8s.#HorizontalPodAutoscaler", 0.95
			case "StorageClass":
				return "k8s.#StorageClass", 0.95
			default:
				// Generic Kubernetes resource for unknown kinds
				return "k8s.#Resource", 0.8
			}
		}
		return "k8s.#Resource", 0.7
	}

	// AWS API Response pattern
	if i.hasFields(dataMap, []string{"ResponseMetadata"}) {
		return "aws.#APIResponse", 0.8
	}

	// Origin-based fallback detection
	if strings.Contains(strings.ToLower(origin), "aws") {
		if strings.Contains(strings.ToLower(origin), "ec2") {
			return "aws.#EC2Resource", 0.6
		}
		if strings.Contains(strings.ToLower(origin), "s3") {
			return "aws.#S3Resource", 0.6
		}
		return "aws.#Resource", 0.5
	}

	if strings.Contains(strings.ToLower(origin), "k8s") || 
		strings.Contains(strings.ToLower(origin), "kube") {
		return "k8s.#Resource", 0.5
	}

	// Default to catchall
	return "unknown.#CatchAll", 0.1
}

// hasFields checks if a map contains all specified fields
func (i *Importer) hasFields(data map[string]interface{}, fields []string) bool {
	for _, field := range fields {
		if _, exists := data[field]; !exists {
			return false
		}
	}
	return true
}

// updateCatalog updates the main data catalog with the new import
func (i *Importer) updateCatalog(metadata ImportMetadata, storedPath, metadataPath string) error {
	// Parse timestamp
	importTime, err := time.Parse(time.RFC3339, metadata.ImportMetadata.Timestamp)
	if err != nil {
		return fmt.Errorf("failed to parse import timestamp: %w", err)
	}

	// Create new catalog entry for database
	entry := database.CatalogEntry{
		ID:              metadata.ID,
		StoredPath:      storedPath,
		MetadataPath:    metadataPath,
		ImportTimestamp: importTime,
		Format:          metadata.ImportMetadata.Format,
		Origin:          metadata.SourceInfo.Origin,
		Schema:          metadata.SchemaInfo.CueDefinition,
		Confidence:      0.8, // Default confidence for now
		RecordCount:     metadata.ImportMetadata.RecordCount,
		SizeBytes:       metadata.ImportMetadata.SizeBytes,
	}

	// Add to database
	return i.catalogDB.AddEntry(entry)
}

// Basic schema definitions for reference
// These will be replaced with actual CUE files later
var basicSchemaDefinitions = map[string]SchemaDefinition{
	"aws.#EC2Instance": {
		Package:        "aws",
		Definition:     "#EC2Instance",
		IdentityFields: []string{"InstanceId"},
		TrackedFields:  []string{"State", "PrivateIpAddress", "Tags", "SecurityGroups"},
		Version:        "v1.0",
	},
	"aws.#S3Bucket": {
		Package:        "aws",
		Definition:     "#S3Bucket",
		IdentityFields: []string{"Name"},
		TrackedFields:  []string{"CreationDate", "BucketPolicy", "Tags"},
		Version:        "v1.0",
	},
	"k8s.#Pod": {
		Package:        "k8s",
		Definition:     "#Pod",
		IdentityFields: []string{"metadata.name", "metadata.namespace"},
		TrackedFields:  []string{"status", "spec"},
		Version:        "v1.0",
	},
	"unknown.#CatchAll": {
		Package:        "unknown",
		Definition:     "#CatchAll",
		IdentityFields: []string{},
		TrackedFields:  []string{},
		Version:        "v1.0",
	},
}

// SchemaDefinition represents a schema definition
type SchemaDefinition struct {
	Package        string   `json:"package"`
	Definition     string   `json:"definition"`
	IdentityFields []string `json:"identity_fields"`
	TrackedFields  []string `json:"tracked_fields"`
	Version        string   `json:"version"`
}

// getSchemaDefinition returns the schema definition for a given schema name
func (i *Importer) getSchemaDefinition(schemaName string) (SchemaDefinition, bool) {
	def, exists := basicSchemaDefinitions[schemaName]
	return def, exists
}
