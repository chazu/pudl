package op

import "context"

// CustomFunction represents a custom function that can be executed.
type CustomFunction interface {
	Execute(ctx context.Context, args []interface{}) (interface{}, error)
}
