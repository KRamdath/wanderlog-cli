package api

import (
	"encoding/json"
	"errors"
	"net/http"
)

// User is the account behind the current session.
type User struct {
	ID       int    `json:"id"`
	Email    string `json:"email"`
	Username string `json:"username"`
	Name     string `json:"name"`
}

// Login exchanges email/password for a session cookie.
//
// Accounts created through Google/Apple/Facebook SSO have no password and will
// fail here with errType "noPasswordAccount" (or similar); those users must
// supply an existing connect.sid cookie instead.
func Login(email, password string) (session string, user *User, err error) {
	c := New("")
	resp, err := c.Do(Request{
		Method: http.MethodPost,
		Path:   "/api/user/login",
		Body:   map[string]string{"email": email, "password": password},
	})
	if err != nil {
		return "", nil, err
	}
	if resp.NewSession == "" {
		return "", nil, errors.New("login succeeded but no session cookie was returned")
	}

	u := &User{}
	// The login response shape has varied; fall back to an explicit whoami.
	if len(resp.User) > 0 {
		_ = json.Unmarshal(resp.User, u)
	} else if len(resp.Data) > 0 {
		_ = json.Unmarshal(resp.Data, u)
	}
	if u.ID == 0 {
		if who, werr := New(resp.NewSession).Whoami(); werr == nil {
			u = who
		}
	}
	return resp.NewSession, u, nil
}

// Whoami returns the signed-in user, or nil when the session is anonymous.
//
// This route answers success:true with a null user rather than an error, so a
// nil result is the "logged out" signal.
func (c *Client) Whoami() (*User, error) {
	resp, err := c.Do(Request{Path: "/api/user"})
	if err != nil {
		return nil, err
	}
	if len(resp.User) == 0 || string(resp.User) == "null" {
		return nil, nil
	}
	u := &User{}
	if err := json.Unmarshal(resp.User, u); err != nil {
		return nil, err
	}
	return u, nil
}

func (c *Client) Logout() error {
	_, err := c.Do(Request{Method: http.MethodPost, Path: "/api/user/logout"})
	return err
}
