package schema

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

// Parse parses a ResourceDefinition from a YAML byte slice.
func Parse(data []byte) (*ResourceDefinition, error) {
	var res ResourceDefinition
	if err := yaml.Unmarshal(data, &res); err != nil {
		return nil, fmt.Errorf("failed to unmarshal resource definition: %w", err)
	}

	// Extract key order for fields using yaml.Node
	var root yaml.Node
	if err := yaml.Unmarshal(data, &root); err == nil {
		res.FieldOrder = extractFieldOrder(&root)
	}

	if err := res.Validate(); err != nil {
		return nil, fmt.Errorf("invalid resource definition: %w", err)
	}
	return &res, nil
}

func extractFieldOrder(root *yaml.Node) []string {
	if root == nil || len(root.Content) == 0 {
		return nil
	}
	doc := root.Content[0]
	if doc.Kind != yaml.MappingNode {
		return nil
	}

	var order []string
	for i := 0; i < len(doc.Content)-1; i += 2 {
		keyNode := doc.Content[i]
		valNode := doc.Content[i+1]
		if keyNode.Value == "fields" && valNode.Kind == yaml.MappingNode {
			for j := 0; j < len(valNode.Content)-1; j += 2 {
				order = append(order, valNode.Content[j].Value)
			}
			break
		}
	}
	return order
}

// ParseReader parses a ResourceDefinition from an io.Reader.
func ParseReader(r io.Reader) (*ResourceDefinition, error) {
	data, err := io.ReadAll(r)
	if err != nil {
		return nil, fmt.Errorf("failed to read data: %w", err)
	}
	return Parse(data)
}

// ParseFile parses a ResourceDefinition from a file path.
func ParseFile(path string) (*ResourceDefinition, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read file '%s': %w", path, err)
	}
	return Parse(data)
}

// LoadDirectory loads all YAML ResourceDefinitions from a directory.
func LoadDirectory(dir string) ([]*ResourceDefinition, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, fmt.Errorf("failed to read directory '%s': %w", dir, err)
	}

	var definitions []*ResourceDefinition
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		ext := strings.ToLower(filepath.Ext(entry.Name()))
		if ext == ".yaml" || ext == ".yml" {
			filePath := filepath.Join(dir, entry.Name())
			def, err := ParseFile(filePath)
			if err != nil {
				return nil, fmt.Errorf("error loading '%s': %w", filePath, err)
			}
			definitions = append(definitions, def)
		}
	}

	if len(definitions) == 0 {
		return nil, fmt.Errorf("no resource definition files (.yaml, .yml) found in '%s'", dir)
	}

	return definitions, nil
}
