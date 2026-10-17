package main

import (
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestGenerateGoFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "icons.go")
	script, err := findGlyphScript(`<html><script>const glyphs = {"music": "f001", "album": "f02f",};</script></html>`)
	if err != nil {
		t.Fatal(err)
	}
	glyphs, err := parseGlyphs(script)
	if err != nil {
		t.Fatal(err)
	}
	if err := generateGoFile(path, glyphs); err != nil {
		t.Fatal(err)
	}
	source, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := parser.ParseFile(token.NewFileSet(), path, source, parser.AllErrors); err != nil {
		t.Fatalf("generated invalid Go: %v", err)
	}
	text := string(source)
	if !strings.Contains(text, `"album": "\uf02f"`) || !strings.Contains(text, `"music": "\uf001"`) || strings.Index(text, `"album"`) > strings.Index(text, `"music"`) {
		t.Fatalf("incorrect or unsorted glyphs: %s", text)
	}
	if err := generateGoFile(t.TempDir(), glyphs); err == nil {
		t.Fatal("writing to a directory succeeded")
	}
}

func TestInvalidGlyphDoesNotOverwriteFile(t *testing.T) {
	for _, value := range []string{"", "nope", "f001garbage", "d800", "110000", "ffffffff"} {
		t.Run(value, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "icons.go")
			if err := os.WriteFile(path, []byte("existing icons"), 0o600); err != nil {
				t.Fatal(err)
			}
			if err := generateGoFile(path, map[string]string{"bad": value}); err == nil {
				t.Fatal("accepted an invalid code point")
			}
			data, err := os.ReadFile(path)
			if err != nil || string(data) != "existing icons" {
				t.Fatalf("existing file changed: %q, %v", data, err)
			}
		})
	}
}

func TestInvalidGlyphScript(t *testing.T) {
	if _, err := findGlyphScript("<html>No glyphs</html>"); err == nil {
		t.Fatal("missing glyphs accepted")
	}
	for _, script := range []string{"", "const glyphs = }{", "const glyphs = {invalid};"} {
		t.Run(script, func(t *testing.T) {
			if _, err := parseGlyphs(script); err == nil {
				t.Fatal("invalid glyph object accepted")
			}
		})
	}
}
