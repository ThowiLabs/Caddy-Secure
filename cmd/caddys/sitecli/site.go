package sitecli

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"time"
)

var pathRx = regexp.MustCompile(`^[a-zA-Z0-9_./-]+$`)
var labelRx = regexp.MustCompile(`^[a-z0-9](?:[a-z0-9-]{0,61}[a-z0-9])?$`)

// validDomain prevents path traversal and malformed Caddyfile site labels.
func validDomain(s string) (string, error) {
	s = strings.ToLower(strings.TrimSpace(s))
	if strings.HasSuffix(s, ".") {
		s = strings.TrimSuffix(s, ".")
	}
	if len(s) > 253 || len(s) < 3 || !strings.Contains(s, ".") || strings.ContainsAny(s, "/\\:* \r\n\t\x00") {
		return "", fmt.Errorf("dominio DNS inválido: %q", s)
	}
	for _, l := range strings.Split(s, ".") {
		if !labelRx.MatchString(l) {
			return "", fmt.Errorf("etiqueta inválida en el dominio: %q", l)
		}
	}
	return s, nil
}
func validMode(s string) bool { return s == "automatic" || s == "cloudflare" || s == "tunnel" }

type siteOptions struct {
	domain, mode, root string
	port               int
	paths              paths
	noReload           bool
}
type siteInfo struct {
	domain, mode, root string
	port               int
}

func validateOptions(o siteOptions) error {
	if _, err := validDomain(o.domain); err != nil {
		return err
	}
	if !validMode(o.mode) {
		return fmt.Errorf("modo inválido: %q", o.mode)
	}
	if o.mode == "tunnel" {
		if o.port < 1024 || o.port > 65535 {
			return fmt.Errorf("puerto de túnel fuera de rango (1024-65535): %d", o.port)
		}
	} else if o.port != 0 {
		return errors.New("--port solo está disponible con --mode tunnel")
	}
	for _, p := range []string{o.root, o.paths.webroot, o.paths.config, o.paths.sites} {
		if !filepath.IsAbs(p) {
			return fmt.Errorf("se requiere ruta absoluta: %q", p)
		}
		if !pathRx.MatchString(p) {
			return fmt.Errorf("ruta no apta para un Caddyfile seguro (usa letras ASCII, números, /, ., _ y -): %q", p)
		}
	}
	if o.paths.group == "" {
		return errors.New("el grupo del servidor es obligatorio")
	}
	base := filepath.Clean(o.paths.webroot)
	root := filepath.Clean(o.root)
	rel, err := filepath.Rel(base, root)
	if err != nil || rel == "." || strings.HasPrefix(rel, ".."+string(os.PathSeparator)) || rel == ".." {
		return fmt.Errorf("la carpeta debe estar dentro de %s para prevenir cambios fuera del directorio web", base)
	}
	if root == "/" || base == "/" {
		return errors.New("no se permite administrar la raíz del sistema")
	}
	if o.paths.sites == "/" || o.paths.config == "/" {
		return errors.New("directorio de configuración inválido")
	}
	return nil
}

func renderSite(o siteOptions) string {
	site := o.domain
	if o.mode == "tunnel" {
		site = "http://:" + strconv.Itoa(o.port)
	}
	var b strings.Builder
	fmt.Fprintf(&b, "# caddys-managed domain=%s mode=%s root=%s port=%d\n", o.domain, o.mode, filepath.ToSlash(o.root), o.port)
	fmt.Fprintf(&b, "%s {\n", site)
	if o.mode == "tunnel" {
		fmt.Fprintln(&b, "    bind 127.0.0.1")
	}
	fmt.Fprintf(&b, "    root * %s\n    encode gzip\n    file_server\n}\n", filepath.ToSlash(o.root))
	return b.String()
}

func mentionsDomain(content []byte, domain string) bool {
	rx := regexp.MustCompile(`(?i)(^|[\s,])(?:https?://)?` + regexp.QuoteMeta(domain) + `(?::[0-9]+)?(?:[\s,{]|$)`)
	for _, line := range strings.Split(string(content), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		if rx.MatchString(line) {
			return true
		}
	}
	return false
}

func freeLocalPort(start int) (int, error) {
	for port := start; port < start+100; port++ {
		l, err := net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", port))
		if err == nil {
			l.Close()
			return port, nil
		}
	}
	return 0, errors.New("no se encontró un puerto local libre entre 8090 y 8189")
}

