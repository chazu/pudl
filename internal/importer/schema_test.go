package importer

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestExtractPackage(t *testing.T) {
	tests := []struct {
		name        string
		schema      string
		expectedPkg string
	}{
		{
			name:        "standard schema",
			schema:      "aws.#EC2Instance",
			expectedPkg: "aws",
		},
		{
			name:        "nested schema",
			schema:      "k8s.apps.#Deployment",
			expectedPkg: "k8s",
		},
		{
			name:        "no package",
			schema:      "#SimpleSchema",
			expectedPkg: "unknown",
		},
		{
			name:        "empty schema",
			schema:      "",
			expectedPkg: "unknown",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pkg := extractPackage(tt.schema)
			assert.Equal(t, tt.expectedPkg, pkg)
		})
	}
}
