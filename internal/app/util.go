package app

import (
	"encoding/base64"
	"encoding/json"
	"strconv"
	"strings"
)

func toInt64(value any) int64 {
	switch v := value.(type) {
	case int64:
		return v
	case int:
		return int64(v)
	case float64:
		return int64(v)
	case json.Number:
		i, _ := v.Int64()
		return i
	case string:
		if v == "" {
			return 0
		}
		i, err := strconv.ParseInt(v, 10, 64)
		if err != nil {
			return 0
		}
		return i
	default:
		return 0
	}
}

func parseJWTClaims(token string) map[string]any {
	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		return nil
	}
	payload, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return nil
	}
	claims := map[string]any{}
	if err := json.Unmarshal(payload, &claims); err != nil {
		return nil
	}
	return claims
}

func extractAccountID(claims map[string]any) string {
	if claims == nil {
		return ""
	}
	if id, _ := claims["chatgpt_account_id"].(string); id != "" {
		return id
	}
	if auth, ok := claims["https://api.openai.com/auth"].(map[string]any); ok {
		if id, _ := auth["chatgpt_account_id"].(string); id != "" {
			return id
		}
	}
	if auth, ok := claims["https://api.openai.com/auth.chatgpt_account_id"].(string); ok && auth != "" {
		return auth
	}
	if orgs, ok := claims["organizations"].([]any); ok && len(orgs) > 0 {
		if first, ok := orgs[0].(map[string]any); ok {
			if id, _ := first["id"].(string); id != "" {
				return id
			}
		}
	}
	return ""
}

func extractEmail(claims map[string]any) string {
	if claims == nil {
		return ""
	}
	if email, _ := claims["email"].(string); email != "" {
		return email
	}
	if profile, ok := claims["https://api.openai.com/profile"].(map[string]any); ok {
		if email, _ := profile["email"].(string); email != "" {
			return email
		}
	}
	return ""
}

func normalizeCredentialIdentity(cred Credential) Credential {
	if claims := parseJWTClaims(cred.Access); claims != nil {
		if accountID := extractAccountID(claims); accountID != "" {
			cred.AccountID = accountID
		}
		if email := extractEmail(claims); email != "" {
			cred.Email = email
		}
	}
	if claims := parseJWTClaims(cred.IDToken); claims != nil {
		if cred.AccountID == "" {
			if accountID := extractAccountID(claims); accountID != "" {
				cred.AccountID = accountID
			}
		}
		if cred.Email == "" {
			if email := extractEmail(claims); email != "" {
				cred.Email = email
			}
		}
	}
	return cred
}
