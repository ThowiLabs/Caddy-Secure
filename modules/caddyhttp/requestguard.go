package caddyhttp

import (
	"net"
	"net/netip"
	"net/url"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

const (
	guardBurst           = 120
	guardRequestsPerSec  = 50
	guardPerIPConcurrent = 24
	guardMaxConcurrent   = 96
	guardMaxClients      = 4096
	guardExpiry          = time.Minute
)

type requestBudget struct {
	tokens   float64
	updated  time.Time
	lastSeen time.Time
	inFlight int
}

type requestGuard struct {
	mu        sync.Mutex
	clients   map[netip.Addr]*requestBudget
	nextSweep time.Time
	inFlight  atomic.Int32
}

func newRequestGuard() *requestGuard {
	return &requestGuard{clients: make(map[netip.Addr]*requestBudget)}
}

// acquire uses only the socket peer address: untrusted forwarding headers cannot bypass limits.
func (g *requestGuard) acquire(remoteAddr string, now time.Time) (func(), bool) {
	if g.inFlight.Add(1) > guardMaxConcurrent {
		g.inFlight.Add(-1)
		return nil, false
	}
	host, _, err := net.SplitHostPort(remoteAddr)
	if err != nil {
		g.inFlight.Add(-1)
		return nil, false
	}
	ip, err := netip.ParseAddr(host)
	if err != nil {
		g.inFlight.Add(-1)
		return nil, false
	}
	ip = ip.Unmap()

	g.mu.Lock()
	if !now.Before(g.nextSweep) {
		for key, bucket := range g.clients {
			if bucket.inFlight == 0 && now.Sub(bucket.lastSeen) > guardExpiry {
				delete(g.clients, key)
			}
		}
		g.nextSweep = now.Add(guardExpiry)
	}
	bucket := g.clients[ip]
	if bucket == nil {
		if len(g.clients) >= guardMaxClients {
			g.mu.Unlock()
			g.inFlight.Add(-1)
			return nil, false
		}
		bucket = &requestBudget{tokens: guardBurst, updated: now}
		g.clients[ip] = bucket
	}
	elapsed := now.Sub(bucket.updated).Seconds()
	if elapsed > 0 {
		bucket.tokens += elapsed * guardRequestsPerSec
		if bucket.tokens > guardBurst {
			bucket.tokens = guardBurst
		}
	}
	bucket.updated = now
	bucket.lastSeen = now
	if bucket.tokens < 1 || bucket.inFlight >= guardPerIPConcurrent {
		g.mu.Unlock()
		g.inFlight.Add(-1)
		return nil, false
	}
	bucket.tokens--
	bucket.inFlight++
	g.mu.Unlock()

	return func() {
		g.mu.Lock()
		bucket.inFlight--
		g.mu.Unlock()
		g.inFlight.Add(-1)
	}, true
}

// protectedAssetPath blocks common leaked secrets and source maps, even through encoded segments.
func protectedAssetPath(path string) bool {
	for i := 0; i < 3; i++ {
		unescaped, err := url.PathUnescape(path)
		if err != nil || unescaped == path {
			break
		}
		path = unescaped
	}
	path = strings.ToLower(strings.ReplaceAll(path, "\\", "/"))
	for _, segment := range strings.Split(path, "/") {
		switch segment {
		case ".git", ".svn", ".hg", ".ssh", ".ds_store", "wp-config.php":
			return true
		}
		if segment == ".env" || strings.HasPrefix(segment, ".env.") || strings.HasSuffix(segment, ".map") {
			return true
		}
	}
	return false
}
