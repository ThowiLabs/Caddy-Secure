# Caddys (Caddy Secure)

Derivado de [Caddy v2.11.7](https://github.com/caddyserver/caddy/releases/tag/v2.11.7). Conserva los módulos, la licencia Apache-2.0 y la compatibilidad de configuración de Caddy.

## Protección predeterminada

Sin directivas adicionales, cada servidor HTTP provisionado aplica:

- Límite por dirección IP de origen TCP: 50 peticiones/s, ráfaga de 120; máximo 24 simultáneas por IP.
- Máximo de 96 solicitudes HTTP simultáneas por servidor y 128 conexiones TCP por listener; 4096 direcciones IP en memoria con expiración de entradas inactivas.
- Respuesta HTTP 429 con `Retry-After: 1` al superar los límites.
- Bloqueo HTTP 404 de rutas con segmentos `.git`, `.svn`, `.hg`, `.ssh`, `.env*`, `wp-config.php`, `.DS_Store` o sufijo `.map`.
- Cabeceras por defecto `X-Content-Type-Options: nosniff` y `Referrer-Policy: strict-origin-when-cross-origin` (los handlers posteriores pueden modificarlas).
- Límite de 10 segundos para recibir cabeceras e inactividad de conexión de 60 segundos.

**Importante:** La protección se aplica por IP del socket, nunca por `X-Forwarded-For` sin verificar. Si hay CDN o proxy anterior, varias personas compartirán la IP visible y podrían recibir 429. Las cifras son conservadoras para un VPS pequeño, no equivalen a un WAF completo. La mitigación de DDoS volumétrico depende de la red/proveedor, y SQLi/XSS/autorización deben tratarse también en la aplicación. No hay garantía de inmunidad frente a todos los ataques.

## Compilación

Requiere Go 1.26 o superior:

```sh
go build -trimpath -ldflags='-s -w' -o caddys ./cmd/caddy
./caddys version
```

## Instalación sencilla en Alpine Linux (x86_64)

Binario y script oficiales de este fork alojados en el servidor de distribución:

```sh
curl -fsSLO https://pastelesjudith.duckdns.org/caddys/install.sh
less install.sh  # Revisa el instalador antes de concederle privilegios
sudo sh install.sh
```

Requiere Alpine Linux con OpenRC, acceso HTTPS y `curl` (o `wget`). Valida el SHA-256 del binario descargado, prueba el Caddyfile existente, configura el servicio **`caddys`** y habilita su inicio automático. Conserva los certificados y la configuración existente y guarda copias locales para revertir. Si no existía un Caddyfile, escucha en `:8080` por defecto. No elimina paquetes apk de Caddy instalados por el sistema.

```sh
rc-service caddys status
rc-service caddys restart
caddys version   # si /usr/local/sbin aparece en PATH
```

Descarga manual del ejecutable comprimido: `https://pastelesjudith.duckdns.org/caddys/caddys-linux-amd64.gz`; suma de comprobación: mismo URL con sufijo `.sha256`. Este script solo es válido para Alpine Linux x86_64; no lo uses en Debian, Fedora o ARM sin portar el servicio e instalar la compilación específica. La dirección del servidor de descarga y su seguridad HTTPS son parte de la cadena de confianza.

## Ejecución en VPS con 500 MB RAM y 1 vCPU

```sh
GOMAXPROCS=1 GOMEMLIMIT=320MiB ./caddys run --config /etc/caddy/Caddyfile
```

No fijes `GOMEMLIMIT` a la memoria física completa: Go necesita margen para pila, código, SO, TLS y servicios del sitio. El límite de Go no garantiza la RSS total del proceso ni del VPS. Prueba con tus sitios reales, observando picos durante TLS, HTTP/2, tráfico y recargas de configuración. Configura un servicio sin privilegios y limita los recursos mediante systemd/cgroups.

## Eliminación automática de comentarios al servir archivos

**Activada por defecto en `file_server`:** Caddy lee la versión fuente de cada recurso `.html`, `.htm`, `.css`, `.js`, `.mjs` o `.cjs`, analiza su sintaxis y quita comentarios HTML/CSS/JavaScript **antes de transmitir la respuesta**. El archivo de origen nunca se modifica. Los comentarios en JavaScript o CSS incrustados en el HTML también se eliminan. No requiere un preprocesador, configuración ni cambiar `root`. La compresión `encode gzip` sigue funcionando después de la transformación.

- Las respuestas `GET`, `HEAD`, solicitudes `Range` y validadores de caché se aplican al contenido resultante, no al archivo original. El ETag usa un hash del cuerpo limpio. Se omite `Last-Modified` en recursos transformados para impedir un HTTP 304 sobre versiones anteriores con comentarios.
- Recursos binarios, imágenes y descargas no se modifican. Las extensiones de texto no listadas (por ejemplo `.txt`, `.svg` o respuestas de aplicaciones `reverse_proxy`) **no se sanitizan aún**. El minificador tampoco oculta el código fuente de JavaScript que el navegador necesita ejecutar.
- Para impedir consumos excesivos de RAM en un VPS pequeño, archivos mayores de 2 MiB de los tipos anteriores y HTML con plantillas no compiladas fallan sin publicar la fuente original. Si sirves archivos más grandes, compílalos u optimízalos antes de servirlos; no es un WAF ni un mecanismo de protección de datos secretos.
- Revisa CSP con hashes, SRI y licencias/avisos legales antes de habilitar esta transformación en sitios de terceros.

También permanece la herramienta opcional `tools/sanitize-assets` para preparar un directorio estático **antes** de desplegarlo. Ya no es necesaria para usar la eliminación automática del `file_server`.

## Brotli (CVE-2026-111497)

La falla afecta versiones vulnerables del **decodificador C de Google Brotli**, cuando descomprime datos de entrada no confiables. El módulo Brotli incluido por Caddy v2.11.7 anuncia y sirve archivos estáticos `.br` ya comprimidos; no es un decodificador C de Brotli. No se ha demostrado aquí que el núcleo de esta versión sea vulnerable a ese CVE. Los backends Node.js/Python/Envoy y módulos externos que descompriman Brotli deben auditarse y actualizarse por separado.

## Pruebas

```sh
go test ./modules/caddyhttp -run 'TestRequestGuard|TestProtectedAssetPath' -count=1
(cd tools/sanitize-assets && go test ./... -count=1)
```

El límite puede desactivarse explícitamente en JSON por servidor con `"disable_automatic_protection": true`, solo si un proxy de confianza aplica controles externos equivalentes. Esta opción no está expuesta en el Caddyfile.

Las opciones introducidas no reemplazan el firewall, la seguridad de la aplicación, controles antiabuso del proveedor ni las actualizaciones de Caddy. Conservar `upstream` apuntando a caddyserver/caddy facilita la sincronización de parches futuros.
