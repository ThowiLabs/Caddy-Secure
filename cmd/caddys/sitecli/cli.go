// Package sitecli provides optional Caddys site management. It never modifies
// the upstream Caddy command package or runs from normal server startup.
package sitecli

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"

	"github.com/spf13/cobra"
)

// Configure installs the site subcommands on the Caddys-only CLI.
func Configure(site *cobra.Command) {
	site.Long = "Asistente opcional de sitios estáticos de Caddys. No modifica nada al iniciar el servidor."
	site.AddCommand(addCommand(), listCommand(), doctorCommand())
}

type paths struct{ config, sites, webroot, group string }

func addPathFlags(cmd *cobra.Command, p *paths) {
	cmd.Flags().StringVar(&p.config, "config", "/etc/caddy/Caddyfile", "Caddyfile principal")
	cmd.Flags().StringVar(&p.sites, "sites-dir", "/etc/caddy/sites", "directorio de configuraciones gestionadas")
	cmd.Flags().StringVar(&p.webroot, "web-root", "/var/www", "carpeta base de documentos web")
	cmd.Flags().StringVar(&p.group, "group", "caddy", "grupo del proceso servidor que necesita lectura")
}

func addCommand() *cobra.Command {
	p := paths{}
	var domain, mode, root string
	var port int
	var yes, noReload bool
	cmd := &cobra.Command{
		Use: "add", Short: "Crear un sitio y su directorio con asistente interactivo",
		Long: "Crea /var/www/<dominio> por defecto; gestiona un fichero Caddyfile separado, valida y solicita confirmación antes de activar. Nunca reemplaza contenido existente.",
		RunE: func(c *cobra.Command, args []string) error {
			if len(args) != 0 {
				return fmt.Errorf("site add no acepta argumentos posicionales; usa --domain")
			}
			rdr := bufio.NewReader(c.InOrStdin())
			var err error
			if domain == "" {
				if yes {
					return errors.New("--yes requiere --domain")
				}
				domain, err = ask(rdr, c.OutOrStdout(), "Dominio", "")
				if err != nil {
					return err
				}
			}
			domain, err = validDomain(domain)
			if err != nil {
				return err
			}
			if mode == "" {
				if yes {
					mode = "automatic"
				} else {
					mode, err = ask(rdr, c.OutOrStdout(), "HTTPS: automatic (Caddys), cloudflare (proxy Full strict) o tunnel (cloudflared)", "automatic")
					if err != nil {
						return err
					}
				}
			}
			mode = strings.ToLower(strings.TrimSpace(mode))
			if !validMode(mode) {
				return fmt.Errorf("modo inválido %q; elige automatic, cloudflare o tunnel", mode)
			}
			if root == "" {
				root = filepath.Join(p.webroot, domain)
			}
			if !yes {
				root, err = ask(rdr, c.OutOrStdout(), "Carpeta pública", root)
				if err != nil {
					return err
				}
			}
			if mode == "tunnel" && port == 0 {
				port, err = freeLocalPort(8090)
				if err != nil {
					return err
				}
				if !yes {
					var answer string
					answer, err = ask(rdr, c.OutOrStdout(), "Puerto local para cloudflared", strconv.Itoa(port))
					if err != nil {
						return err
					}
					port, err = strconv.Atoi(answer)
					if err != nil {
						return fmt.Errorf("puerto inválido: %w", err)
					}
				}
			}
			o := siteOptions{domain: domain, mode: mode, root: root, port: port, paths: p, noReload: noReload}
			if err = validateOptions(o); err != nil {
				return err
			}
			preview := renderSite(o)
			fmt.Fprintf(c.OutOrStdout(), "\nSitio: %s\nModo: %s\nDirectorio: %s\nArchivo: %s\n\n%s\n", domain, mode, root, filepath.Join(p.sites, domain+".caddy"), preview)
			if mode == "cloudflare" {
				fmt.Fprintln(c.OutOrStdout(), "Aviso: configura Cloudflare Full (strict) SOLO cuando haya un certificado válido en el origen; no se administra Cloudflare desde aquí.")
			}
			if mode == "tunnel" {
				fmt.Fprintf(c.OutOrStdout(), "En cloudflared, crea una ruta pública %s hacia http://127.0.0.1:%d. El asistente NO modifica el túnel.\n", domain, port)
			}
			if !yes {
				var reply string
				reply, err = ask(rdr, c.OutOrStdout(), "¿Aplicar y validar? [s/N]", "n")
				if err != nil {
					return err
				}
				if strings.ToLower(reply) != "s" && strings.ToLower(reply) != "si" && strings.ToLower(reply) != "sí" {
					fmt.Fprintln(c.OutOrStdout(), "Cancelado. Sin cambios.")
					return nil
				}
			}
			if err = addSite(o, c.OutOrStdout()); err != nil {
				return err
			}
			fmt.Fprintf(c.OutOrStdout(), "Sitio %s configurado. Ejecuta 'caddys site doctor %s' para una auditoría de permisos.\n", domain, domain)
			return nil
		},
	}
	addPathFlags(cmd, &p)
	cmd.Flags().StringVar(&domain, "domain", "", "dominio DNS del sitio")
	cmd.Flags().StringVar(&mode, "mode", "", "automatic, cloudflare o tunnel")
	cmd.Flags().StringVar(&root, "root", "", "carpeta del sitio (por defecto /var/www/<dominio>)")
	cmd.Flags().IntVar(&port, "port", 0, "puerto local para el modo tunnel")
	cmd.Flags().BoolVar(&yes, "yes", false, "aplicar sin preguntas (requiere --domain)")
	cmd.Flags().BoolVar(&noReload, "no-reload", false, "no recargar servicio, solo validar y escribir")
	return cmd
}

