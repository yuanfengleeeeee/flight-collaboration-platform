package adminauth

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"sync/atomic"
	"testing"
)

func TestWeComProviderBeginBuildsAuthorizationURL(t *testing.T) {
	provider := &WeComProvider{CorpID: "corp-1", AgentID: "agent-7"}
	value, err := provider.Begin(context.Background(), "state-1234567890123456", "https://admin.example.test/sso/callback", "")
	if err != nil {
		t.Fatal(err)
	}
	parsed, err := url.Parse(value)
	if err != nil {
		t.Fatal(err)
	}
	query := parsed.Query()
	if query.Get("appid") != "corp-1" || query.Get("agentid") != "agent-7" || query.Get("scope") != "snsapi_base" || query.Get("state") != "state-1234567890123456" {
		t.Fatalf("unexpected authorization query: %v", query)
	}
	if parsed.Fragment != "wechat_redirect" {
		t.Fatalf("unexpected authorization fragment: %q", parsed.Fragment)
	}
}

func TestWeComProviderExchangeCachesAccessTokenAndMapsMember(t *testing.T) {
	var tokenCalls atomic.Int32
	var userCalls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		switch request.URL.Path {
		case "/token":
			tokenCalls.Add(1)
			if request.URL.Query().Get("corpid") != "corp-1" || request.URL.Query().Get("corpsecret") != "server-secret" {
				http.Error(writer, "unexpected token query", http.StatusBadRequest)
				return
			}
			_ = json.NewEncoder(writer).Encode(map[string]any{"errcode": 0, "access_token": "server-token", "expires_in": 3600})
		case "/userinfo":
			userCalls.Add(1)
			if request.URL.Query().Get("access_token") != "server-token" || request.URL.Query().Get("code") != "sso-code" {
				http.Error(writer, "unexpected userinfo query", http.StatusBadRequest)
				return
			}
			_ = json.NewEncoder(writer).Encode(map[string]any{"errcode": 0, "UserId": "member-1"})
		default:
			http.NotFound(writer, request)
		}
	}))
	defer server.Close()

	provider := &WeComProvider{CorpID: "corp-1", AgentID: "agent-7", Secret: "server-secret", TokenURL: server.URL + "/token", UserInfoURL: server.URL + "/userinfo", HTTPClient: server.Client()}
	first, err := provider.Exchange(context.Background(), "sso-code", "")
	if err != nil {
		t.Fatal(err)
	}
	second, err := provider.Exchange(context.Background(), "sso-code", "")
	if err != nil {
		t.Fatal(err)
	}
	if first.Provider != ProviderWeCom || first.ExternalSub != "member-1" || second.ExternalSub != "member-1" {
		t.Fatalf("unexpected mapped identities: %#v %#v", first, second)
	}
	if tokenCalls.Load() != 1 || userCalls.Load() != 2 {
		t.Fatalf("unexpected provider call counts: token=%d user=%d", tokenCalls.Load(), userCalls.Load())
	}
}
