package mubridge

import (
	"testing"
	"time"

	"pudl/internal/drift"
)

func TestExportMuConfig_NoDrift(t *testing.T) {
	results := []*DriftInput{
		{
			Result: &drift.DriftResult{
				Definition: "my_server",
				Status:     "clean",
				Timestamp:  time.Now(),
			},
			SchemaRef: "file.#Config",
		},
	}

	cfg := ExportMuConfig(results, nil)
	if len(cfg.Targets) != 0 {
		t.Fatalf("expected 0 targets for clean drift, got %d", len(cfg.Targets))
	}
}

func TestExportMuConfig_SingleDrift(t *testing.T) {
	results := []*DriftInput{
		{
			Result: &drift.DriftResult{
				Definition: "nginx_conf",
				Status:     "drifted",
				Timestamp:  time.Now(),
				DeclaredKeys: map[string]interface{}{
					"path":    "/etc/nginx/nginx.conf",
					"content": "server { listen 80; }",
					"mode":    "0644",
				},
				Differences: []drift.FieldDiff{
					{Path: "content", Type: "changed", Declared: "server { listen 80; }", Live: "server { listen 8080; }"},
				},
			},
			SchemaRef: "file.#Config",
			Sources:   []string{"definitions/nginx.cue"},
		},
	}

	cfg := ExportMuConfig(results, nil)

	if len(cfg.Targets) != 1 {
		t.Fatalf("expected 1 target, got %d", len(cfg.Targets))
	}

	target := cfg.Targets[0]
	if target.Name != "//nginx_conf" {
		t.Errorf("target name = %q, want %q", target.Name, "//nginx_conf")
	}
	if target.Toolchain != "file" {
		t.Errorf("toolchain = %q, want %q", target.Toolchain, "file")
	}
	if target.Config["path"] != "/etc/nginx/nginx.conf" {
		t.Errorf("config.path = %v, want %q", target.Config["path"], "/etc/nginx/nginx.conf")
	}
	if target.Config["content"] != "server { listen 80; }" {
		t.Errorf("config.content = %v", target.Config["content"])
	}
	if len(target.Sources) != 1 || target.Sources[0] != "definitions/nginx.cue" {
		t.Errorf("sources = %v, want [definitions/nginx.cue]", target.Sources)
	}
}

func TestExportMuConfig_MultipleDrifts(t *testing.T) {
	results := []*DriftInput{
		{
			Result: &drift.DriftResult{
				Definition:   "web_server",
				Status:       "drifted",
				DeclaredKeys: map[string]interface{}{"replicas": float64(3)},
				Differences:  []drift.FieldDiff{{Path: "replicas", Type: "changed"}},
			},
			SchemaRef: "k8s.#Deployment",
		},
		{
			Result: &drift.DriftResult{
				Definition:   "bucket",
				Status:       "drifted",
				DeclaredKeys: map[string]interface{}{"versioning": true},
				Differences:  []drift.FieldDiff{{Path: "versioning", Type: "changed"}},
			},
			SchemaRef: "s3.#Bucket",
		},
		{
			Result: &drift.DriftResult{
				Definition: "clean_thing",
				Status:     "clean",
			},
			SchemaRef: "file.#Config",
		},
	}

	cfg := ExportMuConfig(results, nil)

	if len(cfg.Targets) != 2 {
		t.Fatalf("expected 2 targets (skipping clean), got %d", len(cfg.Targets))
	}

	if cfg.Targets[0].Toolchain != "k8s" {
		t.Errorf("first target toolchain = %q, want k8s", cfg.Targets[0].Toolchain)
	}
	if cfg.Targets[1].Toolchain != "aws" {
		t.Errorf("second target toolchain = %q, want aws", cfg.Targets[1].Toolchain)
	}
}

func TestExportMuConfig_CustomMappings(t *testing.T) {
	results := []*DriftInput{
		{
			Result: &drift.DriftResult{
				Definition:   "my_resource",
				Status:       "drifted",
				DeclaredKeys: map[string]interface{}{"foo": "bar"},
				Differences:  []drift.FieldDiff{{Path: "foo", Type: "changed"}},
			},
			SchemaRef: "mycloud.#Thing",
		},
	}

	mappings := []ToolchainMapping{
		{Prefix: "mycloud", Toolchain: "mycloud-plugin"},
	}

	cfg := ExportMuConfig(results, mappings)

	if len(cfg.Targets) != 1 {
		t.Fatalf("expected 1 target, got %d", len(cfg.Targets))
	}
	if cfg.Targets[0].Toolchain != "mycloud-plugin" {
		t.Errorf("toolchain = %q, want mycloud-plugin", cfg.Targets[0].Toolchain)
	}
}

func TestExportMuConfig_UnknownSchemaFallsBackToGeneric(t *testing.T) {
	results := []*DriftInput{
		{
			Result: &drift.DriftResult{
				Definition:   "mystery",
				Status:       "drifted",
				DeclaredKeys: map[string]interface{}{"x": 1},
				Differences:  []drift.FieldDiff{{Path: "x", Type: "changed"}},
			},
			SchemaRef: "unknown.#Thing",
		},
	}

	cfg := ExportMuConfig(results, nil)

	if cfg.Targets[0].Toolchain != "generic" {
		t.Errorf("toolchain = %q, want generic", cfg.Targets[0].Toolchain)
	}
}

