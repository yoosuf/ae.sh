package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// A symlink's DirEntry reports the small size of the link itself, but
// os.ReadFile follows it. The secret scan must skip symlinks so a link to a
// device or a large file cannot bypass the size bound.
func TestScanSecretShapesSkipsSymlinks(t *testing.T) {
	root := t.TempDir()
	secretPath := filepath.Join(root, "secret.txt")
	// Build the shape at runtime so the source fixture itself is not a
	// credential-shaped literal that ae audit would flag.
	secret := "token sk-" + strings.Repeat("a", 20) + "\n"
	if err := os.WriteFile(secretPath, []byte(secret), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(secretPath, filepath.Join(root, "link.txt")); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}

	files, findings, paths, err := scanSecretShapes(root)
	if err != nil {
		t.Fatal(err)
	}
	if files != 1 || findings != 1 {
		t.Fatalf("got %d files and %d findings, want 1 and 1 (paths %v)", files, findings, paths)
	}
	if len(paths) != 1 || paths[0] != "secret.txt" {
		t.Fatalf("symlink was scanned: paths %v", paths)
	}
}

func TestScanSecretShapesFailsOnUnreadableRoot(t *testing.T) {
	missing := filepath.Join(t.TempDir(), "missing")
	if _, _, _, err := scanSecretShapes(missing); err == nil {
		t.Fatal("a partial scan with an unreadable root was reported as clean")
	}
}
