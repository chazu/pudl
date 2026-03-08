package executor

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"pudl/internal/definition"
	"pudl/internal/glojure"
	"pudl/internal/model"
)

// testSetup creates a temporary schema directory with model, definition, and
// method files, returning an Executor ready for testing.
type testEnv struct {
	executor   *Executor
	schemaPath string
	methodsDir string
}

func setupTestEnv(t *testing.T) *testEnv {
	t.Helper()
	tmpDir := t.TempDir()

	// Create directory structure — model goes in examples/ so the discoverer
	// produces name "examples.#TestModel" matching the definition's ModelRef
	modelDir := filepath.Join(tmpDir, "examples")
	defDir := filepath.Join(tmpDir, "definitions")
	methodsDir := filepath.Join(tmpDir, "methods", "test_model")
	for _, d := range []string{modelDir, defDir, methodsDir} {
		if err := os.MkdirAll(d, 0755); err != nil {
			t.Fatal(err)
		}
	}

	// Write a test model CUE file
	modelCUE := `package examples

#TestModel: #Model & {
	metadata: {
		name:        "test_model"
		description: "Test model"
		category:    "test"
	}
	methods: {
		do_thing: #Method & {
			kind:        "action"
			description: "Do the thing"
		}
		check_ready: #Method & {
			kind:        "qualification"
			description: "Check readiness"
			blocks: ["do_thing"]
		}
		compute_attr: #Method & {
			kind:        "attribute"
			description: "Compute an attribute"
		}
	}
}
`
	if err := os.WriteFile(filepath.Join(modelDir, "test.cue"), []byte(modelCUE), 0644); err != nil {
		t.Fatal(err)
	}

	// Write a test definition CUE file
	defCUE := `package definitions

test_def: examples.#TestModel & {
	config: {
		value: "hello"
	}
}
`
	if err := os.WriteFile(filepath.Join(defDir, "test_def.cue"), []byte(defCUE), 0644); err != nil {
		t.Fatal(err)
	}

	// Initialize Glojure runtime
	rt := glojure.New()
	if err := rt.Init(); err != nil {
		t.Fatal(err)
	}
	registry := glojure.NewRegistry(rt)

	modelDisc := model.NewDiscoverer(tmpDir)
	defDisc := definition.NewDiscoverer(tmpDir)

	exec := New(rt, registry, modelDisc, defDisc, filepath.Join(tmpDir, "methods"))

	return &testEnv{
		executor:   exec,
		schemaPath: tmpDir,
		methodsDir: methodsDir,
	}
}

// writeMethod writes a .clj file into the test methods directory.
func (te *testEnv) writeMethod(t *testing.T, name, code string) {
	t.Helper()
	path := filepath.Join(te.methodsDir, name+".clj")
	if err := os.WriteFile(path, []byte(code), 0644); err != nil {
		t.Fatal(err)
	}
}

