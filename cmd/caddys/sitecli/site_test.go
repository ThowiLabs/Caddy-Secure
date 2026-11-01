package sitecli

import (
	"bytes"
	"os"
	"os/user"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

func TestDomains(t *testing.T) {
	for _, s := range []string{"Ejemplo.COM", "sub.dominio.mx", "xn--n3h.example", "pastelesjudith.duckdns.org"} {
		if _, err := validDomain(s); err != nil {
			t.Errorf("rechazado %q: %v", s, err)
		}
	}
	for _, s := range []string{"../etc/passwd", "/root", "", "localhost", "a..com", "-a.com", "a-.com", "bad_name.com", "a.com:8080", "a.com\nimport *", "foo{.com", "*.example.com"} {
		if _, err := validDomain(s); err == nil {
			t.Errorf("aceptado peligroso %q", s)
		}
	}
}
func TestOptions(t *testing.T) {
	p := paths{config: "/etc/caddy/Caddyfile", sites: "/etc/caddy/sites", webroot: "/var/www", group: "caddy"}
	for _, x := range []siteOptions{
		{domain: "example.com", mode: "automatic", root: "/var/www/example.com", paths: p},
		{domain: "example.com", mode: "cloudflare", root: "/var/www/example.com", paths: p},
		{domain: "example.com", mode: "tunnel", port: 8090, root: "/var/www/example.com", paths: p},
	} {
		if err := validateOptions(x); err != nil {
			t.Errorf("rechazado: %v", err)
		}
	}
	for _, x := range []siteOptions{
		{domain: "example.com", mode: "tunnel", port: 80, root: "/var/www/example.com", paths: p},
		{domain: "example.com", mode: "automatic", port: 8090, root: "/var/www/example.com", paths: p},
		{domain: "example.com", mode: "automatic", root: "/etc/passwd", paths: p},
		{domain: "example.com", mode: "automatic", root: "/var/www/../etc", paths: p},
		{domain: "example.com", mode: "automatic", root: "/var/www/mi sitio", paths: p},
	} {
		if err := validateOptions(x); err == nil {
			t.Errorf("se permitió opción insegura: %+v", x)
		}
	}
}
func TestRender(t *testing.T) {
	o := siteOptions{domain: "example.com", mode: "automatic", root: "/var/www/example.com"}
	got := renderSite(o)
	if !strings.Contains(got, "example.com {") || !strings.Contains(got, "root * /var/www/example.com") || strings.Contains(got, "bind 127.0.0.1") {
		t.Fatal(got)
	}
	o.mode = "tunnel"
	o.port = 8090
	got = renderSite(o)
	if !strings.Contains(got, "http://:8090 {") || !strings.Contains(got, "bind 127.0.0.1") {
		t.Fatal(got)
	}
}
func TestManagedSiteMetadata(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "example.com.caddy")
	if err := os.WriteFile(p, []byte(renderSite(siteOptions{domain: "example.com", mode: "tunnel", port: 8100, root: "/var/www/example.com"})), 0644); err != nil {
		t.Fatal(err)
	}
	i, err := readSiteInfo(p)
	if err != nil || i.domain != "example.com" || i.port != 8100 || i.mode != "tunnel" {
		t.Fatalf("info=%+v error=%v", i, err)
	}
	if err = os.WriteFile(p, []byte("# unmanaged site\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if _, err = readSiteInfo(p); err == nil {
		t.Fatal("aceptó sitio no administrado")
	}
}
func TestDoctorReadOnlyAndExplicitFix(t *testing.T) {
	group, err := user.LookupGroupId(strconv.Itoa(os.Getgid()))
	if err != nil {
		t.Skipf("grupo no disponible: %v", err)
	}
	root := filepath.Join(t.TempDir(), "example.com")
	if err = os.Mkdir(root, 0750); err != nil {
		t.Fatal(err)
	}
	file := filepath.Join(root, "index.html")
	if err = os.WriteFile(file, []byte("test"), 0640); err != nil {
		t.Fatal(err)
	}
	os.Chmod(root, 0777)
	os.Chmod(file, 0666)
	var b bytes.Buffer
	if err = doctor(root, group.Name, false, &b); err != nil {
		t.Fatal(err)
	}
	st, _ := os.Stat(file)
	if st.Mode().Perm() != 0666 {
		t.Fatalf("doctor sin --fix modificó %o", st.Mode().Perm())
	}
	if !strings.Contains(b.String(), "no se cambió nada") {
		t.Fatal(b.String())
	}
	b.Reset()
	if err = doctor(root, group.Name, true, &b); err != nil {
		t.Fatal(err)
	}
	st, _ = os.Stat(file)
	if st.Mode().Perm() != 0640 {
		t.Fatalf("fix no corrigió: %o", st.Mode().Perm())
	}
	st, _ = os.Stat(root)
	if st.Mode().Perm() != 0750 {
		t.Fatalf("fix no corrigió dir: %o", st.Mode().Perm())
	}
}
func TestDoctorDoesNotFollowSymlinks(t *testing.T) {
	group, err := user.LookupGroupId(strconv.Itoa(os.Getgid()))
	if err != nil {
		t.Skip(err)
	}
	tmp := t.TempDir()
	root := filepath.Join(tmp, "example.com")
	os.Mkdir(root, 0755)
	outside := filepath.Join(tmp, "outside.txt")
	os.WriteFile(outside, []byte("secret"), 0600)
	if err = os.Symlink(outside, filepath.Join(root, "linked")); err != nil {
		t.Skip(err)
	}
	var b bytes.Buffer
	if err = doctor(root, group.Name, true, &b); err == nil {
		t.Fatal("debe rechazar symlinks")
	}
	st, _ := os.Stat(outside)
	if st.Mode().Perm() != 0600 {
		t.Fatal("modificó un archivo fuera del sitio")
	}
	st, _ = os.Stat(root)
	if st.Mode().Perm() != 0755 {
		t.Fatal("modificó archivos antes de detectar symlink")
	}
}