func ask(r *bufio.Reader, w io.Writer, question, def string) (string, error) {
	if def == "" {
		fmt.Fprintf(w, "%s: ", question)
	} else {
		fmt.Fprintf(w, "%s [%s]: ", question, def)
	}
	line, err := r.ReadString('\n')
	if err != nil && !errors.Is(err, io.EOF) {
		return "", err
	}
	line = strings.TrimSpace(line)
	if line == "" {
		line = def
	}
	if line == "" {
		return "", fmt.Errorf("%s: valor requerido", question)
	}
	return line, nil
}

func listCommand() *cobra.Command {
	p := paths{}
	cmd := &cobra.Command{Use: "list", Short: "Listar sitios gestionados por el asistente", RunE: func(c *cobra.Command, args []string) error {
		if len(args) != 0 {
			return errors.New("site list no recibe argumentos")
		}
		entries, err := os.ReadDir(p.sites)
		if os.IsNotExist(err) {
			fmt.Fprintln(c.OutOrStdout(), "No hay sitios gestionados.")
			return nil
		}
		if err != nil {
			return err
		}
		count := 0
		for _, entry := range entries {
			if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".caddy") {
				continue
			}
			domain := strings.TrimSuffix(entry.Name(), ".caddy")
			if _, err = validDomain(domain); err != nil {
				continue
			}
			info, er := readSiteInfo(filepath.Join(p.sites, entry.Name()))
			if er != nil {
				continue
			}
			fmt.Fprintf(c.OutOrStdout(), "%-35s %-11s %s\n", domain, info.mode, info.root)
			count++
		}
		if count == 0 {
			fmt.Fprintln(c.OutOrStdout(), "No hay sitios gestionados.")
		}
		return nil
	}}
	addPathFlags(cmd, &p)
	return cmd
}

func doctorCommand() *cobra.Command {
	p := paths{}
	var fix bool
	cmd := &cobra.Command{Use: "doctor <dominio>", Short: "Revisar permisos (sin cambios) o corregirlos explícitamente", Args: cobra.ExactArgs(1), RunE: func(c *cobra.Command, args []string) error {
		domain, err := validDomain(args[0])
		if err != nil {
			return err
		}
		file := filepath.Join(p.sites, domain+".caddy")
		info, err := readSiteInfo(file)
		if os.IsNotExist(err) {
			// The manual site fallback is restricted to /var/www/<domain>.
			info = siteInfo{domain: domain, mode: "automatic", root: filepath.Join(p.webroot, domain)}
			if _, se := os.Lstat(info.root); se != nil {
				return fmt.Errorf("no hay sitio gestionado ni carpeta %s: %w", info.root, se)
			}
			fmt.Fprintf(c.OutOrStdout(), "Sitio manual detectado: %s (su Caddyfile no se modifica).\n", info.root)
		} else if err != nil {
			return fmt.Errorf("sitio no administrado: %w", err)
		}
		if err := validateOptions(siteOptions{domain: domain, mode: info.mode, root: info.root, port: info.port, paths: p}); err != nil {
			return err
		}
		if fix && runtime.GOOS != "windows" && os.Geteuid() != 0 {
			return errors.New("--fix-permissions requiere sudo")
		}
		return doctor(info.root, p.group, fix, c.OutOrStdout())
	}}
	addPathFlags(cmd, &p)
	cmd.Flags().BoolVar(&fix, "fix-permissions", false, "corregir permisos y propiedad solamente bajo el sitio")
	return cmd
}
