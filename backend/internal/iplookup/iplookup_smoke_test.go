package iplookup

import (
	"context"
	"testing"
	"time"
)

// TestSmokeLive exercises Team Cymru's DNS service against a few known IPs.
// Skipped in -short mode because it requires outbound DNS.
func TestSmokeLive(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping live DNS lookup in -short mode")
	}
	r := New()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	cases := []struct {
		ip       string
		wantASN  uint32
		wantOrg  string
		wantSkip bool
	}{
		{"8.8.8.8", 15169, "GOOGLE", false},
		{"1.1.1.1", 13335, "CLOUDFLARENET", false},
		{"127.0.0.1", 0, "", true},
	}
	for _, tc := range cases {
		got, err := r.Lookup(ctx, tc.ip)
		if err != nil {
			t.Errorf("%s: unexpected error: %v", tc.ip, err)
			continue
		}
		t.Logf("%s -> ASN=%d Org=%q", tc.ip, got.ASN, got.Org)
		if tc.wantSkip {
			if got.ASN != 0 {
				t.Errorf("%s: expected private/skipped, got ASN=%d", tc.ip, got.ASN)
			}
			continue
		}
		if got.ASN != tc.wantASN {
			t.Errorf("%s: ASN=%d, want %d", tc.ip, got.ASN, tc.wantASN)
		}
		if got.Org == "" {
			t.Errorf("%s: empty org", tc.ip)
		}
	}
}
