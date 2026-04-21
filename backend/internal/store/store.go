package store

import (
	"crypto/rand"
	"errors"
	"fmt"
	"math/big"
	"strings"
	"sync"
	"time"
)

var (
	ErrNotFound     = errors.New("no pending verification for this email")
	ErrExpired      = errors.New("verification code has expired")
	ErrInvalidCode  = errors.New("invalid verification code")
	ErrMaxAttempts  = errors.New("too many failed attempts")
)

type Entry struct {
	Code      string
	Name      string
	Email     string
	Message   string
	ExpiresAt time.Time
	Attempts  int
}

type Store struct {
	mu          sync.RWMutex
	entries     map[string]*Entry
	ttl         time.Duration
	maxAttempts int
}

func New(ttl time.Duration, maxAttempts int) *Store {
	s := &Store{
		entries:     make(map[string]*Entry),
		ttl:         ttl,
		maxAttempts: maxAttempts,
	}
	go s.cleanup()
	return s
}

func (s *Store) Put(email, name, message, code string) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.entries[strings.ToLower(email)] = &Entry{
		Code:      code,
		Name:      name,
		Email:     email,
		Message:   message,
		ExpiresAt: time.Now().Add(s.ttl),
		Attempts:  0,
	}
}

func (s *Store) Verify(email, code string) (*Entry, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	key := strings.ToLower(email)
	entry, ok := s.entries[key]
	if !ok {
		return nil, ErrNotFound
	}

	if time.Now().After(entry.ExpiresAt) {
		delete(s.entries, key)
		return nil, ErrExpired
	}

	if entry.Attempts >= s.maxAttempts {
		delete(s.entries, key)
		return nil, ErrMaxAttempts
	}

	if entry.Code != code {
		entry.Attempts++
		return nil, ErrInvalidCode
	}

	delete(s.entries, key)
	return entry, nil
}

func (s *Store) cleanup() {
	ticker := time.NewTicker(1 * time.Minute)
	defer ticker.Stop()

	for range ticker.C {
		s.mu.Lock()
		now := time.Now()
		for key, entry := range s.entries {
			if now.After(entry.ExpiresAt) {
				delete(s.entries, key)
			}
		}
		s.mu.Unlock()
	}
}

func GenerateCode() (string, error) {
	max := big.NewInt(1000000)
	n, err := rand.Int(rand.Reader, max)
	if err != nil {
		return "", fmt.Errorf("generating code: %w", err)
	}
	return fmt.Sprintf("%06d", n.Int64()), nil
}
