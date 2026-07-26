package cloudauth

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

type Client struct {
	httpClient *http.Client
}

type Authorization struct {
	DeviceCode      string `json:"deviceCode"`
	ExpiresIn       int    `json:"expiresIn"`
	Interval        int    `json:"interval"`
	UserCode        string `json:"userCode"`
	VerificationURI string `json:"verificationUri"`
}

type Token struct {
	AccessToken      string `json:"accessToken"`
	ExpiresIn        int    `json:"expiresIn"`
	RefreshExpiresIn int    `json:"refreshExpiresIn"`
	RefreshToken     string `json:"refreshToken"`
	TokenType        string `json:"tokenType"`
}

type Session struct {
	Email  string `json:"email"`
	UserID string `json:"userId"`
}

type problem struct {
	Code   string `json:"code"`
	Detail string `json:"detail"`
}

func NewClient(httpClient *http.Client) *Client {
	if httpClient == nil {
		httpClient = &http.Client{Timeout: 15 * time.Second}
	}
	return &Client{httpClient: httpClient}
}

func (c *Client) Start(ctx context.Context, serviceURL, clientName string) (Authorization, error) {
	var result Authorization
	err := c.doJSON(ctx, http.MethodPost, serviceURL, "/api/console/auth/device-authorizations", "", map[string]string{
		"clientName": clientName,
		"clientType": "cli",
	}, &result)
	return result, err
}

func (c *Client) Poll(ctx context.Context, serviceURL string, authorization Authorization) (Token, error) {
	interval := max(authorization.Interval, 5)
	deadline := time.Now().Add(time.Duration(authorization.ExpiresIn) * time.Second)
	for {
		if time.Now().After(deadline) {
			return Token{}, errors.New("device authorization expired before it was approved")
		}
		timer := time.NewTimer(time.Duration(interval) * time.Second)
		select {
		case <-ctx.Done():
			timer.Stop()
			return Token{}, ctx.Err()
		case <-timer.C:
		}
		var token Token
		err := c.doJSON(ctx, http.MethodPost, serviceURL, "/api/console/auth/device-token", "", map[string]string{
			"deviceCode": authorization.DeviceCode,
		}, &token)
		if err == nil {
			return token, nil
		}
		var remote *RemoteError
		if !errors.As(err, &remote) {
			return Token{}, err
		}
		switch remote.Code {
		case "authorization_pending":
			continue
		case "rate_limited":
			interval += 5
			continue
		default:
			return Token{}, err
		}
	}
}

func (c *Client) Session(ctx context.Context, serviceURL, token string) (Session, error) {
	var result Session
	err := c.doJSON(ctx, http.MethodGet, serviceURL, "/api/console/auth/session", token, nil, &result)
	return result, err
}

func (c *Client) Refresh(ctx context.Context, serviceURL, refreshToken, nextRefreshToken, rotationID string) (Token, error) {
	var result Token
	err := c.doJSON(ctx, http.MethodPost, serviceURL, "/api/console/auth/device-refresh", "", map[string]string{
		"nextRefreshToken": nextRefreshToken,
		"refreshToken":     refreshToken,
		"rotationId":       rotationID,
	}, &result)
	return result, err
}

func (c *Client) Logout(ctx context.Context, serviceURL, token string) error {
	return c.doJSON(ctx, http.MethodDelete, serviceURL, "/api/console/auth/device-session", token, nil, nil)
}

type RemoteError struct {
	Code   string
	Detail string
	Status int
}

func (e *RemoteError) Error() string {
	if e.Detail != "" {
		return e.Detail
	}
	return fmt.Sprintf("file.cheap authentication failed with HTTP %d", e.Status)
}

func (c *Client) doJSON(ctx context.Context, method, serviceURL, path, token string, body any, destination any) error {
	origin, err := validatedOrigin(serviceURL)
	if err != nil {
		return err
	}
	var reader io.Reader
	if body != nil {
		encoded, encodeErr := json.Marshal(body)
		if encodeErr != nil {
			return fmt.Errorf("encode authentication request: %w", encodeErr)
		}
		reader = bytes.NewReader(encoded)
	}
	req, err := http.NewRequestWithContext(ctx, method, origin+path, reader)
	if err != nil {
		return fmt.Errorf("create authentication request: %w", err)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	response, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("send authentication request: %w", err)
	}
	defer response.Body.Close() //nolint:errcheck
	limited := io.LimitReader(response.Body, 64*1024)
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		var value problem
		_ = json.NewDecoder(limited).Decode(&value)
		return &RemoteError{Code: value.Code, Detail: value.Detail, Status: response.StatusCode}
	}
	if destination == nil {
		return nil
	}
	if err := json.NewDecoder(limited).Decode(destination); err != nil {
		return fmt.Errorf("decode authentication response: %w", err)
	}
	return nil
}

func validatedOrigin(raw string) (string, error) {
	value := strings.TrimSpace(raw)
	parsed, err := url.Parse(value)
	if err != nil || parsed.Host == "" || parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" || (parsed.Path != "" && parsed.Path != "/") {
		return "", errors.New("artifact service URL must be a bare HTTPS origin, or HTTP loopback for local development")
	}
	loopback := parsed.Hostname() == "127.0.0.1" || parsed.Hostname() == "localhost" || parsed.Hostname() == "::1"
	if parsed.Scheme != "https" && (parsed.Scheme != "http" || !loopback) {
		return "", errors.New("artifact service URL must use HTTPS outside loopback")
	}
	return strings.TrimSuffix(parsed.String(), "/"), nil
}
