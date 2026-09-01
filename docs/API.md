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
`missingRequestParams`, `MissingRequestParamsOrToken`, `noGeosForTripPlan`,
`googleMapsNotFound`, `incompatibleItineraryConversion`, `unexpectedError`.

`unexpectedError` is the server's catch-all for a malformed body and names
nothing useful — when you hit it, the payload is wrong, not the endpoint.

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
| POST | `/api/tripPlans` | Create (full payload below) → `{id, key, title, viewKey, createdSectionIds}` |
| GET | `/api/tripPlans/{key}` | Full trip document; **requires `clientSchemaVersion`** |
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

### Creating a trip

Verified against the live API. **Every field must be present** — the server
answers a partial body with a generic `unexpectedError` rather than naming the
missing field, so this is not a case where omitting optionals is safe:

```jsonc
POST /api/tripPlans
{
  "geoIds": [2],                          // REQUIRED, non-empty
  "initialMapsPlaceIds": [],
  "initialSections": null,
  "initialEmailId": null,
  "type": "plan",                         // plan | journal | recommendations | story
  "privacy": "friends",                   // private | friends | public
  "isMapEmbed": false,
  "title": null,                          // null → Wanderlog names it "Trip to <geo>"
  "startDate": "2026-10-01",              // null for an undated trip
  "endDate": "2026-10-04",
  "autogenerateItineraryOptions": null,
  "language": "en"
}
→ {"id":20952388,"key":"wgypedspwdjtcvmm","title":"...","viewKey":"hbsfuwzcvu",
   "createdSectionIds":[]}
```

Gotchas confirmed by testing:

- **`geoIds` must be non-empty.** A destination-less trip is refused with
  `noGeosForTripPlan`. Resolve an id via `/api/geo/autocomplete/{q}`.
- **`type` is `"plan"`**, not `"tripPlan"`.
- `createdSectionIds` comes back empty *even when* day sections were generated
  from the date range — read `/sections` instead of trusting it.
- Passing a date range creates one `normal` section per day automatically.

### Reading a trip

`GET /api/tripPlans/{key}?clientSchemaVersion=2`

Two things are unusual here:

- Without `clientSchemaVersion` the server refuses with
  `incompatibleItineraryConversion` — surfaced as "your app is too old".
  The value is baked into the bundle (currently `2`).
- The payload is at the **top level** of the envelope — `tripPlan`,
  `resources`, `guideResources`, `settings` — *not* nested under `data`.

The itinerary lives at `tripPlan.itinerary.sections[].blocks[]`. A place block
has `type: "place"`, a `text` note, and `place` holding the full Google object.

### Sections

`GET /api/tripPlans/{key}/sections`

```jsonc
[{"id": 970390572, "displayHeading": "Notes", "type": "textOnly", "placeMarkerColor": "#000000"},
 {"id": 677463973, "displayHeading": "Places to visit", "type": "normal", ...},
 {"id": 391150968, "displayHeading": "Thursday, October 1st", "type": "normal", ...}]
```

Section **ids are numbers, not strings**, and the label is `displayHeading` —
there is no `name` or `date` field. Types seen: `normal`, `textOnly`.

### Adding places

```
POST /api/tripPlans/{key}/sections/{sectionId}/places
{
  "places": [ { "place": <full Google Places object>, "text": "note or null" } ],
  "addDuplicates": false
}
→ { "addedPlaceIds": [...] }
```

Three things to get right:

- `place` must be the **whole object from `getPlaceDetails/v2`**, not a stub.
  The server keys off its `place_id` and reads other fields off it.
- **`text` is a Quill rich-text delta, not a string.** The correct shape is
  `{"ops":[{"insert":"your note
"}]}`, and a place with no note is stored as
  `{"ops":[{"insert":"
"}]}`. The server accepts a bare string *without
  complaint* and stores it verbatim, producing a note in a shape the editor
  never authors — a silent-corruption trap, since the write appears to succeed.
- Omitting `/{sectionId}` (i.e. posting to `.../sections/places`) lets the
  server choose; in testing it landed in the "Places to visit" section.
- `DELETE` takes the Google `place_id` strings in `{placeIds: [...]}`.
- A `place_id` Google no longer knows is rejected with `googleMapsNotFound`.

### Block times

Place blocks carry `startTime` and `endTime` as `"HH:MM"` strings (or null).
These are the real schedule fields the itinerary timeline renders from — a time
written into the note text does **not** position the item. The
`sections/places` endpoint cannot set them; `applyOps` can.

### `applyOps` — ShareDB json0

`POST /api/tripPlans/{key}/applyOps` with `{"ops":[...]}`. No revision or
version is required. The ops are **ShareDB json0**, with paths rooted at the
trip document:

```jsonc
// set a field that is currently null
{"p":["itinerary","sections",3,"blocks",0,"startTime"], "oi":"10:00"}

// replace an existing value
{"p":["itinerary","sections",3,"blocks",1,"startTime"], "od":"10:00", "oi":"11:30"}

// move a block within its day
{"p":["itinerary","sections",3,"blocks",2], "lm":0}
```

Verified against the live API. Notes:

- Paths use **array indices**, not ids, so read the document first and never
  reuse indices across a write.
- Ops in one batch apply **sequentially** — a move shifts the indices every
  later op sees. Compute a batch against a simulated array.
- `addPlaces` appends to the end of a day. Ordering a timeline therefore means
  writing times and then issuing moves; `wlog trip schedule-day` does both.

The higher-level `sections/places` endpoints remain the right way to add and
remove places; `applyOps` is only needed for field edits and ordering.

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
