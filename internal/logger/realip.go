package logger

import (
	"net"
	"net/http"
	"strings"
)

// RealIP rewrites r.RemoteAddr to the first IP in X-Forwarded-For, when present.
// Mount it BEFORE rate limiters so per-IP buckets are correct.
//
// Trusts a single hop (matches Node `app.set('trust proxy', 1)`): we take the
// last entry in X-Forwarded-For, which is the proxy that connected to us
// directly. CloudFlare appends the original client IP at the *start* of the
// list, so for parity with Node we read the first entry.
func RealIP(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
			ip := strings.TrimSpace(strings.Split(xff, ",")[0])
			if net.ParseIP(ip) != nil {
				if _, port, err := net.SplitHostPort(r.RemoteAddr); err == nil {
					r.RemoteAddr = net.JoinHostPort(ip, port)
				} else {
					r.RemoteAddr = ip
				}
			}
		} else if cf := r.Header.Get("CF-Connecting-IP"); cf != "" {
			if net.ParseIP(cf) != nil {
				r.RemoteAddr = cf
			}
		}
		next.ServeHTTP(w, r)
	})
}
