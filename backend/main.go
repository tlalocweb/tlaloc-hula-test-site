package main

import (
	"flag"
	"fmt"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/tlaloc-llc/tlalocwebsite/backend/internal/classifier"
	"github.com/tlaloc-llc/tlalocwebsite/backend/internal/config"
	"github.com/tlaloc-llc/tlalocwebsite/backend/internal/csvlog"
	"github.com/tlaloc-llc/tlalocwebsite/backend/internal/handler"
	"github.com/tlaloc-llc/tlalocwebsite/backend/internal/iplookup"
	"github.com/tlaloc-llc/tlalocwebsite/backend/internal/mailer"
	"github.com/tlaloc-llc/tlalocwebsite/backend/internal/ratelimit"
	"github.com/tlaloc-llc/tlalocwebsite/backend/internal/store"
)

func main() {
	configPath := flag.String("config", "config.yaml", "path to config file")
	verbose := flag.Bool("v", false, "verbose logging (full request/response details)")
	flag.Parse()

	cfg, err := config.Load(*configPath)
	if err != nil {
		log.Fatalf("failed to load config: %v", err)
	}

	// Log config summary (mask secrets)
	log.Printf("Config loaded from %s", *configPath)
	log.Printf("  turnstile secret: %s", maskSecret(cfg.Turnstile.SecretKey))
	log.Printf("  mailgun domain: %s, api_key: %s", cfg.Mailgun.Domain, maskSecret(cfg.Mailgun.APIKey))
	log.Printf("  openai model: %s, api_key: %s", cfg.OpenAI.Model, maskSecret(cfg.OpenAI.APIKey))
	log.Printf("  allowed_origins: %v", cfg.Server.AllowedOrigins)

	ttl := time.Duration(cfg.Verification.CodeTTLMinutes) * time.Minute
	s := store.New(ttl, cfg.Verification.MaxAttempts)

	m := mailer.New(cfg.Mailgun.Domain, cfg.Mailgun.APIKey, cfg.Mailgun.Sender, cfg.Contact.Recipient)
	m.Verbose = *verbose

	c := classifier.New(cfg.OpenAI.APIKey, cfg.OpenAI.Model)
	c.Verbose = *verbose

	cl, err := csvlog.New(cfg.CSVLog.FilePath)
	if err != nil {
		log.Fatalf("failed to init csv logger: %v", err)
	}
	defer cl.Close()

	rl := ratelimit.New(cfg.RateLimit.MaxPerIPBrowser, cfg.RateLimit.MaxPerIP)
	rl.StartCleanup(time.Duration(cfg.RateLimit.WindowMinutes) * time.Minute)

	ipl := iplookup.New()

	h := handler.New(s, m, cfg.Turnstile.SecretKey, c, cl, rl, ipl)

	mux := http.NewServeMux()
	mux.HandleFunc("/api/contact", h.HandleContact)
	mux.HandleFunc("/api/verify", h.HandleVerify)

	wrapped := corsMiddleware(mux, cfg.Server.AllowedOrigins)

	addr := fmt.Sprintf(":%d", cfg.Server.Port)
	log.Printf("Tlaloc backend listening on %s", addr)
	log.Fatal(http.ListenAndServe(addr, wrapped))
}

func corsMiddleware(next http.Handler, allowedOrigins []string) http.Handler {
	allowed := make(map[string]bool, len(allowedOrigins))
	for _, o := range allowedOrigins {
		allowed[strings.TrimRight(o, "/")] = true
	}

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		origin := r.Header.Get("Origin")
		if allowed[strings.TrimRight(origin, "/")] {
			w.Header().Set("Access-Control-Allow-Origin", origin)
			w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
			w.Header().Set("Access-Control-Allow-Methods", "POST, OPTIONS")
		}

		if r.Method == http.MethodOptions {
			w.Header().Set("Access-Control-Max-Age", "86400")
			w.WriteHeader(http.StatusNoContent)
			return
		}

		next.ServeHTTP(w, r)
	})
}

func maskSecret(s string) string {
	if s == "" {
		return "(empty)"
	}
	if len(s) > 8 && !strings.HasPrefix(s, "{{") {
		return s[:4] + "..." + s[len(s)-4:]
	}
	if strings.HasPrefix(s, "{{") {
		return s + " (NOT SUBSTITUTED)"
	}
	return "****"
}
