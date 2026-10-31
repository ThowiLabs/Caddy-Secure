package main

import (
	"bytes"
	"errors"
	"flag"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/tdewolff/minify/v2"
	"github.com/tdewolff/minify/v2/css"
	"github.com/tdewolff/minify/v2/html"
	"github.com/tdewolff/minify/v2/js"
)

const maxSourceBytes = 16 << 20

func main() {
	source := flag.String("input", "", "directorio de archivos originales")
	target := flag.String("output", "", "directorio nuevo para publicar")
	flag.Parse()
	if err := sanitize(*source, *target); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func sanitize(source, target string) error {
	if source == "" || target == "" {
		return errors.New("indica -input y -output")
	}
	src, err := filepath.Abs(source)
	if err != nil {
		return err
	}
	dst, err := filepath.Abs(target)
	if err != nil {
		return err
	}
	dstParent, err := filepath.EvalSymlinks(filepath.Dir(dst))
	if err != nil {
		return err
	}
	dst = filepath.Join(dstParent, filepath.Base(dst))
	src, err = filepath.EvalSymlinks(src)
	if err != nil {
		return err
	}
	parent, err := filepath.EvalSymlinks(filepath.Dir(dst))
	if err != nil {
		return err
	}
	dst = filepath.Join(parent, filepath.Base(dst))
	if nested(src, dst) || nested(dst, src) {
		return errors.New("los directorios de origen y destino deben ser independientes")
	}
	info, err := os.Stat(src)
	if err != nil || !info.IsDir() {
		return errors.New("el origen debe ser un directorio existente")
	}
	if _, err := os.Lstat(dst); err == nil {
		return errors.New("el directorio destino ya existe")
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}

	tmp, err := os.MkdirTemp(filepath.Dir(dst), ".sanitize-*")
	if err != nil {
		return err
	}
	defer os.RemoveAll(tmp)

	m := minify.New()
	m.Add("text/html", &html.Minifier{KeepDocumentTags: true, KeepEndTags: true, KeepQuotes: true, KeepDefaultAttrVals: true, KeepWhitespace: true})
	m.Add("text/css", &css.Minifier{})
	m.Add("text/javascript", &js.Minifier{KeepVarNames: true})
	m.Add("application/javascript", &js.Minifier{KeepVarNames: true})

	err = filepath.WalkDir(src, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
		if d.Type()&os.ModeSymlink != 0 {
			return fmt.Errorf("enlace simbólico no permitido: %s", rel)
		}
		if rel == "." {
			return nil
		}
		if protectedName(d.Name()) {
			if d.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		out := filepath.Join(tmp, rel)
		if d.IsDir() {
			return os.Mkdir(out, 0755)
		}
		if !d.Type().IsRegular() {
			return fmt.Errorf("archivo no regular: %s", rel)
		}
		input, err := os.Open(path)
		if err != nil {
			return err
		}
		output, err := os.OpenFile(out, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0644)
		if err != nil {
			input.Close()
			return err
		}
		kind := mimeFor(filepath.Ext(d.Name()))
		if kind == "" {
			_, err = io.Copy(output, input)
		} else {
			var b []byte
			b, err = io.ReadAll(io.LimitReader(input, maxSourceBytes+1))
			if err == nil && len(b) > maxSourceBytes {
				err = fmt.Errorf("archivo demasiado grande: %s", rel)
			}
			if err == nil && kind == "text/html" && (bytes.Contains(b, []byte("<?")) || bytes.Contains(b, []byte("{{")) || bytes.Contains(b, []byte("{%"))) {
				err = fmt.Errorf("plantilla HTML requiere compilación previa: %s", rel)
			}
			if err == nil {
				err = m.Minify(kind, output, bytes.NewReader(b))
			}
		}
		inputCloseErr := input.Close()
		outputCloseErr := output.Close()
		if err == nil {
			err = inputCloseErr
		}
		if err == nil {
			err = outputCloseErr
		}
		if err != nil {
			return fmt.Errorf("%s: %w", rel, err)
		}
		return nil
	})
	if err != nil {
		return err
	}
	if err := os.Chmod(tmp, 0755); err != nil {
		return err
	}
	return os.Rename(tmp, dst)
}

func mimeFor(ext string) string {
	switch strings.ToLower(ext) {
	case ".html", ".htm":
		return "text/html"
	case ".css":
		return "text/css"
	case ".js", ".mjs", ".cjs":
		return "text/javascript"
	}
	return ""
}

func protectedName(name string) bool {
	name = strings.ToLower(name)
	switch name {
	case ".git", ".hg", ".svn", ".ssh", ".ds_store", "wp-config.php":
		return true
	}
	return name == ".env" || strings.HasPrefix(name, ".env.") || strings.HasSuffix(name, ".map")
}

func nested(a, b string) bool {
	rel, err := filepath.Rel(a, b)
	return err == nil && (rel == "." || (rel != ".." && !strings.HasPrefix(rel, ".."+string(os.PathSeparator))))
}
