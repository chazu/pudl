package schema

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// Manager handles schema file operations and organization
type Manager struct {
	schemaPath string
}

// NewManager creates a new schema manager
func NewManager(schemaPath string) *Manager {
	return &Manager{
		schemaPath: schemaPath,
	}
}

// SchemaInfo represents information about a schema file
type SchemaInfo struct {
	Package    string `json:"package"`
	Name       string `json:"name"`
	FullName   string `json:"full_name"`   // package.name format
	FilePath   string `json:"file_path"`
	FileName   string `json:"file_name"`
	Definition string `json:"definition"`  // The #Definition name from CUE
	Size       int64  `json:"size"`
}

// ListSchemas returns all available schemas organized by package
func (m *Manager) ListSchemas() (map[string][]SchemaInfo, error) {
	schemas := make(map[string][]SchemaInfo)

	// Walk through the schema directory
	err := filepath.Walk(m.schemaPath, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		// Skip directories and non-CUE files
		if info.IsDir() || !strings.HasSuffix(info.Name(), ".cue") {
			return nil
		}

		// Get relative path from schema root
		relPath, err := filepath.Rel(m.schemaPath, path)
		if err != nil {
			return err
		}

		// Extract package from directory structure
		packageName := filepath.Dir(relPath)
		if packageName == "." {
			packageName = "root"
		}

		// Extract schema name from filename (without .cue extension)
		fileName := info.Name()
		schemaName := strings.TrimSuffix(fileName, ".cue")

		// Create schema info
		schemaInfo := SchemaInfo{
			Package:  packageName,
			Name:     schemaName,
			FullName: fmt.Sprintf("%s.%s", packageName, schemaName),
			FilePath: path,
			FileName: fileName,
			Size:     info.Size(),
		}

		// Try to extract the definition name from the file
		if definition, err := m.extractDefinitionName(path); err == nil {
			schemaInfo.Definition = definition
		}

		// Add to package group
		schemas[packageName] = append(schemas[packageName], schemaInfo)

		return nil
	})

	if err != nil {
		return nil, fmt.Errorf("failed to walk schema directory: %w", err)
	}

	// Sort schemas within each package
	for packageName := range schemas {
		sort.Slice(schemas[packageName], func(i, j int) bool {
			return schemas[packageName][i].Name < schemas[packageName][j].Name
		})
	}

	return schemas, nil
}

// AddSchema adds a new schema file to the appropriate package directory
func (m *Manager) AddSchema(packageName, schemaName, sourceFile string) error {
	// Validate package and schema names
	if err := m.validateNames(packageName, schemaName); err != nil {
		return err
	}

	// Check if source file exists
	if _, err := os.Stat(sourceFile); os.IsNotExist(err) {
		return fmt.Errorf("source file does not exist: %s", sourceFile)
	}

	// Create package directory if it doesn't exist
	packageDir := filepath.Join(m.schemaPath, packageName)
	if err := os.MkdirAll(packageDir, 0755); err != nil {
		return fmt.Errorf("failed to create package directory: %w", err)
	}

	// Determine target file path
	targetFile := filepath.Join(packageDir, schemaName+".cue")

	// Check if schema already exists
	if _, err := os.Stat(targetFile); err == nil {
		return fmt.Errorf("schema already exists: %s.%s", packageName, schemaName)
	}

	// Copy the source file to the target location
	if err := m.copyFile(sourceFile, targetFile); err != nil {
		return fmt.Errorf("failed to copy schema file: %w", err)
	}

	return nil
}

// GetSchema returns information about a specific schema
func (m *Manager) GetSchema(packageName, schemaName string) (*SchemaInfo, error) {
	schemaFile := filepath.Join(m.schemaPath, packageName, schemaName+".cue")
	
	info, err := os.Stat(schemaFile)
	if os.IsNotExist(err) {
		return nil, fmt.Errorf("schema not found: %s.%s", packageName, schemaName)
	}
	if err != nil {
		return nil, fmt.Errorf("failed to stat schema file: %w", err)
	}

	schemaInfo := &SchemaInfo{
		Package:  packageName,
		Name:     schemaName,
		FullName: fmt.Sprintf("%s.%s", packageName, schemaName),
		FilePath: schemaFile,
		FileName: schemaName + ".cue",
		Size:     info.Size(),
	}

	// Try to extract the definition name
	if definition, err := m.extractDefinitionName(schemaFile); err == nil {
		schemaInfo.Definition = definition
	}

	return schemaInfo, nil
}

