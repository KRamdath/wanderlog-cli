package api

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const (
	DefaultBaseURL = "https://wanderlog.com"
	// Wanderlog's edge rejects requests with an empty or scripted UA on some routes.
	DefaultUserAgent = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 " +
		"(KHTML, like Gecko) Chrome/131.0.0.0 Safari/537.36"
	SessionCookieName = "connect.sid"
)

// Client talks to Wanderlog's private web-client API.
//
// Two things about this API drive the design here:
//   - It answers HTTP 200 even for application errors, so the JSON envelope's
//     "success" field is the only reliable signal.
//   - Unknown routes fall through to the SPA and return HTML, which would
//     otherwise surface as an opaque JSON parse error.
type Client struct {
	BaseURL   string
	UserAgent string
	Session   string
	Language  string

	HTTP *http.Client
}

func New(session string) *Client {
	return &Client{
		BaseURL:   DefaultBaseURL,
		UserAgent: DefaultUserAgent,
		Session:   session,
		Language:  "en",
		HTTP: &http.Client{
			Timeout: 60 * time.Second,
			// Redirects would drop us onto HTML pages; surface them instead.
			CheckRedirect: func(*http.Request, []*http.Request) error {
				return http.ErrUseLastResponse
			},
		},
	}
}

// Error is an application-level failure reported inside a 200 response.
type Error struct {
	Status   int
	Message  string
	ErrTypes []string
}

func (e *Error) Error() string {
	if len(e.ErrTypes) > 0 {
		return fmt.Sprintf("%s (%s)", e.Message, strings.Join(e.ErrTypes, ","))
	}
	return e.Message
}

// IsNotLoggedIn reports whether the session cookie is missing or expired.
func (e *Error) IsNotLoggedIn() bool {
	for _, t := range e.ErrTypes {
		if t == "notLoggedIn" {
			return true
		}
	}
	return false
}

type envelope struct {
	Success  bool            `json:"success"`
	Data     json.RawMessage `json:"data"`
	User     json.RawMessage `json:"user"`
	Error    string          `json:"error"`
	Messages []string        `json:"messages"`
	ErrTypes []string        `json:"errTypes"`
}

type Request struct {
	Method string
	Path   string
	Query  url.Values
	Body   any
}

// Response carries the decoded envelope plus any session cookie the server
// rotated in on this call.
type Response struct {
	Data       json.RawMessage
	User       json.RawMessage
	NewSession string
	Raw        []byte
}

func (c *Client) Do(req Request) (*Response, error) {
	u := c.BaseURL + req.Path
	if len(req.Query) > 0 {
		u += "?" + req.Query.Encode()
	}

	var body io.Reader
	if req.Body != nil {
		b, err := json.Marshal(req.Body)
		if err != nil {
			return nil, fmt.Errorf("encoding request body: %w", err)
		}
		body = bytes.NewReader(b)
	}

	method := req.Method
	if method == "" {
		method = http.MethodGet
	}

	httpReq, err := http.NewRequest(method, u, body)
	if err != nil {
		return nil, err
	}
	httpReq.Header.Set("Accept", "application/json")
	httpReq.Header.Set("User-Agent", c.UserAgent)
	if req.Body != nil {
		httpReq.Header.Set("Content-Type", "application/json")
	}
	if c.Session != "" {
		httpReq.AddCookie(&http.Cookie{Name: SessionCookieName, Value: c.Session})
	}

	resp, err := c.HTTP.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("requesting %s %s: %w", method, req.Path, err)
	}
	defer resp.Body.Close()

	raw, err := io.ReadAll(io.LimitReader(resp.Body, 32<<20))
	if err != nil {
		return nil, fmt.Errorf("reading response: %w", err)
	}

	out := &Response{Raw: raw}
	for _, ck := range resp.Cookies() {
		if ck.Name == SessionCookieName && ck.Value != "" {
			out.NewSession = ck.Value
		}
	}

	// An unknown route renders the SPA shell rather than 404ing.
	if ct := resp.Header.Get("Content-Type"); strings.Contains(ct, "text/html") {
		return out, &Error{
			Status:   resp.StatusCode,
			Message:  fmt.Sprintf("endpoint %s %s returned HTML, not JSON — the route likely does not exist", method, req.Path),
			ErrTypes: []string{"notJson"},
		}
	}

	var env envelope
	if err := json.Unmarshal(raw, &env); err != nil {
		snippet := string(raw)
		if len(snippet) > 200 {
			snippet = snippet[:200] + "..."
		}
		return out, fmt.Errorf("decoding response from %s: %w (body: %s)", req.Path, err, snippet)
	}

	if !env.Success {
		msg := env.Error
		if len(env.Messages) > 0 {
			msg = strings.Join(env.Messages, "; ")
		}
		if msg == "" {
			msg = fmt.Sprintf("request failed with HTTP %d", resp.StatusCode)
		}
		return out, &Error{Status: resp.StatusCode, Message: msg, ErrTypes: env.ErrTypes}
	}

	out.Data = env.Data
	out.User = env.User
	return out, nil
}

// JSON runs a request and unmarshals the envelope's data field into v.
func (c *Client) JSON(req Request, v any) error {
	resp, err := c.Do(req)
	if err != nil {
		return err
	}
	if c.Session == "" && resp.NewSession != "" {
		c.Session = resp.NewSession
	}
	if v == nil || len(resp.Data) == 0 {
		return nil
	}
	return json.Unmarshal(resp.Data, v)
}
