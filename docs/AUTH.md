# Wanderlog authentication

How auth actually works on wanderlog.com, and what the CLI does about it.
Everything below was determined by probing the live site and reading the
compiled web-client bundles; there is no official documentation.

## Summary

| Question | Answer |
|---|---|
| Public API? | No. There is no developer program, API key, or OAuth app. |
| Auth mechanism | An Express session cookie, `connect.sid` |
| Token format | Signed cookie, URL-encoded, e.g. `s%3A<24 chars>.<27 chars>` |
| Lifetime | ~1 year (`Expires` is set one year out and refreshed on use) |
| Cookie flags | `HttpOnly; Secure; SameSite=Lax; Path=/` |
| CSRF/XSRF token | **None.** No XSRF cookie is issued and no such header is required. |
| Bearer tokens | Not supported. `Authorization` headers are ignored. |

The session cookie is the entire credential. Anyone holding it can act as the
account, so it must be handled like a password.

## Determining this

`GET /api/config/globalConfig` on a clean client:

```
HTTP/1.1 200 OK
X-Powered-By: Express
Set-Cookie: connect.sid=s%3ANYvrOKkHP5LKS0VWCO3_Feq0CQJpJ9vo.Aink0zIyByg9sx6y1FGpdrQXAfh5zjTbr0KFh9zSef0;
            Path=/; Expires=Tue, 31 Aug 2027 15:14:10 GMT; HttpOnly; Secure; SameSite=Lax
```

A session is minted for anonymous visitors too; it only becomes privileged
after a successful login. Requesting a protected route without one:

```
$ curl https://wanderlog.com/api/tripPlans
{"error":"ApplicationError: You must be logged in","success":false,
 "messages":["You must be logged in"],"errTypes":["notLoggedIn"]}
```

Only `connect.sid` and `ajs_anonymous_id` (Segment analytics, irrelevant to
auth) are ever set. No XSRF cookie appears, which rules out a double-submit
CSRF scheme.

## Obtaining a session

### Email + password — `POST /api/user/login`

The web client posts `{email, password}` as JSON. The response sets a fresh
privileged `connect.sid`.

```
$ curl -X POST https://wanderlog.com/api/user/login \
    -H 'Content-Type: application/json' \
    -d '{"email":"nobody@example.invalid","password":"x"}'
{"success":false,"messages":["We could not find a user with the given username or email"],
 "errTypes":["noEmailAccount"]}
```

This is what `wlog auth login --email --password` uses. It means the CLI can
authenticate on its own — no browser or DevTools step — which matters for an
agent running unattended.

Note the endpoint is `/api/user/login`. `/api/auth/login` does not exist and
returns a 500 HTML error page.

### Social sign-in

The bundles also expose:

```
/api/user/loginGoogleAuthCode/v2
/api/user/loginGoogleIdToken
/api/user/loginAppleAuthCode
/api/user/loginFacebookAccessToken
/api/user/loginToken/login
```

These take provider tokens the CLI cannot mint on its own — they need the
provider's interactive consent flow with Wanderlog's own OAuth client id.
Accounts created through SSO have **no password**, so `POST /api/user/login`
cannot work for them.

For those accounts, supply an existing cookie instead:

```
wlog auth login --cookie 's%3A....'
```

Read it from a signed-in browser: DevTools → Application → Cookies →
`https://wanderlog.com` → `connect.sid`. Because the cookie is `HttpOnly`,
`document.cookie` will not show it — the cookie store is the only source.

### Verifying a session — `GET /api/user`

```
$ curl https://wanderlog.com/api/user
{"success":true,"user":null}
```

This route reports a logged-out session as `success:true` with a **null user**
rather than as an error, so a null `user` is the "not signed in" signal. `wlog
auth status` uses it.

## Two API quirks worth knowing

**Errors come back as HTTP 200.** Application failures are signalled only by
`"success": false` in the body. Branching on the HTTP status will silently
treat every failure as a success. The client keys off the envelope instead.

**Unknown routes return HTML.** A path the server does not recognise falls
through to the single-page app and returns a `text/html` document, not a 404.
The client detects the content type and reports the route as nonexistent
rather than surfacing a JSON parse error.

## How the CLI stores credentials

Resolution order:

1. `WANDERLOG_SESSION` environment variable
2. `~/.config/wanderlog/credentials.json` (override the directory with
   `WANDERLOG_CONFIG_DIR`)

The file is written `0600` inside a `0700` directory. On Windows the mode bits
are inert, but the file lands under the user profile whose ACL is already
owner-scoped.

For CI or a sandboxed agent, prefer the environment variable so no credential
is written to disk:

```bash
export WANDERLOG_SESSION='s%3A...'
wlog trip list
```

## Security notes

- The cookie is a **bearer credential valid for about a year**. Leaking it is
  equivalent to leaking the password, and there is no scoping or revocation
  short of logging out.
- `wlog auth logout` calls `POST /api/user/logout` and clears local state.
- Prefer `WANDERLOG_PASSWORD` over `--password`, which is visible in the
  process list and shell history.
- This is a private API driving a real account. Use it only with your own
  credentials and keep request volume comparable to normal interactive use.
