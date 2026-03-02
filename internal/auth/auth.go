package auth

import (
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

const maxTimestampSkew = 5 * time.Minute

// VerifyRequest checks the Ed25519 signature on an incoming request.
// Returns the hex-encoded agent public key or an error.
func VerifyRequest(r *http.Request, body []byte) (string, error) {
	agentID := r.Header.Get("X-Agent-Id")
	if agentID == "" {
		return "", fmt.Errorf("missing X-Agent-Id header")
	}

	timestamp := r.Header.Get("X-Timestamp")
	if timestamp == "" {
		return "", fmt.Errorf("missing X-Timestamp header")
	}

	authHeader := r.Header.Get("Authorization")
	if !strings.HasPrefix(authHeader, "Signature ") {
		return "", fmt.Errorf("missing or invalid Authorization header")
	}
	sigB64 := strings.TrimPrefix(authHeader, "Signature ")

	// Parse timestamp and check skew
	ts, err := time.Parse(time.RFC3339, timestamp)
	if err != nil {
		return "", fmt.Errorf("invalid timestamp: %w", err)
	}
	skew := time.Since(ts)
	if skew < 0 {
		skew = -skew
	}
	if skew > maxTimestampSkew {
		return "", fmt.Errorf("timestamp too far from server time")
	}

	// Decode public key
	pubBytes, err := hex.DecodeString(agentID)
	if err != nil || len(pubBytes) != ed25519.PublicKeySize {
		return "", fmt.Errorf("invalid agent ID (expected %d hex bytes)", ed25519.PublicKeySize)
	}
	pubKey := ed25519.PublicKey(pubBytes)

	// Decode signature
	sig, err := base64.StdEncoding.DecodeString(sigB64)
	if err != nil {
		return "", fmt.Errorf("invalid signature encoding: %w", err)
	}

	// Build signed payload: method + path + timestamp + sha256(body)
	bodyHash := sha256.Sum256(body)
	payload := r.Method + r.URL.Path + timestamp + fmt.Sprintf("%x", bodyHash)

	if !ed25519.Verify(pubKey, []byte(payload), sig) {
		return "", fmt.Errorf("signature verification failed")
	}

	return agentID, nil
}

// SignRequest signs an HTTP request with the given Ed25519 private key.
// Used by agents (and tests) to create authenticated requests.
func SignRequest(r *http.Request, privKey ed25519.PrivateKey, body []byte) {
	pubKey := privKey.Public().(ed25519.PublicKey)
	agentID := hex.EncodeToString(pubKey)
	timestamp := time.Now().UTC().Format(time.RFC3339)

	bodyHash := sha256.Sum256(body)
	payload := r.Method + r.URL.Path + timestamp + fmt.Sprintf("%x", bodyHash)
	sig := ed25519.Sign(privKey, []byte(payload))

	r.Header.Set("X-Agent-Id", agentID)
	r.Header.Set("X-Timestamp", timestamp)
	r.Header.Set("Authorization", "Signature "+base64.StdEncoding.EncodeToString(sig))
}

// ReadBody reads the request body up to a reasonable limit.
func ReadBody(r *http.Request) ([]byte, error) {
	return io.ReadAll(io.LimitReader(r.Body, 1*1024*1024)) // 1MB max
}

// AdminKeyMiddleware checks for a static admin API key.
func AdminKeyMiddleware(adminKey string, next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		auth := r.Header.Get("Authorization")
		if auth != "Bearer "+adminKey {
			http.Error(w, `{"error":"unauthorized"}`, http.StatusUnauthorized)
			return
		}
		next(w, r)
	}
}

// AgentAuthMiddleware verifies Ed25519 signatures and injects the agent ID into the request header.
func AgentAuthMiddleware(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		body, err := ReadBody(r)
		if err != nil {
			http.Error(w, `{"error":"failed to read body"}`, http.StatusBadRequest)
			return
		}
		agentID, err := VerifyRequest(r, body)
		if err != nil {
			http.Error(w, fmt.Sprintf(`{"error":"%s"}`, err.Error()), http.StatusUnauthorized)
			return
		}
		// Stash body and agent ID for handlers
		r.Header.Set("X-Verified-Agent", agentID)
		r.Header.Set("X-Body-Cache", base64.StdEncoding.EncodeToString(body))
		next(w, r)
	}
}

// GetVerifiedAgent extracts the verified agent ID set by AgentAuthMiddleware.
func GetVerifiedAgent(r *http.Request) string {
	return r.Header.Get("X-Verified-Agent")
}

// GetCachedBody retrieves the body cached by AgentAuthMiddleware.
func GetCachedBody(r *http.Request) ([]byte, error) {
	cached := r.Header.Get("X-Body-Cache")
	if cached == "" {
		return nil, fmt.Errorf("no cached body")
	}
	return base64.StdEncoding.DecodeString(cached)
}
