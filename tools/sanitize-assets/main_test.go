package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSanitize(t *testing.T) {
	base := t.TempDir()
	src := filepath.Join(base, "source")
	dst := filepath.Join(base, "built")
	if err := os.Mkdir(src, 0700); err != nil {
		t.Fatal(err)
	}
	files := map[string]string{
		"index.html": "<!doctype html><html><head></head><body><!-- hidden -->visible<script>const url='https://example.com';// comment\nconsole.log(url);</script></body></html>",
		"style.css":  "/* private */ body { color: red; }",
		"app.js":     "/* private */ const x='https://example.com'; // comment\nconsole.log(x);",
		"image.bin":  "unmodified",
		"app.js.map": "source map",
		".env":       "SECRET=1",
	}
	for name, content := range files {
		if err := os.WriteFile(filepath.Join(src, name), []byte(content), 0600); err != nil {
			t.Fatal(err)
		}
	}
	if err := sanitize(src, dst); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"index.html", "style.css", "app.js"} {
		b, err := os.ReadFile(filepath.Join(dst, name))
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(string(b), "comment") || strings.Contains(string(b), "private") || strings.Contains(string(b), "hidden") {
			t.Errorf("comentario filtrado en %s: %s", name, b)
		}
	}
	for _, name := range []string{".env", "app.js.map"} {
		if _, err := os.Stat(filepath.Join(dst, name)); !os.IsNotExist(err) {
			t.Errorf("archivo privado publicado: %s", name)
		}
	}
	if _, err := os.Stat(filepath.Join(src, ".env")); err != nil {
		t.Fatal("el origen fue modificado", err)
	}
	if err := sanitize(src, src); err == nil {
		t.Fatal("permitió origen como destino")
	}
	if err := sanitize(src, dst); err == nil {
		t.Fatal("sobrescribió salida")
	}
}

func TestRejectTemplateAndSymlink(t *testing.T) {
	base := t.TempDir()
	src := filepath.Join(base, "src")
	os.Mkdir(src, 0700)
	os.WriteFile(filepath.Join(src, "page.html"), []byte("{{ dynamic }}"), 0600)
	if err := sanitize(src, filepath.Join(base, "out")); err == nil {
		t.Fatal("HTML de plantilla aceptado")
	}
	os.Remove(filepath.Join(src, "page.html"))
	os.Symlink("/etc/passwd", filepath.Join(src, "passwd"))
	if err := sanitize(src, filepath.Join(base, "out")); err == nil {
		t.Fatal("symlink aceptado")
	}
}
