package turnstile

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"time"
)

const verifyURL = "https://challenges.cloudflare.com/turnstile/v0/siteverify"

type verifyResponse struct {
	Success bool     `json:"success"`
	Errors  []string `json:"error-codes"`
}

func Verify(token, secretKey, remoteIP string) (bool, error) {
	client := &http.Client{Timeout: 10 * time.Second}

	resp, err := client.PostForm(verifyURL, url.Values{
		"secret":   {secretKey},
		"response": {token},
		"remoteip": {remoteIP},
	})
	if err != nil {
		return false, fmt.Errorf("turnstile request failed: %w", err)
	}
	defer resp.Body.Close()

	var result verifyResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return false, fmt.Errorf("decoding turnstile response: %w", err)
	}

	return result.Success, nil
}
