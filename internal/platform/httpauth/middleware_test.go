package httpauth

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/yuanfengleeeeee/flight-collaboration-platform/internal/platform/security"
)

type fakeAuthenticator struct {
	principal security.Principal
	err       error
}

func (f fakeAuthenticator) AuthenticateToken(string) (security.Principal, error) {
	return f.principal, f.err
}

func TestRequireJWTRejectsMissingToken(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.GET("/protected", RequireJWT(fakeAuthenticator{}, nil, false), func(c *gin.Context) { c.Status(http.StatusNoContent) })

	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/protected", nil))
	if recorder.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", recorder.Code)
	}
}

func TestRequireJWTInjectsPrincipal(t *testing.T) {
	gin.SetMode(gin.TestMode)
	want := security.Principal{Type: security.HumanPrincipal, PublicID: "staff-1"}
	router := gin.New()
	router.GET("/protected", RequireJWT(fakeAuthenticator{principal: want}, nil, false), func(c *gin.Context) {
		got, ok := Principal(c)
		if !ok || got.PublicID != want.PublicID {
			t.Fatalf("principal = %#v, ok=%v", got, ok)
		}
		c.Status(http.StatusNoContent)
	})

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/protected", nil)
	request.Header.Set("Authorization", "Bearer signed-token")
	router.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want 204", recorder.Code)
	}
}

func TestRequireJWTDevelopmentHeadersAreExplicit(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.GET("/protected", RequireJWT(nil, nil, true), func(c *gin.Context) {
		if !DevelopmentActorHeadersAllowed(c) {
			t.Fatal("development actor header adapter was not enabled")
		}
		c.Status(http.StatusNoContent)
	})

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/protected", nil)
	request.Header.Set("X-Employee-Public-ID", "staff-1")
	router.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want 204", recorder.Code)
	}
}
