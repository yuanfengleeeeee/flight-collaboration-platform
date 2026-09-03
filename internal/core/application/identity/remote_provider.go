package identity

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
)

const (
	defaultWeChatCode2SessionURL = "https://api.weixin.qq.com/sns/jscode2session"
	defaultWeComTokenURL         = "https://qyapi.weixin.qq.com/cgi-bin/gettoken"
	defaultWeComUserInfoURL      = "https://qyapi.weixin.qq.com/cgi-bin/user/getuserinfo"
)

var ErrProviderUnavailable = errors.New("identity provider is unavailable")

// RemoteProviderConfig contains only server-side provider settings. Secrets
// must come from FLIGHT_* environment variables or an external secret store;
// this type is never serialized to a client response.
type RemoteProviderConfig struct {
	PersonalWeChatAppID           string
	PersonalWeChatSecret          string
	PersonalWeChatCode2SessionURL string
	WeComCorpID                   string
	WeComAgentID                  string
	WeComSecret                   string
	WeComTokenURL                 string
	WeComUserInfoURL              string
	HTTPClient                    *http.Client
}

// RemoteProviderVerifier converts one-time platform authorization codes into
// the canonical external identity used by the Core binding table. It does not
// persist platform session_key/access_token values and never returns them to
// Edge or a client.
type RemoteProviderVerifier struct {
	config RemoteProviderConfig
	client *http.Client

	tokenMu       sync.Mutex
	weComToken    string
	weComTokenExp time.Time
}

func NewRemoteProviderVerifier(config RemoteProviderConfig) (*RemoteProviderVerifier, error) {
	config.PersonalWeChatAppID = strings.TrimSpace(config.PersonalWeChatAppID)
	config.PersonalWeChatSecret = strings.TrimSpace(config.PersonalWeChatSecret)
	config.WeComCorpID = strings.TrimSpace(config.WeComCorpID)
	config.WeComAgentID = strings.TrimSpace(config.WeComAgentID)
	config.WeComSecret = strings.TrimSpace(config.WeComSecret)
	if config.PersonalWeChatCode2SessionURL == "" {
		config.PersonalWeChatCode2SessionURL = defaultWeChatCode2SessionURL
	}
	if config.WeComTokenURL == "" {
		config.WeComTokenURL = defaultWeComTokenURL
	}
	if config.WeComUserInfoURL == "" {
		config.WeComUserInfoURL = defaultWeComUserInfoURL
	}
	if config.PersonalWeChatAppID == "" || config.PersonalWeChatSecret == "" || config.WeComCorpID == "" || config.WeComAgentID == "" || config.WeComSecret == "" {
		return nil, fmt.Errorf("real identity provider requires personal WeChat and WeCom credentials")
	}
	client := config.HTTPClient
	if client == nil {
		client = &http.Client{Timeout: 5 * time.Second}
	}
	return &RemoteProviderVerifier{config: config, client: client}, nil
}

func (v *RemoteProviderVerifier) ProviderApp(provider string) string {
	if v == nil {
		return ""
	}
	switch strings.TrimSpace(provider) {
	case ProviderPersonalWechat:
		return v.config.PersonalWeChatAppID
	case ProviderWeCom:
		// The agent ID is part of the WeCom application scope. Keeping CorpID
		// in the key prevents accidental collisions between enterprises.
		return v.config.WeComCorpID + ":" + v.config.WeComAgentID
	default:
		return ""
	}
}

func (v *RemoteProviderVerifier) Verify(ctx context.Context, provider, code, _ string) (ExternalIdentity, error) {
	if v == nil || v.client == nil || strings.TrimSpace(code) == "" {
		return ExternalIdentity{}, ErrProviderCodeInvalid
	}
	switch strings.TrimSpace(provider) {
	case ProviderPersonalWechat:
		return v.verifyPersonalWeChat(ctx, code)
	case ProviderWeCom:
		return v.verifyWeCom(ctx, code)
	default:
		return ExternalIdentity{}, ErrUnsupportedProvider
	}
}

type weChatCode2SessionResponse struct {
	OpenID     string `json:"openid"`
	UnionID    string `json:"unionid"`
	SessionKey string `json:"session_key"`
	ErrCode    int    `json:"errcode"`
	ErrMsg     string `json:"errmsg"`
}

