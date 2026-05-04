// Package iplookup resolves an IP address to its origin ASN and organization
// name using Team Cymru's free DNS-based whois service (origin.asn.cymru.com,
// AS<n>.asn.cymru.com). No API key, no signup; results are cached in-process.
package iplookup

import (
	"context"
	"fmt"
	"net"
	"strconv"
	"strings"
	"sync"
	"time"
)

type Result struct {
	ASN uint32
	Org string
}

type Resolver struct {
	mu       sync.RWMutex
	cache    map[string]cachedResult
	ttl      time.Duration
	timeout  time.Duration
	resolver *net.Resolver
}

type cachedResult struct {
	res    Result
	expiry time.Time
}

func New() *Resolver {
	return &Resolver{
		cache:    make(map[string]cachedResult),
		ttl:      6 * time.Hour,
		timeout:  3 * time.Second,
		resolver: net.DefaultResolver,
	}
}

func (r *Resolver) Lookup(ctx context.Context, ipStr string) (Result, error) {
	if host, _, err := net.SplitHostPort(ipStr); err == nil {
		ipStr = host
	}
	ipStr = strings.Trim(ipStr, "[]")
	ip := net.ParseIP(ipStr)
	if ip == nil {
		return Result{}, fmt.Errorf("invalid IP %q", ipStr)
	}
	if isPrivateOrSpecial(ip) {
		return Result{}, nil
	}

	key := ip.String()
	r.mu.RLock()
	c, ok := r.cache[key]
	r.mu.RUnlock()
	if ok && time.Now().Before(c.expiry) {
		return c.res, nil
	}

	lookupCtx, cancel := context.WithTimeout(ctx, r.timeout)
	defer cancel()

	asn, err := r.lookupASN(lookupCtx, ip)
	if err != nil {
		return Result{}, err
	}
	res := Result{ASN: asn}
	if asn != 0 {
		if org, err := r.lookupASNName(lookupCtx, asn); err == nil {
			res.Org = org
		}
	}

	r.mu.Lock()
	r.cache[key] = cachedResult{res: res, expiry: time.Now().Add(r.ttl)}
	r.mu.Unlock()
	return res, nil
}

func (r *Resolver) lookupASN(ctx context.Context, ip net.IP) (uint32, error) {
	var domain string
	if v4 := ip.To4(); v4 != nil {
		domain = fmt.Sprintf("%d.%d.%d.%d.origin.asn.cymru.com", v4[3], v4[2], v4[1], v4[0])
	} else {
		v6 := ip.To16()
		var sb strings.Builder
		for i := len(v6) - 1; i >= 0; i-- {
			b := v6[i]
			sb.WriteString(fmt.Sprintf("%x.%x.", b&0x0f, b>>4))
		}
		sb.WriteString("origin6.asn.cymru.com")
		domain = sb.String()
	}
	txts, err := r.resolver.LookupTXT(ctx, domain)
	if err != nil {
		return 0, err
	}
	if len(txts) == 0 {
		return 0, nil
	}
	fields := strings.Split(txts[0], "|")
	asnPart := strings.TrimSpace(fields[0])
	if sp := strings.IndexByte(asnPart, ' '); sp >= 0 {
		asnPart = asnPart[:sp]
	}
	n, err := strconv.ParseUint(asnPart, 10, 32)
	if err != nil {
		return 0, fmt.Errorf("parsing ASN %q: %w", asnPart, err)
	}
	return uint32(n), nil
}

func (r *Resolver) lookupASNName(ctx context.Context, asn uint32) (string, error) {
	domain := fmt.Sprintf("AS%d.asn.cymru.com", asn)
	txts, err := r.resolver.LookupTXT(ctx, domain)
	if err != nil {
		return "", err
	}
	if len(txts) == 0 {
		return "", nil
	}
	fields := strings.Split(txts[0], "|")
	if len(fields) < 5 {
		return "", nil
	}
	return strings.TrimSpace(fields[4]), nil
}

func isPrivateOrSpecial(ip net.IP) bool {
	return ip.IsLoopback() || ip.IsPrivate() || ip.IsLinkLocalUnicast() ||
		ip.IsLinkLocalMulticast() || ip.IsUnspecified()
}