func TestRunSimpleMethod(t *testing.T) {
	env := setupTestEnv(t)
	env.writeMethod(t, "do_thing", `(defn run [args] "action completed")`)
	env.writeMethod(t, "check_ready", `(defn run [args] {:passed true :message "ready"})`)

	result, err := env.executor.Run(context.Background(), RunOptions{
		DefinitionName: "test_def",
		MethodName:     "do_thing",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result.Output != "action completed" {
		t.Errorf("expected output %q, got %v", "action completed", result.Output)
	}
	if result.MethodName != "do_thing" {
		t.Errorf("expected method name %q, got %q", "do_thing", result.MethodName)
	}
	if result.DefinitionName != "test_def" {
		t.Errorf("expected definition name %q, got %q", "test_def", result.DefinitionName)
	}
}

func TestQualificationPasses(t *testing.T) {
	env := setupTestEnv(t)
	env.writeMethod(t, "check_ready", `(defn run [args] {:passed true :message "all good"})`)
	env.writeMethod(t, "do_thing", `(defn run [args] "done")`)

	result, err := env.executor.Run(context.Background(), RunOptions{
		DefinitionName: "test_def",
		MethodName:     "do_thing",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(result.Qualifications) != 1 {
		t.Fatalf("expected 1 qualification, got %d", len(result.Qualifications))
	}
	if !result.Qualifications[0].Passed {
		t.Error("expected qualification to pass")
	}
	if result.Qualifications[0].Message != "all good" {
		t.Errorf("expected message %q, got %q", "all good", result.Qualifications[0].Message)
	}
	if result.Output != "done" {
		t.Errorf("expected output %q, got %v", "done", result.Output)
	}
}

func TestQualificationFails(t *testing.T) {
	env := setupTestEnv(t)
	env.writeMethod(t, "check_ready", `(defn run [args] {:passed false :message "not ready"})`)
	env.writeMethod(t, "do_thing", `(defn run [args] "should not run")`)

	result, err := env.executor.Run(context.Background(), RunOptions{
		DefinitionName: "test_def",
		MethodName:     "do_thing",
	})
	if err == nil {
		t.Fatal("expected error from failed qualification")
	}
	if result == nil {
		t.Fatal("expected result even on failure")
	}
	if result.Qualifications[0].Passed {
		t.Error("expected qualification to fail")
	}
	if result.Output != nil {
		t.Error("expected no output when qualification fails")
	}
}

func TestDryRun(t *testing.T) {
	env := setupTestEnv(t)
	env.writeMethod(t, "check_ready", `(defn run [args] {:passed true :message "ok"})`)
	// do_thing .clj intentionally not created — dry-run shouldn't need it

	result, err := env.executor.Run(context.Background(), RunOptions{
		DefinitionName: "test_def",
		MethodName:     "do_thing",
		DryRun:         true,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Output != nil {
		t.Error("expected no output in dry-run")
	}
	if len(result.Qualifications) != 1 {
		t.Fatalf("expected 1 qualification, got %d", len(result.Qualifications))
	}
}

func TestSkipAdvice(t *testing.T) {
	env := setupTestEnv(t)
	// No check_ready .clj — skip-advice means it won't be called
	env.writeMethod(t, "do_thing", `(defn run [args] "skipped advice")`)

	result, err := env.executor.Run(context.Background(), RunOptions{
		DefinitionName: "test_def",
		MethodName:     "do_thing",
		SkipAdvice:     true,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result.Qualifications) != 0 {
		t.Errorf("expected 0 qualifications with skip-advice, got %d", len(result.Qualifications))
	}
	if result.Output != "skipped advice" {
		t.Errorf("expected output %q, got %v", "skipped advice", result.Output)
	}
}

func TestMethodNotFound(t *testing.T) {
	env := setupTestEnv(t)
	// No .clj files at all

	_, err := env.executor.Run(context.Background(), RunOptions{
		DefinitionName: "test_def",
		MethodName:     "do_thing",
		SkipAdvice:     true, // skip so we go straight to the action
	})
	if err == nil {
		t.Fatal("expected error for missing method file")
	}
}

func TestMethodList(t *testing.T) {
	env := setupTestEnv(t)
	env.writeMethod(t, "do_thing", `(defn run [args] nil)`)
	// check_ready has no .clj file

	methods, err := env.executor.ListMethods("test_def")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(methods) != 3 {
		t.Fatalf("expected 3 methods, got %d", len(methods))
	}

	found := map[string]MethodStatus{}
	for _, m := range methods {
		found[m.Name] = m
	}

	if !found["do_thing"].HasImplementation {
		t.Error("expected do_thing to have implementation")
	}
	if found["check_ready"].HasImplementation {
		t.Error("expected check_ready to not have implementation")
	}
	if found["do_thing"].Kind != "action" {
		t.Errorf("expected do_thing kind=action, got %q", found["do_thing"].Kind)
	}
	if found["check_ready"].Kind != "qualification" {
		t.Errorf("expected check_ready kind=qualification, got %q", found["check_ready"].Kind)
	}
}

func TestLifecycleOrder(t *testing.T) {
	env := setupTestEnv(t)

	// Track execution order by returning sequence numbers
	env.writeMethod(t, "check_ready", `(defn run [args] {:passed true :message "qual ran"})`)
	env.writeMethod(t, "do_thing", `(defn run [args] "action ran")`)
	env.writeMethod(t, "compute_attr", `(defn run [args] "attr ran")`)

	result, err := env.executor.Run(context.Background(), RunOptions{
		DefinitionName: "test_def",
		MethodName:     "do_thing",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Qualification should have run
	if len(result.Qualifications) != 1 {
		t.Fatalf("expected 1 qualification, got %d", len(result.Qualifications))
	}
	if !result.Qualifications[0].Passed {
		t.Error("qualification should have passed")
	}

	// Action should have run
	if result.Output != "action ran" {
		t.Errorf("expected output %q, got %v", "action ran", result.Output)
	}

	// Post-action should have run
	if len(result.PostActions) != 1 {
		t.Fatalf("expected 1 post-action, got %d", len(result.PostActions))
	}
	if result.PostActions[0].Name != "compute_attr" {
		t.Errorf("expected post-action name %q, got %q", "compute_attr", result.PostActions[0].Name)
	}
	if result.PostActions[0].Error != nil {
		t.Errorf("unexpected post-action error: %v", result.PostActions[0].Error)
	}
}

func TestTagsPassedToArgs(t *testing.T) {
	env := setupTestEnv(t)
	env.writeMethod(t, "do_thing", `(defn run [args] (get args "env"))`)

	result, err := env.executor.Run(context.Background(), RunOptions{
		DefinitionName: "test_def",
		MethodName:     "do_thing",
		SkipAdvice:     true,
		Tags:           map[string]string{"env": "staging"},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Output != "staging" {
		t.Errorf("expected output %q, got %v", "staging", result.Output)
	}
}
