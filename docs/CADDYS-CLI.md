# Caddys: manual de comandos y administración de sitios

> Distribución de Caddy con defensas integradas y utilidades opcionales. La sintaxis original del Caddyfile se conserva. Esta guía cubre las funciones específicas de **Caddys**; para el resto de los parámetros originales consulta `caddys <comando> --help` y https://caddyserver.com/docs/command-line.

## Inicio rápido (Alpine/OpenRC)

```sh
caddys version
caddys --help
caddys site --help
sudo caddys site add
sudo caddys site list
sudo caddys site doctor ejemplo.com
sudo caddys site doctor ejemplo.com --fix-permissions
```

**Ninguna herramienta cambia permisos en segundo plano.** El servidor normal (`run`, `reload`, `start`, etc.) no corrige archivos. `site add` crea y protege solo la carpeta e `index.html` nuevos; los archivos preexistentes no se sobreescriben. `site doctor` solo informa por defecto. Para corregir permisos existentes necesitas `--fix-permissions` de forma explícita.

## `site add`: flujo interactivo

```sh
sudo caddys site add
```

1. Introduces un dominio completo válido, por ejemplo `tienda.ejemplo.com`.
2. Eliges `automatic`, `cloudflare` o `tunnel` (opción predeterminada: `automatic`).
3. Confirmas la carpeta propuesta: **`/var/www/tienda.ejemplo.com/`**. Enter conserva esa ruta. Todas las carpetas administradas deben estar bajo `/var/www` por seguridad.
4. En `tunnel` se solicita un puerto local libre (comenzando en 8090).
5. El asistente muestra la configuración y solicita confirmación. Hasta ese momento no crea nada.
6. Si no existe, crea el directorio con `0750` y grupo `caddy`, y un `index.html` inicial `0640` (si ya hay sitio o archivos, no los reemplaza).
7. Crea `/etc/caddy/sites/tienda.ejemplo.com.caddy` y agrega una sola vez `import /etc/caddy/sites/*.caddy` al Caddyfile principal, conservando el resto de la configuración.
8. Comprueba la configuración mediante `caddys validate`; deja una copia de respaldo antes de modificar el Caddyfile principal; intenta recargar Caddys y revierte los archivos nuevos en caso de fallo de validación/recarga.

**Precaución:** la herramienta no crea DNS, certificados externos, reglas de firewall ni configuraciones de Cloudflare. Si el servicio está detenido, no se inicia automáticamente. Si ya tienes el mismo dominio definido manualmente en el Caddyfile, no lo agregues nuevamente; revisa o migra esa configuración de forma controlada.

### Modos de conexión

**`automatic` (predeterminado)**

```caddyfile
miweb.com {
    root * /var/www/miweb.com
    encode gzip
    file_server
}
```

Caddys gestiona HTTPS igual que Caddy, si el dominio resuelve hacia el servidor y puede completar la validación ACME. Para funcionamiento público normal permite HTTP/HTTPS (normalmente puertos 80 y 443) y revisa las reglas del firewall.

**`cloudflare` (proxy con DNS naranja)**

El Caddyfile del origen es igual al modo `automatic`. Se espera cifrado **Cloudflare ↔ Caddys** con un certificado válido y Cloudflare SSL/TLS **Full (strict)**; el asistente **no** configura el panel de Cloudflare. Evita el modo Flexible: puede producir redirecciones infinitas y deja sin cifrar el tramo Cloudflare-origen. Si necesitas Cloudflare Origin CA, instala ese certificado y configura el sitio de forma manual; el asistente básico no gestiona esos certificados.

**`tunnel` (servicio cloudflared)**

```caddyfile
http://:8090 {
    bind 127.0.0.1
    root * /var/www/miweb.com
    encode gzip
    file_server
}
```

El puerto 8090 es un ejemplo, se elige uno libre; Caddys lo ata solo a la interfaz loopback. En la configuración de `cloudflared`, enlaza el nombre público a `http://127.0.0.1:8090`. El TLS externo lo maneja Cloudflare Tunnel, no Caddys. El asistente no instala ni gestiona `cloudflared`. Cada sitio de túnel necesita un puerto diferente. El proceso `cloudflared` debe ejecutarse **en el mismo host** o un entorno con acceso explícito al loopback del origen.

### Sin preguntas, para scripts

```sh
sudo caddys site add --domain tienda.ejemplo.com --mode automatic --yes
sudo caddys site add --domain web.ejemplo.net --mode tunnel --port 8091 --yes
sudo caddys site add --domain web.ejemplo.com --mode cloudflare --yes
```

`--yes` requiere `--domain` y omite la confirmación. No aplica cambios automáticos en Cloudflare ni DNS.

### Flags de `site add`

