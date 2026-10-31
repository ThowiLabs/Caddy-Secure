# Caddy Secure

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
go build -trimpath -ldflags='-s -w' -o caddy-secure ./cmd/caddy
./caddy-secure version
```

## Ejecución en VPS con 500 MB RAM y 1 vCPU

```sh
GOMAXPROCS=1 GOMEMLIMIT=320MiB ./caddy-secure run --config /etc/caddy/Caddyfile
```

No fijes `GOMEMLIMIT` a la memoria física completa: Go necesita margen para pila, código, SO, TLS y servicios del sitio. El límite de Go no garantiza la RSS total del proceso ni del VPS. Prueba con tus sitios reales, observando picos durante TLS, HTTP/2, tráfico y recargas de configuración. Configura un servicio sin privilegios y limita los recursos mediante systemd/cgroups.

## Retirar comentarios de archivos estáticos (operación de despliegue)

Se incluye `tools/sanitize-assets`, una utilidad separada para no añadir costo de CPU/RAM al proceso de Caddy. Produce un directorio **nuevo**, dejando los archivos originales intactos:

```sh
cd tools/sanitize-assets
go run . -input /ruta/sitio-original -output /ruta/sitio-publico
```

La utilidad usa un parser de HTML/CSS/JS, no expresiones regulares. También excluye `.env`, `.git` y archivos `.map`, y rechaza enlaces simbólicos y plantillas HTML no compiladas. En el Caddyfile configura `root` para apuntar al directorio generado. Verifica la web antes de publicarla; la transformación puede invalidar SRI, CSP basada en hashes, firmas o cachés existentes. Conserva los avisos legales/licencias requeridos en la distribución.

No oculta el código JavaScript que necesariamente recibe el navegador, ni altera el historial Git de un repositorio público. Las rutas `.git` y mapas de fuentes se bloquean por defecto incluso sin ejecutar la utilidad. El contenido HTML generado por aplicaciones backend no se transforma.

## Brotli (CVE-2026-111497)

La falla afecta versiones vulnerables del **decodificador C de Google Brotli**, cuando descomprime datos de entrada no confiables. El módulo Brotli incluido por Caddy v2.11.7 anuncia y sirve archivos estáticos `.br` ya comprimidos; no es un decodificador C de Brotli. No se ha demostrado aquí que el núcleo de esta versión sea vulnerable a ese CVE. Los backends Node.js/Python/Envoy y módulos externos que descompriman Brotli deben auditarse y actualizarse por separado.

## Pruebas

```sh
go test ./modules/caddyhttp -run 'TestRequestGuard|TestProtectedAssetPath' -count=1
(cd tools/sanitize-assets && go test ./... -count=1)
```

El límite puede desactivarse explícitamente en JSON por servidor con `"disable_automatic_protection": true`, solo si un proxy de confianza aplica controles externos equivalentes. Esta opción no está expuesta en el Caddyfile.

Las opciones introducidas no reemplazan el firewall, la seguridad de la aplicación, controles antiabuso del proveedor ni las actualizaciones de Caddy. Conservar `upstream` apuntando a caddyserver/caddy facilita la sincronización de parches futuros.
