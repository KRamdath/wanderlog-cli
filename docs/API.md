# Wanderlog private API notes

Reverse-engineered from the compiled web-client bundles at
`itin-compiled.azureedge.net/<build>/compiled/` and verified against the live
site. Unofficial and subject to change without notice.

Base URL: `https://wanderlog.com`

## Response envelope

Every JSON route wraps its payload:

```jsonc
{ "success": true,  "data": <payload> }        // most routes
{ "success": true,  "user": <user|null> }      // /api/user
{ "success": false, "error": "ApplicationError: ...",
  "messages": ["..."], "errTypes": ["notLoggedIn"] }
```

**Errors are returned with HTTP 200.** Check `success`, never the status code.
Unknown routes return the SPA's HTML instead of a 404.

Observed `errTypes`: `notLoggedIn`, `noEmailAccount`, `badMapsPlaceId`,
`missingRequestParams`, `MissingRequestParamsOrToken`.

## Auth

See [AUTH.md](AUTH.md).

| Method | Path | Notes |
|---|---|---|
| POST | `/api/user/login` | `{email, password}` → sets `connect.sid` |
| GET | `/api/user` | Whoami; `user` is null when logged out |
| POST | `/api/user/logout` | |
| POST | `/api/user/loginGoogleIdToken` | Provider token required |
| POST | `/api/user/loginAppleAuthCode` | Provider token required |

## Destinations (geos)

A *geo* is a Wanderlog destination — a city, region, or country. It is distinct
from a Google place, and it carries the coordinates used to bias place search.

| Method | Path | Notes |
|---|---|---|
| GET | `/api/geo/autocomplete/{query}` | **No auth required** |
| GET | `/api/geo/autocompleteGeoAndUser/{query}` | Geos plus user profiles |
| GET | `/api/geo/countries` | |

```
$ curl 'https://wanderlog.com/api/geo/autocomplete/paris'
{"success":true,"data":[{"id":9614,"name":"Paris","countryName":"France",
 "latitude":48.857037,"longitude":2.349401,"subcategory":"city",
 "bounds":[2.2242,48.81557,2.46992,48.90214],"mapsPlaceId":19084}, ...]}
```

Note `mapsPlaceId` here is Wanderlog's **internal integer** id, not a Google
place id string. `GET /api/places/{id}` takes that integer.

## Places

These proxy the Google Places API. Both search routes take a single `request`
query parameter containing a JSON object.

| Method | Path | Params |
|---|---|---|
| GET | `/api/placesAPI/autocomplete/v2` | `request={input, sessiontoken, location, radius, language}` |
| GET | `/api/placesAPI/getPlaceDetails/v2` | `placeId`, `language` |
| GET | `/api/placesAPI/getMultiplePlaceDetails` | `placeIds`, `language` |
| GET | `/api/placesAPI/getPlaceDetailsAndCardData` | `placeId`, `language` |
| GET | `/api/placesAPI/findPlaceFromLngLat` | |
| GET | `/api/places/{internalId}` | Wanderlog place page data |

**All five of `input`, `sessiontoken`, `location`, `radius`, `language` are
required** for autocomplete; omitting any yields
`MissingRequestParamsOrToken`. `sessiontoken` is a client-generated UUID —
the server only checks that it is present. `location` accepts `"lat,lng"`.

```
$ curl -G 'https://wanderlog.com/api/placesAPI/autocomplete/v2' \
    --data-urlencode 'request={"input":"eiffel tower","sessiontoken":"<uuid>",
                               "location":"48.8566,2.3522","radius":50000,"language":"en"}'
{"success":true,"data":[{"place_id":"ChIJLU7jZClu5kcR4PcOOO6p3I0",
 "description":"Eiffel Tower, Avenue Gustave Eiffel, Paris, France", ...}]}
```

`getPlaceDetails/v2` returns a **Google Places object** — `place_id`, `name`,
`geometry`, `formatted_address`, `rating`, `opening_hours`, and so on. This
object is what the itinerary endpoints expect to receive back verbatim.

## Trips

A trip is identified by its `key`, the short id in `wanderlog.com/plan/<key>`.
All of these require auth.

| Method | Path | Body / notes |
|---|---|---|
| GET | `/api/tripPlans` | List your trips |
| POST | `/api/tripPlans` | `{title, startDate, endDate, type, language}` → `{key, ...}` |
| GET | `/api/tripPlans/{key}` | Full trip document |
| DELETE | `/api/tripPlans/{key}` | |
| POST | `/api/tripPlans/{key}/restore` | Undo a delete |
| GET | `/api/tripPlans/{key}/sections` | Days/sections; optional `datePreference` |
| GET | `/api/tripPlans/{key}/places` | Places already in the trip |
| POST | `/api/tripPlans/{key}/sections/{sectionId}/places` | Add places (below) |
| DELETE | `/api/tripPlans/{key}/sections/{sectionId}/places` | `{placeIds: [...]}` |
| POST | `/api/tripPlans/{key}/applyOps` | `{ops: [...]}` — low-level OT channel |
| POST | `/api/tripPlans/{key}/invite` | `{invitees: [...], message}` |
| GET | `/api/tripPlans/{key}/invites` | |
| POST | `/api/tripPlans/{key}/collaborator` | `{userId}` |
| POST | `/api/tripPlans/{key}/like` | `{liked: bool}` |
| POST | `/api/tripPlans/{key}/export/v2` | |
| GET | `/api/tripPlans/{key}/expensesAsCSV` | |
| GET | `/api/tripPlans/home` | Dashboard feed |

### Adding places

```
POST /api/tripPlans/{key}/sections/{sectionId}/places
{
  "places": [ { "place": <full Google Places object>, "text": "note or null" } ],
  "addDuplicates": false
}
→ { "addedPlaceIds": [...] }
```

Two things to get right:

- `place` must be the **whole object from `getPlaceDetails/v2`**, not a stub.
  The server keys off its `place_id` and reads other fields off it.
- Omitting `/{sectionId}` (i.e. posting to `.../sections/places`) adds to the
  trip's unscheduled list rather than to a particular day.

### `applyOps`

The itinerary is edited through an operational-transform log. The web client
composes `ops` arrays and posts them to `applyOps` for fine-grained edits —
reordering, timings, notes, arbitrary block mutations. The op schema is large,
versioned (`clientSchemaVersion`, checked via
`GET /api/tripPlans/{key}/updateRequired`), and changes often.

This CLI deliberately uses the higher-level `sections/places` endpoints, which
are stable and cover trip building. `wlog api` is available if you need to
drive `applyOps` directly.

## Other namespaces seen in the bundles

`/api/payments/*`, `/api/user/notifications*`, `/api/user/following*`,
`/api/tripPlans/browse/guides`, `/api/tripPlans/feed/v2`,
`/api/tripPlans/autofillDay`, `/api/tripPlans/flights`,
`/api/tripPlans/wsCursors/*` and `/api/tripPlans/wsOverall/*` (realtime
collaboration).

`/api/tripPlans/autofillDay` is likely Wanderlog's own itinerary autofill and
may be interesting for agent use; its request shape was not investigated.

## Reproducing this map

```bash
# The SPA shell lists the eager bundles.
curl -s https://wanderlog.com/api/auth/me | grep -oE 'https://[^"]*\.js'

# main.js embeds a webpack manifest of ~275 lazily-loaded chunks;
# the placesAPI routes appear only in those.
grep -ohE '"/api/[A-Za-z0-9_/.-]*"' *.js | tr -d '"' | sort -u
```
