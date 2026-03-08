package workflow

import (
	"fmt"
	"regexp"
	"sort"
)

// DAG represents the dependency graph between workflow steps.
// An edge from A to B means "A depends on B" (B must complete before A starts).
type DAG struct {
	nodes map[string]bool
	deps  map[string]map[string]bool // node -> set of nodes it depends on
	rdeps map[string]map[string]bool // node -> set of nodes that depend on it
}

// stepRefRe matches references like steps.create_file.outputs.file_path or steps.create_file.status
var stepRefRe = regexp.MustCompile(`steps\.(\w+)\.(?:outputs|status)`)

// BuildDAG constructs a dependency graph from a workflow's step references.
func BuildDAG(wf *Workflow) (*DAG, error) {
	d := &DAG{
		nodes: make(map[string]bool),
		deps:  make(map[string]map[string]bool),
		rdeps: make(map[string]map[string]bool),
	}

	// Register all steps as nodes
	for name := range wf.Steps {
		d.nodes[name] = true
	}

	// Build edges from step input references
	for name, step := range wf.Steps {
		refs := extractStepRefs(step)
		for _, ref := range refs {
			if !d.nodes[ref] {
				return nil, fmt.Errorf("step %q references unknown step %q", name, ref)
			}
			if ref == name {
				return nil, fmt.Errorf("step %q references itself", name)
			}
			d.addDep(name, ref)
		}
	}

	return d, nil
}

// addDep records that `from` depends on `dep`.
func (d *DAG) addDep(from, dep string) {
	if d.deps[from] == nil {
		d.deps[from] = make(map[string]bool)
	}
	if d.rdeps[dep] == nil {
		d.rdeps[dep] = make(map[string]bool)
	}
	d.deps[from][dep] = true
	d.rdeps[dep][from] = true
}

// TopologicalSort returns steps in dependency order using Kahn's algorithm.
// Returns an error if a cycle is detected.
func (d *DAG) TopologicalSort() ([]string, error) {
	if len(d.nodes) == 0 {
		return nil, nil
	}

	inDegree := make(map[string]int)
	for node := range d.nodes {
		inDegree[node] = len(d.deps[node])
	}

	var queue []string
	for node := range d.nodes {
		if inDegree[node] == 0 {
			queue = append(queue, node)
		}
	}
	sort.Strings(queue)

	var result []string

	for len(queue) > 0 {
		node := queue[0]
		queue = queue[1:]
		result = append(result, node)

		dependents := d.rdeps[node]
		for dep := range dependents {
			inDegree[dep]--
			if inDegree[dep] == 0 {
				inserted := false
				for i, q := range queue {
					if dep < q {
						queue = append(queue[:i+1], queue[i:]...)
						queue[i] = dep
						inserted = true
						break
					}
				}
				if !inserted {
					queue = append(queue, dep)
				}
			}
		}
	}

	if len(result) != len(d.nodes) {
		return nil, fmt.Errorf("cycle detected in workflow step dependencies")
	}

	return result, nil
}

// GetReadySteps returns steps whose dependencies are all in the completed set.
func (d *DAG) GetReadySteps(completed map[string]bool) []string {
	var ready []string
	for node := range d.nodes {
		if completed[node] {
			continue
		}
		allDepsComplete := true
		for dep := range d.deps[node] {
			if !completed[dep] {
				allDepsComplete = false
				break
			}
		}
		if allDepsComplete {
			ready = append(ready, node)
		}
	}
	sort.Strings(ready)
	return ready
}

// GetDependencies returns the steps that the given step depends on.
func (d *DAG) GetDependencies(name string) []string {
	deps := d.deps[name]
	if len(deps) == 0 {
		return nil
	}
	var result []string
	for dep := range deps {
		result = append(result, dep)
	}
	sort.Strings(result)
	return result
}

// extractStepRefs extracts referenced step names from a step's inputs and condition.
func extractStepRefs(step Step) []string {
	seen := make(map[string]bool)
	var refs []string

	// Check inputs
	for _, val := range step.Inputs {
		matches := stepRefRe.FindAllStringSubmatch(val, -1)
		for _, m := range matches {
			if !seen[m[1]] {
				seen[m[1]] = true
				refs = append(refs, m[1])
			}
		}
	}

	// Check condition
	if step.Condition != "" {
		matches := stepRefRe.FindAllStringSubmatch(step.Condition, -1)
		for _, m := range matches {
			if !seen[m[1]] {
				seen[m[1]] = true
				refs = append(refs, m[1])
			}
		}
	}

	sort.Strings(refs)
	return refs
}
