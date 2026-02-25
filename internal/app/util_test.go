package app

import (
	"encoding/base64"
	"encoding/json"
	"testing"
)

func makeJWT(t *testing.T, claims map[string]any) string {
	t.Helper()
	header := base64.RawURLEncoding.EncodeToString([]byte(`{"alg":"none","typ":"JWT"}`))
	payloadBytes, err := json.Marshal(claims)
	if err != nil {
		t.Fatalf("marshal claims: %v", err)
	}
	payload := base64.RawURLEncoding.EncodeToString(payloadBytes)
	signature := base64.RawURLEncoding.EncodeToString([]byte("sig"))
	return header + "." + payload + "." + signature
}

func TestExtractAccountID(t *testing.T) {
	jwt := makeJWT(t, map[string]any{
		"chatgpt_account_id": "org_1",
	})
	claims := parseJWTClaims(jwt)
	if got := extractAccountID(claims); got != "org_1" {
		t.Fatalf("expected org_1, got %q", got)
	}
}

func TestExtractEmail(t *testing.T) {
	jwt := makeJWT(t, map[string]any{
		"email": "test@example.com",
	})
	claims := parseJWTClaims(jwt)
	if got := extractEmail(claims); got != "test@example.com" {
		t.Fatalf("expected test@example.com, got %q", got)
	}
}
