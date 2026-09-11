package auth

import (
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/yuanfengleeeeee/flight-collaboration-platform/internal/config"
)

func testJWTConfig() config.JWTConfig {
	return config.JWTConfig{Secret: "test-secret-value", ExpireHours: 1, Issuer: "flight-test"}
}

func TestGenerateAndParseToken(t *testing.T) {
	cfg := testJWTConfig()
	token, err := GenerateToken(cfg, 7, "leader-one", RoleLeader)
	if err != nil {
		t.Fatal(err)
	}
	claims, err := ParseToken(cfg, token)
	if err != nil {
		t.Fatal(err)
	}
	if claims.UserID != 7 || claims.Username != "leader-one" || claims.Role != RoleLeader {
		t.Fatalf("unexpected claims: %+v", claims)
	}
}

func TestParseTokenRejectsIssuerAndAlgorithmMismatch(t *testing.T) {
	cfg := testJWTConfig()
	token, err := GenerateToken(cfg, 7, "leader-one", RoleLeader)
	if err != nil {
		t.Fatal(err)
	}
	wrongIssuer := cfg
	wrongIssuer.Issuer = "other"
	if _, err := ParseToken(wrongIssuer, token); err == nil {
		t.Fatal("expected issuer mismatch error")
	}

	badToken := jwt.NewWithClaims(jwt.SigningMethodHS512, Claims{
		UserID: 7,
		RegisteredClaims: jwt.RegisteredClaims{
			Issuer:    cfg.Issuer,
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Hour)),
		},
	})
	badSigned, err := badToken.SignedString([]byte(cfg.Secret))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := ParseToken(cfg, badSigned); err == nil {
		t.Fatal("expected algorithm mismatch error")
	}
}

func TestParseTokenRejectsExpiredToken(t *testing.T) {
	cfg := testJWTConfig()
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, Claims{
		UserID: 7,
		RegisteredClaims: jwt.RegisteredClaims{
			Issuer:    cfg.Issuer,
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(-time.Minute)),
		},
	})
	signed, err := token.SignedString([]byte(cfg.Secret))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := ParseToken(cfg, signed); err == nil {
		t.Fatal("expected expired token error")
	}
}
