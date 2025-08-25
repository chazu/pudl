package op

import (
	"fmt"
	"strings"
)

// CustomFunction represents a custom function that can be executed
type CustomFunction interface {
	Execute(args []interface{}) (interface{}, error)
}

// UppercaseFunction implements the #Uppercase function
type UppercaseFunction struct{}

func (f *UppercaseFunction) Execute(args []interface{}) (interface{}, error) {
	if len(args) != 1 {
		return nil, fmt.Errorf("uppercase function expects exactly 1 argument, got %d", len(args))
	}
	
	str, ok := args[0].(string)
	if !ok {
		return nil, fmt.Errorf("uppercase function expects string argument, got %T", args[0])
	}
	
	return strings.ToUpper(str), nil
}

// LowercaseFunction implements the #Lowercase function
type LowercaseFunction struct{}

func (f *LowercaseFunction) Execute(args []interface{}) (interface{}, error) {
	if len(args) != 1 {
		return nil, fmt.Errorf("lowercase function expects exactly 1 argument, got %d", len(args))
	}
	
	str, ok := args[0].(string)
	if !ok {
		return nil, fmt.Errorf("lowercase function expects string argument, got %T", args[0])
	}
	
	return strings.ToLower(str), nil
}

// ConcatFunction implements the #Concat function
type ConcatFunction struct{}

func (f *ConcatFunction) Execute(args []interface{}) (interface{}, error) {
	if len(args) == 0 {
		return "", nil
	}
	
	var parts []string
	for i, arg := range args {
		str, ok := arg.(string)
		if !ok {
			return nil, fmt.Errorf("concat function expects string arguments, got %T at position %d", arg, i)
		}
		parts = append(parts, str)
	}
	
	return strings.Join(parts, ""), nil
}

// GetFunction returns a custom function by name
func GetFunction(name string) CustomFunction {
	switch name {
	case "#Uppercase":
		return &UppercaseFunction{}
	case "#Lowercase":
		return &LowercaseFunction{}
	case "#Concat":
		return &ConcatFunction{}
	default:
		return nil
	}
}
