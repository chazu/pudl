package executor

import (
	"context"
	"fmt"
	"strings"

	"github.com/glojurelang/glojure/pkg/lang"

	"pudl/internal/definition"
	"pudl/internal/glojure"
	"pudl/internal/model"
	"pudl/internal/vault"
)

// Executor orchestrates method execution with lifecycle dispatch.
// It loads .clj method files, runs qualifications before actions,
// and executes post-actions (attribute/codegen) after.
type Executor struct {
	runtime    *glojure.Runtime
	registry   *glojure.Registry
	modelDisc  *model.Discoverer
	defDisc    *definition.Discoverer
	methodsDir string // base dir for .clj files
	vault      vault.Vault
}

// RunOptions configures a method execution.
type RunOptions struct {
	DefinitionName string
	MethodName     string
	DryRun         bool              // run qualifications only
	SkipAdvice     bool              // skip qualifications
	Tags           map[string]string // extra args merged into method args
}

// RunResult holds the outcome of a method execution.
type RunResult struct {
	MethodName     string
	DefinitionName string
	Output         interface{}
	Qualifications []QualificationOutcome
	PostActions    []PostActionOutcome
	Effects        []Effect
	EffectOutcomes []EffectOutcome
}

// QualificationOutcome records whether a qualification passed.
type QualificationOutcome struct {
	Name    string
	Passed  bool
	Message string
}

// PostActionOutcome records the result of a post-action method.
type PostActionOutcome struct {
	Name   string
	Output interface{}
	Error  error
}

// New creates a new Executor. The vault parameter is optional (may be nil).
func New(rt *glojure.Runtime, reg *glojure.Registry, modelDisc *model.Discoverer, defDisc *definition.Discoverer, methodsDir string, v vault.Vault) *Executor {
	return &Executor{
		runtime:    rt,
		registry:   reg,
		modelDisc:  modelDisc,
		defDisc:    defDisc,
		methodsDir: methodsDir,
		vault:      v,
	}
}

// Run executes a method on a definition with full lifecycle dispatch.
//
// Flow:
//  1. Look up definition and its model
//  2. Resolve lifecycle (qualifications, action, post-actions)
//  3. Run qualifications (unless SkipAdvice); abort if any fail
//  4. If DryRun, return after qualifications
//  5. Execute the action .clj file
//  6. Run post-actions
//  7. Return RunResult
func (e *Executor) Run(ctx context.Context, opts RunOptions) (*RunResult, error) {
	// Look up definition
	def, err := e.defDisc.GetDefinition(opts.DefinitionName)
	if err != nil {
		return nil, fmt.Errorf("definition %q: %w", opts.DefinitionName, err)
	}

	// Look up model
	modelInfo, err := e.modelDisc.GetModel(def.SchemaRef)
	if err != nil {
		return nil, fmt.Errorf("model %q for definition %q: %w", def.SchemaRef, opts.DefinitionName, err)
	}

	// Resolve lifecycle
	lifecycle, err := model.ResolveLifecycle(modelInfo, opts.MethodName)
	if err != nil {
		return nil, fmt.Errorf("lifecycle for %q: %w", opts.MethodName, err)
	}

	// Build args from definition
	args := e.resolveArgs(def, opts.Tags)

	result := &RunResult{
		MethodName:     opts.MethodName,
		DefinitionName: opts.DefinitionName,
	}

	// Run qualifications (advice)
	if !opts.SkipAdvice {
		for _, qualName := range lifecycle.Qualifications {
			outcome, err := e.runQualification(ctx, modelInfo, qualName, args)
			if err != nil {
				return nil, fmt.Errorf("qualification %q: %w", qualName, err)
			}
			result.Qualifications = append(result.Qualifications, *outcome)
			if !outcome.Passed {
				return result, fmt.Errorf("qualification %q failed: %s", qualName, outcome.Message)
			}
		}
	}

	// If dry-run, return after qualifications
	if opts.DryRun {
		return result, nil
	}

	// Execute the action
	output, err := e.loadAndRun(ctx, modelInfo.Metadata.Name, opts.MethodName, args)
	if err != nil {
		return nil, fmt.Errorf("method %q: %w", opts.MethodName, err)
	}
	result.Output = output

	// Check for effects in output
	if effects, found := ParseEffects(output); found {
		result.Effects = effects
		if opts.DryRun {
			// In dry-run mode, list effects but don't execute them
			for _, e := range effects {
				result.EffectOutcomes = append(result.EffectOutcomes, EffectOutcome{
					Effect: e,
					Status: "skipped",
				})
			}
		}
	}

	// Run post-actions
	for _, postName := range lifecycle.PostActions {
		postOutput, postErr := e.loadAndRun(ctx, modelInfo.Metadata.Name, postName, args)
		result.PostActions = append(result.PostActions, PostActionOutcome{
			Name:   postName,
			Output: postOutput,
			Error:  postErr,
		})
	}

	return result, nil
}

