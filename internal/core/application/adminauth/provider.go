package adminauth

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
)

var ErrProviderUnavailable = errors.New("admin SSO provider request failed")

const (
	ProviderWeCom        = "wecom"
	DefaultWeComAuthURL  = "https://open.weixin.qq.com/connect/oauth2/authorize"
	DefaultWeComTokenURL = "https://qyapi.weixin.qq.com/cgi-bin/gettoken"
	DefaultWeComUserURL  = "https://qyapi.weixin.qq.com/cgi-bin/user/getuserinfo"
)

// WeComProvider implements the enterprise-WeChat browser authorization-code
// flow directly. It intentionally resolves only enterprise member UserId;
// external contacts must never become management principals.
type WeComProvider struct {
	CorpID       string
	AgentID      string
	Secret       string
	AuthorizeURL string
	TokenURL     string
	UserInfoURL  string
	HTTPClient   *http.Client

	mu            sync.Mutex
	accessToken   string
	accessExpires time.Time
}

func (p *WeComProvider) Begin(_ context.Context, state, redirectURI, _ string) (string, error) {
	if p == nil || strings.TrimSpace(p.CorpID) == "" || strings.TrimSpace(p.AgentID) == "" || !validHTTPURL(redirectURI) {
		return "", ErrSSOUnavailable
	}
	authorizeURL := strings.TrimSpace(p.AuthorizeURL)
	if authorizeURL == "" {
		authorizeURL = DefaultWeComAuthURL
	}
	parsed, err := url.Parse(authorizeURL)
	if err != nil || !validHTTPURL(authorizeURL) {
		return "", ErrSSOUnavailable
	}
	query := parsed.Query()
	query.Set("appid", p.CorpID)
	query.Set("agentid", p.AgentID)
	query.Set("redirect_uri", redirectURI)
	query.Set("response_type", "code")
	query.Set("scope", "snsapi_base")
	query.Set("state", state)
	parsed.RawQuery = query.Encode()
	return parsed.String() + "#wechat_redirect", nil
}

func (p *WeComProvider) Exchange(ctx context.Context, code, _ string) (ExternalIdentity, error) {
	if p == nil || strings.TrimSpace(code) == "" || strings.TrimSpace(p.CorpID) == "" || strings.TrimSpace(p.Secret) == "" {
		return ExternalIdentity{}, ErrSSOUnavailable
	}
	accessToken, err := p.getAccessToken(ctx)
	if err != nil {
		return ExternalIdentity{}, err
	}
	userInfoURL := strings.TrimSpace(p.UserInfoURL)
	if userInfoURL == "" {
		userInfoURL = DefaultWeComUserURL
	}
	parsed, err := url.Parse(userInfoURL)
	if err != nil || !validHTTPURL(userInfoURL) {
		return ExternalIdentity{}, ErrSSOUnavailable
	}
	query := parsed.Query()
	query.Set("access_token", accessToken)
	query.Set("code", strings.TrimSpace(code))
	parsed.RawQuery = query.Encode()
	var response weComUserInfoResponse
	if err := p.getJSON(ctx, parsed.String(), &response); err != nil {
		return ExternalIdentity{}, err
	}
	if response.ErrCode != 0 {
		return ExternalIdentity{}, ErrCodeInvalid
	}
	if strings.TrimSpace(response.UserID) == "" {
		return ExternalIdentity{}, ErrCodeInvalid
	}
	return ExternalIdentity{Provider: ProviderWeCom, ExternalSub: strings.TrimSpace(response.UserID), DisplayName: strings.TrimSpace(response.UserID)}, nil
}

type weComTokenResponse struct {
	ErrCode     int    `json:"errcode"`
	AccessToken string `json:"access_token"`
	ExpiresIn   int    `json:"expires_in"`
}

type weComUserInfoResponse struct {
	ErrCode int    `json:"errcode"`
	UserID  string `json:"UserId"`
}

