package middleware

import (
	"log"
	"net"
	"net/http"
	"sync"
	"time"
)

// loginLimiter tiene traccia dei tentativi falliti di login per IP.
// Max 5 fallimenti in 15 minuti, poi blocca con 429.
var loginLimiter = &rateLimiter{
	attempts: make(map[string][]time.Time),
	max:      5,
	window:   15 * time.Minute,
}

type rateLimiter struct {
	mu       sync.Mutex
	attempts map[string][]time.Time
	max      int
	window   time.Duration
}

func (rl *rateLimiter) allowed(ip string) bool {
	rl.mu.Lock()
	defer rl.mu.Unlock()
	now := time.Now()
	cutoff := now.Add(-rl.window)
	prev := rl.attempts[ip]
	var recent []time.Time
	for _, t := range prev {
		if t.After(cutoff) {
			recent = append(recent, t)
		}
	}
	rl.attempts[ip] = recent
	return len(recent) < rl.max
}

func (rl *rateLimiter) record(ip string) {
	rl.mu.Lock()
	defer rl.mu.Unlock()
	rl.attempts[ip] = append(rl.attempts[ip], time.Now())
}

// LoginRateLimit applica rate limiting a POST /login.
// Chiama RecordLoginFailure dopo ogni credenziale sbagliata.
func LoginRateLimit(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ip, _, _ := net.SplitHostPort(r.RemoteAddr)
		if !loginLimiter.allowed(ip) {
			http.Error(w, "Troppi tentativi. Riprova tra 15 minuti.", http.StatusTooManyRequests)
			return
		}
		next.ServeHTTP(w, r)
	})
}

// RecordLoginFailure registra un tentativo fallito per l'IP della request.
func RecordLoginFailure(r *http.Request) {
	ip, _, _ := net.SplitHostPort(r.RemoteAddr)
	loginLimiter.record(ip)
}

func SecurityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Frame-Options", "DENY")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("Referrer-Policy", "strict-origin-when-cross-origin")
		// 'unsafe-inline' richiesto da HTMX e template con stili inline
		w.Header().Set("Content-Security-Policy",
			"default-src 'self'; script-src 'self' 'unsafe-inline'; style-src 'self' 'unsafe-inline'; img-src 'self' data: https:")
		next.ServeHTTP(w, r)
	})
}

func Recovery(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if rc := recover(); rc != nil {
				log.Printf("panic: %v", rc)
				http.Error(w, "Errore interno del server", http.StatusInternalServerError)
			}
		}()
		next.ServeHTTP(w, r)
	})
}
