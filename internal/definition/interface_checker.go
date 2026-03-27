package definition

import (
	"fmt"
	"strings"

	"cuelang.org/go/cue"
	"cuelang.org/go/cue/cuecontext"
	"cuelang.org/go/cue/load"
)

// InterfaceViolation describes a component that fails to satisfy its interface.
type InterfaceViolation struct {
	ComponentName string
	InterfaceName string
	Errors        []string
}

// InterfaceCheckResult summarizes the results of interface enforcement.
type InterfaceCheckResult struct {
	Interfaces int
	Components int
	Violations []InterfaceViolation
	Orphans    []string // components referencing non-existent interfaces
}

// InterfaceChecker validates that BRICK components satisfy their declared interfaces.
type InterfaceChecker struct {
	schemaPath string
}

// NewInterfaceChecker creates a checker that loads definitions from the given schema path.
func NewInterfaceChecker(schemaPath string) *InterfaceChecker {
	return &InterfaceChecker{schemaPath: schemaPath}
}

// Check loads all definitions, finds interfaces and components, and validates
// that each component satisfies its declared interface's contract via CUE unification.
func (ic *InterfaceChecker) Check() (*InterfaceCheckResult, error) {
	result := &InterfaceCheckResult{}

	// Load definitions directory as a CUE package.
	ctx := cuecontext.New()
	config := &load.Config{
		Dir: ic.schemaPath,
	}
	instances := load.Instances([]string{"./definitions"}, config)

	if len(instances) == 0 {
		return result, nil
	}

	// Collect interfaces and components from all loaded instances.
	type interfaceInfo struct {
		name     string
		contract cue.Value
	}
	type componentInfo struct {
		name       string
		implements string
		value      cue.Value
	}

	var interfaces []interfaceInfo
	var components []componentInfo

	for _, inst := range instances {
		if inst.Err != nil {
			return nil, fmt.Errorf("loading definitions: %w", inst.Err)
		}

		value := ctx.BuildInstance(inst)
		if value.Err() != nil {
			return nil, fmt.Errorf("building definitions: %w", value.Err())
		}

		// Iterate non-definition (non-#) fields — these are the actual definitions.
		iter, err := value.Fields(cue.Optional(true))
		if err != nil {
			continue
		}

		for iter.Next() {
			label := iter.Label()
			if strings.HasPrefix(label, "#") || strings.HasPrefix(label, "_") {
				continue
			}

			v := iter.Value()

			// Check if this is an interface (has kind:"interface" and contract).
			kindVal := v.LookupPath(cue.ParsePath("kind"))
			if kindVal.Err() == nil {
				kindStr, _ := kindVal.String()

				if kindStr == "interface" {
					contractVal := v.LookupPath(cue.ParsePath("contract"))
					nameVal := v.LookupPath(cue.ParsePath("name"))
					if contractVal.Err() == nil && nameVal.Err() == nil {
						nameStr, _ := nameVal.String()
						interfaces = append(interfaces, interfaceInfo{
							name:     nameStr,
							contract: contractVal,
						})
					}
				}
			}

			// Check if this is a component with implements.
			implVal := v.LookupPath(cue.ParsePath("implements"))
			if implVal.Err() == nil {
				implStr, err := implVal.String()
				if err == nil && implStr != "" {
					nameVal := v.LookupPath(cue.ParsePath("name"))
					nameStr, _ := nameVal.String()
					components = append(components, componentInfo{
						name:       nameStr,
						implements: implStr,
						value:      v,
					})
				}
			}
		}
	}

	result.Interfaces = len(interfaces)
	result.Components = len(components)

	// Index interfaces by name.
	ifaceMap := make(map[string]interfaceInfo, len(interfaces))
	for _, iface := range interfaces {
		ifaceMap[iface.name] = iface
	}

	// Validate each component against its interface's contract.
	for _, comp := range components {
		iface, ok := ifaceMap[comp.implements]
		if !ok {
			result.Orphans = append(result.Orphans, fmt.Sprintf(
				"%s implements %s (interface not found)", comp.name, comp.implements))
			continue
		}

		// Unify the component with the interface contract.
		// If the contract says toolchain:"lint", the component must have toolchain:"lint".
		// CUE unification produces errors for mismatches.
		violation := checkContract(comp, iface)
		if violation != nil {
			result.Violations = append(result.Violations, *violation)
		}
	}

	return result, nil
}

// checkContract validates a component against an interface contract by checking
// that every field in the contract is present in the component with a compatible value.
func checkContract(comp struct {
	name       string
	implements string
	value      cue.Value
}, iface struct {
	name     string
	contract cue.Value
}) *InterfaceViolation {
	var errs []string

	// Walk each field in the contract and check it exists in the component
	// with a compatible value.
	iter, err := iface.contract.Fields(cue.Optional(true))
	if err != nil {
		return nil
	}

	for iter.Next() {
		field := iter.Label()
		contractFieldVal := iter.Value()
		compFieldVal := comp.value.LookupPath(cue.ParsePath(field))

		if compFieldVal.Err() != nil {
			errs = append(errs, fmt.Sprintf("missing required field %q", field))
			continue
		}

		// Unify the component's field value with the contract's field value.
		unified := compFieldVal.Unify(contractFieldVal)
		if unified.Err() != nil {
			errs = append(errs, fmt.Sprintf("field %q: %v", field, unified.Err()))
		}
	}

	if len(errs) > 0 {
		return &InterfaceViolation{
			ComponentName: comp.name,
			InterfaceName: iface.name,
			Errors:        errs,
		}
	}
	return nil
}
