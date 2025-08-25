# PUDL - CUE Language Extension Utility

PUDL (Process, Unify, Data Language) is a Go utility that extends CUE's unification functionality by allowing custom functions to be executed during the AST processing phase.

## Features

- **Custom Function Execution**: Execute custom Go functions referenced in CUE files
- **AST Processing**: Walk and modify CUE AST before unification
- **Public API Only**: Uses only public portions of the CUE Go API
- **Extensible**: Easy to add new custom functions

## Supported Custom Functions

The utility currently supports the following custom functions from the "op" package:

- `op.#Uppercase`: Converts a string to uppercase
- `op.#Lowercase`: Converts a string to lowercase  
- `op.#Concat`: Concatenates multiple strings

## Usage

```bash
go run . <cue-file>
```

### Example

Given a CUE file `example.cue`:

```cue
// Example of using custom functions imported via package imports
import "op"

upper: op.#Uppercase & {
    args: ["hello world"]
}

lower: op.#Lowercase & {
    args: ["HELLO WORLD"]
}

concat: op.#Concat & {
    args: ["Hello", " ", "World"]
}

uppercaseResult: upper.result // should be "HELLO WORLD"
lowercaseResult: lower.result // should be "hello world"
concatResult: concat.result   // should be "Hello World"
```

Run the utility:

```bash
go run . example.cue
```

Output:
```
=== Processed CUE (after custom function execution) ===
upper: result: "HELLO WORLD"

lower: result: "hello world"

concat: result: "Hello World"

uppercaseResult: upper.result

lowercaseResult: lower.result

concatResult: concat.result

=== Final Unified CUE Result ===
{"upper":{"result":"HELLO WORLD"},"lower":{"result":"hello world"},"concat":{"result":"Hello World"},"uppercaseResult":"HELLO WORLD","lowercaseResult":"hello world","concatResult":"Hello World"}
```

## How It Works

1. **Parse**: Reads and parses the CUE file into an AST
2. **Process**: Walks the AST looking for custom function calls (e.g., `op.#Uppercase & { args: [...] }`)
3. **Execute**: Executes the custom functions with the provided arguments
4. **Transform**: Replaces the function calls with result structures containing the computed values
5. **Unify**: Performs standard CUE unification on the transformed AST
6. **Output**: Prints both the intermediate processed CUE and the final unified result

## Architecture

- `main.go`: Main utility with AST processing logic
- `op/functions.go`: Custom function implementations
- Uses CUE Go API packages:
  - `cuelang.org/go/cue/ast` - AST manipulation
  - `cuelang.org/go/cue/parser` - CUE parsing
  - `cuelang.org/go/cue/cuecontext` - CUE evaluation context
  - `cuelang.org/go/cue/format` - CUE formatting

## Adding New Custom Functions

To add a new custom function:

1. Implement the `CustomFunction` interface in `op/functions.go`
2. Add the function to the `GetFunction` switch statement
3. Use the function in CUE files with the pattern: `op.#FunctionName & { args: [...] }`

Example:

```go
// ReverseFunction implements string reversal
type ReverseFunction struct{}

func (f *ReverseFunction) Execute(args []interface{}) (interface{}, error) {
    if len(args) != 1 {
        return nil, fmt.Errorf("reverse function expects exactly 1 argument")
    }
    
    str, ok := args[0].(string)
    if !ok {
        return nil, fmt.Errorf("reverse function expects string argument")
    }
    
    runes := []rune(str)
    for i, j := 0, len(runes)-1; i < j; i, j = i+1, j-1 {
        runes[i], runes[j] = runes[j], runes[i]
    }
    
    return string(runes), nil
}
```

## Requirements

- Go 1.23+
- CUE Go API v0.14.0+

## Installation

```bash
git clone <repository>
cd pudl
go mod tidy
```

## License

This project demonstrates extending CUE's functionality using its public Go API.
