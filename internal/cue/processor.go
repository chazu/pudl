package cue

import (
	"context"
	"crypto/sha256"
	"fmt"
	"os"
	"strings"
	"sync"

	"cuelang.org/go/cue"
	"cuelang.org/go/cue/ast"
	"cuelang.org/go/cue/cuecontext"
	"cuelang.org/go/cue/format"
	"cuelang.org/go/cue/parser"
	"cuelang.org/go/cue/token"

	"pudl/internal/glojure"
)

// CUEProcessor handles the processing of CUE files with custom functions
type CUEProcessor struct {
	ctx      *cue.Context
	registry *glojure.Registry

	cacheMu sync.Mutex
	cache   map[string]interface{}
}

// NewCUEProcessor creates a new CUE processor backed by the given function registry.
func NewCUEProcessor(registry *glojure.Registry) *CUEProcessor {
	return &CUEProcessor{
		ctx:      cuecontext.New(),
		registry: registry,
		cache:    make(map[string]interface{}),
	}
}

// ProcessFile processes a single CUE file, executing custom functions and performing unification
func (p *CUEProcessor) ProcessFile(filename string) error {
	// Read the CUE file
	content, err := os.ReadFile(filename)
	if err != nil {
		return fmt.Errorf("failed to read file %s: %w", filename, err)
	}

	// Parse the CUE file into an AST
	file, err := parser.ParseFile(filename, content, parser.ParseComments)
	if err != nil {
		return fmt.Errorf("failed to parse CUE file %s: %w", filename, err)
	}

	// Walk the AST and process custom functions
	modifiedFile, err := p.processAST(file)
	if err != nil {
		return fmt.Errorf("failed to process AST: %w", err)
	}

	// Convert the modified AST back to CUE source
	modifiedSource, err := format.Node(modifiedFile)
	if err != nil {
		return fmt.Errorf("failed to format modified AST: %w", err)
	}

	fmt.Println("=== Processed CUE (after custom function execution) ===")
	fmt.Println(string(modifiedSource))

	// Build the modified CUE value from the processed source
	value := p.ctx.CompileString(string(modifiedSource))
	if err := value.Err(); err != nil {
		return fmt.Errorf("failed to build CUE value: %w", err)
	}

	// Perform unification and validation
	if err := value.Validate(); err != nil {
		return fmt.Errorf("validation failed: %w", err)
	}

	// Print the unified result
	fmt.Println("\n=== Final Unified CUE Result ===")
	result, err := value.MarshalJSON()
	if err != nil {
		return fmt.Errorf("failed to marshal result: %w", err)
	}
	fmt.Println(string(result))

	return nil
}

// processAST walks the AST and processes custom function calls
func (p *CUEProcessor) processAST(file *ast.File) (*ast.File, error) {
	// Create a copy of the file to modify
	newFile := &ast.File{
		Filename: file.Filename,
		Decls:    []ast.Decl{},
	}

	// Walk through declarations and process them, filtering out the "op" import
	for _, decl := range file.Decls {
		// Skip import declarations for "op" package
		if importDecl, ok := decl.(*ast.ImportDecl); ok {
			// Check if this is an import of the "op" package
			if len(importDecl.Specs) > 0 {
				importSpec := importDecl.Specs[0]
				if importSpec.Path != nil && strings.Trim(importSpec.Path.Value, `"`) == "op" {
					continue // Skip this import
				}
			}
		}

		processedDecl, err := p.processDeclaration(decl)
		if err != nil {
			return nil, err
		}
		newFile.Decls = append(newFile.Decls, processedDecl)
	}

	return newFile, nil
}

// processDeclaration processes a single declaration
func (p *CUEProcessor) processDeclaration(decl ast.Decl) (ast.Decl, error) {
	switch d := decl.(type) {
	case *ast.Field:
		processedValue, err := p.processExpression(d.Value)
		if err != nil {
			return nil, err
		}
		return &ast.Field{
			Label: d.Label,
			Value: processedValue,
		}, nil
	default:
		return decl, nil
	}
}

// processExpression processes expressions, looking for custom function calls
func (p *CUEProcessor) processExpression(expr ast.Expr) (ast.Expr, error) {
	switch e := expr.(type) {
	case *ast.BinaryExpr:
		return p.processBinaryExpr(e)
	case *ast.StructLit:
		return p.processStructLit(e)
	default:
		return expr, nil
	}
}

// processStructLit processes struct literals
func (p *CUEProcessor) processStructLit(structLit *ast.StructLit) (ast.Expr, error) {
	newElts := []ast.Decl{}
	for _, elt := range structLit.Elts {
		processedElt, err := p.processDeclaration(elt)
		if err != nil {
			return nil, err
		}
		newElts = append(newElts, processedElt)
	}
	return &ast.StructLit{Elts: newElts}, nil
}

// processBinaryExpr processes binary expressions, looking for custom function calls
func (p *CUEProcessor) processBinaryExpr(expr *ast.BinaryExpr) (ast.Expr, error) {
	// Check if this is a custom function call (op.#Function & {...})
	if expr.Op == token.AND {
		if result, processed, err := p.tryProcessCustomFunction(expr); err != nil {
			return nil, err
		} else if processed {
			return result, nil
		}
	}

	// Process left and right sides recursively
	processedX, err := p.processExpression(expr.X)
	if err != nil {
		return nil, err
	}
	processedY, err := p.processExpression(expr.Y)
	if err != nil {
		return nil, err
	}

	return &ast.BinaryExpr{
		X:  processedX,
		Op: expr.Op,
		Y:  processedY,
	}, nil
}