func readSiteInfo(filename string) (siteInfo, error) {
	var info siteInfo
	file, err := os.Open(filename)
	if err != nil {
		return info, err
	}
	defer file.Close()
	buf := make([]byte, 2048)
	n, err := file.Read(buf)
	if err != nil && !errors.Is(err, io.EOF) {
		return info, err
	}
	line, _, _ := strings.Cut(string(buf[:n]), "\n")
	if !strings.HasPrefix(line, "# caddys-managed ") {
		return info, errors.New("no generado por Caddys")
	}
	for _, field := range strings.Fields(strings.TrimPrefix(line, "# caddys-managed ")) {
		k, v, ok := strings.Cut(field, "=")
		if !ok {
			continue
		}
		switch k {
		case "domain":
			info.domain = v
		case "mode":
			info.mode = v
		case "root":
			info.root = v
		case "port":
			info.port, _ = strconv.Atoi(v)
		}
	}
	domain, err := validDomain(info.domain)
	if err != nil {
		return siteInfo{}, err
	}
	if !validMode(info.mode) || !filepath.IsAbs(info.root) || strings.TrimSpace(info.root) == "/" {
		return siteInfo{}, errors.New("metadatos inválidos")
	}
	info.domain = domain
	if filenameDomain := strings.TrimSuffix(filepath.Base(filename), ".caddy"); filenameDomain != domain {
		return siteInfo{}, errors.New("metadatos del dominio no coinciden con el nombre del archivo")
	}
	return info, nil
}

func ensureRealDirectories(path string) error {
	clean := filepath.Clean(path)
	for cur := clean; cur != "/" && cur != "."; cur = filepath.Dir(cur) {
		fi, err := os.Lstat(cur)
		if os.IsNotExist(err) {
			continue
		}
		if err != nil {
			return err
		}
		if fi.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("no se permite un enlace simbólico en la ruta: %s", cur)
		}
		if !fi.IsDir() {
			return fmt.Errorf("la ruta no es un directorio: %s", cur)
		}
	}
	return nil
}

func makeDir(path string, mode os.FileMode) (bool, error) {
	if err := ensureRealDirectories(path); err != nil {
		return false, err
	}
	if _, err := os.Lstat(path); err == nil {
		return false, nil
	} else if !os.IsNotExist(err) {
		return false, err
	}
	// Parent directories need traversal permission for the caddy service user.
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return false, err
	}
	if err := ensureRealDirectories(path); err != nil {
		return false, err
	}
	if err := os.Mkdir(path, mode); err != nil {
		return false, err
	}
	if err := os.Chmod(path, mode); err != nil {
		return true, err
	}
	return true, nil
}

func executable() string {
	if p, err := os.Executable(); err == nil {
		return p
	}
	return "caddys"
}
func validateConfig(conf string) error {
	cmd := exec.Command(executable(), "validate", "--config", conf, "--adapter", "caddyfile")
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("no se pudo validar Caddyfile: %w: %s", err, strings.TrimSpace(string(out)))
	}
	return nil
}

func reloadConfig(conf string) error {
	// OpenRC uses the unprivileged caddy user and already knows the service flags.
	if _, err := exec.LookPath("rc-service"); err == nil && conf == "/etc/caddy/Caddyfile" {
		if exec.Command("rc-service", "caddys", "status").Run() == nil {
			out, err := exec.Command("rc-service", "caddys", "reload").CombinedOutput()
			if err != nil {
				return fmt.Errorf("falló rc-service caddys reload: %w: %s", err, out)
			}
			return nil
		}
		// A stopped service is not started implicitly by a site operation.
		return nil
	}
	cmd := exec.Command(executable(), "reload", "--config", conf, "--adapter", "caddyfile")
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("recarga fallida: %w: %s", err, strings.TrimSpace(string(out)))
	}
	return nil
}

func atomicReplace(path string, data []byte, mode os.FileMode) error {
	dir := filepath.Dir(path)
	f, err := os.CreateTemp(dir, ".caddys-site-*")
	if err != nil {
		return err
	}
	defer os.Remove(f.Name())
	if err = f.Chmod(mode); err != nil {
		f.Close()
		return err
	}
	if _, err = f.Write(data); err != nil {
		f.Close()
		return err
	}
	if err = f.Sync(); err != nil {
		f.Close()
		return err
	}
	if err = f.Close(); err != nil {
		return err
	}
	if err = copyOwnership(path, f.Name()); err != nil {
		return err
	}
	return os.Rename(f.Name(), path)
}

