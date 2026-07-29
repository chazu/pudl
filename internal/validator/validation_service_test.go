package validator

import (
	"os"
	"path/filepath"
	"testing"
)

func TestValidationServiceValidatesTypedOpenSchema(t *testing.T) {
	root := writeModuleDir(t, "k8s", "k8s.cue", `package k8s

#Resource: {
	_pudl: {
		schema_type: "base"
		resource_type: "k8s.resource"
	}
	apiVersion: string
	kind: string
	metadata: {
		name: string
		...
	}
	...
}
`)

	service, err := NewValidationService(root)
	if err != nil {
		t.Fatal(err)
	}
	result := service.ValidateDataAgainstSchema(map[string]any{
		"_schema":    "k8s.resource",
		"apiVersion": "v1",
		"kind":       "Pod",
		"metadata":   map[string]any{"name": "fixture"},
	}, "k8s.#Resource")

	if !result.Valid {
		t.Fatalf("typed record did not validate: %+v", result)
	}
	if result.AssignedSchema != "k8s.#Resource" {
		t.Fatalf("assigned schema = %q, want k8s.#Resource", result.AssignedSchema)
	}
}

func TestValidationServiceValidatesBootstrapK8sRecord(t *testing.T) {
	content, err := os.ReadFile(filepath.Join("..", "importer", "bootstrap", "pudl", "k8s", "k8s.cue"))
	if err != nil {
		t.Fatal(err)
	}
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "cue.mod"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "cue.mod", "module.cue"), []byte("module: \"pudl.schemas@v0\"\nlanguage: version: \"v0.16.0\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(root, "pudl", "k8s"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "pudl", "k8s", "k8s.cue"), content, 0o644); err != nil {
		t.Fatal(err)
	}

	service, err := NewValidationService(root)
	if err != nil {
		t.Fatal(err)
	}
	result := service.ValidateDataAgainstSchema(map[string]any{
		"_schema":    "k8s.resource",
		"apiVersion": "v1",
		"kind":       "Pod",
		"metadata": map[string]any{
			"name":      "fixture-pod",
			"namespace": "default",
			"uid":       "uid-fixture",
		},
		"spec": map[string]any{
			"containers": []any{map[string]any{"name": "app", "image": "fixture:latest"}},
		},
		"status": map[string]any{"phase": "Running"},
	}, "pudl/k8s.#Resource")

	if !result.Valid {
		t.Fatalf("bootstrap k8s record did not validate: %+v", result)
	}
}

func TestValidationServiceValidatesBootstrapAWSInstanceWithNullOptionals(t *testing.T) {
	content, err := os.ReadFile(filepath.Join("..", "importer", "bootstrap", "pudl", "aws", "aws.cue"))
	if err != nil {
		t.Fatal(err)
	}
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "cue.mod"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "cue.mod", "module.cue"), []byte("module: \"pudl.schemas@v0\"\nlanguage: version: \"v0.16.0\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(root, "pudl", "aws"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "pudl", "aws", "aws.cue"), content, 0o644); err != nil {
		t.Fatal(err)
	}

	service, err := NewValidationService(root)
	if err != nil {
		t.Fatal(err)
	}
	result := service.ValidateDataAgainstSchema(map[string]any{
		"_schema":         "aws.ec2.instance",
		"instance_id":     "i-fixture",
		"instance_type":   "t3.micro",
		"state":           "running",
		"vpc_id":          nil,
		"subnet_id":       nil,
		"private_ip":      "10.0.0.5",
		"public_ip":       nil,
		"image_id":        nil,
		"tags":            []any{},
		"security_groups": []any{},
		"iam_profile":     nil,
	}, "pudl/aws.#Instance")

	if !result.Valid {
		t.Fatalf("bootstrap AWS instance did not validate: %+v", result)
	}
}

func TestValidationServiceReportsMissingSchemaClearly(t *testing.T) {
	root := writeModuleDir(t, "k8s", "k8s.cue", `package k8s

#Resource: {name: string}
`)
	service, err := NewValidationService(root)
	if err != nil {
		t.Fatal(err)
	}

	result := service.ValidateDataAgainstSchema(map[string]any{"name": "fixture"}, "pudl.schemas/pudl/missing@v0:#Thing")
	if result.Valid {
		t.Fatal("missing schema unexpectedly validated")
	}
	if result.SchemaAvailable {
		t.Fatal("missing schema reported as available")
	}
	if result.SchemaName != "pudl/missing.#Thing" {
		t.Fatalf("schema name = %q, want canonical missing schema", result.SchemaName)
	}
	if result.ErrorMessage != "schema pudl/missing.#Thing is not loaded" {
		t.Fatalf("error = %q, want explicit missing-schema diagnostic", result.ErrorMessage)
	}
}
