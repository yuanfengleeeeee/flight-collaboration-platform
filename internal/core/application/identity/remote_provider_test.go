package identity

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestRemoteProviderVerifierResolvesPersonalWeChatCode(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.URL.Path != "/jscode2session" {
			t.Fatalf("unexpected provider path %q", request.URL.Path)
		}
		if request.URL.Query().Get("js_code") != "one-time-code" || request.URL.Query().Get("appid") != "wx-app" {
			t.Fatalf("provider query did not contain the expected public app parameters")
		}
		writer.Header().Set("Content-Type", "application/json; charset=utf-8")
		_, _ = writer.Write([]byte(`{"openid":"wx-openid","unionid":"wx-unionid","session_key":"server-only"}`))
	}))
	defer server.Close()

	verifier, err := NewRemoteProviderVerifier(RemoteProviderConfig{
		PersonalWeChatAppID:           "wx-app",
		PersonalWeChatSecret:          "server-secret",
		PersonalWeChatCode2SessionURL: server.URL + "/jscode2session",
		WeComCorpID:                   "corp-id",
		WeComAgentID:                  "7",
		WeComSecret:                   "wecom-secret",
	})
	if err != nil {
		t.Fatal(err)
	}

	identity, err := verifier.Verify(context.Background(), ProviderPersonalWechat, "one-time-code", "")
	if err != nil {
		t.Fatal(err)
	}
	if identity.Provider != ProviderPersonalWechat || identity.ProviderApp != "wx-app" || identity.ExternalSubject != "wx-openid" {
		t.Fatalf("unexpected personal WeChat identity: %#v", identity)
	}
}

func TestRemoteProviderVerifierResolvesWeComMemberAndCachesAccessToken(t *testing.T) {
	tokenCalls := 0
	userCalls := 0
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("Content-Type", "application/json; charset=utf-8")
		switch request.URL.Path {
		case "/gettoken":
			tokenCalls++
			_, _ = writer.Write([]byte(`{"access_token":"server-only-token","expires_in":3600}`))
		case "/getuserinfo":
			userCalls++
			if request.URL.Query().Get("access_token") != "server-only-token" || request.URL.Query().Get("code") != "wecom-code" {
				t.Fatalf("unexpected WeCom userinfo query")
			}
			_, _ = writer.Write([]byte(`{"UserId":"wecom-user","DeviceId":"server-only"}`))
		default:
			t.Fatalf("unexpected provider path %q", request.URL.Path)
		}
	}))
	defer server.Close()

	verifier, err := NewRemoteProviderVerifier(RemoteProviderConfig{
		PersonalWeChatAppID:  "wx-app",
		PersonalWeChatSecret: "wx-secret",
		WeComCorpID:          "corp-id",
		WeComAgentID:         "7",
		WeComSecret:          "wecom-secret",
		WeComTokenURL:        server.URL + "/gettoken",
		WeComUserInfoURL:     server.URL + "/getuserinfo",
	})
	if err != nil {
		t.Fatal(err)
	}

	for range 2 {
		identity, verifyErr := verifier.Verify(context.Background(), ProviderWeCom, "wecom-code", "")
		if verifyErr != nil {
			t.Fatal(verifyErr)
		}
		if identity.ProviderApp != "corp-id:7" || identity.ExternalSubject != "wecom-user" {
			t.Fatalf("unexpected WeCom identity: %#v", identity)
		}
	}
	if tokenCalls != 1 || userCalls != 2 {
		t.Fatalf("unexpected provider call counts: token=%d user=%d", tokenCalls, userCalls)
	}
}

func TestRemoteProviderVerifierUsesWeComMiniappCode2Session(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("Content-Type", "application/json; charset=utf-8")
		switch request.URL.Path {
		case "/gettoken":
			_, _ = writer.Write([]byte(`{"access_token":"miniapp-token","expires_in":3600}`))
		case "/miniapp-jscode2session":
			if request.URL.Query().Get("access_token") != "miniapp-token" || request.URL.Query().Get("js_code") != "miniapp-code" || request.URL.Query().Get("grant_type") != "authorization_code" {
				t.Fatalf("unexpected WeCom miniapp code2Session query")
			}
			_, _ = writer.Write([]byte(`{"userid":"wecom-miniapp-user","corpid":"corp-id","session_key":"server-only"}`))
		default:
			t.Fatalf("unexpected provider path %q", request.URL.Path)
		}
	}))
	defer server.Close()

	verifier, err := NewRemoteProviderVerifier(RemoteProviderConfig{
		PersonalWeChatAppID:         "wx-app",
		PersonalWeChatSecret:        "wx-secret",
		WeComCorpID:                 "corp-id",
		WeComAgentID:                "7",
		WeComSecret:                 "wecom-secret",
		WeComTokenURL:               server.URL + "/gettoken",
		WeComMiniappCode2SessionURL: server.URL + "/miniapp-jscode2session",
	})
	if err != nil {
		t.Fatal(err)
	}

	identity, err := verifier.VerifyForClient(context.Background(), ProviderWeCom, "miniapp-code", ClientEmployeeWeComMiniapp, "")
	if err != nil {
		t.Fatal(err)
	}
	if identity.ProviderApp != "corp-id:7" || identity.ExternalSubject != "wecom-miniapp-user" {
		t.Fatalf("unexpected WeCom miniapp identity: %#v", identity)
	}
}
