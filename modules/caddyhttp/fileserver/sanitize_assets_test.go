package fileserver

import (
	"bytes"
	"strings"
	"testing"
)

func TestSanitizeStaticAsset(t *testing.T) {
	for _, tc := range []struct{ name, source, wanted string }{
		{"index.html", "<!doctype html><!-- private note --><html><head><script>/*secret*/const x=1;//hidden\nconsole.log(x)</script></head><body>Hello</body></html>", "private note"},
		{"style.css", "/* private style */ body { color: red; }", "private style"},
		{"app.js", "/* private script */const x=1; //hidden\nconsole.log(x)", "private script"},
	} {
		got, err := sanitizeStaticAsset(tc.name, strings.NewReader(tc.source), int64(len(tc.source)))
		if err != nil {
			t.Fatalf("%s: %v", tc.name, err)
		}
		if bytes.Contains(got, []byte(tc.wanted)) || bytes.Contains(got, []byte("hidden")) {
			t.Errorf("%s: comment leaked: %q", tc.name, got)
		}
		if len(got) == 0 {
			t.Errorf("%s: empty result", tc.name)
		}
	}
}

func TestSanitizeOversizeFailsClosed(t *testing.T) {
	_, err := sanitizeStaticAsset("index.html", strings.NewReader("body"), maxAutomaticSanitizeSize+1)
	if err == nil {
		t.Fatal("expected oversized source to fail closed")
	}
}

func TestSanitizeTemplateFailsClosed(t *testing.T) {
	src := "<div>{{.Name}}</div>"
	_, err := sanitizeStaticAsset("index.html", strings.NewReader(src), int64(len(src)))
	if err == nil {
		t.Fatal("expected template failure")
	}
}

func TestSanitizeFileTypes(t *testing.T) {
	for _, ext := range []string{".png", ".webp", ".pdf", ".zip", ".map"} {
		if kind := sanitizableAssetType("file" + ext); kind != "" {
			t.Fatalf("unexpected type %q for %s", kind, ext)
		}
	}
}
