package fileserver

import (
	"bytes"
	"fmt"
	"io"
	"path/filepath"
	"strings"
	"sync"

	"github.com/tdewolff/minify/v2"
	"github.com/tdewolff/minify/v2/css"
	"github.com/tdewolff/minify/v2/html"
	"github.com/tdewolff/minify/v2/js"
)

// Limit memory amplification under hostile traffic. Larger sanitizable assets
// fail closed instead of accidentally exposing their original source comments.
const maxAutomaticSanitizeSize = 2 << 20

var assetMinifier = sync.OnceValue(func() *minify.M {
	m := minify.New()
	m.Add("text/html", &html.Minifier{
		KeepDocumentTags: true, KeepEndTags: true,
		KeepQuotes: true, KeepDefaultAttrVals: true, KeepWhitespace: true,
	})
	m.Add("text/css", &css.Minifier{})
	m.Add("text/javascript", &js.Minifier{KeepVarNames: true})
	m.Add("application/javascript", &js.Minifier{KeepVarNames: true})
	return m
})

func sanitizableAssetType(filename string) string {
	switch strings.ToLower(filepath.Ext(filename)) {
	case ".html", ".htm":
		return "text/html"
	case ".css":
		return "text/css"
	case ".js", ".mjs", ".cjs":
		return "text/javascript"
	default:
		return ""
	}
}

func sanitizeStaticAsset(filename string, src io.Reader, size int64) ([]byte, error) {
	kind := sanitizableAssetType(filename)
	if kind == "" {
		return nil, fmt.Errorf("unsupported sanitization type: %s", filename)
	}
	if size > maxAutomaticSanitizeSize {
		return nil, fmt.Errorf("refusing oversized sanitizable asset %s (%d bytes)", filename, size)
	}
	raw, err := io.ReadAll(io.LimitReader(src, maxAutomaticSanitizeSize+1))
	if err != nil {
		return nil, fmt.Errorf("reading %s: %w", filename, err)
	}
	if len(raw) > maxAutomaticSanitizeSize {
		return nil, fmt.Errorf("refusing oversized sanitizable asset %s", filename)
	}
	if kind == "text/html" && (bytes.Contains(raw, []byte("<?")) || bytes.Contains(raw, []byte("{{")) || bytes.Contains(raw, []byte("{%"))) {
		return nil, fmt.Errorf("uncompiled HTML template requires rendering before sanitization: %s", filename)
	}
	var clean bytes.Buffer
	if err := assetMinifier().Minify(kind, &clean, bytes.NewReader(raw)); err != nil {
		return nil, fmt.Errorf("sanitizing %s: %w", filename, err)
	}
	return clean.Bytes(), nil
}
