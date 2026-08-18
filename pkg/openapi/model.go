package openapi

// OpenAPI represents the root OpenAPI 3.1.0 document.
type OpenAPI struct {
	OpenAPI    string                `json:"openapi" yaml:"openapi"`
	Info       Info                  `json:"info" yaml:"info"`
	Servers    []Server              `json:"servers,omitempty" yaml:"servers,omitempty"`
	Security   []map[string][]string `json:"security,omitempty" yaml:"security,omitempty"`
	Paths      map[string]PathItem   `json:"paths" yaml:"paths"`
	Components Components            `json:"components" yaml:"components"`
}

type Info struct {
	Title       string `json:"title" yaml:"title"`
	Version     string `json:"version" yaml:"version"`
	Description string `json:"description,omitempty" yaml:"description,omitempty"`
}

type Server struct {
	URL         string `json:"URL" yaml:"URL"`
	Description string `json:"description,omitempty" yaml:"description,omitempty"`
}

type PathItem struct {
	Summary     string     `json:"summary,omitempty" yaml:"summary,omitempty"`
	Description string     `json:"description,omitempty" yaml:"description,omitempty"`
	Get         *Operation `json:"get,omitempty" yaml:"get,omitempty"`
	Post        *Operation `json:"post,omitempty" yaml:"post,omitempty"`
	Patch       *Operation `json:"patch,omitempty" yaml:"patch,omitempty"`
	Delete      *Operation `json:"delete,omitempty" yaml:"delete,omitempty"`
}

type Operation struct {
	Tags        []string              `json:"tags,omitempty" yaml:"tags,omitempty"`
	Summary     string                `json:"summary,omitempty" yaml:"summary,omitempty"`
	Description string                `json:"description,omitempty" yaml:"description,omitempty"`
	OperationID string                `json:"operationId,omitempty" yaml:"operationId,omitempty"`
	Parameters  []Parameter           `json:"parameters,omitempty" yaml:"parameters,omitempty"`
	RequestBody *RequestBody          `json:"requestBody,omitempty" yaml:"requestBody,omitempty"`
	Responses   map[string]Response   `json:"responses" yaml:"responses"`
}

type Parameter struct {
	Name        string  `json:"name" yaml:"name"`
	In          string  `json:"in" yaml:"in"` // query, path, header
	Description string  `json:"description,omitempty" yaml:"description,omitempty"`
	Required    bool    `json:"required,omitempty" yaml:"required,omitempty"`
	Schema      *Schema `json:"schema,omitempty" yaml:"schema,omitempty"`
}

type RequestBody struct {
	Description string               `json:"description,omitempty" yaml:"description,omitempty"`
	Required    bool                 `json:"required,omitempty" yaml:"required,omitempty"`
	Content     map[string]MediaType `json:"content" yaml:"content"`
}

type MediaType struct {
	Schema *Schema `json:"schema" yaml:"schema"`
}

type Response struct {
	Description string               `json:"description" yaml:"description"`
	Content     map[string]MediaType `json:"content,omitempty" yaml:"content,omitempty"`
}

type Components struct {
	Schemas         map[string]*Schema        `json:"schemas,omitempty" yaml:"schemas,omitempty"`
	SecuritySchemes map[string]SecurityScheme `json:"securitySchemes,omitempty" yaml:"securitySchemes,omitempty"`
}

type SecurityScheme struct {
	Type         string `json:"type" yaml:"type"` // http, apiKey
	Scheme       string `json:"scheme,omitempty" yaml:"scheme,omitempty"`
	BearerFormat string `json:"bearerFormat,omitempty" yaml:"bearerFormat,omitempty"`
	In           string `json:"in,omitempty" yaml:"in,omitempty"` // header
	Name         string `json:"name,omitempty" yaml:"name,omitempty"`
	Description  string `json:"description,omitempty" yaml:"description,omitempty"`
}

type Schema struct {
	Type        string             `json:"type,omitempty" yaml:"type,omitempty"`
	Format      string             `json:"format,omitempty" yaml:"format,omitempty"`
	Description string             `json:"description,omitempty" yaml:"description,omitempty"`
	Required    []string           `json:"required,omitempty" yaml:"required,omitempty"`
	Properties  map[string]*Schema `json:"properties,omitempty" yaml:"properties,omitempty"`
	Items       *Schema            `json:"items,omitempty" yaml:"items,omitempty"`
	Ref         string             `json:"$ref,omitempty" yaml:"$ref,omitempty"`
	Enum        []interface{}      `json:"enum,omitempty" yaml:"enum,omitempty"`
	ReadOnly    bool               `json:"readOnly,omitempty" yaml:"readOnly,omitempty"`
}