func TestResolveToolchain(t *testing.T) {
	tests := []struct {
		schemaRef string
		want      string
	}{
		{"ec2.#Instance", "aws"},
		{"s3.#Bucket", "aws"},
		{"k8s.#Deployment", "k8s"},
		{"kubernetes.#Pod", "k8s"},
		{"file.#Config", "file"},
		{"config.#AppSettings", "file"},
		{"terraform.#Module", "terraform"},
		{"tf.#Resource", "terraform"},
		{"docker.#Image", "docker"},
		{"container.#Build", "docker"},
		{"shell.#Command", "shell"},
		{"exec.#Script", "shell"},
		{"zig.#Build", "zig"},
		{"unknown.#Foo", "generic"},
	}

	for _, tt := range tests {
		got := resolveToolchain(tt.schemaRef, DefaultMappings)
		if got != tt.want {
			t.Errorf("resolveToolchain(%q) = %q, want %q", tt.schemaRef, got, tt.want)
		}
	}
}

func TestExportMuConfig_BrickToolchain(t *testing.T) {
	results := []*DriftInput{
		{
			Result: &drift.DriftResult{
				Definition: "my_app",
				Status:     "drifted",
				DeclaredKeys: map[string]interface{}{
					"name":      "my-app",
					"kind":      "component",
					"toolchain": "k8s",
					"config": map[string]interface{}{
						"replicas": 3,
						"image":    "nginx:latest",
					},
				},
				Differences: []drift.FieldDiff{{Path: "config.replicas", Type: "changed"}},
			},
			SchemaRef:      "brick.#Target",
			BrickToolchain: "k8s",
		},
	}

	cfg := ExportMuConfig(results, nil)

	if len(cfg.Targets) != 1 {
		t.Fatalf("expected 1 target, got %d", len(cfg.Targets))
	}
	if cfg.Targets[0].Toolchain != "k8s" {
		t.Errorf("toolchain = %q, want %q", cfg.Targets[0].Toolchain, "k8s")
	}
}

func TestExportMuConfig_BrickToolchainPrecedence(t *testing.T) {
	// BrickToolchain should override prefix heuristic even when SchemaRef
	// would match a different toolchain.
	results := []*DriftInput{
		{
			Result: &drift.DriftResult{
				Definition:   "hybrid_target",
				Status:       "drifted",
				DeclaredKeys: map[string]interface{}{"foo": "bar"},
				Differences:  []drift.FieldDiff{{Path: "foo", Type: "changed"}},
			},
			SchemaRef:      "k8s.#Deployment",
			BrickToolchain: "shell",
		},
	}

	cfg := ExportMuConfig(results, nil)

	if len(cfg.Targets) != 1 {
		t.Fatalf("expected 1 target, got %d", len(cfg.Targets))
	}
	if cfg.Targets[0].Toolchain != "shell" {
		t.Errorf("toolchain = %q, want %q (BrickToolchain should win over prefix)", cfg.Targets[0].Toolchain, "shell")
	}
}

func TestExportMuConfig_NoBrickFallback(t *testing.T) {
	// When BrickToolchain is empty, the prefix heuristic should still work.
	results := []*DriftInput{
		{
			Result: &drift.DriftResult{
				Definition:   "web_server",
				Status:       "drifted",
				DeclaredKeys: map[string]interface{}{"replicas": 3},
				Differences:  []drift.FieldDiff{{Path: "replicas", Type: "changed"}},
			},
			SchemaRef:      "k8s.#Deployment",
			BrickToolchain: "",
		},
	}

	cfg := ExportMuConfig(results, nil)

	if len(cfg.Targets) != 1 {
		t.Fatalf("expected 1 target, got %d", len(cfg.Targets))
	}
	if cfg.Targets[0].Toolchain != "k8s" {
		t.Errorf("toolchain = %q, want %q (should fall back to prefix heuristic)", cfg.Targets[0].Toolchain, "k8s")
	}
}

func TestExportMuConfig_BrickConfig(t *testing.T) {
	// When BrickConfig is set, only those fields should appear in config,
	// not the full DeclaredKeys (which include BRICK metadata).
	brickConfig := map[string]any{
		"replicas": 3,
		"image":    "nginx:latest",
	}
	results := []*DriftInput{
		{
			Result: &drift.DriftResult{
				Definition: "my_app",
				Status:     "drifted",
				DeclaredKeys: map[string]interface{}{
					"name":      "my-app",
					"kind":      "component",
					"toolchain": "k8s",
					"config": map[string]interface{}{
						"replicas": 3,
						"image":    "nginx:latest",
					},
				},
				Differences: []drift.FieldDiff{{Path: "config.replicas", Type: "changed"}},
			},
			SchemaRef:      "brick.#Target",
			BrickToolchain: "k8s",
			BrickConfig:    brickConfig,
		},
	}

	cfg := ExportMuConfig(results, nil)

	if len(cfg.Targets) != 1 {
		t.Fatalf("expected 1 target, got %d", len(cfg.Targets))
	}

	target := cfg.Targets[0]

	// Config should only have the BRICK config fields, not metadata.
	if _, ok := target.Config["kind"]; ok {
		t.Error("config should not contain BRICK metadata field 'kind'")
	}
	if _, ok := target.Config["toolchain"]; ok {
		t.Error("config should not contain BRICK metadata field 'toolchain'")
	}
	if _, ok := target.Config["name"]; ok {
		t.Error("config should not contain BRICK metadata field 'name'")
	}

	if target.Config["replicas"] != 3 {
		t.Errorf("config.replicas = %v, want 3", target.Config["replicas"])
	}
	if target.Config["image"] != "nginx:latest" {
		t.Errorf("config.image = %v, want %q", target.Config["image"], "nginx:latest")
	}
}
