//go:build acceptance

package acceptance

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/wait"

	"pudl/internal/executor"
	"pudl/internal/workflow"
)

// mockSSHExecutor simulates SSH-based method execution for acceptance testing.
// In a real deployment, the Glojure .clj files would invoke SSH commands.
type mockSSHExecutor struct {
	sshHost string
	sshPort string
}

func (m *mockSSHExecutor) Run(ctx context.Context, opts executor.RunOptions) (*executor.RunResult, error) {
	switch opts.MethodName {
	case "create_file":
		// Simulate creating a file on the remote host
		return &executor.RunResult{
			MethodName:     opts.MethodName,
			DefinitionName: opts.DefinitionName,
			Output: map[string]interface{}{
				"file_path": "/tmp/pudl_test_file.txt",
				"content":   "hello from pudl workflow",
				"host":      m.sshHost,
				"port":      m.sshPort,
			},
		}, nil

	case "read_file":
		filePath := opts.Tags["file_path"]
		if filePath == "" {
			return nil, fmt.Errorf("file_path not provided")
		}
		return &executor.RunResult{
			MethodName:     opts.MethodName,
			DefinitionName: opts.DefinitionName,
			Output: map[string]interface{}{
				"content":   "hello from pudl workflow",
				"file_path": filePath,
			},
		}, nil

	case "verify_file":
		content := opts.Tags["content"]
		if content == "" {
			return nil, fmt.Errorf("content not provided")
		}
		return &executor.RunResult{
			MethodName:     opts.MethodName,
			DefinitionName: opts.DefinitionName,
			Output: map[string]interface{}{
				"verified": true,
				"content":  content,
			},
		}, nil

	default:
		return nil, fmt.Errorf("unknown method: %s", opts.MethodName)
	}
}

func TestWorkflowSSHPipeline(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping acceptance test in short mode")
	}

	ctx := context.Background()

	// Start SSHD container
	req := testcontainers.ContainerRequest{
		Image:        "lscr.io/linuxserver/openssh-server:latest",
		ExposedPorts: []string{"2222/tcp"},
		Env: map[string]string{
			"PUID":            "1000",
			"PGID":            "1000",
			"PASSWORD_ACCESS": "true",
			"USER_PASSWORD":   "testpass",
			"USER_NAME":       "testuser",
		},
		WaitingFor: wait.ForListeningPort("2222/tcp").WithStartupTimeout(60 * time.Second),
	}

	container, err := testcontainers.GenericContainer(ctx, testcontainers.GenericContainerRequest{
		ContainerRequest: req,
		Started:          true,
	})
	if err != nil {
		t.Fatalf("failed to start SSHD container: %v", err)
	}
	defer container.Terminate(ctx)

	host, err := container.Host(ctx)
	if err != nil {
		t.Fatalf("failed to get container host: %v", err)
	}

	port, err := container.MappedPort(ctx, "2222")
	if err != nil {
		t.Fatalf("failed to get mapped port: %v", err)
	}

	t.Logf("SSHD container running at %s:%s", host, port.Port())

	// Create temp schema dir with workflow CUE
	schemaDir := t.TempDir()
	wfDir := filepath.Join(schemaDir, "workflows")
	os.MkdirAll(wfDir, 0755)

	workflowCUE := `package workflows

file_pipeline: {
    description: "Create and verify files on remote host"
    abort_on_failure: true

    steps: {
        create_file: {
            definition: "test_file_writer"
            method:     "create_file"
            timeout:    "30s"
        }
        read_file: {
            definition: "test_file_reader"
            method:     "read_file"
            inputs: {
                file_path: steps.create_file.outputs.file_path
            }
        }
        verify: {
            definition: "test_file_verifier"
            method:     "verify_file"
            inputs: {
                content: steps.read_file.outputs.content
            }
        }
    }
}
`
	os.WriteFile(filepath.Join(wfDir, "pipeline.cue"), []byte(workflowCUE), 0644)

	// Discover the workflow
	disc := workflow.NewDiscoverer(schemaDir)
	wf, err := disc.GetWorkflow("file_pipeline")
	if err != nil {
		t.Fatalf("failed to discover workflow: %v", err)
	}

	// Validate the workflow
	dag, err := workflow.BuildDAG(wf)
	if err != nil {
		t.Fatalf("failed to build DAG: %v", err)
	}
	order, err := dag.TopologicalSort()
	if err != nil {
		t.Fatalf("topo sort failed: %v", err)
	}
	t.Logf("execution order: %v", order)

	// Run the workflow with mock SSH executor
	dataDir := t.TempDir()
	mockExec := &mockSSHExecutor{
		sshHost: host,
		sshPort: port.Port(),
	}

	runner := workflow.NewRunner(mockExec, nil, dataDir)
	result, err := runner.Run(ctx, wf, workflow.RunOptions{})
	if err != nil {
		t.Fatalf("workflow run failed: %v", err)
	}

	// Verify all steps succeeded
	if result.Status != "success" {
		t.Errorf("expected workflow status 'success', got %q", result.Status)
		for name, sr := range result.Steps {
			t.Logf("step %s: status=%s error=%s", name, sr.Status, sr.Error)
		}
	}

	// Verify step count
	if len(result.Steps) != 3 {
		t.Errorf("expected 3 step results, got %d", len(result.Steps))
	}

	// Verify individual step results
	for _, name := range []string{"create_file", "read_file", "verify"} {
		sr := result.Steps[name]
		if sr == nil {
			t.Errorf("missing result for step %q", name)
			continue
		}
		if sr.Status != "success" {
			t.Errorf("step %s: expected success, got %q (error: %s)", name, sr.Status, sr.Error)
		}
	}

	// Verify output threading: read_file should have received file_path from create_file
	if readResult := result.Steps["read_file"]; readResult != nil {
		if output, ok := readResult.Output.(map[string]interface{}); ok {
			if fp, ok := output["file_path"]; ok {
				if fp != "/tmp/pudl_test_file.txt" {
					t.Errorf("expected file_path '/tmp/pudl_test_file.txt', got %q", fp)
				}
			}
		}
	}

	// Verify manifest was saved
	store := workflow.NewManifestStore(dataDir)
	ids, err := store.List("file_pipeline")
	if err != nil {
		t.Fatalf("failed to list manifests: %v", err)
	}
	if len(ids) != 1 {
		t.Errorf("expected 1 manifest, got %d", len(ids))
	}

	// Load and verify manifest
	if len(ids) > 0 {
		manifest, err := store.Get("file_pipeline", ids[0])
		if err != nil {
			t.Fatalf("failed to get manifest: %v", err)
		}
		if manifest.Status != "success" {
			t.Errorf("manifest status: expected 'success', got %q", manifest.Status)
		}
		if len(manifest.Steps) != 3 {
			t.Errorf("manifest steps: expected 3, got %d", len(manifest.Steps))
		}
	}

	t.Logf("Workflow completed in %s", result.Duration)
}
