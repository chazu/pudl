package database

import (
	"database/sql/driver"
	"fmt"

	"modernc.org/sqlite"
)

func init() {
	// -> returns JSON text, whereas json_extract has already coerced a numeric
	// token to INTEGER/REAL. Check the original token before SQLite comparisons,
	// joins, DISTINCT, aggregation, or recursive primary-key deduplication.
	// Registration precedes all catalog connections, including read-only opens.
	sqlite.MustRegisterDeterministicScalarFunction("pudl_query_value", 1,
		func(_ *sqlite.FunctionContext, args []driver.Value) (driver.Value, error) {
			if args[0] == nil {
				return nil, nil
			}
			raw, ok := args[0].(string)
			if !ok {
				return nil, fmt.Errorf("query value requires JSON text")
			}
			value, err := DecodeQueryJSON(raw)
			if err != nil {
				return nil, err
			}
			switch v := value.(type) {
			case bool:
				if v {
					return int64(1), nil
				}
				return int64(0), nil
			case map[string]interface{}, []interface{}:
				// Match json_extract's existing structured-value TEXT projection.
				return raw, nil
			default:
				return v, nil
			}
		})
}
