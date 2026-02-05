package schemaname

import (
	"testing"
)

func TestNormalize(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		// Full CUE internal format
		{"full CUE format", "pudl.schemas/aws/ec2@v0:#Instance", "aws/ec2.#Instance"},
		{"full CUE format no version", "pudl.schemas/aws/ec2:#Instance", "aws/ec2.#Instance"},
		{"full CUE format core", "pudl.schemas/pudl/core@v0:#Item", "pudl/core.#Item"},

		// Partial formats
		{"with version no prefix", "aws/ec2@v0:#Instance", "aws/ec2.#Instance"},
		{"colon separator", "aws/ec2:#Instance", "aws/ec2.#Instance"},
		{"dot separator", "aws/ec2.#Instance", "aws/ec2.#Instance"},

		// Legacy short names
		{"legacy core.#Item", "core.#Item", "pudl/core.#Item"},
		{"legacy core.#Collection", "core.#Collection", "pudl/core.#Collection"},

		// Already canonical
		{"already canonical", "aws/ec2.#Instance", "aws/ec2.#Instance"},
		{"already canonical core", "pudl/core.#Item", "pudl/core.#Item"},

		// Edge cases
		{"empty string", "", ""},
		{"version v1", "aws/ec2@v1:#Instance", "aws/ec2.#Instance"},
		{"version v10", "aws/ec2@v10:#Instance", "aws/ec2.#Instance"},

		// EKS example
		{"eks cluster", "pudl.schemas/aws/eks@v0:#Cluster", "aws/eks.#Cluster"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := Normalize(tt.input)
			if result != tt.expected {
				t.Errorf("Normalize(%q) = %q, want %q", tt.input, result, tt.expected)
			}
		})
	}
}

func TestParse(t *testing.T) {
	tests := []struct {
		name       string
		input      string
		wantPkg    string
		wantDef    string
	}{
		{"full format", "aws/ec2.#Instance", "aws/ec2", "#Instance"},
		{"CUE format", "pudl.schemas/aws/ec2@v0:#Instance", "aws/ec2", "#Instance"},
		{"core", "pudl/core.#Item", "pudl/core", "#Item"},
		{"legacy core", "core.#Item", "pudl/core", "#Item"},
		{"empty", "", "", ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pkg, def, err := Parse(tt.input)
			if err != nil {
				t.Errorf("Parse(%q) returned error: %v", tt.input, err)
			}
			if pkg != tt.wantPkg {
				t.Errorf("Parse(%q) pkg = %q, want %q", tt.input, pkg, tt.wantPkg)
			}
			if def != tt.wantDef {
				t.Errorf("Parse(%q) def = %q, want %q", tt.input, def, tt.wantDef)
			}
		})
	}
}

func TestFormat(t *testing.T) {
	tests := []struct {
		name     string
		pkg      string
		def      string
		expected string
	}{
		{"basic", "aws/ec2", "#Instance", "aws/ec2.#Instance"},
		{"without hash", "aws/ec2", "Instance", "aws/ec2.#Instance"},
		{"with version", "aws/ec2@v0", "#Instance", "aws/ec2.#Instance"},
		{"core", "pudl/core", "#Item", "pudl/core.#Item"},
		{"empty pkg", "", "#Instance", ""},
		{"empty def", "aws/ec2", "", "aws/ec2"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := Format(tt.pkg, tt.def)
			if result != tt.expected {
				t.Errorf("Format(%q, %q) = %q, want %q", tt.pkg, tt.def, result, tt.expected)
			}
		})
	}
}

func TestIsEquivalent(t *testing.T) {
	tests := []struct {
		name     string
		a        string
		b        string
		expected bool
	}{
		{"same canonical", "aws/ec2.#Instance", "aws/ec2.#Instance", true},
		{"CUE vs canonical", "pudl.schemas/aws/ec2@v0:#Instance", "aws/ec2.#Instance", true},
		{"legacy vs canonical", "core.#Item", "pudl/core.#Item", true},
		{"different schemas", "aws/ec2.#Instance", "aws/eks.#Cluster", false},
		{"different definitions", "aws/ec2.#Instance", "aws/ec2.#InstanceCollection", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := IsEquivalent(tt.a, tt.b)
			if result != tt.expected {
				t.Errorf("IsEquivalent(%q, %q) = %v, want %v", tt.a, tt.b, result, tt.expected)
			}
		})
	}
}

func TestIsFallbackSchema(t *testing.T) {
	tests := []struct {
		input    string
		expected bool
	}{
		{"pudl/core.#Item", true},
		{"core.#Item", true},
		{"pudl.schemas/pudl/core@v0:#Item", true},
		{"pudl/core.#Collection", true},
		{"pudl/core.#CatchAll", true},
		{"core.#CatchAll", true},
		{"pudl.schemas/pudl/core:#CatchAll", true},
		{"aws/ec2.#Instance", false},
		{"k8s.#Pod", false},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			result := IsFallbackSchema(tt.input)
			if result != tt.expected {
				t.Errorf("IsFallbackSchema(%q) = %v, want %v", tt.input, result, tt.expected)
			}
		})
	}
}

