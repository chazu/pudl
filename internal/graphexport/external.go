package graphexport

import (
	"net"
	"sort"
)

// External endpoints. A rule may name an address range ("0.0.0.0/0") or
// "*" (any principal) as an edge end. Those are not uncollected resources,
// so they must not read as "not in inventory": they become external
// resources, outside every account.
const (
	externalCIDRType = "external.cidr"
	externalAnyType  = "external.principal"
)

// externalFor returns the external resource a value denotes, if any.
func externalFor(value string) (Resource, bool) {
	if value == "*" {
		return Resource{
			ID: "ext:any:*", Type: externalAnyType, Name: "Anyone (*)", Provider: "external", NativeID: "*",
			Properties: map[string]any{"note": "Any principal. Named by a rule, not a cataloged resource."},
			Evidence:   []Evidence{},
		}, true
	}
	if _, _, err := net.ParseCIDR(value); err != nil {
		return Resource{}, false
	}
	name := value
	if value == "0.0.0.0/0" || value == "::/0" {
		name = "Anywhere (" + value + ")"
	}
	return Resource{
		ID: "ext:cidr:" + value, Type: externalCIDRType, Name: name, Provider: "external", NativeID: value,
		Properties: map[string]any{"cidr": value, "note": "Address range named by a rule, not a cataloged resource."},
		Evidence:   []Evidence{},
	}, true
}

// externalResources lists the external resources an index handed out.
func (x *refIndex) externalResources() []Resource {
	out := make([]Resource, 0, len(x.externals))
	for _, r := range x.externals {
		out = append(out, r)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out
}
