package atomicfile

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestWriteCreatesAndReplaces(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "state.json")

	for _, content := range []string{"first", "second"} {
		if err := Write(path, []byte(content)); err != nil {
			t.Fatal(err)
		}
		got, err := os.ReadFile(path) // #nosec G304 -- path is in the test temp directory
		if err != nil {
			t.Fatal(err)
		}
		if string(got) != content {
			t.Errorf("file = %q, want %q", got, content)
		}
	}

	if runtime.GOOS != "windows" {
		info, err := os.Stat(path)
		if err != nil {
			t.Fatal(err)
		}
		if perm := info.Mode().Perm(); perm != 0o600 {
			t.Errorf("mode = %o, want 600", perm)
		}
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 {
		t.Errorf("directory has %d entries, want only the file (no temporary files left)", len(entries))
	}
}

func TestWriteFailsInMissingDirectory(t *testing.T) {
	if err := Write(filepath.Join(t.TempDir(), "missing", "f"), []byte("x")); err == nil {
		t.Error("no error for a missing directory")
	}
}
