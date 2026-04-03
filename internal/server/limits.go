package server

import (
	"crypto/ed25519"
	"encoding/base64"
	"encoding/json"
	"log"
	"os"
	"strings"
	"time"
)

const publicKeyHex = "3af8f9593b3331c27994f1eeacf111c727ff6015016b0af44ed3ca6934d40b13"

type Limits struct {
	MaxUpstreams        int
	MaxRequestsPerMonth int
	HistoryDays         int
	AnomalyDetection    bool
	SpendAlerts         bool
	SpendTrends         bool
	Tier                string
}

func FreeLimits() Limits {
	return Limits{
		MaxUpstreams:        1,
		MaxRequestsPerMonth: 5000,
		HistoryDays:         7,
		AnomalyDetection:    false,
		SpendAlerts:         false,
		SpendTrends:         false,
		Tier:                "free",
	}
}

func ProLimits() Limits {
	return Limits{
		MaxUpstreams:        0,
		MaxRequestsPerMonth: 0,
		HistoryDays:         90,
		AnomalyDetection:    true,
		SpendAlerts:         true,
		SpendTrends:         true,
		Tier:                "pro",
	}
}

func DefaultLimits() Limits {
	key := os.Getenv("STOCKYARD_LICENSE_KEY")
	if key == "" {
		log.Printf("[license] No license key — running on free tier (1 upstream, 5K req/mo)")
		log.Printf("[license] Set STOCKYARD_LICENSE_KEY to unlock Pro features")
		log.Printf("[license] Get a key at https://stockyard.dev/trough/")
		return FreeLimits()
	}
	if validateLicenseKey(key, "trough") {
		log.Printf("[license] Valid Pro license — all features unlocked")
		return ProLimits()
	}
	log.Printf("[license] Invalid license key — running on free tier")
	return FreeLimits()
}

func LimitReached(limit, current int) bool {
	if limit == 0 { return false }
	return current >= limit
}

func validateLicenseKey(key, product string) bool {
	if !strings.HasPrefix(key, "SY-") { return false }
	key = key[3:]
	parts := strings.SplitN(key, ".", 2)
	if len(parts) != 2 { return false }
	payloadBytes, err := base64.RawURLEncoding.DecodeString(parts[0])
	if err != nil { return false }
	sigBytes, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil || len(sigBytes) != ed25519.SignatureSize { return false }
	pubKeyBytes, err := hexDecode(publicKeyHex)
	if err != nil || len(pubKeyBytes) != ed25519.PublicKeySize { return false }
	if !ed25519.Verify(ed25519.PublicKey(pubKeyBytes), payloadBytes, sigBytes) { return false }
	var payload struct {
		Product   string `json:"p"`
		ExpiresAt int64  `json:"x"`
	}
	if err := json.Unmarshal(payloadBytes, &payload); err != nil { return false }
	if payload.ExpiresAt > 0 && time.Now().Unix() > payload.ExpiresAt { return false }
	if payload.Product != "*" && payload.Product != "stockyard" && payload.Product != product { return false }
	return true
}

func hexDecode(s string) ([]byte, error) {
	if len(s)%2 != 0 { return nil, os.ErrInvalid }
	b := make([]byte, len(s)/2)
	for i := 0; i < len(s); i += 2 {
		high := hexVal(s[i]); low := hexVal(s[i+1])
		if high == 255 || low == 255 { return nil, os.ErrInvalid }
		b[i/2] = high<<4 | low
	}
	return b, nil
}

func hexVal(c byte) byte {
	switch {
	case c >= '0' && c <= '9': return c - '0'
	case c >= 'a' && c <= 'f': return c - 'a' + 10
	case c >= 'A' && c <= 'F': return c - 'A' + 10
	}
	return 255
}
