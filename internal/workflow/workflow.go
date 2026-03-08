package workflow

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// Workflow represents a parsed workflow definition from a CUE file.
type Workflow struct {
	Name           string
	Description    string
	FilePath       string
	AbortOnFailure bool
	Steps          map[string]Step
}

// Step represents a single step in a workflow.
type Step struct {
	Name       string
	Definition string
	Method     string
	Condition  string
	Inputs     map[string]string
	Timeout    time.Duration
	Retries    int
}

// Discoverer finds and parses workflow definitions from CUE files.
type Discoverer struct {
	schemaPath string
}

// NewDiscoverer creates a new workflow discoverer.
func NewDiscoverer(schemaPath string) *Discoverer {
	return &Discoverer{schemaPath: schemaPath}
}

// ListWorkflows finds all workflows in the schema path under workflows/*.cue.
func (d *Discoverer) ListWorkflows() ([]Workflow, error) {
	workflowDir := filepath.Join(d.schemaPath, "workflows")
	if _, err := os.Stat(workflowDir); os.IsNotExist(err) {
		return nil, nil
	}

	var workflows []Workflow

	err := filepath.Walk(workflowDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() || !strings.HasSuffix(info.Name(), ".cue") {
			return nil
		}

		fileWorkflows, err := d.parseWorkflowsFromFile(path)
		if err != nil {
			return nil // Skip files that can't be parsed
		}
		workflows = append(workflows, fileWorkflows...)
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("failed to walk workflows directory: %w", err)
	}

	return workflows, nil
}

// GetWorkflow finds a specific workflow by name.
func (d *Discoverer) GetWorkflow(name string) (*Workflow, error) {
	workflows, err := d.ListWorkflows()
	if err != nil {
		return nil, err
	}

	for _, wf := range workflows {
		if wf.Name == name {
			return &wf, nil
		}
	}

	return nil, fmt.Errorf("workflow not found: %s", name)
}

var (
	definitionRe = regexp.MustCompile(`definition:\s*"([^"]+)"`)
	methodRe     = regexp.MustCompile(`method:\s*"([^"]+)"`)
	conditionRe  = regexp.MustCompile(`condition:\s*"([^"]+)"`)
	wfTimeoutRe  = regexp.MustCompile(`timeout:\s*"([^"]+)"`)
	wfRetriesRe  = regexp.MustCompile(`retries:\s*(\d+)`)
	wfDescRe     = regexp.MustCompile(`description:\s*"([^"]+)"`)
	abortRe      = regexp.MustCompile(`abort_on_failure:\s*(true|false)`)
	inputEntryRe = regexp.MustCompile(`(\w+):\s*(?:steps\.(\w+)\.(?:outputs|status)\.?(\w*)|"([^"]*)")`)
	stepBlockRe  = regexp.MustCompile(`(?m)^\s+(\w+):\s*\{`)
)

// parseWorkflowsFromFile extracts workflow definitions from a CUE file.
func (d *Discoverer) parseWorkflowsFromFile(filePath string) ([]Workflow, error) {
	content, err := os.ReadFile(filePath)
	if err != nil {
		return nil, err
	}

	text := string(content)
	var workflows []Workflow

	// Find top-level workflow definitions: identifiers followed by { at depth 0
	topLevelRe := regexp.MustCompile(`(?m)^(\w+)\s*:\s*\{`)
	matches := topLevelRe.FindAllStringSubmatchIndex(text, -1)

	for _, match := range matches {
		name := text[match[2]:match[3]]
		if name == "package" {
			continue
		}

		body := extractBody(text, match[0])
		if body == "" {
			continue
		}

		// Must have a steps section to be a workflow
		stepsIdx := strings.Index(body, "steps:")
		if stepsIdx < 0 {
			continue
		}

		wf := Workflow{
			Name:           name,
			FilePath:       filePath,
			AbortOnFailure: true, // default
			Steps:          make(map[string]Step),
		}

		if m := wfDescRe.FindStringSubmatch(body[:stepsIdx]); len(m) > 1 {
			wf.Description = m[1]
		}
		if m := abortRe.FindStringSubmatch(body); len(m) > 1 {
			wf.AbortOnFailure = m[1] == "true"
		}

		// Parse steps section
		stepsBody := extractSection(body, stepsIdx)
		parseSteps(stepsBody, &wf)

		workflows = append(workflows, wf)
	}

	return workflows, nil
}

// parseSteps extracts individual steps from the steps section body.
func parseSteps(stepsBody string, wf *Workflow) {
	matches := stepBlockRe.FindAllStringSubmatchIndex(stepsBody, -1)

	for _, match := range matches {
		stepName := stepsBody[match[2]:match[3]]
		if stepName == "steps" {
			continue
		}

		stepBody := extractBody(stepsBody, match[0])
		if stepBody == "" {
			continue
		}

		step := Step{
			Name:   stepName,
			Inputs: make(map[string]string),
		}

		if m := definitionRe.FindStringSubmatch(stepBody); len(m) > 1 {
			step.Definition = m[1]
		}
		if m := methodRe.FindStringSubmatch(stepBody); len(m) > 1 {
			step.Method = m[1]
		}
		if m := conditionRe.FindStringSubmatch(stepBody); len(m) > 1 {
			step.Condition = m[1]
		}
		if m := wfTimeoutRe.FindStringSubmatch(stepBody); len(m) > 1 {
			if d, err := time.ParseDuration(m[1]); err == nil {
				step.Timeout = d
			}
		}
		if m := wfRetriesRe.FindStringSubmatch(stepBody); len(m) > 1 {
			step.Retries, _ = strconv.Atoi(m[1])
		}

		// Parse inputs section
		inputsIdx := strings.Index(stepBody, "inputs:")
		if inputsIdx >= 0 {
			inputsBody := extractSection(stepBody, inputsIdx)
			parseInputs(inputsBody, &step)
		}

		if step.Definition != "" && step.Method != "" {
			wf.Steps[stepName] = step
		}
	}
}

// parseInputs extracts input mappings from an inputs section.
func parseInputs(inputsBody string, step *Step) {
	matches := inputEntryRe.FindAllStringSubmatch(inputsBody, -1)
	for _, m := range matches {
		key := m[1]
		if key == "inputs" {
			continue
		}
		if m[2] != "" {
			// Reference: steps.<step>.<field>.<subfield>
			ref := "steps." + m[2]
			if m[3] != "" {
				ref += ".outputs." + m[3]
			}
			step.Inputs[key] = ref
		} else if m[4] != "" {
			// Literal string value
			step.Inputs[key] = m[4]
		}
	}
}

// extractBody returns the text from the start position to the matching closing brace.
func extractBody(text string, start int) string {
	depth := 0
	inBody := false
	for i := start; i < len(text); i++ {
		switch text[i] {
		case '{':
			depth++
			inBody = true
		case '}':
			depth--
			if inBody && depth == 0 {
				return text[start : i+1]
			}
		}
	}
	return ""
}

// extractSection returns the text for a section starting at the given offset.
func extractSection(body string, start int) string {
	depth := 0
	inSection := false
	for i := start; i < len(body); i++ {
		switch body[i] {
		case '{':
			depth++
			inSection = true
		case '}':
			depth--
			if inSection && depth == 0 {
				return body[start : i+1]
			}
		}
	}
	return body[start:]
}