func (p *WeComProvider) getAccessToken(ctx context.Context) (string, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	now := time.Now().UTC()
	if p.accessToken != "" && p.accessExpires.After(now.Add(30*time.Second)) {
		return p.accessToken, nil
	}
	tokenURL := strings.TrimSpace(p.TokenURL)
	if tokenURL == "" {
		tokenURL = DefaultWeComTokenURL
	}
	parsed, err := url.Parse(tokenURL)
	if err != nil || !validHTTPURL(tokenURL) {
		return "", ErrSSOUnavailable
	}
	query := parsed.Query()
	query.Set("corpid", p.CorpID)
	query.Set("corpsecret", p.Secret)
	parsed.RawQuery = query.Encode()
	var response weComTokenResponse
	if err := p.getJSON(ctx, parsed.String(), &response); err != nil {
		return "", err
	}
	if response.ErrCode != 0 || strings.TrimSpace(response.AccessToken) == "" {
		return "", fmt.Errorf("%w: token response rejected", ErrProviderUnavailable)
	}
	expiresIn := time.Duration(response.ExpiresIn) * time.Second
	if expiresIn <= 0 {
		expiresIn = 5 * time.Minute
	}
	p.accessToken = response.AccessToken
	p.accessExpires = now.Add(expiresIn)
	return p.accessToken, nil
}

func (p *WeComProvider) getJSON(ctx context.Context, endpoint string, target any) error {
	client := p.HTTPClient
	if client == nil {
		client = &http.Client{Timeout: 5 * time.Second}
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return fmt.Errorf("%w: create request", ErrProviderUnavailable)
	}
	request.Header.Set("Accept", "application/json")
	response, err := client.Do(request)
	if err != nil {
		return fmt.Errorf("%w: provider request", ErrProviderUnavailable)
	}
	defer response.Body.Close()
	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		return fmt.Errorf("%w: provider returned HTTP %d", ErrProviderUnavailable, response.StatusCode)
	}
	if err := json.NewDecoder(io.LimitReader(response.Body, 1<<20)).Decode(target); err != nil {
		return fmt.Errorf("%w: invalid provider response", ErrProviderUnavailable)
	}
	return nil
}

// OIDCProvider uses the authorization-code flow and resolves the subject from
// the provider's userinfo endpoint. Core never forwards the provider token to
// a browser and the provider subject must still be provisioned in Core.
type OIDCProvider struct {
	AuthorizeURL string
	TokenURL     string
	UserInfoURL  string
	ClientID     string
	ClientSecret string
	HTTPClient   *http.Client
}

func (p *OIDCProvider) Begin(_ context.Context, state, redirectURI, _ string) (string, error) {
	if p == nil || strings.TrimSpace(p.AuthorizeURL) == "" || strings.TrimSpace(p.ClientID) == "" || !validHTTPURL(p.AuthorizeURL) {
		return "", ErrSSOUnavailable
	}
	parsed, err := url.Parse(p.AuthorizeURL)
	if err != nil {
		return "", fmt.Errorf("%w: parse authorization endpoint", ErrProviderUnavailable)
	}
	query := parsed.Query()
	query.Set("response_type", "code")
	query.Set("client_id", p.ClientID)
	query.Set("redirect_uri", redirectURI)
	query.Set("scope", "openid profile email")
	query.Set("state", state)
	parsed.RawQuery = query.Encode()
	return parsed.String(), nil
}

