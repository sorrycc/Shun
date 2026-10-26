// Package mon signs Shun in to Mon, the account behind sorrycc's apps at
// mon.sorrycc.dev, syncs its settings and sends crash reports.
//
// It is Mayoi's copy of github.com/sorrycc/mon/client, at Mon 17fc8af, and
// its crashes.go at Mon 631d84a, which is a private module: change both
// together. Sign-in uses the OAuth device flow: the app shows a short
// code, the person approves it on the Mon website, and the app receives an
// access token.
//
//	c := mon.New("https://mon.sorrycc.dev")
//	dc, err := c.StartDeviceLogin(ctx, "shun", hostname)
//	// Show dc.UserCode and open dc.VerificationURIComplete in a browser.
//	token, err := c.WaitForDeviceToken(ctx, dc)
//	c.Token = token // store it in the keychain
//
//	st, err := c.GetSettings(ctx, "shun")
//	st, err = c.PutSettings(ctx, "shun", st.Version, mySettings)
//
//	text, err := c.StreamMessage(ctx, mon.MessageRequest{
//		Model: "deepseek-flash", MaxTokens: 4096,
//		Messages: []mon.Message{{Role: "user", Content: "Hello"}},
//	}, func(s string) { fmt.Print(s) })
package mon

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

const deviceGrantType = "urn:ietf:params:oauth:grant-type:device_code"

var (
	// ErrAuthorizationPending means the person has not decided yet.
	ErrAuthorizationPending = errors.New("mon: waiting for approval")
	// ErrSlowDown means the app polled too often.
	ErrSlowDown = errors.New("mon: polling too often")
	// ErrAccessDenied means the person declined the sign-in.
	ErrAccessDenied = errors.New("mon: sign-in was declined")
	// ErrExpired means the device code expired before it was approved.
	ErrExpired = errors.New("mon: sign-in expired")
	// ErrUnauthorized means the token is missing, invalid or revoked; the app
	// should forget it and offer to sign in again.
	ErrUnauthorized = errors.New("mon: not signed in")
)

type Client struct {
	BaseURL    string
	Token      string
	HTTPClient *http.Client
}

func New(baseURL string) *Client {
	return &Client{BaseURL: strings.TrimRight(baseURL, "/"), HTTPClient: &http.Client{Timeout: 30 * time.Second}}
}

// Error is an error response from Mon.
type Error struct {
	Status      int    `json:"-"`
	Code        string `json:"error"`
	Description string `json:"error_description"`
}

func (e *Error) Error() string {
	if e.Description != "" {
		return fmt.Sprintf("mon: %s (%s)", e.Description, e.Code)
	}
	return fmt.Sprintf("mon: HTTP %d %s", e.Status, e.Code)
}

// ConflictError is returned by PutSettings when the settings changed since
// the base version. Current holds what is stored now.
type ConflictError struct {
	Current *Settings
}

func (e *ConflictError) Error() string {
	return fmt.Sprintf("mon: settings changed on the server (now version %d)", e.Current.Version)
}

type DeviceCode struct {
	DeviceCode              string `json:"device_code"`
	UserCode                string `json:"user_code"`
	VerificationURI         string `json:"verification_uri"`
	VerificationURIComplete string `json:"verification_uri_complete"`
	ExpiresIn               int    `json:"expires_in"`
	Interval                int    `json:"interval"`
}

// StartDeviceLogin begins signing in. deviceName, such as the computer's
// name, is shown to the person so they can tell their machines apart.
func (c *Client) StartDeviceLogin(ctx context.Context, clientID, deviceName string) (*DeviceCode, error) {
	var dc DeviceCode
	err := c.do(ctx, "POST", "/api/v1/device/code", url.Values{"client_id": {clientID}, "device_name": {deviceName}}, &dc)
	if err != nil {
		return nil, err
	}
	return &dc, nil
}

// ExchangeDeviceCode asks once whether the sign-in was approved. It returns
// the access token, or ErrAuthorizationPending, ErrSlowDown, ErrAccessDenied
// or ErrExpired.
func (c *Client) ExchangeDeviceCode(ctx context.Context, deviceCode string) (string, error) {
	var out struct {
		AccessToken string `json:"access_token"`
	}
	err := c.do(ctx, "POST", "/api/v1/device/token", url.Values{"grant_type": {deviceGrantType}, "device_code": {deviceCode}}, &out)
	var e *Error
	if errors.As(err, &e) {
		switch e.Code {
		case "authorization_pending":
			return "", ErrAuthorizationPending
		case "slow_down":
			return "", ErrSlowDown
		case "access_denied":
			return "", ErrAccessDenied
		case "expired_token", "invalid_grant":
			return "", ErrExpired
		}
	}
	return out.AccessToken, err
}

