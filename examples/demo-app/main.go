// Command demo-app is a dependency-free hello application used by AEF examples.
package main

import (
	"fmt"
	"strings"
)

// Greet returns a deterministic greeting for name. Empty names use "world".
func Greet(name string) string {
	name = strings.TrimSpace(name)
	if name == "" {
		name = "world"
	}
	return fmt.Sprintf("hello, %s", name)
}

func main() {
	fmt.Println(Greet(""))
}