| Flag | Predeterminado | Función |
| --- | --- | --- |
| `--domain` | Pregunta en CLI | Dominio del sitio |
| `--mode` | `automatic` | `automatic`, `cloudflare` o `tunnel` |
| `--root` | `/var/www/<dominio>` | Directorio público; debe estar dentro de `--web-root` |
| `--port` | Puerto libre desde 8090 | Puerto loopback, solo para `tunnel` |
| `--yes` | `false` | No preguntar, aplicar directamente |
| `--no-reload` | `false` | Crear y validar sin recargar el servicio |
| `--config` | `/etc/caddy/Caddyfile` | Caddyfile principal |
| `--sites-dir` | `/etc/caddy/sites` | Configuraciones individuales |
| `--web-root` | `/var/www` | Directorio base de sitios |
| `--group` | `caddy` | Grupo que necesita leer los archivos |

Se admiten rutas absolutas compuestas por caracteres ASCII seguros (`a-z`, `A-Z`, números, `/`, `.`, `_` y `-`). No se siguen enlaces simbólicos en rutas administradas. El asistente requiere `sudo` en Linux.

## `site list`

```sh
caddys site list
```

Muestra **solo los sitios creados por `site add`**, leyendo el encabezado de sus ficheros `.caddy` del directorio administrado. No significa que liste todas las webs definidas manualmente.

## `site doctor`

```sh
sudo caddys site doctor tienda.ejemplo.com
```

Examina permisos y grupo de archivos bajo el directorio del sitio. Por defecto no modifica ningún archivo. Requisitos esperados en una web estática administrada: carpetas `0750`, archivos `0640`, grupo `caddy`. Las carpetas padre deben permitir la traversía al usuario `caddy`. No corrige ACL ni SELinux (si existieran), ni inspecciona el contenido de archivos para buscar malware.

**Corrección explícita**:

```sh
sudo caddys site doctor tienda.ejemplo.com --fix-permissions
```

Recorre **solamente la carpeta pública del sitio**. Si es un sitio antiguo sin archivo `.caddy` gestionado, acepta la carpeta predeterminada `/var/www/<dominio>` siempre que exista y nunca cambia su Caddyfile. Rechaza enlaces simbólicos o archivos especiales para evitar cambios fuera del sitio. En la configuración estática predeterminada, fija permisos `0750`/`0640` y propietario `root:grupo` al ejecutar la corrección como root; puede quitar bits ejecutables de archivos que los tengan. Si sirves CGI u otras aplicaciones que necesitan ejecución, emplea configuración de permisos específica y evita esta corrección masiva.

## Compatibilidad con Caddy upstream

Comandos originales disponibles sin cambios de sintaxis: `caddys run`, `start`, `stop`, `reload`, `validate`, `adapt`, `fmt`, `version`, `list-modules`, entre otros. Por ejemplo:

```sh
sudo caddys validate --config /etc/caddy/Caddyfile --adapter caddyfile
sudo rc-service caddys status
sudo rc-service caddys reload
caddys version
caddys --help
```

Las defensas HTTP y la sanitización de `.html`/`.css`/`.js` de `file_server` se activan automáticamente según las capacidades documentadas en `README-SECURE.md`; no hace falta añadirlas al Caddyfile. **No son un WAF ni reemplazan la seguridad de aplicaciones**. Para sitios existentes, la nueva herramienta no cambia configuraciones ni permisos por su cuenta.

## Recuperación y copias

El asistente rechaza registrar de nuevo un dominio que ya aparezca en el Caddyfile principal o en los fragmentos administrados. Si quieres auditar permisos de una web creada manualmente con carpeta `/var/www/<dominio>`, puedes utilizar `site doctor <dominio>` sin crear otra configuración.

Cuando `site add` necesita añadir la primera directiva `import`, guarda una copia del Caddyfile original en un archivo `.bak.caddys-site-<fecha>`. Un error de validación no activa la configuración. Si falla la recarga, se intenta restaurar la configuración anterior; revisa `rc-service caddys status` y los logs. Para quitar un sitio, **no existe un comando destructivo automático**: elimina conscientemente su fragmento `.caddy`, conserva o respalda sus archivos y ejecuta `caddys validate` y `rc-service caddys reload`.

## Recomendaciones operativas

- Crea un dominio de prueba antes de mover sitios existentes.
- Mantén por separado las copias de los archivos web, la configuración y los certificados de Caddys.
- Configura DNS y proxy según el modo elegido, y prueba HTTPS hasta el origen.
- No uses `chmod -R 777` en las webs.
- Si utilizas Cloudflare con proxy, los límites automáticos por IP de origen TCP pueden agrupar muchos visitantes; no confíes en un encabezado IP arbitrario sin verificar.
