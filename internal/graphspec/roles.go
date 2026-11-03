package graphspec

import (
	"encoding/json"
	"fmt"
	"slices"
	"strings"
	"unicode/utf8"
)

// Classes are the visual classes a resource type can declare. pudl ui styles
// each one; the list is closed so a typo cannot silently restyle a type.
var Classes = []string{"admin", "network", "compute", "data", "messaging", "edge", "security", "identity", "logical", "external"}

// MaxCodeLen is the longest display code: the code is drawn on the node.
const MaxCodeLen = 5

// Display is how pudl ui labels a resource type.
type Display struct {
	Code  string `json:"code"`
	Label string `json:"label"`
	Class string `json:"class"`
}

// Admin marks an administrative node (account, project, folder,
// organization). Parent: edge types to its administrative parent; none means
// the top of the hierarchy. Owner is not written in a schema: it is set for
// the type a provider's graph_account_type fact names (the type other
// resources' `account` attribute names).
type Admin struct {
	Owner  bool     `json:"owner"`
	Parent []string `json:"parent"`
}

type rawAdmin struct {
	Parent []string `json:"parent"`
}

// NetworkRole places a type in the network view. Group: it contains other
// resources. Via: placed under the target of the first of these edge types
// it has, else under its owner. Region: each parent is wrapped in a region
// group when the resource has a region.
type NetworkRole struct {
	Group  bool     `json:"group"`
	Via    []string `json:"via"`
	Region bool     `json:"region"`
}

// SecurityRole: Control types are always shown in the security view.
type SecurityRole struct {
	Control bool `json:"control"`
}

// InventoryRole: Collapsed types start with their type bucket collapsed.
type InventoryRole struct {
	Collapsed bool `json:"collapsed"`
}

// ApplicationRole places a type in the application view. Group: it contains
// members. Via: placed under the target of the first of these edge types.
type ApplicationRole struct {
	Group bool     `json:"group"`
	Via   []string `json:"via"`
}

// Roles are a type's parts in pudl ui's views. A nil block means the type
// declares no role there.
type Roles struct {
	Admin       *Admin           `json:"admin"`
	Network     *NetworkRole     `json:"network"`
	PortOf      string           `json:"portOf"`
	Security    *SecurityRole    `json:"security"`
	Inventory   *InventoryRole   `json:"inventory"`
	Application *ApplicationRole `json:"application"`
}

// EdgeTypes lists every edge type the roles name, for checking that each is
// declared.
func (r Roles) EdgeTypes() []string {
	var out []string
	if r.Admin != nil {
		out = append(out, r.Admin.Parent...)
	}
	if r.Network != nil {
		out = append(out, r.Network.Via...)
	}
	if r.PortOf != "" {
		out = append(out, r.PortOf)
	}
	if r.Application != nil {
		out = append(out, r.Application.Via...)
	}
	return out
}

func parseDisplay(raw json.RawMessage) (*Display, error) {
	var d Display
	if err := decodeStrict(raw, &d); err != nil {
		return nil, fmt.Errorf("display: want {code, label, class}: %w", err)
	}
	if err := CheckDisplay(d); err != nil {
		return nil, fmt.Errorf("display: %w", err)
	}
	return &d, nil
}

// CheckDisplay validates display info from a schema or a graph_resource_type
// fact.
func CheckDisplay(d Display) error {
	if d.Code == "" || d.Label == "" || d.Class == "" {
		return fmt.Errorf("code, label and class are required")
	}
	if utf8.RuneCountInString(d.Code) > MaxCodeLen {
		return fmt.Errorf("code %q is longer than %d characters", d.Code, MaxCodeLen)
	}
	if !slices.Contains(Classes, d.Class) {
		return fmt.Errorf("class %q is not one of %s", d.Class, strings.Join(Classes, ", "))
	}
	return nil
}

type rawRoles struct {
	Admin       json.RawMessage `json:"admin"`
	Network     json.RawMessage `json:"network"`
	PortOf      *string         `json:"port_of"`
	Security    json.RawMessage `json:"security"`
	Inventory   json.RawMessage `json:"inventory"`
	Application json.RawMessage `json:"application"`
}

func parseRoles(r rawRoles) (Roles, error) {
	var out Roles
	blocks := []struct {
		name string
		raw  json.RawMessage
		dst  any
	}{
		{"admin", r.Admin, &rawAdmin{}},
		{"network", r.Network, &NetworkRole{}},
		{"security", r.Security, &SecurityRole{}},
		{"inventory", r.Inventory, &InventoryRole{}},
		{"application", r.Application, &ApplicationRole{}},
	}
	for _, b := range blocks {
		if b.raw == nil {
			continue
		}
		if err := decodeStrict(b.raw, b.dst); err != nil {
			return Roles{}, fmt.Errorf("%s: %w", b.name, err)
		}
		switch v := b.dst.(type) {
		case *rawAdmin:
			out.Admin = &Admin{Parent: v.Parent}
		case *NetworkRole:
			out.Network = v
		case *SecurityRole:
			out.Security = v
		case *InventoryRole:
			out.Inventory = v
		case *ApplicationRole:
			out.Application = v
		}
	}
	if r.PortOf != nil {
		if *r.PortOf == "" {
			return Roles{}, fmt.Errorf("port_of: empty edge type")
		}
		out.PortOf = *r.PortOf
	}
	for _, t := range out.EdgeTypes() {
		if t == "" {
			return Roles{}, fmt.Errorf("empty edge type in admin.parent, network.via or application.via")
		}
	}
	return out, nil
}
