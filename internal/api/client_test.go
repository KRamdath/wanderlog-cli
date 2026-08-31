package api

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
)

// The API answers HTTP 200 for application errors, so the envelope's success
// field is the only reliable signal.
func TestDoTreatsSuccessFalseAsError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"success":false,"messages":["You must be logged in"],"errTypes":["notLoggedIn"]}`))
	}))
	defer srv.Close()

	c := New("")
	c.BaseURL = srv.URL

	_, err := c.Do(Request{Path: "/api/tripPlans"})
	if err == nil {
		t.Fatal("expected an error for success:false")
	}

	var apiErr *Error
	if !errors.As(err, &apiErr) {
		t.Fatalf("expected *api.Error, got %T", err)
	}
	if !apiErr.IsNotLoggedIn() {
		t.Errorf("IsNotLoggedIn() = false, want true (errTypes=%v)", apiErr.ErrTypes)
	}
	if apiErr.Message != "You must be logged in" {
		t.Errorf("Message = %q", apiErr.Message)
	}
}

// Unknown routes fall through to the SPA and return HTML.
func TestDoRejectsHTMLResponses(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write([]byte("<!DOCTYPE html><html></html>"))
	}))
	defer srv.Close()

	c := New("")
	c.BaseURL = srv.URL

	_, err := c.Do(Request{Path: "/api/nope"})
	var apiErr *Error
	if !errors.As(err, &apiErr) {
		t.Fatalf("expected *api.Error, got %v", err)
	}
	if len(apiErr.ErrTypes) == 0 || apiErr.ErrTypes[0] != "notJson" {
		t.Errorf("ErrTypes = %v, want [notJson]", apiErr.ErrTypes)
	}
}

func TestDoSendsSessionCookieAndCapturesRotation(t *testing.T) {
	var gotCookie string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if ck, err := r.Cookie(SessionCookieName); err == nil {
			gotCookie = ck.Value
		}
		http.SetCookie(w, &http.Cookie{Name: SessionCookieName, Value: "rotated"})
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"success":true,"data":{"ok":true}}`))
	}))
	defer srv.Close()

	c := New("s%3Aoriginal")
	c.BaseURL = srv.URL

	resp, err := c.Do(Request{Path: "/api/user"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if gotCookie != "s%3Aoriginal" {
		t.Errorf("server saw cookie %q, want %q", gotCookie, "s%3Aoriginal")
	}
	if resp.NewSession != "rotated" {
		t.Errorf("NewSession = %q, want %q", resp.NewSession, "rotated")
	}
}

// /api/user reports a logged-out session as success:true with a null user
// rather than as an error.
func TestWhoamiReturnsNilWhenLoggedOut(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"success":true,"user":null}`))
	}))
	defer srv.Close()

	c := New("whatever")
	c.BaseURL = srv.URL

	user, err := c.Whoami()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if user != nil {
		t.Errorf("user = %+v, want nil", user)
	}
}

func TestWhoamiParsesUser(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"success":true,"user":{"id":42,"email":"a@b.co","username":"ab"}}`))
	}))
	defer srv.Close()

	c := New("whatever")
	c.BaseURL = srv.URL

	user, err := c.Whoami()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if user == nil || user.ID != 42 || user.Email != "a@b.co" {
		t.Errorf("user = %+v", user)
	}
}

