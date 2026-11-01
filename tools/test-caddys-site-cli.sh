#!/bin/sh
# End-to-end tests against an isolated Caddyfile. Does not access production.
set -eu
[ "$(id -u)" -eq 0 ] || { echo 'Test requiere permisos root en entorno de pruebas'; exit 1; }
BIN=${CADDYS_BIN:-./caddys}
[ -x "$BIN" ] || { echo 'Ejecuta make build antes'; exit 1; }
d=$(mktemp -d)
trap 'rm -rf "$d"' EXIT HUP INT TERM
cat > "$d/Caddyfile" <<'CONF'
:9098 {
    respond "ok" 200
}
CONF
common="--config $d/Caddyfile --sites-dir $d/sites --web-root $d/www --group root"
$BIN site add --domain tienda.example.com --mode automatic --no-reload --yes $common >/dev/null
$BIN site add --domain prueba.example.net --mode cloudflare --no-reload --yes $common >/dev/null
$BIN site add --domain tunel.example.org --mode tunnel --port 8091 --no-reload --yes $common >/dev/null
$BIN validate --config "$d/Caddyfile" --adapter caddyfile >/dev/null 2>&1
[ "$(grep -c '^import ' "$d/Caddyfile")" -eq 1 ]
[ -f "$d/www/tienda.example.com/index.html" ]
[ -f "$d/sites/tienda.example.com.caddy" ]
[ "$(stat -c '%a' "$d/www/tienda.example.com")" = 750 ]
[ "$(stat -c '%a' "$d/www/tienda.example.com/index.html")" = 640 ]
$BIN site list $common | grep -q 'tunel.example.org'
$BIN site doctor tienda.example.com $common | grep -q 'No se realizaron cambios'
if $BIN site add --domain tienda.example.com --mode automatic --no-reload --yes $common >/dev/null 2>&1; then echo 'ERROR: se sobrescribió sitio';exit 10;fi
if $BIN site add --domain bad/../path --no-reload --yes $common >/dev/null 2>&1; then echo 'ERROR: se aceptó dominio peligroso';exit 11;fi
if $BIN site add --domain other.example.org --mode tunnel --port 8091 --no-reload --yes $common >/dev/null 2>&1; then echo 'ERROR: puerto duplicado';exit 12;fi
mkdir -p "$d/www/manual.example.net"
chmod 777 "$d/www/manual.example.net"
$BIN site doctor manual.example.net $common | grep -q 'no se cambió nada'
[ "$(stat -c '%a' "$d/www/manual.example.net")" = 777 ]
$BIN site doctor manual.example.net --fix-permissions $common | grep -q 'Corregidos'
[ "$(stat -c '%a' "$d/www/manual.example.net")" = 750 ]
# Existing main Caddyfile domains must never be silently replaced.
printf '\nmanual2.example.com {\n  respond ok\n}\n' >> "$d/Caddyfile"
if $BIN site add --domain manual2.example.com --mode automatic --no-reload --yes $common >/dev/null 2>&1; then echo 'ERROR: dominio manual duplicado';exit 13;fi
printf 'Pruebas end-to-end del asistente: OK\n'
