package cmd

import (
	"encoding/json"
	"fmt"
	"math/big"
	"strings"

	"gopkg.in/yaml.v3"
)

// YAML's generic decoder converts decimals to float64. Export works from the
// retained source nodes so numeric tokens reach JSON without that rounding.
func exactExportYAML(node *yaml.Node) (any, error) {
	// Preserve yaml.v3 validation of duplicate keys, invalid aliases, and merges.
	var validated any
	if err := node.Decode(&validated); err != nil {
		return nil, err
	}
	return exportYAMLNode(node, map[*yaml.Node]bool{})
}
func exportYAMLNode(node *yaml.Node, active map[*yaml.Node]bool) (any, error) {
	if active[node] {
		return nil, fmt.Errorf("cyclic YAML alias")
	}
	active[node] = true
	defer delete(active, node)
	switch node.Kind {
	case yaml.DocumentNode:
		if len(node.Content) == 0 {
			return nil, nil
		}
		return exportYAMLNode(node.Content[0], active)
	case yaml.AliasNode:
		return exportYAMLNode(node.Alias, active)
	case yaml.SequenceNode:
		list := make([]any, 0, len(node.Content))
		for _, child := range node.Content {
			v, err := exportYAMLNode(child, active)
			if err != nil {
				return nil, err
			}
			list = append(list, v)
		}
		return list, nil
	case yaml.MappingNode:
		result := map[string]any{}
		explicit := map[string]bool{}
		// Explicit keys override merged keys regardless of their written order.
		for i := 0; i < len(node.Content); i += 2 {
			key := node.Content[i]
			if key.Tag == "!!merge" {
				continue
			}
			if key.Tag != "!!str" {
				return nil, fmt.Errorf("YAML export requires string mapping keys")
			}
			value, err := exportYAMLNode(node.Content[i+1], active)
			if err != nil {
				return nil, err
			}
			result[key.Value] = value
			explicit[key.Value] = true
		}
		for i := 0; i < len(node.Content); i += 2 {
			if node.Content[i].Tag != "!!merge" {
				continue
			}
			value, err := exportYAMLNode(node.Content[i+1], active)
			if err != nil {
				return nil, err
			}
			sources := []any{value}
			if list, ok := value.([]any); ok {
				sources = list
			}
			for _, source := range sources {
				merged, ok := source.(map[string]any)
				if !ok {
					return nil, fmt.Errorf("YAML merge source is not a mapping")
				}
				for key, v := range merged {
					if _, exists := result[key]; !exists && !explicit[key] {
						result[key] = v
					}
				}
			}
		}
		return result, nil
	case yaml.ScalarNode:
		raw := strings.ReplaceAll(node.Value, "_", "")
		switch node.Tag {
		case "!!int":
			number, ok := new(big.Int).SetString(raw, 0)
			if !ok {
				return nil, fmt.Errorf("invalid YAML integer %q", node.Value)
			}
			return json.Number(number.String()), nil
		case "!!float":
			raw = strings.TrimPrefix(raw, "+")
			if strings.HasPrefix(raw, ".") {
				raw = "0" + raw
			}
			if strings.HasPrefix(raw, "-.") {
				raw = "-0" + raw[1:]
			}
			parts := strings.SplitN(raw, "e", 2)
			if len(parts) == 1 {
				parts = strings.SplitN(raw, "E", 2)
			}
			if strings.HasSuffix(parts[0], ".") {
				parts[0] += "0"
			}
			// YAML accepts leading zeroes in decimal floats; JSON does not.
			sign := ""
			if strings.HasPrefix(parts[0], "-") {
				sign = "-"
				parts[0] = parts[0][1:]
			}
			mantissa := strings.SplitN(parts[0], ".", 2)
			mantissa[0] = strings.TrimLeft(mantissa[0], "0")
			if mantissa[0] == "" {
				mantissa[0] = "0"
			}
			parts[0] = sign + strings.Join(mantissa, ".")
			raw = strings.Join(parts, "e")
			if !json.Valid([]byte(raw)) {
				return nil, fmt.Errorf("YAML numeric value %q is not representable in JSON", node.Value)
			}
			return json.Number(raw), nil
		default:
			var value any
			if err := node.Decode(&value); err != nil {
				return nil, err
			}
			return value, nil
		}
	default:
		return nil, fmt.Errorf("unsupported YAML node kind %d", node.Kind)
	}
}
