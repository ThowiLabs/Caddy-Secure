#!/bin/sh
# Caddys installer for Alpine Linux/OpenRC (x86_64).
# Source: https://github.com/ThowiLabs/Caddy-Secure
set -eu

BASE='https://pastelesjudith.duckdns.org/caddys'
NAME='caddys'
BIN='/usr/local/sbin/caddys'
SERVICE='/etc/init.d/caddys'
ENVFILE='/etc/conf.d/caddys'

fail() { printf 'ERROR: %s\n' "$*" >&2; exit 1; }
[ "$(id -u)" -eq 0 ] || fail 'ejecuta este script como root (sudo sh install.sh)'
[ -f /etc/alpine-release ] || fail 'instalador disponible únicamente para Alpine Linux'
[ "$(uname -m)" = 'x86_64' ] || fail 'esta versión está compilada únicamente para Linux x86_64'
command -v rc-service >/dev/null || fail 'OpenRC no está disponible'
command -v gzip >/dev/null || fail 'gzip no está disponible'
command -v sha256sum >/dev/null || fail 'sha256sum no está disponible'

if command -v curl >/dev/null; then
    fetch() { curl --fail --silent --show-error --location --retry 2 --connect-timeout 15 --max-time 240 "$1" -o "$2"; }
elif command -v wget >/dev/null; then
    fetch() { wget -q -T 240 -O "$2" "$1"; }
else
    fail 'necesitas curl o wget para descargar Caddys'
fi

# Downloads live in a private working directory and are verified prior to
# replacing the active executable. Never execute a network-fetched binary
# before its checksum has been checked.
work=$(mktemp -d /tmp/caddys-install.XXXXXXXX) || exit 1
trap 'rm -rf "$work"' 0
trap 'exit 130' 1 2 3 15
printf 'Descargando Caddys para Alpine Linux x86_64...\n'
fetch "$BASE/caddys-linux-amd64.gz" "$work/caddys-linux-amd64.gz"
fetch "$BASE/caddys-linux-amd64.gz.sha256" "$work/caddys-linux-amd64.gz.sha256"
(cd "$work" && sha256sum -c caddys-linux-amd64.gz.sha256) || fail 'verificación SHA-256 fallida'
gzip -dc "$work/caddys-linux-amd64.gz" > "$work/$NAME"
chmod 755 "$work/$NAME"
"$work/$NAME" version

# Preserve any Caddyfile and certificate storage so an existing HTTPS site
# retains its identity. A new installation listens on :8080, not 80/443.
mkdir -p /etc/caddy /var/lib/caddy /usr/local/sbin /etc/conf.d /var/backups/caddys
if ! grep -q '^caddy:' /etc/group; then addgroup -S caddy; fi
if ! grep -q '^caddy:' /etc/passwd; then
    adduser -S -D -H -h /var/lib/caddy -s /sbin/nologin -G caddy caddy
fi
chown caddy:caddy /var/lib/caddy
if [ ! -s /etc/caddy/Caddyfile ]; then
    cat > /etc/caddy/Caddyfile <<'CADDYFILE'
:8080 {
    respond "Caddys instalado" 200
}
CADDYFILE
    chmod 644 /etc/caddy/Caddyfile
fi

# Preflight against the actual deployment config before stopping old services.
"$work/$NAME" validate --config /etc/caddy/Caddyfile --adapter caddyfile >/dev/null \
    || fail 'el Caddyfile existente no es compatible; no se cambió el servicio'

stamp=$(date -u +%Y%m%dT%H%M%SZ)
backup="/var/backups/caddys/${stamp}-$$"
mkdir -p "$backup"
chmod 700 "$backup"
cp -p /etc/caddy/Caddyfile "$backup/Caddyfile"
for f in "$BIN" "$SERVICE" "$ENVFILE" /usr/local/sbin/caddy-secure /etc/init.d/caddy-secure /etc/conf.d/caddy-secure /usr/sbin/caddy /etc/init.d/caddy; do
    [ ! -f "$f" ] || cp -p "$f" "$backup/$(printf '%s' "$f" | tr '/' '_')"
done

if rc-service caddys status >/dev/null 2>&1; then
    previous=caddys
elif rc-service caddy-secure status >/dev/null 2>&1; then
    previous=caddy-secure
elif rc-service caddy status >/dev/null 2>&1; then
    previous=caddy
else
    previous=''
fi

# Atomic executable replacement; a partially copied binary is never launched.
install -m 0755 "$work/$NAME" "$BIN.new.$$"
mv -f "$BIN.new.$$" "$BIN"
cat > "$SERVICE" <<'OPENRC'
#!/sbin/openrc-run
supervisor=supervise-daemon
name="Caddys web server"
description="Secure Caddy-derived HTTP server"
: ${caddys_opts:="--config /etc/caddy/Caddyfile --adapter caddyfile"}
command=/usr/local/sbin/caddys
command_args="run $caddys_opts"
command_user=caddy:caddy
capabilities="^cap_net_bind_service"
extra_commands="checkconfig"
extra_started_commands="reload"
depend() {
    need net localmount
    after firewall
}
checkconfig() {
    ebegin "Validando configuración de Caddys"
    su caddy -s /bin/sh -c "$command validate $caddys_opts"
    eend $?
}
reload() {
    ebegin "Recargando Caddys"
    su caddy -s /bin/sh -c "$command reload --force $caddys_opts"
    eend $?
}
stop_pre() {
    if [ "$RC_CMD" = restart ]; then checkconfig || return $?; fi
}
OPENRC
chmod 755 "$SERVICE"
if [ ! -f "$ENVFILE" ]; then
    printf 'export GOMEMLIMIT=256MiB\nexport GOMAXPROCS=1\n' > "$ENVFILE"
fi
chmod 644 "$ENVFILE"

if [ -n "$previous" ]; then
    printf 'Deteniendo servicio anterior: %s\n' "$previous"
    rc-service "$previous" stop || fail 'no se pudo detener el servicio previo; comprueba el servicio manualmente'
fi

if ! rc-service caddys start || ! rc-service caddys status; then
    printf 'No inició Caddys. Restaurando respaldo y servicio anterior...\n' >&2
    rc-service caddys stop >/dev/null 2>&1 || true
    for target in "$BIN" "$SERVICE" "$ENVFILE"; do
        saved="$backup/$(printf '%s' "$target" | tr '/' '_')"
        if [ -f "$saved" ]; then cp -p "$saved" "$target"; else rm -f "$target"; fi
    done
    if [ -n "$previous" ]; then rc-service "$previous" start || true; fi
    fail 'la instalación falló y se intentó restaurar el servicio anterior'
fi

rc-update add caddys default
for legacy in caddy caddy-secure; do
    if [ -L "/etc/runlevels/default/$legacy" ]; then
        rc-update del "$legacy" default 2>/dev/null || true
    fi
done
# Only remove the old custom name after successfully starting Caddys.
# Never delete apk-owned /usr/sbin/caddy: package upgrades own that path.
if [ "$previous" = caddy-secure ]; then
    rm -f /etc/init.d/caddy-secure /etc/conf.d/caddy-secure /usr/local/sbin/caddy-secure
fi
printf '\nCaddys instalado y activo.\n'
printf 'Binario: %s\nServicio: %s\nRespaldo: %s\n' "$BIN" "$SERVICE" "$backup"
printf 'Administrar: rc-service caddys {status|restart|stop}\n'
printf 'Código fuente: https://github.com/ThowiLabs/Caddy-Secure\n'
