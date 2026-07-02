// Package auth provides password hashing (stdlib PBKDF2) and signed tokens
// (HMAC-SHA256) for account sessions — no external dependencies.
package auth

import (
	"crypto/hmac"
	"crypto/pbkdf2"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
)

const (
	pbkdf2Iterations = 210_000
	pbkdf2KeyLength  = 32
	saltLength       = 16
)

var b64 = base64.RawURLEncoding

// HashPassword returns a self-describing PBKDF2 hash: pbkdf2$<iter>$<salt>$<hash>.
func HashPassword(password string) (string, error) {
	salt := make([]byte, saltLength)
	if _, err := rand.Read(salt); err != nil {
		return "", err
	}
	key, err := pbkdf2.Key(sha256.New, password, salt, pbkdf2Iterations, pbkdf2KeyLength)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("pbkdf2$%d$%s$%s", pbkdf2Iterations, b64.EncodeToString(salt), b64.EncodeToString(key)), nil
}

// VerifyPassword checks a password against a stored PBKDF2 hash in constant time.
func VerifyPassword(password, stored string) bool {
	parts := strings.Split(stored, "$")
	if len(parts) != 4 || parts[0] != "pbkdf2" {
		return false
	}
	var iter int
	if _, err := fmt.Sscanf(parts[1], "%d", &iter); err != nil || iter <= 0 {
		return false
	}
	salt, err := b64.DecodeString(parts[2])
	if err != nil {
		return false
	}
	want, err := b64.DecodeString(parts[3])
	if err != nil {
		return false
	}
	got, err := pbkdf2.Key(sha256.New, password, salt, iter, len(want))
	if err != nil {
		return false
	}
	return subtle.ConstantTimeCompare(got, want) == 1
}

type tokenPayload struct {
	Sub      string `json:"sub"`
	Username string `json:"username,omitempty"`
	Role     string `json:"role,omitempty"`
	Exp      int64  `json:"exp"`
}

type Claims struct {
	Subject  string
	Username string
	Role     string
	Expires  time.Time
}

// SignToken returns base64(payload).base64(hmac) signed with secret.
func SignToken(secret []byte, subject string, ttl time.Duration) (string, time.Time, error) {
	return SignSessionToken(secret, subject, "", "", ttl)
}

// SignSessionToken returns base64(payload).base64(hmac) signed with secret.
func SignSessionToken(secret []byte, subject, username, role string, ttl time.Duration) (string, time.Time, error) {
	expires := time.Now().Add(ttl).UTC()
	payload, err := json.Marshal(tokenPayload{Sub: subject, Username: username, Role: role, Exp: expires.Unix()})
	if err != nil {
		return "", time.Time{}, err
	}
	encoded := b64.EncodeToString(payload)
	return encoded + "." + b64.EncodeToString(sign(secret, encoded)), expires, nil
}

// VerifyToken validates the signature and expiry, returning the subject.
func VerifyToken(secret []byte, token string) (string, error) {
	claims, err := VerifySessionToken(secret, token)
	if err != nil {
		return "", err
	}
	return claims.Subject, nil
}

// VerifySessionToken validates the signature and expiry, returning all claims.
func VerifySessionToken(secret []byte, token string) (Claims, error) {
	encoded, sig, found := strings.Cut(token, ".")
	if !found {
		return Claims{}, errors.New("malformed token")
	}
	gotSig, err := b64.DecodeString(sig)
	if err != nil {
		return Claims{}, errors.New("malformed signature")
	}
	if !hmac.Equal(gotSig, sign(secret, encoded)) {
		return Claims{}, errors.New("invalid signature")
	}
	raw, err := b64.DecodeString(encoded)
	if err != nil {
		return Claims{}, errors.New("malformed payload")
	}
	var payload tokenPayload
	if err := json.Unmarshal(raw, &payload); err != nil {
		return Claims{}, errors.New("malformed payload")
	}
	if time.Now().Unix() > payload.Exp {
		return Claims{}, errors.New("token expired")
	}
	return Claims{
		Subject:  payload.Sub,
		Username: payload.Username,
		Role:     payload.Role,
		Expires:  time.Unix(payload.Exp, 0).UTC(),
	}, nil
}

func sign(secret []byte, message string) []byte {
	mac := hmac.New(sha256.New, secret)
	mac.Write([]byte(message))
	return mac.Sum(nil)
}
