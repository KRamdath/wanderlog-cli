package cli

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"net/url"
	"strings"

	"github.com/KRamdath/wanderlog-cli/internal/api"
)

// rawAPI is the escape hatch. Wanderlog's private API is larger than the
// modelled commands and shifts without notice, so an agent needs a way to call
// an endpoint this CLI does not wrap yet.
func rawAPI(args []string) error {
	fs := flag.NewFlagSet("api", flag.ContinueOnError)
	data := fs.String("data", "", "JSON request body")
	query := fs.String("query", "", "query string, e.g. 'a=1&b=2'")
	if err := parseFlags(fs, args); err != nil {
		return err
	}
	if fs.NArg() < 2 {
		return errors.New("usage: wlog api <METHOD> <path> [--data JSON] [--query STR]")
	}

	method := strings.ToUpper(fs.Arg(0))
	path := normalizePath(fs.Arg(1))

	var body any
	if *data != "" {
		if err := json.Unmarshal([]byte(*data), &body); err != nil {
			return fmt.Errorf("--data is not valid JSON: %w", err)
		}
	}

	var q url.Values
	if *query != "" {
		parsed, err := url.ParseQuery(*query)
		if err != nil {
			return fmt.Errorf("--query is not a valid query string: %w", err)
		}
		q = parsed
	}

	c, err := client()
	if err != nil {
		return err
	}
	resp, err := c.Do(api.Request{Method: method, Path: path, Query: q, Body: body})
	if err != nil {
		return err
	}
	// Data may be absent on endpoints that return a bare envelope; fall back to
	// the whole response so nothing is silently swallowed.
	if len(resp.Data) > 0 {
		return emitRaw(resp.Data)
	}
	return emitRaw(resp.Raw)
}

// normalizePath repairs a path argument before it is sent.
//
// Git Bash on Windows rewrites a leading-slash argument into a Windows path
// ("/api/user" becomes "C:/Program Files/Git/api/user"), which would otherwise
// produce a confusing 404. Anything preceding the "/api/" segment is dropped.
func normalizePath(p string) string {
	p = strings.ReplaceAll(p, "\\", "/")
	if i := strings.Index(p, "/api/"); i > 0 {
		p = p[i:]
	}
	if !strings.HasPrefix(p, "/") {
		p = "/" + p
	}
	return p
}
