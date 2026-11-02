package systemmodel

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/chazu/pudl/internal/inference"
	"github.com/chazu/pudl/internal/validator"
)

type contractSource map[string]validator.SchemaMetadata

func (s contractSource) GetAvailableSchemas() []string {
	names := make([]string, 0, len(s))
	for name := range s {
		names = append(names, name)
	}
	return names
}

func (s contractSource) GetSchemaMetadata(name string) (validator.SchemaMetadata, bool) {
	meta, ok := s[name]
	return meta, ok
}

func (s contractSource) GetInheritanceGraph() *inference.InheritanceGraph {
	return inference.BuildInheritanceGraph(s)
}

// contractSchemas: vault.#Token declares a sensitive secret; vault.#Lease
// inherits it; vault.#Compliant is a policy over vault.#Token.
var contractSchemas = contractSource{
	"vault.#Token":     {SchemaType: "base", ResourceType: "vault.token", IdentityFields: []string{"name"}, SensitiveFields: []string{"auth.secret"}},
	"vault.#Lease":     {SchemaType: "custom", ResourceType: "vault.lease", BaseSchema: "vault.#Token", IdentityFields: []string{"name"}},
	"vault.#Compliant": {SchemaType: validator.SchemaTypePolicy, BaseSchema: "vault.#Token"},
	"vault.#Plain":     {SchemaType: "base", ResourceType: "vault.plain", IdentityFields: []string{"name"}},
}

func contractModel(populateSchema string, desired ...map[string]any) *SystemModel {
	return &SystemModel{Name: "vault", Populate: Populate{Schema: populateSchema}, Desired: desired}
}

func TestCheckSchemaContractsRejectsDesiredAtSensitivePath(t *testing.T) {
	err := CheckSchemaContracts(contractModel("", map[string]any{
		"_schema": "vault.#Token", "name": "ci", "auth": map[string]any{"secret": "hunter2"},
	}), contractSchemas)
	require.ErrorContains(t, err, `model "vault": desired[0] (vault.#Token) sets sensitive path "auth.secret"`)
}

func TestCheckSchemaContractsRejectsInheritedSensitivePathByResourceType(t *testing.T) {
	err := CheckSchemaContracts(contractModel("", map[string]any{
		"_schema": "vault.lease", "name": "ci", "auth": map[string]any{"secret": "hunter2"},
	}), contractSchemas)
	require.ErrorContains(t, err, `desired[0] (vault.lease) sets sensitive path "auth.secret"`)
}

func TestCheckSchemaContractsFailsClosedOnBrokenDeclaration(t *testing.T) {
	broken := contractSource{"vault.#Token": {SchemaType: "base", SensitiveError: "sensitive_fields: not a list"}}
	err := CheckSchemaContracts(contractModel("", map[string]any{"_schema": "vault.#Token", "name": "ci"}), broken)
	require.ErrorContains(t, err, `model "vault": desired[0] (vault.#Token): cannot check sensitive paths`)
	require.ErrorContains(t, err, "not a list")
}

func TestCheckSchemaContractsRejectsPolicyPopulateSchema(t *testing.T) {
	err := CheckSchemaContracts(contractModel("vault.#Compliant"), contractSchemas)
	var policyErr *validator.PolicySchemaError
	require.True(t, errors.As(err, &policyErr), "got %v", err)
	require.Equal(t, "vault.#Token", policyErr.Type)
	require.ErrorContains(t, err, `model "vault": populate.schema: vault.#Compliant is a policy schema`)
}

func TestCheckSchemaContractsLeavesUnaffectedModelsAlone(t *testing.T) {
	require.NoError(t, CheckSchemaContracts(contractModel("",
		map[string]any{"_schema": "vault.#Token", "name": "ci", "auth": map[string]any{"ttl": "1h"}},
		map[string]any{"_schema": "vault.#Plain", "name": "p", "secret": "visible"},
		map[string]any{"_schema": "not.loaded", "name": "x", "auth": map[string]any{"secret": "s"}},
	), contractSchemas))
	require.NoError(t, CheckSchemaContracts(contractModel("not.#Loaded"), contractSchemas))
}