func (p *OIDCProvider) Exchange(ctx context.Context, code, redirectURI string) (ExternalIdentity, error) {
	if p == nil || p.HTTPClient == nil || !validHTTPURL(p.TokenURL) || !validHTTPURL(p.UserInfoURL) || strings.TrimSpace(code) == "" {
		return ExternalIdentity{}, ErrSSOUnavailable
	}
	form := url.Values{}
	form.Set("grant_type", "authorization_code")
	form.Set("code", code)
	form.Set("redirect_uri", redirectURI)
	form.Set("client_id", p.ClientID)
	form.Set("client_secret", p.ClientSecret)
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, p.TokenURL, strings.NewReader(form.Encode()))
	if err != nil {
		return ExternalIdentity{}, fmt.Errorf("%w: create token request", ErrProviderUnavailable)
	}
	request.Header.Set("Accept", "application/json")
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	response, err := p.HTTPClient.Do(request)
	if err != nil {
		return ExternalIdentity{}, fmt.Errorf("%w: token request", ErrProviderUnavailable)
	}
	defer response.Body.Close()
	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		return ExternalIdentity{}, fmt.Errorf("%w: token endpoint returned HTTP %d", ErrProviderUnavailable, response.StatusCode)
	}
	var token tokenResponse
	if err := json.NewDecoder(io.LimitReader(response.Body, 1<<20)).Decode(&token); err != nil || strings.TrimSpace(token.AccessToken) == "" {
		return ExternalIdentity{}, fmt.Errorf("%w: invalid token response", ErrProviderUnavailable)
	}
	userRequest, err := http.NewRequestWithContext(ctx, http.MethodGet, p.UserInfoURL, nil)
	if err != nil {
		return ExternalIdentity{}, fmt.Errorf("%w: create userinfo request", ErrProviderUnavailable)
	}
	userRequest.Header.Set("Accept", "application/json")
	userRequest.Header.Set("Authorization", "Bearer "+token.AccessToken)
	userResponse, err := p.HTTPClient.Do(userRequest)
	if err != nil {
		return ExternalIdentity{}, fmt.Errorf("%w: userinfo request", ErrProviderUnavailable)
	}
	defer userResponse.Body.Close()
	if userResponse.StatusCode < http.StatusOK || userResponse.StatusCode >= http.StatusMultipleChoices {
		return ExternalIdentity{}, fmt.Errorf("%w: userinfo endpoint returned HTTP %d", ErrProviderUnavailable, userResponse.StatusCode)
	}
	var profile userInfoResponse
	if err := json.NewDecoder(io.LimitReader(userResponse.Body, 1<<20)).Decode(&profile); err != nil || strings.TrimSpace(profile.Subject) == "" {
		return ExternalIdentity{}, fmt.Errorf("%w: invalid userinfo response", ErrProviderUnavailable)
	}
	displayName := firstNonEmpty(profile.Name, profile.PreferredUsername, profile.Email, profile.Subject)
	return ExternalIdentity{Provider: ProviderOIDC, ExternalSub: profile.Subject, DisplayName: displayName}, nil
}

type tokenResponse struct {
	AccessToken string `json:"access_token"`
}

type userInfoResponse struct {
	Subject           string `json:"sub"`
	Name              string `json:"name"`
	PreferredUsername string `json:"preferred_username"`
	Email             string `json:"email"`
}

// DevelopmentProvider is explicit and should only be wired in non-release
// environments. It still requires a pre-provisioned Core identity record.
type DevelopmentProvider struct {
	CallbackSubject string
}

func (p DevelopmentProvider) Begin(_ context.Context, state, redirectURI, _ string) (string, error) {
	if strings.TrimSpace(p.CallbackSubject) == "" || !validHTTPURL(redirectURI) {
		return "", ErrSSOUnavailable
	}
	parsed, err := url.Parse(redirectURI)
	if err != nil {
		return "", ErrSSOUnavailable
	}
	query := parsed.Query()
	query.Set("code", "mock-admin:"+strings.TrimSpace(p.CallbackSubject))
	query.Set("state", state)
	parsed.RawQuery = query.Encode()
	return parsed.String(), nil
}

func (p DevelopmentProvider) Exchange(_ context.Context, code, _ string) (ExternalIdentity, error) {
	const prefix = "mock-admin:"
	if !strings.HasPrefix(code, prefix) {
		return ExternalIdentity{}, ErrCodeInvalid
	}
	subject := strings.TrimSpace(strings.TrimPrefix(code, prefix))
	if subject == "" || strings.Contains(subject, ":") {
		return ExternalIdentity{}, ErrCodeInvalid
	}
	return ExternalIdentity{Provider: ProviderDevelopment, ExternalSub: subject, DisplayName: subject}, nil
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if value = strings.TrimSpace(value); value != "" {
			return value
		}
	}
	return ""
}
