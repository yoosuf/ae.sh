package routing

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestNormalizeFailsClosedOnUnknownProfile(t *testing.T) {
	if got, err := Normalize(""); err != nil || got != Balanced {
		t.Fatalf("empty profile: got %q, err %v", got, err)
	}
	if _, err := Normalize("cheapest"); err == nil {
		t.Fatal("unknown routing profile was accepted")
	}
}

func TestValidateRoutesRejectsCredentialShapedModel(t *testing.T) {
	routes := map[string]map[string]string{
		Fast: {"claude": "haiku"},
	}
	if err := ValidateRoutes(routes); err != nil {
		t.Fatalf("valid routes rejected: %v", err)
	}
	routes[Fast]["claude"] = "haiku?token=secret"
	if err := ValidateRoutes(routes); err == nil {
		t.Fatal("credential-shaped model was accepted")
	}
}

func TestModelForMissingMappingFailsClosed(t *testing.T) {
	routes := map[string]map[string]string{Fast: {"claude": "haiku"}}
	if model, err := ModelFor(routes, "codex", Fast); err == nil || model != "" {
		t.Fatalf("missing mapping did not fail closed: model %q, err %v", model, err)
	}
	model, err := ModelFor(routes, "claude", Fast)
	if err != nil || model != "haiku" {
		t.Fatalf("valid mapping: got %q, err %v", model, err)
	}
}

func TestLoadCanonicalProfiles(t *testing.T) {
	catalogue, err := Load(filepath.Join("..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	if len(catalogue.Profiles) != len(Profiles) {
		t.Fatalf("got %d routing profiles, want %d", len(catalogue.Profiles), len(Profiles))
	}
	for _, profile := range catalogue.Profiles {
		if !strings.Contains(profile.Description, " ") || len(profile.UseFor) == 0 {
			t.Fatalf("incomplete routing profile %q", profile.ID)
		}
	}
}