// tryProcessCustomFunction attempts to process a custom function call
func (p *CUEProcessor) tryProcessCustomFunction(expr *ast.BinaryExpr) (ast.Expr, bool, error) {
	// Check if left side is a selector expression like op.#Function
	selector, ok := expr.X.(*ast.SelectorExpr)
	if !ok {
		return nil, false, nil
	}

	// Check if it's from the "op" package
	ident, ok := selector.X.(*ast.Ident)
	if !ok || ident.Name != "op" {
		return nil, false, nil
	}

	// Extract function name from selector
	var functionName string
	if sel, ok := selector.Sel.(*ast.Ident); ok {
		functionName = sel.Name
	} else {
		return nil, false, nil
	}

	// Look up function in registry
	entry, found := p.registry.Get(functionName)
	if !found {
		return nil, false, nil
	}

	// Extract arguments from the right side (should be a struct with args field)
	structLit, ok := expr.Y.(*ast.StructLit)
	if !ok {
		return nil, false, fmt.Errorf("function %s: expected struct literal for arguments", functionName)
	}

	args, err := p.extractArguments(structLit)
	if err != nil {
		return nil, false, fmt.Errorf("function %s: %w", functionName, err)
	}

	// Check cache for cacheable functions
	var cacheKey string
	if entry.Cacheable {
		cacheKey = p.cacheKey(functionName, args)
		p.cacheMu.Lock()
		if cached, ok := p.cache[cacheKey]; ok {
			p.cacheMu.Unlock()
			return p.resultStruct(cached), true, nil
		}
		p.cacheMu.Unlock()
	}

	// Create context with timeout
	ctx, cancel := context.WithTimeout(context.Background(), entry.Timeout)
	defer cancel()

	// Execute the custom function
	result, err := entry.Impl.Execute(ctx, args)
	if err != nil {
		return nil, false, fmt.Errorf("function %s(%v) failed: %w", functionName, args, err)
	}

	// Store in cache if cacheable
	if entry.Cacheable && cacheKey != "" {
		p.cacheMu.Lock()
		p.cache[cacheKey] = result
		p.cacheMu.Unlock()
	}

	return p.resultStruct(result), true, nil
}

// cacheKey generates a cache key from function name and arguments.
func (p *CUEProcessor) cacheKey(name string, args []interface{}) string {
	h := sha256.New()
	h.Write([]byte(name))
	for _, a := range args {
		h.Write([]byte(fmt.Sprintf("|%v", a)))
	}
	return fmt.Sprintf("%x", h.Sum(nil))
}

// resultStruct creates a CUE struct literal with a result field.
func (p *CUEProcessor) resultStruct(value interface{}) ast.Expr {
	return &ast.StructLit{
		Elts: []ast.Decl{
			&ast.Field{
				Label: ast.NewIdent("result"),
				Value: p.createLiteralFromValue(value),
			},
		},
	}
}

// extractArguments extracts arguments from a struct literal
func (p *CUEProcessor) extractArguments(structLit *ast.StructLit) ([]interface{}, error) {
	for _, elt := range structLit.Elts {
		if field, ok := elt.(*ast.Field); ok {
			if ident, ok := field.Label.(*ast.Ident); ok && ident.Name == "args" {
				if listLit, ok := field.Value.(*ast.ListLit); ok {
					var args []interface{}
					for _, elem := range listLit.Elts {
						value, err := p.extractValue(elem)
						if err != nil {
							return nil, err
						}
						args = append(args, value)
					}
					return args, nil
				}
			}
		}
	}
	return nil, fmt.Errorf("no args field found in function call")
}

// extractValue extracts a Go value from a CUE AST expression
func (p *CUEProcessor) extractValue(expr ast.Expr) (interface{}, error) {
	switch e := expr.(type) {
	case *ast.BasicLit:
		switch e.Kind {
		case token.STRING:
			// Remove quotes from string literal
			return strings.Trim(e.Value, `"`), nil
		case token.INT:
			return e.Value, nil
		case token.FLOAT:
			return e.Value, nil
		default:
			return e.Value, nil
		}
	case *ast.Ident:
		// Handle identifiers (like true, false, null)
		switch e.Name {
		case "true":
			return true, nil
		case "false":
			return false, nil
		case "null":
			return nil, nil
		default:
			return e.Name, nil
		}
	default:
		return nil, fmt.Errorf("unsupported expression type for argument: %T", expr)
	}
}

// createLiteralFromValue creates a CUE AST literal from a Go value
func (p *CUEProcessor) createLiteralFromValue(value interface{}) ast.Expr {
	switch v := value.(type) {
	case string:
		return &ast.BasicLit{
			Kind:  token.STRING,
			Value: fmt.Sprintf(`"%s"`, v),
		}
	case int:
		return &ast.BasicLit{
			Kind:  token.INT,
			Value: fmt.Sprintf("%d", v),
		}
	case int64:
		return &ast.BasicLit{
			Kind:  token.INT,
			Value: fmt.Sprintf("%d", v),
		}
	case float64:
		return &ast.BasicLit{
			Kind:  token.FLOAT,
			Value: fmt.Sprintf("%f", v),
		}
	case bool:
		return ast.NewIdent(fmt.Sprintf("%t", v))
	case nil:
		return ast.NewIdent("null")
	default:
		// Fallback: convert to string
		return &ast.BasicLit{
			Kind:  token.STRING,
			Value: fmt.Sprintf(`"%v"`, v),
		}
	}
}