// WaitForDeviceToken polls until the person approves or declines, the code
// expires, or ctx is done.
func (c *Client) WaitForDeviceToken(ctx context.Context, dc *DeviceCode) (string, error) {
	interval := time.Duration(dc.Interval) * time.Second
	if interval <= 0 {
		interval = 5 * time.Second
	}
	for {
		select {
		case <-ctx.Done():
			return "", ctx.Err()
		case <-time.After(interval):
		}
		token, err := c.ExchangeDeviceCode(ctx, dc.DeviceCode)
		switch {
		case errors.Is(err, ErrAuthorizationPending):
		case errors.Is(err, ErrSlowDown):
			interval += 5 * time.Second
		default:
			return token, err
		}
	}
}

type User struct {
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	Email     string    `json:"email"`
	AvatarURL string    `json:"avatar_url"`
	CreatedAt time.Time `json:"created_at"`
}

// Me returns the signed-in user.
func (c *Client) Me(ctx context.Context) (*User, error) {
	var u User
	if err := c.do(ctx, "GET", "/api/v1/me", nil, &u); err != nil {
		return nil, err
	}
	return &u, nil
}

// Logout revokes the token on the server.
func (c *Client) Logout(ctx context.Context) error {
	return c.do(ctx, "POST", "/api/v1/logout", nil, nil)
}

// Settings is an app's synced settings. Version is 0 and Data is null until
// the first PutSettings.
type Settings struct {
	App       string          `json:"app"`
	Version   int64           `json:"version"`
	Data      json.RawMessage `json:"data"`
	UpdatedAt *time.Time      `json:"updated_at"`
}

func (c *Client) GetSettings(ctx context.Context, app string) (*Settings, error) {
	var st Settings
	if err := c.do(ctx, "GET", "/api/v1/settings/"+url.PathEscape(app), nil, &st); err != nil {
		return nil, err
	}
	return &st, nil
}

// PutSettings saves data, which must encode to a JSON object, if the stored
// settings are still at baseVersion. Otherwise it returns a *ConflictError
// with the current settings, so the app can merge and try again.
func (c *Client) PutSettings(ctx context.Context, app string, baseVersion int64, data any) (*Settings, error) {
	body := map[string]any{"base_version": baseVersion, "data": data}
	var st Settings
	err := c.do(ctx, "PUT", "/api/v1/settings/"+url.PathEscape(app), body, &st)
	var e *conflictResponse
	if errors.As(err, &e) {
		return nil, &ConflictError{Current: &e.Current}
	}
	if err != nil {
		return nil, err
	}
	return &st, nil
}

type conflictResponse struct {
	Code    string   `json:"error"`
	Current Settings `json:"current"`
}

func (e *conflictResponse) Error() string { return "mon: version conflict" }

// Message is a turn of a conversation with the AI gateway: Role is "user"
// or "assistant".
type Message struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// MessageRequest asks the AI gateway for an answer, as the Anthropic
// Messages API does.
type MessageRequest struct {
	Model     string    `json:"model"`
	MaxTokens int       `json:"max_tokens"`
	System    string    `json:"system,omitempty"`
	Messages  []Message `json:"messages"`
}

// RateLimitError is returned by StreamMessage when the account used up its
// daily tokens or its requests a minute. RetryAfter is how long to wait, 0
// when Mon did not say.
type RateLimitError struct {
	RetryAfter time.Duration
	Message    string
}

func (e *RateLimitError) Error() string {
	if e.Message != "" {
		return "mon: " + e.Message
	}
	return "mon: over the AI gateway's limit"
}

// aiErrorBody is the Anthropic API's shape of an error, which the AI
// gateway answers with.
type aiErrorBody struct {
	Error struct {
		Type    string `json:"type"`
		Message string `json:"message"`
	} `json:"error"`
}