// validateNames validates package and schema names
func (m *Manager) validateNames(packageName, schemaName string) error {
	// Package name validation
	if packageName == "" {
		return fmt.Errorf("package name cannot be empty")
	}
	if strings.Contains(packageName, "..") || strings.Contains(packageName, "/") {
		return fmt.Errorf("invalid package name: %s (cannot contain .. or /)", packageName)
	}

	// Schema name validation
	if schemaName == "" {
		return fmt.Errorf("schema name cannot be empty")
	}
	if strings.Contains(schemaName, "/") || strings.Contains(schemaName, "\\") {
		return fmt.Errorf("invalid schema name: %s (cannot contain path separators)", schemaName)
	}
	if strings.HasPrefix(schemaName, ".") || strings.HasSuffix(schemaName, ".") {
		return fmt.Errorf("invalid schema name: %s (cannot start or end with .)", schemaName)
	}

	return nil
}

// copyFile copies a file from source to destination
func (m *Manager) copyFile(src, dst string) error {
	sourceFile, err := os.Open(src)
	if err != nil {
		return err
	}
	defer sourceFile.Close()

	destFile, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer destFile.Close()

	// Copy file contents
	_, err = destFile.ReadFrom(sourceFile)
	if err != nil {
		return err
	}

	// Copy file permissions
	sourceInfo, err := os.Stat(src)
	if err != nil {
		return err
	}
	
	return os.Chmod(dst, sourceInfo.Mode())
}

// extractDefinitionName attempts to extract the main definition name from a CUE file
func (m *Manager) extractDefinitionName(filePath string) (string, error) {
	content, err := os.ReadFile(filePath)
	if err != nil {
		return "", err
	}

	lines := strings.Split(string(content), "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		// Look for lines that start with #DefinitionName:
		if strings.HasPrefix(line, "#") && strings.Contains(line, ":") {
			parts := strings.SplitN(line, ":", 2)
			if len(parts) == 2 {
				return strings.TrimSpace(parts[0]), nil
			}
		}
	}

	return "", fmt.Errorf("no definition found")
}

// SchemaExists checks if a schema already exists
func (m *Manager) SchemaExists(packageName, schemaName string) bool {
	schemaFile := filepath.Join(m.schemaPath, packageName, schemaName+".cue")
	_, err := os.Stat(schemaFile)
	return err == nil
}

// GetPackages returns all available packages
func (m *Manager) GetPackages() ([]string, error) {
	var packages []string

	entries, err := os.ReadDir(m.schemaPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read schema directory: %w", err)
	}

	for _, entry := range entries {
		if entry.IsDir() && !strings.HasPrefix(entry.Name(), ".") {
			packages = append(packages, entry.Name())
		}
	}

	sort.Strings(packages)
	return packages, nil
}

// GetSchemasInPackage returns all schemas in a specific package
func (m *Manager) GetSchemasInPackage(packageName string) ([]SchemaInfo, error) {
	packageDir := filepath.Join(m.schemaPath, packageName)

	// Check if package directory exists
	if _, err := os.Stat(packageDir); os.IsNotExist(err) {
		return nil, fmt.Errorf("package not found: %s", packageName)
	}

	entries, err := os.ReadDir(packageDir)
	if err != nil {
		return nil, fmt.Errorf("failed to read package directory: %w", err)
	}

	var schemas []SchemaInfo
	for _, entry := range entries {
		if !entry.IsDir() && strings.HasSuffix(entry.Name(), ".cue") {
			schemaName := strings.TrimSuffix(entry.Name(), ".cue")
			schemaPath := filepath.Join(packageDir, entry.Name())

			info, err := entry.Info()
			if err != nil {
				continue
			}

			schemaInfo := SchemaInfo{
				Package:  packageName,
				Name:     schemaName,
				FullName: fmt.Sprintf("%s.%s", packageName, schemaName),
				FilePath: schemaPath,
				FileName: entry.Name(),
				Size:     info.Size(),
			}

			// Try to extract definition name
			if definition, err := m.extractDefinitionName(schemaPath); err == nil {
				schemaInfo.Definition = definition
			}

			schemas = append(schemas, schemaInfo)
		}
	}

	// Sort by schema name
	sort.Slice(schemas, func(i, j int) bool {
		return schemas[i].Name < schemas[j].Name
	})

	return schemas, nil
}

// ParseSchemaName parses a full schema name (package.name) into components
func ParseSchemaName(fullName string) (packageName, schemaName string, err error) {
	parts := strings.SplitN(fullName, ".", 2)
	if len(parts) != 2 {
		return "", "", fmt.Errorf("invalid schema name format: %s (expected package.name)", fullName)
	}

	packageName = strings.TrimSpace(parts[0])
	schemaName = strings.TrimSpace(parts[1])

	if packageName == "" {
		return "", "", fmt.Errorf("package name cannot be empty")
	}
	if schemaName == "" {
		return "", "", fmt.Errorf("schema name cannot be empty")
	}

	return packageName, schemaName, nil
}
