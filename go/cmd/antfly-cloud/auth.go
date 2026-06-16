package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"os/exec"
	"runtime"
	"strings"
	"time"
)

const (
	refreshSkew = 2 * time.Minute
)

type oidcTokenResponse struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	IDToken      string `json:"id_token"`
	ExpiresIn    int64  `json:"expires_in"`
	TokenType    string `json:"token_type"`
	Error        string `json:"error"`
	Description  string `json:"error_description"`
}

type oidcTokenError struct {
	Code        string
	Description string
	StatusCode  int
}

func (e *oidcTokenError) Error() string {
	if e.Description != "" {
		return fmt.Sprintf("token endpoint returned HTTP %d: %s", e.StatusCode, e.Description)
	}
	if e.Code != "" {
		return fmt.Sprintf("token endpoint returned HTTP %d: %s", e.StatusCode, e.Code)
	}
	return fmt.Sprintf("token endpoint returned HTTP %d", e.StatusCode)
}

type deviceAuthorizationResponse struct {
	DeviceCode              string `json:"device_code"`
	UserCode                string `json:"user_code"`
	VerificationURI         string `json:"verification_uri"`
	VerificationURIComplete string `json:"verification_uri_complete"`
	ExpiresIn               int64  `json:"expires_in"`
	Interval                int64  `json:"interval"`
	Error                   string `json:"error"`
	Description             string `json:"error_description"`
}

func inferOIDCConfig(apiURL string) (issuer, clientID string) {
	clientID = defaultOIDCClientID
	u, err := url.Parse(apiURL)
	if err != nil {
		return "https://auth.antfly.io", clientID
	}
	host := u.Hostname()
	switch {
	case host == "localhost" || host == "127.0.0.1" || host == "::1":
		return "http://localhost:8084", clientID
	case strings.Contains(host, "-dev.") || strings.HasPrefix(host, "dev.") || strings.Contains(host, "platform-dev"):
		return "https://auth-dev.antfly.io", clientID
	default:
		return "https://auth.antfly.io", clientID
	}
}

func openBrowser(url string) error {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		cmd = exec.Command("open", url)
	case "windows":
		cmd = exec.Command("rundll32", "url.dll,FileProtocolHandler", url)
	default:
		cmd = exec.Command("xdg-open", url)
	}
	return cmd.Start()
}

func startDeviceAuthorization(ctx context.Context, httpClient *http.Client, issuer, clientID string) (*deviceAuthorizationResponse, error) {
	if httpClient == nil {
		httpClient = http.DefaultClient
	}
	body := url.Values{}
	body.Set("client_id", clientID)
	body.Set("scope", "openid profile email offline_access")
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, strings.TrimRight(issuer, "/")+"/device_authorization", strings.NewReader(body.Encode()))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	var ar deviceAuthorizationResponse
	if err := json.NewDecoder(resp.Body).Decode(&ar); err != nil {
		return nil, fmt.Errorf("decode device authorization response: %w", err)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		if ar.Description != "" {
			return nil, fmt.Errorf("device authorization endpoint returned HTTP %d: %s", resp.StatusCode, ar.Description)
		}
		if ar.Error != "" {
			return nil, fmt.Errorf("device authorization endpoint returned HTTP %d: %s", resp.StatusCode, ar.Error)
		}
		return nil, fmt.Errorf("device authorization endpoint returned HTTP %d", resp.StatusCode)
	}
	if ar.DeviceCode == "" || ar.UserCode == "" || ar.VerificationURI == "" {
		return nil, errors.New("device authorization response was missing required fields")
	}
	if ar.Interval <= 0 {
		ar.Interval = 5
	}
	return &ar, nil
}

func exchangeDeviceCode(ctx context.Context, httpClient *http.Client, issuer, clientID, deviceCode string) (*oidcTokenResponse, error) {
	body := url.Values{}
	body.Set("grant_type", "urn:ietf:params:oauth:grant-type:device_code")
	body.Set("device_code", deviceCode)
	body.Set("client_id", clientID)
	return postToken(ctx, httpClient, issuer, body)
}

