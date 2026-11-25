package main

import "testing"

func TestGreet(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{name: "empty input", input: "", want: "hello, world"},
		{name: "whitespace input", input: "   ", want: "hello, world"},
		{name: "explicit name", input: "AE", want: "hello, AE"},
		{name: "trims surrounding space", input: "  Ada  ", want: "hello, Ada"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := Greet(tt.input); got != tt.want {
				t.Fatalf("Greet(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}