// StreamMessage sends req to the AI gateway and calls onText, if not nil,
// with each piece of the answer's text as it arrives, leaving out thinking.
// It returns the whole text. It fails with ErrUnauthorized when the token
// is no longer good, a *RateLimitError over a limit, and an *Error for
// anything else Mon or the provider refused.
func (c *Client) StreamMessage(ctx context.Context, req MessageRequest, onText func(string)) (string, error) {
	body, err := json.Marshal(struct {
		MessageRequest
		Stream bool `json:"stream"`
	}{req, true})
	if err != nil {
		return "", err
	}
	hr, err := http.NewRequestWithContext(ctx, "POST", c.BaseURL+"/api/v1/ai/v1/messages", bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	hr.Header.Set("Content-Type", "application/json")
	hr.Header.Set("Accept", "text/event-stream")
	hr.Header.Set("anthropic-version", "2023-06-01")
	if c.Token != "" {
		hr.Header.Set("Authorization", "Bearer "+c.Token)
	}
	// An answer can take longer than the client's timeout for the rest of
	// the API: ctx bounds it instead.
	hc := http.Client{}
	if c.HTTPClient != nil {
		hc = *c.HTTPClient
		hc.Timeout = 0
	}
	resp, err := hc.Do(hr)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		raw, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
		return "", aiError(resp, raw)
	}

	var text strings.Builder
	sc := bufio.NewScanner(resp.Body)
	sc.Buffer(make([]byte, 64<<10), 4<<20)
	for sc.Scan() {
		data, ok := strings.CutPrefix(sc.Text(), "data:")
		if !ok {
			continue
		}
		var ev struct {
			Type  string `json:"type"`
			Delta struct {
				Type string `json:"type"`
				Text string `json:"text"`
			} `json:"delta"`
			aiErrorBody
		}
		if json.Unmarshal([]byte(strings.TrimSpace(data)), &ev) != nil {
			continue
		}
		switch ev.Type {
		case "content_block_delta":
			if ev.Delta.Type == "text_delta" && ev.Delta.Text != "" {
				text.WriteString(ev.Delta.Text)
				if onText != nil {
					onText(ev.Delta.Text)
				}
			}
		case "error":
			return text.String(), &Error{Status: http.StatusOK, Code: ev.Error.Type, Description: ev.Error.Message}
		case "message_stop":
			return text.String(), nil
		}
	}
	if err := sc.Err(); err != nil {
		return text.String(), err
	}
	return text.String(), io.ErrUnexpectedEOF
}

// aiError is the error of a refused AI gateway request.
func aiError(resp *http.Response, raw []byte) error {
	var b aiErrorBody
	json.Unmarshal(raw, &b)
	switch resp.StatusCode {
	case http.StatusUnauthorized:
		return ErrUnauthorized
	case http.StatusTooManyRequests:
		e := &RateLimitError{Message: b.Error.Message}
		if secs, err := strconv.Atoi(resp.Header.Get("Retry-After")); err == nil && secs > 0 {
			e.RetryAfter = time.Duration(secs) * time.Second
		}
		return e
	}
	return &Error{Status: resp.StatusCode, Code: b.Error.Type, Description: b.Error.Message}
}

// do sends body as a form for url.Values and as JSON otherwise, and decodes
// a successful response into out.
func (c *Client) do(ctx context.Context, method, path string, body any, out any) error {
	var r io.Reader
	var contentType string
	switch b := body.(type) {
	case nil:
	case url.Values:
		r, contentType = strings.NewReader(b.Encode()), "application/x-www-form-urlencoded"
	default:
		raw, err := json.Marshal(b)
		if err != nil {
			return err
		}
		r, contentType = strings.NewReader(string(raw)), "application/json"
	}
	req, err := http.NewRequestWithContext(ctx, method, c.BaseURL+path, r)
	if err != nil {
		return err
	}
	req.Header.Set("Accept", "application/json")
	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}
	if c.Token != "" {
		req.Header.Set("Authorization", "Bearer "+c.Token)
	}
	hc := c.HTTPClient
	if hc == nil {
		hc = http.DefaultClient
	}
	resp, err := hc.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	if err != nil {
		return err
	}
	if resp.StatusCode >= 300 {
		if resp.StatusCode == http.StatusUnauthorized {
			return ErrUnauthorized
		}
		if resp.StatusCode == http.StatusConflict {
			var cr conflictResponse
			if json.Unmarshal(raw, &cr) == nil && cr.Code == "version_conflict" {
				return &cr
			}
		}
		e := &Error{Status: resp.StatusCode}
		json.Unmarshal(raw, e)
		return e
	}
	if out == nil || len(raw) == 0 {
		return nil
	}
	return json.Unmarshal(raw, out)
}
