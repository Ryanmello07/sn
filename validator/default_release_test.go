package validator

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
)

func TestOmittedReleaseConfigReadsBundledDocument(t *testing.T) {
	path, data, err := readReleaseConfigFile("")
	if err != nil {
		t.Fatal(err)
	}
	want, err := os.ReadFile("release.yml")
	if err != nil {
		t.Fatal(err)
	}
	state, err := defaultStateDir()
	if err != nil {
		t.Fatal(err)
	}
	if path != filepath.Join(state, "release.yml") || !bytes.Equal(data, want) {
		t.Fatalf("default did not use the bundled document and state-dir anchor: %s", path)
	}
	data[0] ^= 1
	_, again, err := readReleaseConfigFile("")
	if err != nil || !bytes.Equal(again, want) {
		t.Fatal("caller mutated the embedded default")
	}
}

func TestExplicitReleaseConfigNeverFallsBackOrMerges(t *testing.T) {
	path := filepath.Join(t.TempDir(), "release.yml")
	if _, _, err := readReleaseConfigFile(path); !os.IsNotExist(err) {
		t.Fatalf("missing explicit file fell back: %v", err)
	}
	want := []byte("explicit configuration bytes\n")
	if err := os.WriteFile(path, want, 0o600); err != nil {
		t.Fatal(err)
	}
	_, got, err := readReleaseConfigFile(path)
	if err != nil || !bytes.Equal(got, want) {
		t.Fatalf("explicit file changed: %q, %v", got, err)
	}
	if _, _, err := readReleaseConfigFile("   "); err == nil {
		t.Fatal("an explicitly invalid path selected defaults")
	}
}

func TestBundledReleaseConfigDoesNotBypassProductionValidation(t *testing.T) {
	if cfg, err := LoadReleaseConfig(""); err == nil || cfg != nil {
		t.Fatal("incomplete production defaults acquired runtime authority")
	}
}
