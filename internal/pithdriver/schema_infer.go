package pithdriver

import (
	"github.com/chazu/pith"

	"github.com/chazu/pudl/internal/inference"
)

// schemaInferWords returns schema/match and schema/infer driver words.
func schemaInferWords(inferrer *inference.SchemaInferrer) map[string]pith.Word {
	return map[string]pith.Word{
		"match": wordSchemaMatch(inferrer),
		"infer": wordSchemaInfer(inferrer),
	}
}

// schema/match ( data -- schema_name )
// Returns the best matching schema name, or nil if no match.
func wordSchemaMatch(inferrer *inference.SchemaInferrer) pith.Word {
	return func(vm *pith.VM) error {
		data, err := vm.Pop()
		if err != nil {
			return err
		}
		result, err := inferrer.Infer(data, inference.InferenceHints{})
		if err != nil {
			return err
		}
		if result.MatchedAt < 0 || result.Confidence < 0.2 {
			vm.Push(nil)
			return nil
		}
		vm.Push(result.Schema)
		return nil
	}
}

// schema/infer ( data -- result )
// Returns full inference result: {schema, confidence, reason, matched_at}.
func wordSchemaInfer(inferrer *inference.SchemaInferrer) pith.Word {
	return func(vm *pith.VM) error {
		data, err := vm.Pop()
		if err != nil {
			return err
		}
		result, err := inferrer.Infer(data, inference.InferenceHints{})
		if err != nil {
			return err
		}
		m, err := structToMap(result)
		if err != nil {
			return err
		}
		vm.Push(m)
		return nil
	}
}
