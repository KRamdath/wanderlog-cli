package cli

import (
	"errors"
	"flag"
	"fmt"
	"os"
	"strings"

	"github.com/KRamdath/wanderlog-cli/internal/api"
	"github.com/KRamdath/wanderlog-cli/internal/config"
)

var authLogin = wrap(func(args []string) error {
	fs := flag.NewFlagSet("auth login", flag.ContinueOnError)
	email := fs.String("email", "", "account email")
	password := fs.String("password", "", "account password (prefer WANDERLOG_PASSWORD)")
	cookie := fs.String("cookie", "", "an existing connect.sid cookie value")
	if err := parseFlags(fs, args); err != nil {
		return err
	}

	if *password == "" {
		*password = os.Getenv("WANDERLOG_PASSWORD")
	}

	// SSO accounts have no password, so pasting a browser cookie is the only
	// way in for them.
	if *cookie != "" {
		return loginWithCookie(*cookie)
	}
	if *email == "" || *password == "" {
		return errors.New("provide --email and --password, or --cookie with a connect.sid value")
	}

	session, user, err := api.Login(*email, *password)
	if err != nil {
		var apiErr *api.Error
		if errors.As(err, &apiErr) {
			for _, t := range apiErr.ErrTypes {
				if strings.Contains(strings.ToLower(t), "password") ||
					strings.Contains(strings.ToLower(t), "google") {
					return fmt.Errorf("%w — this account may use social sign-in; "+
						"copy the connect.sid cookie from your browser and use --cookie", err)
				}
			}
		}
		return err
	}

	cfg := &config.Config{Session: session, Email: *email}
	if user != nil {
		cfg.UserID = user.ID
		if user.Email != "" {
			cfg.Email = user.Email
		}
	}
	if err := cfg.Save(); err != nil {
		return err
	}
	return emit(map[string]any{"loggedIn": true, "email": cfg.Email, "userId": cfg.UserID})
})

func loginWithCookie(cookie string) error {
	cookie = strings.TrimSpace(cookie)
	cookie = strings.TrimPrefix(cookie, config.EnvSession+"=")
	cookie = strings.TrimPrefix(cookie, api.SessionCookieName+"=")

	c := api.New(cookie)
	user, err := c.Whoami()
	if err != nil {
		return err
	}
	if user == nil {
		return errors.New("that cookie is not associated with a signed-in session — " +
			"copy the current connect.sid value from your browser's cookie store")
	}

	cfg := &config.Config{Session: cookie, Email: user.Email, UserID: user.ID}
	if err := cfg.Save(); err != nil {
		return err
	}
	return emit(map[string]any{"loggedIn": true, "email": user.Email, "userId": user.ID})
}

var authStatus = wrap(func(args []string) error {
	c, err := client()
	if err != nil {
		return err
	}
	if c.Session == "" {
		return emit(map[string]any{"loggedIn": false, "reason": "no session stored"})
	}
	user, err := c.Whoami()
	if err != nil {
		return err
	}
	if user == nil {
		return emit(map[string]any{"loggedIn": false, "reason": "session expired or invalid"})
	}
	return emit(map[string]any{
		"loggedIn": true,
		"userId":   user.ID,
		"email":    user.Email,
		"username": user.Username,
		"name":     user.Name,
	})
})

var authLogout = wrap(func(args []string) error {
	c, err := client()
	if err != nil {
		return err
	}
	if c.Session != "" {
		// Best effort: the local credential is cleared regardless.
		_ = c.Logout()
	}
	if err := config.Clear(); err != nil {
		return err
	}
	return emit(map[string]any{"loggedIn": false})
})