func addSite(o siteOptions, w io.Writer) (err error) {
	if err = validateOptions(o); err != nil {
		return err
	}
	if runtime.GOOS != "windows" && os.Geteuid() != 0 {
		return errors.New("site add modifica /etc/caddy y /var/www: ejecútalo con sudo")
	}
	sitePath := filepath.Join(o.paths.sites, o.domain+".caddy")
	if _, er := os.Lstat(sitePath); er == nil {
		return fmt.Errorf("el sitio ya existe: %s (no se sobrescribe)", sitePath)
	} else if !os.IsNotExist(er) {
		return er
	}
	for _, p := range []string{filepath.Dir(o.paths.config), o.paths.sites, o.root} {
		if er := ensureRealDirectories(p); er != nil {
			return er
		}
	}
	// Avoid changing existing non-managed configurations and validate before touching them.
	orig, er := os.ReadFile(o.paths.config)
	if er != nil && !os.IsNotExist(er) {
		return er
	}
	existed := er == nil
	if existed {
		if match := mentionsDomain(orig, o.domain); match {
			return fmt.Errorf("el dominio %s ya aparece en el Caddyfile principal; revisa su configuración manual antes de registrarlo", o.domain)
		}
	}
	entries, scanErr := os.ReadDir(o.paths.sites)
	if scanErr != nil && !os.IsNotExist(scanErr) {
		return scanErr
	}
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".caddy") {
			continue
		}
		// Existing unmanaged fragments may define the domain too.
		content, fe := os.ReadFile(filepath.Join(o.paths.sites, entry.Name()))
		if fe != nil {
			return fe
		}
		if mentionsDomain(content, o.domain) {
			return fmt.Errorf("el dominio %s ya figura en %s", o.domain, entry.Name())
		}
	}
	originalMode := os.FileMode(0644)
	if existed {
		if fi, se := os.Stat(o.paths.config); se == nil {
			originalMode = fi.Mode().Perm()
		}
	}
	if existed {
		if er = validateConfig(o.paths.config); er != nil {
			return fmt.Errorf("la configuración actual debe ser válida antes de crear sitios: %w", er)
		}
	}
	_, er = groupID(o.paths.group)
	if er != nil {
		return er
	}
	if o.mode == "tunnel" {
		entries, scanErr := os.ReadDir(o.paths.sites)
		if scanErr != nil && !os.IsNotExist(scanErr) {
			return scanErr
		}
		for _, entry := range entries {
			if !strings.HasSuffix(entry.Name(), ".caddy") || entry.IsDir() {
				continue
			}
			existing, er := readSiteInfo(filepath.Join(o.paths.sites, entry.Name()))
			if er == nil && existing.mode == "tunnel" && existing.port == o.port {
				return fmt.Errorf("el puerto %d ya está reservado por el sitio %s", o.port, existing.domain)
			}
		}
		l, e := net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", o.port))
		if e != nil {
			return fmt.Errorf("el puerto local %d no está disponible: %w", o.port, e)
		}
		l.Close()
	}
	// Only create the web root if missing. Never rewrite existing user content.
	rootCreated, er := makeDir(o.root, 0750)
	if er != nil {
		return er
	}
	if rootCreated {
		if er = securePath(o.root, true, o.paths.group); er != nil {
			return er
		}
	}
	placeholder := filepath.Join(o.root, "index.html")
	placeholderCreated := false
	if _, er = os.Lstat(placeholder); os.IsNotExist(er) {
		sample := []byte("<!doctype html><html lang=\"es\"><meta charset=\"utf-8\"><title>" + o.domain + "</title><h1>" + o.domain + "</h1></html>\n")
		f, fe := os.OpenFile(placeholder, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0640)
		if fe != nil {
			return fe
		}
		_, fe = f.Write(sample)
		closeErr := f.Close()
		if fe == nil {
			fe = closeErr
		}
		if fe != nil {
			os.Remove(placeholder)
			return fe
		}
		placeholderCreated = true
		if fe = securePath(placeholder, false, o.paths.group); fe != nil {
			os.Remove(placeholder)
			return fe
		}
	} else if er != nil {
		return er
	}
	// Keep external files intact on failure; only remove files created here.
	committed := false
	siteCreated := false
	configChanged := false
	defer func() {
		if committed {
			return
		}
		if configChanged {
			if existed {
				_ = atomicReplace(o.paths.config, orig, originalMode)
			} else {
				_ = os.Remove(o.paths.config)
			}
		}
		if siteCreated {
			_ = os.Remove(sitePath)
		}
		if placeholderCreated {
			_ = os.Remove(placeholder)
		}
		if rootCreated {
			_ = os.Remove(o.root)
		} // only if it is now empty
	}()
	if _, er = makeDir(o.paths.sites, 0755); er != nil {
		return er
	}
	newSite := []byte(renderSite(o))
	f, er := os.OpenFile(sitePath, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0644)
	if er != nil {
		return er
	}
	siteCreated = true
	_, er = f.Write(newSite)
	closeErr := f.Close()
	if er == nil {
		er = closeErr
	}
	if er != nil {
		return er
	}
	// Main configuration is left untouched when it already imports the sites dir.
	importLine := "import " + filepath.ToSlash(filepath.Join(o.paths.sites, "*.caddy"))
	hasImport := false
	for _, line := range strings.Split(string(orig), "\n") {
		if strings.TrimSpace(line) == importLine {
			hasImport = true
			break
		}
	}
	candidate := orig
	if !hasImport {
		if len(candidate) > 0 && !bytes.HasSuffix(candidate, []byte("\n")) {
			candidate = append(candidate, '\n')
		}
		candidate = append(candidate, []byte("\n# Sitios gestionados por caddys site add\n"+importLine+"\n")...)
	}
	// Validate with a temporary candidate Caddyfile in its normal directory.
	confDir := filepath.Dir(o.paths.config)
	if _, er = os.Stat(confDir); os.IsNotExist(er) {
		if _, er = makeDir(confDir, 0755); er != nil {
			return er
		}
	} else if er != nil {
		return er
	}
	if hasImport {
		if er = validateConfig(o.paths.config); er != nil {
			return er
		}
	} else {
		tmp, te := os.CreateTemp(confDir, ".caddys-check-*.caddy")
		if te != nil {
			return te
		}
		tempName := tmp.Name()
		defer os.Remove(tempName)
		_, te = tmp.Write(candidate)
		cl := tmp.Close()
		if te == nil {
			te = cl
		}
		if te != nil {
			return te
		}
		if er = validateConfig(tempName); er != nil {
			return er
		}
	}
	if !hasImport {
		if existed {
			current, re := os.ReadFile(o.paths.config)
			if re != nil {
				return re
			}
			if !bytes.Equal(current, orig) {
				return errors.New("Caddyfile cambiado durante la operación; no se sobrescribe")
			}
		}
		if existed {
			backup := o.paths.config + ".bak.caddys-site-" + time.Now().UTC().Format("20060102T150405.000000000Z")
			b, be := os.OpenFile(backup, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
			if be != nil {
				return be
			}
			_, be = b.Write(orig)
			ce := b.Close()
			if be == nil {
				be = ce
			}
			if be != nil {
				os.Remove(backup)
				return be
			}
			fmt.Fprintf(w, "Respaldo: %s\n", backup)
		}
		mode := os.FileMode(0644)
		if existed {
			if st, e := os.Stat(o.paths.config); e == nil {
				mode = st.Mode().Perm()
			}
		}
		if er = atomicReplace(o.paths.config, candidate, mode); er != nil {
			return er
		}
		configChanged = true
	}
	if !o.noReload {
		if er = reloadConfig(o.paths.config); er != nil {
			if configChanged {
				if existed {
					_ = atomicReplace(o.paths.config, orig, originalMode)
				} else {
					_ = os.Remove(o.paths.config)
				}
				_ = reloadConfig(o.paths.config) // best-effort restore of the previous running config
			}
			return fmt.Errorf("la recarga falló, se intentó restaurar la configuración anterior: %w", er)
		}
	}
	committed = true
	fmt.Fprintf(w, "Archivo del sitio: %s\nCarpeta: %s\n", sitePath, o.root)
	if o.noReload {
		fmt.Fprintln(w, "Configuración validada. No se recargó el servicio (--no-reload).")
	}
	return nil
}
