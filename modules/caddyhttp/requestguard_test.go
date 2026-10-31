package caddyhttp

import (
	"fmt"
	"testing"
	"time"
)

func TestRequestGuardLimitsAndRefills(t *testing.T) {
	g := newRequestGuard()
	now := time.Now()
	for i := 0; i < guardBurst; i++ {
		done, ok := g.acquire("192.0.2.15:4040", now)
		if !ok {
			t.Fatalf("request %d rejected before burst exhausted", i)
		}
		done()
	}
	if _, ok := g.acquire("192.0.2.15:4040", now); ok {
		t.Fatal("burst did not enforce rate limit")
	}
	done, ok := g.acquire("192.0.2.15:4040", now.Add(time.Second))
	if !ok {
		t.Fatal("tokens did not refill")
	}
	done()
}

func TestRequestGuardConcurrentBound(t *testing.T) {
	g := newRequestGuard()
	now := time.Now()
	var releases []func()
	for i := 0; i < guardPerIPConcurrent; i++ {
		done, ok := g.acquire("2001:db8::1:1234", now)
		if ok {
			t.Fatal("malformed socket address admitted")
		}
		_ = done
		done, ok = g.acquire("[2001:db8::1]:1234", now)
		if !ok {
			t.Fatalf("concurrent request %d rejected", i)
		}
		releases = append(releases, done)
	}
	if _, ok := g.acquire("[2001:db8::1]:1234", now); ok {
		t.Fatal("concurrent request limit bypassed")
	}
	for _, release := range releases {
		release()
	}
	if g.inFlight.Load() != 0 {
		t.Fatal("in-flight counter leaked")
	}
}

func TestRequestGuardBoundedClients(t *testing.T) {
	g := newRequestGuard()
	now := time.Now()
	for i := 0; i < guardMaxClients; i++ {
		done, ok := g.acquire(fmt.Sprintf("198.%d.%d.%d:1234", (i>>16)&255, (i>>8)&255, i&255), now)
		if !ok {
			t.Fatalf("client %d rejected", i)
		}
		done()
	}
	if _, ok := g.acquire("203.0.113.9:3456", now); ok {
		t.Fatal("client map grew past bound")
	}
	if len(g.clients) != guardMaxClients {
		t.Fatalf("tracked clients = %d", len(g.clients))
	}
}

func TestProtectedAssetPath(t *testing.T) {
	for _, path := range []string{"/.git/config", "/%2egit/config", "/%252egit/config", "/.ENV.production", "/src/app.js.map", "/foo\\.ssh\\id_rsa", "/wp-config.php"} {
		if !protectedAssetPath(path) {
			t.Errorf("%q not protected", path)
		}
	}
	for _, path := range []string{"/", "/index.html", "/.well-known/acme-challenge/token", "/img/map.png", "/file.css"} {
		if protectedAssetPath(path) {
			t.Errorf("%q incorrectly blocked", path)
		}
	}
}