func (v *RemoteProviderVerifier) verifyPersonalWeChat(ctx context.Context, code string) (ExternalIdentity, error) {
	query := url.Values{}
	query.Set("appid", v.config.PersonalWeChatAppID)
	query.Set("secret", v.config.PersonalWeChatSecret)
	query.Set("js_code", strings.TrimSpace(code))
	query.Set("grant_type", "authorization_code")
	var response weChatCode2SessionResponse
	if err := v.getJSON(ctx, v.config.PersonalWeChatCode2SessionURL, query, &response); err != nil {
		return ExternalIdentity{}, err
	}
	if response.ErrCode != 0 || strings.TrimSpace(response.OpenID) == "" || strings.TrimSpace(response.SessionKey) == "" {
		return ExternalIdentity{}, ErrProviderCodeInvalid
	}
	return ExternalIdentity{Provider: ProviderPersonalWechat, ProviderApp: v.ProviderApp(ProviderPersonalWechat), ExternalSubject: strings.TrimSpace(response.OpenID)}, nil
}

type weComTokenResponse struct {
	AccessToken string `json:"access_token"`
	ExpiresIn   int    `json:"expires_in"`
	ErrCode     int    `json:"errcode"`
	ErrMsg      string `json:"errmsg"`
}

type weComUserInfoResponse struct {
	UserID  string `json:"UserId"`
	OpenID  string `json:"OpenId"`
	ErrCode int    `json:"errcode"`
	ErrMsg  string `json:"errmsg"`
}

func (v *RemoteProviderVerifier) verifyWeCom(ctx context.Context, code string) (ExternalIdentity, error) {
	accessToken, err := v.weComAccessToken(ctx)
	if err != nil {
		return ExternalIdentity{}, err
	}
	query := url.Values{}
	query.Set("access_token", accessToken)
	query.Set("code", strings.TrimSpace(code))
	var response weComUserInfoResponse
	if err := v.getJSON(ctx, v.config.WeComUserInfoURL, query, &response); err != nil {
		return ExternalIdentity{}, err
	}
	if response.ErrCode != 0 || strings.TrimSpace(response.UserID) == "" {
		// OpenId may identify an external contact, but this application maps
		// only enterprise members to internal Staff records.
		return ExternalIdentity{}, ErrProviderCodeInvalid
	}
	return ExternalIdentity{Provider: ProviderWeCom, ProviderApp: v.ProviderApp(ProviderWeCom), ExternalSubject: strings.TrimSpace(response.UserID)}, nil
}

func (v *RemoteProviderVerifier) weComAccessToken(ctx context.Context) (string, error) {
	now := time.Now().UTC()
	v.tokenMu.Lock()
	if v.weComToken != "" && v.weComTokenExp.After(now.Add(30*time.Second)) {
		token := v.weComToken
		v.tokenMu.Unlock()
		return token, nil
	}
	v.tokenMu.Unlock()

	query := url.Values{}
	query.Set("corpid", v.config.WeComCorpID)
	query.Set("corpsecret", v.config.WeComSecret)
	var response weComTokenResponse
	if err := v.getJSON(ctx, v.config.WeComTokenURL, query, &response); err != nil {
		return "", err
	}
	if response.ErrCode != 0 || strings.TrimSpace(response.AccessToken) == "" || response.ExpiresIn <= 0 {
		return "", ErrProviderCodeInvalid
	}

	v.tokenMu.Lock()
	v.weComToken = response.AccessToken
	v.weComTokenExp = now.Add(time.Duration(response.ExpiresIn) * time.Second)
	v.tokenMu.Unlock()
	return response.AccessToken, nil
}

func (v *RemoteProviderVerifier) getJSON(ctx context.Context, endpoint string, query url.Values, result any) error {
	requestURL, err := url.Parse(endpoint)
	if err != nil || requestURL.Scheme == "" || requestURL.Host == "" {
		return fmt.Errorf("%w: provider endpoint is invalid", ErrProviderUnavailable)
	}
	requestURL.RawQuery = query.Encode()
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, requestURL.String(), nil)
	if err != nil {
		return fmt.Errorf("%w: create provider request", ErrProviderUnavailable)
	}
	request.Header.Set("Accept", "application/json")
	response, err := v.client.Do(request)
	if err != nil {
		return fmt.Errorf("%w: provider request failed", ErrProviderUnavailable)
	}
	defer response.Body.Close()
	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		return fmt.Errorf("%w: provider returned HTTP %d", ErrProviderUnavailable, response.StatusCode)
	}
	if err := json.NewDecoder(response.Body).Decode(result); err != nil {
		return fmt.Errorf("%w: decode provider response", ErrProviderUnavailable)
	}
	return nil
}
