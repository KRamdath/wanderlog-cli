package cli

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"strings"

	"github.com/KRamdath/wanderlog-cli/internal/api"
	"github.com/KRamdath/wanderlog-cli/internal/config"
)

// Exit codes are stable so an agent can branch on them without parsing text.
const (
	ExitOK          = 0
	ExitError       = 1
	ExitUsage       = 2
	ExitNotLoggedIn = 3
)

const usage = `wlog — a Wanderlog trip-building CLI for AI agents

Usage:
  wlog <command> [subcommand] [flags]

Auth:
  wlog auth login --email E --password P   Log in with email/password
  wlog auth login --cookie <connect.sid>   Log in with a browser session cookie
  wlog auth status                         Show the signed-in user
  wlog auth logout                         Clear the stored session

Destinations & places:
  wlog geo search <query>                  Find a destination (city/region/country)
  wlog place search <query> --near LAT,LNG Find places near a point
  wlog place get <googlePlaceId>           Fetch full place details

Trips:
  wlog trip list                           List your trips
  wlog trip create --geo NAME [--title T] [--start D] [--end D]
  wlog trip get <key>                      Fetch the raw trip document
  wlog trip delete <key>                   Delete a trip
  wlog trip sections <key>                 List a trip's days/sections
  wlog trip add-place <key> --place P [--section S] [--note N]
  wlog trip remove-place <key> --place-id P [--section S]

Escape hatch:
  wlog api <METHOD> <path> [--data JSON]   Call any endpoint directly

Environment:
  WANDERLOG_SESSION      Session cookie, overrides the stored credentials
  WANDERLOG_CONFIG_DIR   Credential directory (default ~/.config/wanderlog)

Exit codes: 0 ok, 1 error, 2 usage, 3 not logged in

Wanderlog has no public API. This tool drives the private web-client API and
may break without notice. Use it only with your own account.
`

func Run(args []string) int {
	if len(args) < 1 {
		fmt.Fprint(os.Stderr, usage)
		return ExitUsage
	}

	switch args[0] {
	case "-h", "--help", "help":
		fmt.Print(usage)
		return ExitOK
	case "auth":
		return dispatch(args[1:], map[string]handler{
			"login":  authLogin,
			"status": authStatus,
			"logout": authLogout,
		}, "auth")
	case "geo":
		return dispatch(args[1:], map[string]handler{"search": geoSearch}, "geo")
	case "place":
		return dispatch(args[1:], map[string]handler{
			"search": placeSearch,
			"get":    placeGet,
		}, "place")
	case "trip":
		return dispatch(args[1:], map[string]handler{
			"list":         tripList,
			"create":       tripCreate,
			"get":          tripGet,
			"delete":       tripDelete,
			"sections":     tripSections,
			"add-place":    tripAddPlace,
			"remove-place": tripRemovePlace,
		}, "trip")
	case "api":
		return wrap(rawAPI)(args[1:])
	default:
		fmt.Fprintf(os.Stderr, "unknown command %q\n\n%s", args[0], usage)
		return ExitUsage
	}
}

type handler func([]string) int

func dispatch(args []string, table map[string]handler, group string) int {
	if len(args) < 1 {
		fmt.Fprintf(os.Stderr, "%s: expected a subcommand\n\n%s", group, usage)
		return ExitUsage
	}
	h, ok := table[args[0]]
	if !ok {
		fmt.Fprintf(os.Stderr, "unknown %s subcommand %q\n", group, args[0])
		return ExitUsage
	}
	return h(args[1:])
}

// wrap turns an error-returning command into one that maps errors onto exit
// codes and prints a hint when the session has lapsed.
func wrap(fn func([]string) error) handler {
	return func(args []string) int {
		err := fn(args)
		if err == nil {
			return ExitOK
		}
		var apiErr *api.Error
		if errors.As(err, &apiErr) && apiErr.IsNotLoggedIn() {
			fmt.Fprintln(os.Stderr, "error: not logged in — run `wlog auth login` first")
			return ExitNotLoggedIn
		}
		if errors.Is(err, flag.ErrHelp) {
			return ExitUsage
		}
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		return ExitError
	}
}

// client builds an authenticated client from the environment or stored config.
func client() (*api.Client, error) {
	cfg, err := config.Load()
	if err != nil {
		return nil, err
	}
	c := api.New(cfg.Session)
	if base := os.Getenv(config.EnvBaseURL); base != "" {
		c.BaseURL = base
	}
	return c, nil
}

// authedClient additionally refuses to proceed without a stored session, so
// commands fail fast with a clear message instead of a server-side error.
func authedClient() (*api.Client, error) {
	c, err := client()
	if err != nil {
		return nil, err
	}
	if c.Session == "" {
		return nil, &api.Error{
			Message:  "no session stored",
			ErrTypes: []string{"notLoggedIn"},
		}
	}
	return c, nil
}

func emit(v any) error {
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	return enc.Encode(v)
}

// emitRaw pretty-prints an already-encoded JSON document.
func emitRaw(raw json.RawMessage) error {
	if len(raw) == 0 {
		return emit(map[string]any{})
	}
	var v any
	if err := json.Unmarshal(raw, &v); err != nil {
		fmt.Println(string(raw))
		return nil
	}
	return emit(v)
}

func requireArg(fs *flag.FlagSet, name string) (string, error) {
	if fs.NArg() < 1 {
		return "", fmt.Errorf("missing required argument <%s>", name)
	}
	return fs.Arg(0), nil
}

// parseFlags parses args allowing flags and positionals to be interleaved.
//
// Go's flag package stops at the first positional, so `place search "x" --geo y`
// would silently drop --geo. Agents write arguments in natural order, so the
// tokens are permuted into flags-then-positionals before parsing.
func parseFlags(fs *flag.FlagSet, args []string) error {
	var flags, positional []string

	for i := 0; i < len(args); i++ {
		a := args[i]

		if a == "--" {
			positional = append(positional, args[i+1:]...)
			break
		}
		if len(a) < 2 || a[0] != '-' {
			positional = append(positional, a)
			continue
		}

		name := strings.TrimLeft(a, "-")
		if eq := strings.IndexByte(name, '='); eq >= 0 {
			flags = append(flags, a)
			continue
		}

		flags = append(flags, a)
		// A non-boolean flag consumes the following token as its value.
		if f := fs.Lookup(name); f != nil && !isBoolFlag(f) && i+1 < len(args) {
			flags = append(flags, args[i+1])
			i++
		}
	}

	// The "--" terminator keeps a positional that looks like a flag (or one the
	// caller escaped) from being reparsed as one.
	return fs.Parse(append(flags, append([]string{"--"}, positional...)...))
}

func isBoolFlag(f *flag.Flag) bool {
	bf, ok := f.Value.(interface{ IsBoolFlag() bool })
	return ok && bf.IsBoolFlag()
}
