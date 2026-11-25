// Package routing defines provider-neutral task routing intents and resolves
// them to provider/model IDs from local configuration. Canonical assets never
// contain vendor model names; untrusted task content never selects a route.
package routing

import (
	"fmt"
	"os"
	"regexp"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/yoosuf/ae.sh/internal/framework"
	"github.com/yoosuf/ae.sh/internal/schema"
	"github.com/yoosuf/ae.sh/internal/util"
)

// Canonical routing profile IDs.
const (
	Fast              = "fast"
	Balanced          = "balanced"
	DeepReasoning     = "deep-reasoning"
	IndependentReview = "independent-review"
	Recovery          = "recovery"
)

// Profiles is the canonical profile set in documented order.
var Profiles = []string{Fast, Balanced, DeepReasoning, IndependentReview, Recovery}

// Providers that may be named in a local model map. Gemini is listed for
// forward compatibility; there is no Gemini adapter in this tree.
var Providers = []string{"claude", "codex", "gemini"}

// Profile is one provider-neutral routing intent from framework/routing.
type Profile struct {
	ID          string            `yaml:"id" json:"id"`
	Description string            `yaml:"description" json:"description"`
	UseFor      []string          `yaml:"useFor" json:"useFor"`
	Constraints map[string]string `yaml:"constraints,omitempty" json:"constraints,omitempty"`
}

// Catalogue is the validated canonical routing profile document.
type Catalogue struct {
	SchemaVersion string    `yaml:"schemaVersion" json:"schemaVersion"`
	Profiles      []Profile `yaml:"profiles" json:"profiles"`
	Path          string    `yaml:"-" json:"-"`
}

// Load reads and validates framework/routing/profiles.yaml.
func Load(root string) (Catalogue, error) {
	path := framework.Dir(root, framework.Routing, "profiles.yaml")
	raw, err := os.ReadFile(path)
	if err != nil {
		return Catalogue{}, fmt.Errorf("read routing profiles: %w", err)
	}
	var generic map[string]any
	if err := yaml.Unmarshal(raw, &generic); err != nil {
		return Catalogue{}, fmt.Errorf("parse routing profiles: %w", err)
	}
	doc, err := util.JSONValue(generic)
	if err != nil {
		return Catalogue{}, fmt.Errorf("routing profiles must be JSON-serializable: %w", err)
	}
	if err := schema.Validate("routing", doc); err != nil {
		return Catalogue{}, err
	}
	var catalogue Catalogue
	if err := yaml.Unmarshal(raw, &catalogue); err != nil {
		return Catalogue{}, err
	}
	catalogue.Path = path
	return catalogue, nil
}

// Normalize validates a routing profile. An empty value means the balanced
// default; unknown values fail closed.
func Normalize(value string) (string, error) {
	if strings.TrimSpace(value) == "" {
		return Balanced, nil
	}
	for _, profile := range Profiles {
		if value == profile {
			return value, nil
		}
	}
	return "", fmt.Errorf("unknown routing profile %q", value)
}

var modelPattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._:/\-\[\]]*$`)

// ValidateRoutes checks local profile -> provider -> model mappings. Model
// values are names or aliases, never credentials.
func ValidateRoutes(routes map[string]map[string]string) error {
	profiles := sortedKeys(routes)
	for _, profile := range profiles {
		if _, err := Normalize(profile); err != nil {
			return err
		}
		providers := sortedKeys(routes[profile])
		if len(providers) == 0 {
			return fmt.Errorf("routing profile %q has no provider mapping", profile)
		}
		for _, provider := range providers {
			if !isProvider(provider) {
				return fmt.Errorf("unsupported routing provider %q", provider)
			}
			model := routes[profile][provider]
			if model == "" {
				return fmt.Errorf("routing profile %q provider %q has an empty model", profile, provider)
			}
			if len(model) > 128 || !modelPattern.MatchString(model) {
				return fmt.Errorf("routing profile %q provider %q has an invalid model name", profile, provider)
			}
		}
	}
	return nil
}

// ModelFor resolves one profile for one provider. Missing mappings fail closed
// so a live run never silently falls back to an unintended model.
func ModelFor(routes map[string]map[string]string, provider, profile string) (string, error) {
	resolved, err := Normalize(profile)
	if err != nil {
		return "", err
	}
	byProvider, ok := routes[resolved]
	if !ok {
		return "", fmt.Errorf("no local model mapping for routing profile %q", resolved)
	}
	model, ok := byProvider[provider]
	if !ok || model == "" {
		return "", fmt.Errorf("no local model mapping for routing profile %q provider %q", resolved, provider)
	}
	return model, nil
}

func isProvider(name string) bool {
	for _, provider := range Providers {
		if name == provider {
			return true
		}
	}
	return false
}

func sortedKeys[V any](values map[string]V) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}
