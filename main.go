package main

import (
	"fmt"
	"log"
	"os"
	"strings"

	"cuelang.org/go/cue"
	"cuelang.org/go/cue/ast"
	"cuelang.org/go/cue/cuecontext"
	"cuelang.org/go/cue/format"
	"cuelang.org/go/cue/parser"
	"cuelang.org/go/cue/token"

	"pudl/op"
)

// CUEProcessor handles the processing of CUE files with custom functions
type CUEProcessor struct {
	ctx *cue.Context
}

// NewCUEProcessor creates a new CUE processor
func NewCUEProcessor() *CUEProcessor {
	return &CUEProcessor{
		ctx: cuecontext.New(),
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

// processDeclaration processes a single declaration in the AST
func (p *CUEProcessor) processDeclaration(decl ast.Decl) (ast.Decl, error) {
	switch d := decl.(type) {
	case *ast.Field:
		return p.processField(d)
	default:
		return decl, nil
	}
}

// processField processes a field declaration
func (p *CUEProcessor) processField(field *ast.Field) (*ast.Field, error) {
	newField := &ast.Field{
		Label: field.Label,
		Token: field.Token,
		Value: field.Value,
		Attrs: field.Attrs,
	}

	// Process the field value
	processedValue, err := p.processExpression(field.Value)
	if err != nil {
		return nil, err
	}
	newField.Value = processedValue

	return newField, nil
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

// processBinaryExpr processes binary expressions (like unification with &)
func (p *CUEProcessor) processBinaryExpr(expr *ast.BinaryExpr) (ast.Expr, error) {
	if expr.Op == token.AND {
		// Check if this is a custom function call pattern: op.#Function & { args: [...] }
		if result, isCustomFunc, err := p.tryProcessCustomFunction(expr); err != nil {
			return nil, err
		} else if isCustomFunc {
			return result, nil
		}
	}

	// Process left and right expressions recursively
	left, err := p.processExpression(expr.X)
	if err != nil {
		return nil, err
	}
	right, err := p.processExpression(expr.Y)
	if err != nil {
		return nil, err
	}

	return &ast.BinaryExpr{
		X:  left,
		Op: expr.Op,
		Y:  right,
	}, nil
}

// processStructLit processes struct literals
func (p *CUEProcessor) processStructLit(expr *ast.StructLit) (ast.Expr, error) {
	newElts := make([]ast.Decl, len(expr.Elts))
	for i, elt := range expr.Elts {
		processedElt, err := p.processDeclaration(elt)
		if err != nil {
			return nil, err
		}
		newElts[i] = processedElt
	}

	return &ast.StructLit{
		Lbrace: expr.Lbrace,
		Elts:   newElts,
		Rbrace: expr.Rbrace,
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
	if ident, ok := selector.Sel.(*ast.Ident); ok {
		functionName = ident.Name
	} else {
		return nil, false, nil
	}

	customFunc := op.GetFunction(functionName)
	if customFunc == nil {
		return nil, false, nil
	}

	// Extract arguments from the right side (should be a struct with args field)
	structLit, ok := expr.Y.(*ast.StructLit)
	if !ok {
		return nil, false, fmt.Errorf("expected struct literal for function arguments")
	}

	args, err := p.extractArguments(structLit)
	if err != nil {
		return nil, false, err
	}

	// Execute the custom function
	result, err := customFunc.Execute(args)
	if err != nil {
		return nil, false, fmt.Errorf("failed to execute function %s: %w", functionName, err)
	}

	// Create a struct literal with the result
	resultStruct := &ast.StructLit{
		Elts: []ast.Decl{
			&ast.Field{
				Label: ast.NewIdent("result"),
				Value: p.createLiteralFromValue(result),
			},
		},
	}

	return resultStruct, true, nil
}

// extractArguments extracts arguments from a struct literal
func (p *CUEProcessor) extractArguments(structLit *ast.StructLit) ([]interface{}, error) {
	for _, elt := range structLit.Elts {
		field, ok := elt.(*ast.Field)
		if !ok {
			continue
		}

		label, ok := field.Label.(*ast.Ident)
		if !ok || label.Name != "args" {
			continue
		}

		// Extract list literal
		listLit, ok := field.Value.(*ast.ListLit)
		if !ok {
			return nil, fmt.Errorf("args field must be a list")
		}

		var args []interface{}
		for _, elem := range listLit.Elts {
			value, err := p.extractLiteralValue(elem)
			if err != nil {
				return nil, err
			}
			args = append(args, value)
		}

		return args, nil
	}

	return nil, fmt.Errorf("no args field found in function call")
}

// extractLiteralValue extracts a literal value from an AST expression
func (p *CUEProcessor) extractLiteralValue(expr ast.Expr) (interface{}, error) {
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
	case *ast.SelectorExpr:
		// For selector expressions like greeting.result, we can't evaluate them at this stage
		// since they reference other parts of the CUE that haven't been processed yet.
		// For now, we'll return a placeholder that indicates this needs to be resolved later.
		return fmt.Sprintf("{{%s}}", p.formatSelectorExpr(e)), nil
	default:
		return nil, fmt.Errorf("unsupported literal type: %T", expr)
	}
}

// formatSelectorExpr formats a selector expression as a string
func (p *CUEProcessor) formatSelectorExpr(expr *ast.SelectorExpr) string {
	var parts []string

	// Walk up the selector chain
	current := expr
	for current != nil {
		if ident, ok := current.Sel.(*ast.Ident); ok {
			parts = append([]string{ident.Name}, parts...)
		}

		if selector, ok := current.X.(*ast.SelectorExpr); ok {
			current = selector
		} else if ident, ok := current.X.(*ast.Ident); ok {
			parts = append([]string{ident.Name}, parts...)
			break
		} else {
			break
		}
	}

	return strings.Join(parts, ".")
}

// createLiteralFromValue creates an AST literal from a Go value
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
	case float64:
		return &ast.BasicLit{
			Kind:  token.FLOAT,
			Value: fmt.Sprintf("%f", v),
		}
	default:
		return &ast.BasicLit{
			Kind:  token.STRING,
			Value: fmt.Sprintf(`"%v"`, v),
		}
	}
}

func main() {
	if len(os.Args) < 2 {
		log.Fatal("Usage: pudl <cue-file>")
	}

	filename := os.Args[1]
	if !strings.HasSuffix(filename, ".cue") {
		log.Fatal("File must have .cue extension")
	}

	if _, err := os.Stat(filename); os.IsNotExist(err) {
		log.Fatalf("File %s does not exist", filename)
	}

	processor := NewCUEProcessor()
	if err := processor.ProcessFile(filename); err != nil {
		log.Fatalf("Error processing file: %v", err)
	}
}
