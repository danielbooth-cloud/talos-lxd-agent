package payload

import (
	"os"
	"path/filepath"
	"testing"
)

func TestCopyTree(t *testing.T) {
	src := t.TempDir()
	dst := t.TempDir()

	if err := os.Mkdir(filepath.Join(src, "nested"), 0o750); err != nil {
		t.Fatal(err)
	}

	if err := os.WriteFile(filepath.Join(src, "lxd-agent"), []byte("agent"), 0o755); err != nil {
		t.Fatal(err)
	}

	if err := os.WriteFile(filepath.Join(src, "nested", "agent.crt"), []byte("certificate"), 0o600); err != nil {
		t.Fatal(err)
	}

	if err := os.Symlink("lxd-agent", filepath.Join(src, "agent-link")); err != nil {
		t.Fatal(err)
	}

	if err := CopyTree(src, dst); err != nil {
		t.Fatal(err)
	}

	assertFile(t, filepath.Join(dst, "lxd-agent"), "agent", 0o755)
	assertFile(t, filepath.Join(dst, "nested", "agent.crt"), "certificate", 0o600)

	link, err := os.Readlink(filepath.Join(dst, "agent-link"))
	if err != nil {
		t.Fatal(err)
	}

	if link != "lxd-agent" {
		t.Fatalf("unexpected link target %q", link)
	}
}

func TestClean(t *testing.T) {
	root := t.TempDir()

	for _, name := range []string{"keep", "remove"} {
		if err := os.Mkdir(filepath.Join(root, name), 0o700); err != nil {
			t.Fatal(err)
		}
	}

	if err := Clean(root, "keep"); err != nil {
		t.Fatal(err)
	}

	if _, err := os.Stat(filepath.Join(root, "keep")); err != nil {
		t.Fatalf("kept path missing: %v", err)
	}

	if _, err := os.Stat(filepath.Join(root, "remove")); !os.IsNotExist(err) {
		t.Fatalf("removed path still exists: %v", err)
	}
}

func assertFile(t *testing.T, path, want string, mode os.FileMode) {
	t.Helper()

	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}

	if string(got) != want {
		t.Fatalf("unexpected content %q", got)
	}

	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}

	if info.Mode().Perm() != mode {
		t.Fatalf("unexpected mode %o", info.Mode().Perm())
	}
}