func TestAddPlacesBodyShape(t *testing.T) {
	var got map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewDecoder(r.Body).Decode(&got)
		if r.URL.Path != "/api/tripPlans/KEY/sections/SEC/places" {
			t.Errorf("path = %q", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"success":true,"data":{"addedPlaceIds":[1]}}`))
	}))
	defer srv.Close()

	c := New("sess")
	c.BaseURL = srv.URL

	_, err := c.AddPlaces("KEY", "SEC", []PlaceWithNote{{
		Place: json.RawMessage(`{"place_id":"ChIJabc","name":"Eiffel Tower"}`),
		Text:  NoteDelta("try the roof"),
	}}, false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	places, ok := got["places"].([]any)
	if !ok || len(places) != 1 {
		t.Fatalf("places = %v", got["places"])
	}
	entry := places[0].(map[string]any)
	place, ok := entry["place"].(map[string]any)
	if !ok || place["place_id"] != "ChIJabc" {
		t.Errorf("place = %v, want the full Google place object", entry["place"])
	}
	// Notes are Quill deltas; a bare string is stored verbatim by the server
	// and produces a note the editor did not author.
	delta, ok := entry["text"].(map[string]any)
	if !ok {
		t.Fatalf("text = %v, want a Quill delta object", entry["text"])
	}
	ops, ok := delta["ops"].([]any)
	if !ok || len(ops) != 1 {
		t.Fatalf("ops = %v", delta["ops"])
	}
	if insert := ops[0].(map[string]any)["insert"]; insert != "try the roof\n" {
		t.Errorf("insert = %q, want %q", insert, "try the roof\n")
	}
	if got["addDuplicates"] != false {
		t.Errorf("addDuplicates = %v", got["addDuplicates"])
	}
}

// Omitting the section id lets the server choose, which lands in "Places to visit".
func TestPlacesPathWithoutSection(t *testing.T) {
	if got := placesPath("K", ""); got != "/api/tripPlans/K/sections/places" {
		t.Errorf("placesPath = %q", got)
	}
	if got := placesPath("K", "S"); got != "/api/tripPlans/K/sections/S/places" {
		t.Errorf("placesPath = %q", got)
	}
}

// The create endpoint answers a partial body with a generic "unexpectedError",
// so every field must be present — nulls and empty arrays included.
func TestCreateTripSendsCompletePayload(t *testing.T) {
	var got map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewDecoder(r.Body).Decode(&got)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"success":true,"data":{"key":"abc","title":"T"}}`))
	}))
	defer srv.Close()

	c := New("sess")
	c.BaseURL = srv.URL

	title := "Kyoto in spring"
	if _, err := c.CreateTrip(CreateTripInput{GeoIDs: []int{2}, Title: &title}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	for _, k := range []string{
		"geoIds", "initialMapsPlaceIds", "initialSections", "initialEmailId",
		"type", "privacy", "isMapEmbed", "title", "startDate", "endDate",
		"autogenerateItineraryOptions", "language",
	} {
		if _, ok := got[k]; !ok {
			t.Errorf("payload is missing required key %q", k)
		}
	}

	if got["type"] != TypePlan {
		t.Errorf("type = %v, want %q", got["type"], TypePlan)
	}
	if got["privacy"] != PrivacyFriends {
		t.Errorf("privacy = %v, want %q", got["privacy"], PrivacyFriends)
	}
	// An unset date must serialise as null, not "".
	if got["startDate"] != nil {
		t.Errorf("startDate = %v, want null", got["startDate"])
	}
	// Empty id lists must be arrays, not null.
	if _, ok := got["initialMapsPlaceIds"].([]any); !ok {
		t.Errorf("initialMapsPlaceIds = %v, want []", got["initialMapsPlaceIds"])
	}
}

// Sections have numeric ids and carry displayHeading rather than name/date.
func TestSectionsDecodeNumericIDs(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"success":true,"data":[
		  {"id":970390572,"displayHeading":"Notes","type":"textOnly"},
		  {"id":512522282,"displayHeading":"Thursday, October 1st","type":"normal"}]}`))
	}))
	defer srv.Close()

	c := New("sess")
	c.BaseURL = srv.URL

	secs, err := c.Sections("KEY")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(secs) != 2 {
		t.Fatalf("got %d sections", len(secs))
	}
	if secs[1].ID != 512522282 || secs[1].DisplayHeading != "Thursday, October 1st" {
		t.Errorf("section = %+v", secs[1])
	}
}

// This route nests its payload at the top level, not under "data".
func TestGetTripReturnsTopLevelBody(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("clientSchemaVersion") != ClientSchemaVersion {
			t.Errorf("clientSchemaVersion = %q", r.URL.Query().Get("clientSchemaVersion"))
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"success":true,"tripPlan":{"key":"abc"},"settings":{}}`))
	}))
	defer srv.Close()

	c := New("sess")
	c.BaseURL = srv.URL

	raw, err := c.GetTrip("abc")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	var body map[string]any
	if err := json.Unmarshal(raw, &body); err != nil {
		t.Fatalf("result is not JSON: %v", err)
	}
	if _, ok := body["tripPlan"]; !ok {
		t.Errorf("tripPlan missing from %v", body)
	}
	if _, ok := body["success"]; ok {
		t.Errorf("success should have been stripped")
	}
}

func TestNoteDelta(t *testing.T) {
	tests := []struct{ in, want string }{
		{"go at sunrise", `{"ops":[{"insert":"go at sunrise\n"}]}`},
		// Quill documents always end in a newline; don't double it.
		{"already ends\n", `{"ops":[{"insert":"already ends\n"}]}`},
		// An empty note matches what the client sends for "no note".
		{"", `{"ops":[{"insert":"\n"}]}`},
	}
	for _, tc := range tests {
		if got := string(NoteDelta(tc.in)); got != tc.want {
			t.Errorf("NoteDelta(%q) = %s, want %s", tc.in, got, tc.want)
		}
	}
}