func pollDeviceToken(ctx context.Context, httpClient *http.Client, issuer, clientID string, authz *deviceAuthorizationResponse) (*oidcTokenResponse, error) {
	if authz.ExpiresIn <= 0 {
		authz.ExpiresIn = 15 * 60
	}
	interval := time.Duration(authz.Interval) * time.Second
	deadline := time.Now().Add(time.Duration(authz.ExpiresIn) * time.Second)
	for {
		wait := time.NewTimer(interval)
		select {
		case <-ctx.Done():
			wait.Stop()
			return nil, ctx.Err()
		case <-wait.C:
		}

		if time.Now().After(deadline) {
			return nil, errors.New("device login expired; run `antfly-cloud login` again")
		}

		tokens, err := exchangeDeviceCode(ctx, httpClient, issuer, clientID, authz.DeviceCode)
		if err == nil {
			return tokens, nil
		}
		var tokenErr *oidcTokenError
		if !errors.As(err, &tokenErr) {
			return nil, err
		}
		switch tokenErr.Code {
		case "authorization_pending":
			continue
		case "slow_down":
			interval += 5 * time.Second
			continue
		case "access_denied":
			return nil, errors.New("device login was denied")
		case "expired_token":
			return nil, errors.New("device login expired; run `antfly-cloud login` again")
		default:
			return nil, err
		}
	}
}

func refreshOIDCToken(ctx context.Context, httpClient *http.Client, issuer, clientID, refreshToken string) (*oidcTokenResponse, error) {
	body := url.Values{}
	body.Set("grant_type", "refresh_token")
	body.Set("refresh_token", refreshToken)
	body.Set("client_id", clientID)
	return postToken(ctx, httpClient, issuer, body)
}

func postToken(ctx context.Context, httpClient *http.Client, issuer string, body url.Values) (*oidcTokenResponse, error) {
	if httpClient == nil {
		httpClient = http.DefaultClient
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, strings.TrimRight(issuer, "/")+"/token", strings.NewReader(body.Encode()))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	var tr oidcTokenResponse
	if err := json.NewDecoder(resp.Body).Decode(&tr); err != nil {
		return nil, fmt.Errorf("decode token response: %w", err)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, &oidcTokenError{
			Code:        tr.Error,
			Description: tr.Description,
			StatusCode:  resp.StatusCode,
		}
	}
	if tr.IDToken == "" {
		return nil, errors.New("token response did not include id_token")
	}
	return &tr, nil
}

func tokenExpiry(expiresIn int64) time.Time {
	if expiresIn <= 0 {
		return time.Now().Add(time.Hour)
	}
	return time.Now().Add(time.Duration(expiresIn) * time.Second)
}

func refreshConfigIfNeeded(ctx context.Context, cfg Config, httpClient *http.Client) (Config, bool, error) {
	if cfg.Auth.Type != authTypeOIDC || cfg.Auth.RefreshToken == "" || cfg.Auth.IDToken == "" {
		return cfg, false, nil
	}
	if !cfg.Auth.ExpiresAt.IsZero() && time.Now().Add(refreshSkew).Before(cfg.Auth.ExpiresAt) {
		return cfg, false, nil
	}
	issuer := cfg.Auth.Issuer
	clientID := cfg.Auth.ClientID
	if issuer == "" || clientID == "" {
		issuer, clientID = inferOIDCConfig(cfg.APIURL)
	}
	tr, err := refreshOIDCToken(ctx, httpClient, issuer, clientID, cfg.Auth.RefreshToken)
	if err != nil {
		return cfg, false, fmt.Errorf("session expired or refresh failed; run `antfly-cloud login`: %w", err)
	}
	cfg.Auth.Type = authTypeOIDC
	cfg.Auth.Issuer = issuer
	cfg.Auth.ClientID = clientID
	cfg.Auth.IDToken = tr.IDToken
	cfg.Auth.ExpiresAt = tokenExpiry(tr.ExpiresIn)
	if tr.RefreshToken != "" {
		cfg.Auth.RefreshToken = tr.RefreshToken
	}
	cfg.Token = ""
	return cfg, true, nil
}