// runQualification executes a qualification method and interprets its result.
// The .clj file must return a map with :passed (bool) and :message (string).
func (e *Executor) runQualification(ctx context.Context, modelInfo *model.ModelInfo, qualName string, args map[string]interface{}) (*QualificationOutcome, error) {
	raw, err := e.loadAndRun(ctx, modelInfo.Metadata.Name, qualName, args)
	if err != nil {
		return nil, err
	}

	return parseQualificationResult(qualName, raw)
}

// parseQualificationResult extracts passed/message from a qualification return value.
func parseQualificationResult(name string, raw interface{}) (*QualificationOutcome, error) {
	outcome := &QualificationOutcome{Name: name}

	m, ok := toStringKeyMap(raw)
	if !ok {
		return nil, fmt.Errorf("qualification %q must return a map with :passed and :message, got %T", name, raw)
	}

	passed, ok := m["passed"]
	if !ok {
		return nil, fmt.Errorf("qualification %q result missing :passed key", name)
	}
	passedBool, ok := passed.(bool)
	if !ok {
		return nil, fmt.Errorf("qualification %q :passed must be bool, got %T", name, passed)
	}
	outcome.Passed = passedBool

	if msg, ok := m["message"]; ok {
		if msgStr, ok := msg.(string); ok {
			outcome.Message = msgStr
		}
	}

	return outcome, nil
}

// toStringKeyMap converts a Glojure map (IPersistentMap or Go map) to map[string]interface{}.
// Glojure keyword keys like :passed become "passed" (without the colon).
func toStringKeyMap(v interface{}) (map[string]interface{}, bool) {
	// Direct Go maps
	if m, ok := v.(map[string]interface{}); ok {
		return m, true
	}
	if m, ok := v.(map[interface{}]interface{}); ok {
		result := make(map[string]interface{}, len(m))
		for k, val := range m {
			result[keyString(k)] = val
		}
		return result, true
	}

	// Glojure persistent map — iterate via lang.Seq
	if _, ok := v.(lang.Seqable); ok {
		result := make(map[string]interface{})
		for seq := lang.Seq(v); seq != nil; seq = seq.Next() {
			entry, ok := seq.First().(lang.IMapEntry)
			if !ok {
				continue
			}
			result[keyString(entry.Key())] = entry.Val()
		}
		if len(result) > 0 {
			return result, true
		}
	}

	return nil, false
}

// keyString converts a Glojure key to a plain string.
// Keywords (:foo) have their Name() return the interned value which may
// include the colon prefix — we strip it for ergonomic Go map access.
func keyString(k interface{}) string {
	if named, ok := k.(lang.Named); ok {
		name := named.Name()
		return strings.TrimPrefix(name, ":")
	}
	s := fmt.Sprint(k)
	return strings.TrimPrefix(s, ":")
}

// MethodStatus describes whether a method has an implementation file.
type MethodStatus struct {
	Name           string
	Kind           string
	Description    string
	HasImplementation bool
}

// ListMethods returns the methods for a definition's model with implementation status.
func (e *Executor) ListMethods(defName string) ([]MethodStatus, error) {
	def, err := e.defDisc.GetDefinition(defName)
	if err != nil {
		return nil, fmt.Errorf("definition %q: %w", defName, err)
	}

	modelInfo, err := e.modelDisc.GetModel(def.SchemaRef)
	if err != nil {
		return nil, fmt.Errorf("model %q: %w", def.SchemaRef, err)
	}

	var methods []MethodStatus
	for name, m := range modelInfo.Methods {
		path := e.methodFilePath(modelInfo.Metadata.Name, name)
		methods = append(methods, MethodStatus{
			Name:              name,
			Kind:              m.Kind,
			Description:       m.Description,
			HasImplementation: fileExists(path),
		})
	}

	return methods, nil
}
