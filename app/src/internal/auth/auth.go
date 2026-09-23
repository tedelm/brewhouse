package auth

import (
	"fmt"
	"strconv"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

// Claims are JWT claims issued by this application.
type Claims struct {
	UserID     int64  `json:"uid"`
	Role       string `json:"role"`
	CanElevate bool   `json:"can_elevate,omitempty"`
	jwt.RegisteredClaims
}

// TokenIssuer creates and validates HS256 JWTs for authenticated users.
type TokenIssuer struct {
	secret []byte
	ttl    time.Duration
}

// NewTokenIssuer creates a TokenIssuer with the given HMAC secret and token TTL.
func NewTokenIssuer(secret string, ttl time.Duration) *TokenIssuer {
	return &TokenIssuer{
		secret: []byte(secret),
		ttl:    ttl,
	}
}

// Issue returns a signed JWT for the given user id, username, effective role, and elevate flag.
func (t *TokenIssuer) Issue(userID int64, username, role string, canElevate bool) (string, error) {
	now := time.Now()
	claims := Claims{
		UserID:     userID,
		Role:       role,
		CanElevate: canElevate,
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   username,
			IssuedAt:  jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(now.Add(t.ttl)),
			ID:        strconv.FormatInt(userID, 10),
		},
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	signed, err := token.SignedString(t.secret)
	if err != nil {
		return "", fmt.Errorf("sign token: %w", err)
	}
	return signed, nil
}

// Parse validates a signed JWT and returns its claims.
func (t *TokenIssuer) Parse(tokenString string) (*Claims, error) {
	token, err := jwt.ParseWithClaims(tokenString, &Claims{}, func(token *jwt.Token) (any, error) {
		if token.Method != jwt.SigningMethodHS256 {
			return nil, fmt.Errorf("unexpected signing method: %v", token.Header["alg"])
		}
		return t.secret, nil
	})
	if err != nil {
		return nil, fmt.Errorf("parse token: %w", err)
	}

	claims, ok := token.Claims.(*Claims)
	if !ok || !token.Valid {
		return nil, fmt.Errorf("invalid token claims")
	}
	return claims, nil
}
