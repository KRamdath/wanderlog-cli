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

	note := "try the roof"
	_, err := c.AddPlaces("KEY", "SEC", []PlaceWithNote{{
		Place: json.RawMessage(`{"place_id":"ChIJabc","name":"Eiffel Tower"}`),
		Text:  &note,
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
	if entry["text"] != "try the roof" {
		t.Errorf("text = %v", entry["text"])
	}
	if got["addDuplicates"] != false {
		t.Errorf("addDuplicates = %v", got["addDuplicates"])
	}
}

// Omitting the section id targets the trip's unscheduled list.
func TestPlacesPathWithoutSection(t *testing.T) {
	if got := placesPath("K", ""); got != "/api/tripPlans/K/sections/places" {
		t.Errorf("placesPath = %q", got)
	}
	if got := placesPath("K", "S"); got != "/api/tripPlans/K/sections/S/places" {
		t.Errorf("placesPath = %q", got)
	}
}
