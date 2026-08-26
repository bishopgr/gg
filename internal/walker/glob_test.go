package walker

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestExpandGlobs_LiteralExistingFileWinsOverGlobInterpretation(t *testing.T) {
	dir := t.TempDir()
	// A real file whose name contains glob metacharacters must be kept
	// literal, not glob-interpreted.
	literal := filepath.Join(dir, "a[1].txt")
	mustWriteFile(t, literal, "x")

	got, err := ExpandGlobs([]string{literal})
	if err != nil {
		t.Fatalf("ExpandGlobs error: %v", err)
	}
	want := []string{literal}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("ExpandGlobs(%v) = %v, want %v (literal-file precedence)", literal, got, want)
	}
}

func TestExpandGlobs_ExpandsAndSorts(t *testing.T) {
	dir := t.TempDir()
	mustWriteFile(t, filepath.Join(dir, "c.go"), "")
	mustWriteFile(t, filepath.Join(dir, "a.go"), "")
	mustWriteFile(t, filepath.Join(dir, "b.go"), "")
	mustWriteFile(t, filepath.Join(dir, "readme.md"), "")

	got, err := ExpandGlobs([]string{filepath.Join(dir, "*.go")})
	if err != nil {
		t.Fatalf("ExpandGlobs error: %v", err)
	}
	want := []string{
		filepath.Join(dir, "a.go"),
		filepath.Join(dir, "b.go"),
		filepath.Join(dir, "c.go"),
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("ExpandGlobs(*.go) = %v, want %v", got, want)
	}
}

func TestExpandGlobs_NoMatchPassesThroughUnchanged(t *testing.T) {
	dir := t.TempDir()
	pattern := filepath.Join(dir, "*.nonexistent")

	got, err := ExpandGlobs([]string{pattern})
	if err != nil {
		t.Fatalf("ExpandGlobs error: %v", err)
	}
	want := []string{pattern}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("ExpandGlobs(no-match) = %v, want %v (passthrough)", got, want)
	}
}

func TestExpandGlobs_NoMetacharactersPassesThroughUnchanged(t *testing.T) {
	got, err := ExpandGlobs([]string{"plain-arg-not-a-file"})
	if err != nil {
		t.Fatalf("ExpandGlobs error: %v", err)
	}
	want := []string{"plain-arg-not-a-file"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("ExpandGlobs(plain) = %v, want %v", got, want)
	}
}

func TestExpandGlobs_MixedLiteralAndGlobArgs(t *testing.T) {
	dir := t.TempDir()
	mustWriteFile(t, filepath.Join(dir, "one.txt"), "")
	mustWriteFile(t, filepath.Join(dir, "two.txt"), "")
	literalOnly := filepath.Join(dir, "only.md")
	mustWriteFile(t, literalOnly, "")

	got, err := ExpandGlobs([]string{literalOnly, filepath.Join(dir, "*.txt")})
	if err != nil {
		t.Fatalf("ExpandGlobs error: %v", err)
	}
	want := []string{
		literalOnly,
		filepath.Join(dir, "one.txt"),
		filepath.Join(dir, "two.txt"),
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("ExpandGlobs(mixed) = %v, want %v", got, want)
	}
}

func TestExpandGlobs_BadPatternReturnsError(t *testing.T) {
	_, err := ExpandGlobs([]string{"[unclosed"})
	if err == nil {
		t.Fatal("expected an error for an unclosed character class, got nil")
	}
}

func mustWriteFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}
