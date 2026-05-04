package handler

import (
	"encoding/json"
	"log"
	"net/http"
	"net/mail"
	"strings"

	"github.com/tlaloc-llc/tlalocwebsite/backend/internal/classifier"
	"github.com/tlaloc-llc/tlalocwebsite/backend/internal/csvlog"
	"github.com/tlaloc-llc/tlalocwebsite/backend/internal/iplookup"
	"github.com/tlaloc-llc/tlalocwebsite/backend/internal/mailer"
	"github.com/tlaloc-llc/tlalocwebsite/backend/internal/ratelimit"
	"github.com/tlaloc-llc/tlalocwebsite/backend/internal/store"
	"github.com/tlaloc-llc/tlalocwebsite/backend/internal/turnstile"
)

type Handler struct {
	store           *store.Store
	mailer          *mailer.Mailer
	turnstileSecret string
	classifier      *classifier.Classifier
	csvlog          *csvlog.Logger
	ratelimit       *ratelimit.Limiter
	iplookup        *iplookup.Resolver
}

func New(s *store.Store, m *mailer.Mailer, turnstileSecret string, c *classifier.Classifier, cl *csvlog.Logger, rl *ratelimit.Limiter, ipl *iplookup.Resolver) *Handler {
	return &Handler{
		store:           s,
		mailer:          m,
		turnstileSecret: turnstileSecret,
		classifier:      c,
		csvlog:          cl,
		ratelimit:       rl,
		iplookup:        ipl,
	}
}

type contactRequest struct {
	Name           string `json:"name"`
	Email          string `json:"email"`
	Message        string `json:"message"`
	TurnstileToken string `json:"turnstileToken"`
	BrowserID      string `json:"browserId"`
}

type verifyRequest struct {
	Email string `json:"email"`
	Code  string `json:"code"`
}

type response struct {
	Success bool   `json:"success"`
	Error   string `json:"error,omitempty"`
	Message string `json:"message,omitempty"`
	Label   string `json:"label,omitempty"`
}

func (h *Handler) HandleContact(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, response{Error: "method not allowed"})
		return
	}

	var req contactRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, response{Error: "invalid request body"})
		return
	}

	req.Name = strings.TrimSpace(req.Name)
	req.Email = strings.TrimSpace(req.Email)
	req.Message = strings.TrimSpace(req.Message)

	if req.Name == "" || req.Email == "" || req.Message == "" {
		writeJSON(w, http.StatusBadRequest, response{Error: "all fields are required"})
		return
	}

	if _, err := mail.ParseAddress(req.Email); err != nil {
		writeJSON(w, http.StatusBadRequest, response{Error: "invalid email address"})
		return
	}

	if req.TurnstileToken == "" {
		writeJSON(w, http.StatusBadRequest, response{Error: "please complete the security check"})
		return
	}

	remoteIP := r.RemoteAddr
	if forwarded := r.Header.Get("X-Forwarded-For"); forwarded != "" {
		remoteIP = strings.TrimSpace(strings.Split(forwarded, ",")[0])
	}

	// Turnstile verification
	ok, err := turnstile.Verify(req.TurnstileToken, h.turnstileSecret, remoteIP)
	if err != nil {
		log.Printf("turnstile verification error: %v", err)
		writeJSON(w, http.StatusInternalServerError, response{Error: "security verification failed"})
		return
	}
	if !ok {
		writeJSON(w, http.StatusBadRequest, response{Error: "security check failed, please try again"})
		return
	}

	// Rate limiting
	if !h.ratelimit.Allow(remoteIP, req.BrowserID) {
		writeJSON(w, http.StatusTooManyRequests, response{Error: "Too many submissions. Please try again later."})
		return
	}

	// Classify submission
	submission := classifier.ContactSubmission{
		Name:    req.Name,
		Email:   req.Email,
		Comment: req.Message,
	}

	result, err := h.classifier.Classify(r.Context(), submission)
	if err != nil {
		log.Printf("classification error: %v", err)
		writeJSON(w, http.StatusInternalServerError, response{Error: "unable to process your message right now"})
		return
	}

	ipInfo, err := h.iplookup.Lookup(r.Context(), remoteIP)
	if err != nil {
		log.Printf("ip lookup error for %s: %v", remoteIP, err)
	}

	// Log to CSV (always)
	if err := h.csvlog.Log(csvlog.Entry{
		IP:                remoteIP,
		ASN:               ipInfo.ASN,
		ASNOrg:            ipInfo.Org,
		BrowserID:         req.BrowserID,
		Name:              req.Name,
		Email:             req.Email,
		Message:           req.Message,
		FinalLabel:        result.FinalLabel,
		RealInterestScore: result.RealInterestScore,
		SpamScore:         result.SpamScore,
		SalesPitchScore:   result.SalesPitchScore,
		Confidence:        result.Confidence,
		ReasonShort:       result.ReasonShort,
		UsedModel:         result.UsedModel,
	}); err != nil {
		log.Printf("csv log error: %v", err)
	}

	// Respond based on classification
	switch result.FinalLabel {
	case "spam":
		writeJSON(w, http.StatusOK, response{
			Success: false,
			Label:   "spam",
			Message: "We love you... but we don't love spam. Adjust your message.",
		})
		return
	case "sales_pitch":
		writeJSON(w, http.StatusOK, response{
			Success: false,
			Label:   "sales_pitch",
			Message: "We love you... but we are not looking for your services. Adjust your message. Good luck!",
		})
		return
	}

	// real_interest — proceed to email verification
	code, err := store.GenerateCode()
	if err != nil {
		log.Printf("code generation error: %v", err)
		writeJSON(w, http.StatusInternalServerError, response{Error: "internal error"})
		return
	}

	h.store.Put(req.Email, req.Name, req.Message, code)

	if err := h.mailer.SendVerificationCode(req.Email, req.Name, code); err != nil {
		log.Printf("email send error: %v", err)
		writeJSON(w, http.StatusInternalServerError, response{Error: "failed to send verification email"})
		return
	}

	writeJSON(w, http.StatusOK, response{
		Success: true,
		Label:   "real_interest",
		Message: "Ok! I think your message is legit! We will now validate your email!",
	})
}

func (h *Handler) HandleVerify(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, response{Error: "method not allowed"})
		return
	}

	var req verifyRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, response{Error: "invalid request body"})
		return
	}

	req.Email = strings.TrimSpace(req.Email)
	req.Code = strings.TrimSpace(req.Code)

	if req.Email == "" || req.Code == "" {
		writeJSON(w, http.StatusBadRequest, response{Error: "email and code are required"})
		return
	}

	entry, err := h.store.Verify(req.Email, req.Code)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, response{Error: err.Error()})
		return
	}

	if err := h.mailer.SendContactNotification(entry.Name, entry.Email, entry.Message); err != nil {
		log.Printf("notification email error: %v", err)
		writeJSON(w, http.StatusInternalServerError, response{Error: "failed to send your message"})
		return
	}

	writeJSON(w, http.StatusOK, response{Success: true, Message: "Thank you! We'll be in touch soon."})
}

func writeJSON(w http.ResponseWriter, status int, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(data)
}
