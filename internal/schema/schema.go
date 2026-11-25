// Package schema validates AE YAML and JSON documents against the canonical
// JSON Schemas shipped with the framework. Schemas are embedded so that an
// installed ae binary works without the source checkout.
package schema

import (
	"bytes"
	"embed"
	"fmt"
	"sort"
	"strings"

	jsonschema "github.com/santhosh-tekuri/jsonschema/v6"
)

//go:embed assets/*.json
var assets embed.FS

// Names are the canonical schema identifiers accepted by Validate.
var Names = []string{"skill", "workflow", "agent", "policy", "routing", "issue", "evidence", "manifest", "review"}

// Asset returns the raw bytes of a canonical schema.
func Asset(name string) ([]byte, error) {
	data, err := assets.ReadFile("assets/" + name + ".schema.json")
	if err != nil {
		return nil, fmt.Errorf("read embedded schema %q: %w", name, err)
	}
	return data, nil
}

// Validate checks instance against the named schema. Errors are deterministic
// and sorted so CLI output and tests are stable.
func Validate(name string, instance any) error {
	data, err := Asset(name)
	if err != nil {
		return err
	}
	compiler := jsonschema.NewCompiler()
	compiler.DefaultDraft(jsonschema.Draft2020)
	resource, err := jsonschema.UnmarshalJSON(bytes.NewReader(data))
	if err != nil {
		return fmt.Errorf("parse schema %q: %w", name, err)
	}
	if err := compiler.AddResource("https://ae.sh/schemas/"+name+".schema.json", resource); err != nil {
		return fmt.Errorf("register schema %q: %w", name, err)
	}
	compiled, err := compiler.Compile("https://ae.sh/schemas/" + name + ".schema.json")
	if err != nil {
		return fmt.Errorf("compile schema %q: %w", name, err)
	}
	if err := compiled.Validate(instance); err != nil {
		var validationErr *jsonschema.ValidationError
		if ok := asValidationError(err, &validationErr); ok {
			return fmt.Errorf("%s schema validation failed: %s", name, formatValidationError(validationErr))
		}
		return fmt.Errorf("%s schema validation failed: %w", name, err)
	}
	return nil
}

func asValidationError(err error, target **jsonschema.ValidationError) bool {
	ve, ok := err.(*jsonschema.ValidationError)
	if ok {
		*target = ve
	}
	return ok
}

func formatValidationError(err *jsonschema.ValidationError) string {
	if err == nil {
		return ""
	}
	location := "/" + strings.Join(err.InstanceLocation, "/")
	if len(err.Causes) == 0 {
		return fmt.Sprintf("%s at %s", err.ErrorKind, location)
	}
	lines := make([]string, 0, len(err.Causes))
	for _, cause := range err.Causes {
		lines = append(lines, formatValidationError(cause))
	}
	sort.Strings(lines)
	return strings.Join(lines, "; ")
}
