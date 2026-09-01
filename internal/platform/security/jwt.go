package security

import (
	"fmt"
	"strings"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/yuanfengleeeeee/flight-collaboration-platform/internal/platform/config"
	"github.com/yuanfengleeeeee/flight-collaboration-platform/internal/shared/id"
)

type JWTAuthenticator struct {
	secret   []byte
	issuer   string
	audience string
}

type accessClaims struct {
	SessionID string   `json:"sid"`
	Roles     []string `json:"roles,omitempty"`
	Global    bool     `json:"global,omitempty"`
	AreaIDs   []uint64 `json:"area_ids,omitempty"`
	TeamIDs   []uint64 `json:"team_ids,omitempty"`
	UserID    uint64   `json:"user_id,omitempty"`
	jwt.RegisteredClaims
}

func NewJWTAuthenticator(cfg config.JWTConfig) (*JWTAuthenticator, error) {
	if len(cfg.Secret) < 16 || cfg.Issuer == "" || cfg.Audience == "" {
		return nil, fmt.Errorf("jwt secret, issuer and audience are required")
	}
	return &JWTAuthenticator{secret: []byte(cfg.Secret), issuer: cfg.Issuer, audience: cfg.Audience}, nil
}

func (a *JWTAuthenticator) Issue(principal Principal, ttl time.Duration) (string, error) {
	if a == nil || len(a.secret) == 0 || principal.PublicID == "" {
		return "", fmt.Errorf("jwt authenticator or principal is not configured")
	}
	if ttl <= 0 {
		ttl = 15 * time.Minute
	}
	now := time.Now().UTC()
	sessionID, err := id.NewPublicID()
	if err != nil {
		return "", err
	}
	claims := accessClaims{SessionID: sessionID, RegisteredClaims: jwt.RegisteredClaims{Subject: principal.PublicID, Issuer: a.issuer, Audience: jwt.ClaimStrings{a.audience}, IssuedAt: jwt.NewNumericDate(now), ExpiresAt: jwt.NewNumericDate(now.Add(ttl))}}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString(a.secret)
}

func (a *JWTAuthenticator) AuthenticateToken(tokenValue string) (Principal, error) {
	if a == nil || len(a.secret) == 0 {
		return Principal{}, fmt.Errorf("jwt authenticator is not configured")
	}
	tokenValue = strings.TrimSpace(strings.TrimPrefix(tokenValue, "Bearer "))
	if tokenValue == "" {
		return Principal{}, fmt.Errorf("token is empty")
	}
	var claims accessClaims
	token, err := jwt.ParseWithClaims(tokenValue, &claims, func(token *jwt.Token) (any, error) {
		if token.Method != jwt.SigningMethodHS256 {
			return nil, fmt.Errorf("unexpected jwt signing method")
		}
		return a.secret, nil
	}, jwt.WithIssuer(a.issuer), jwt.WithAudience(a.audience), jwt.WithValidMethods([]string{jwt.SigningMethodHS256.Alg()}))
	if err != nil || token == nil || !token.Valid || claims.Subject == "" || claims.SessionID == "" {
		return Principal{}, fmt.Errorf("invalid access token")
	}
	return Principal{Type: HumanPrincipal, PublicID: claims.Subject, Subject: claims.Subject, Roles: append([]string(nil), claims.Roles...), Scopes: AccessScope{Global: claims.Global, AreaIDs: append([]uint64(nil), claims.AreaIDs...), TeamIDs: append([]uint64(nil), claims.TeamIDs...), UserID: claims.UserID}}, nil
}
