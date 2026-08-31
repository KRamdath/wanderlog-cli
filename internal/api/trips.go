package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
)

// TripPlan is a Wanderlog trip. Key is the short id used in trip URLs
// (wanderlog.com/plan/<key>) and in every trip-scoped endpoint.
type TripPlan struct {
	Key       string `json:"key"`
	Title     string `json:"title"`
	Type      string `json:"type,omitempty"`
	StartDate string `json:"startDate,omitempty"`
	EndDate   string `json:"endDate,omitempty"`
}

// Section is one day (or unscheduled bucket) within a trip's itinerary.
type Section struct {
	ID    string          `json:"id"`
	Name  string          `json:"name,omitempty"`
	Date  string          `json:"date,omitempty"`
	Type  string          `json:"type,omitempty"`
	Block json.RawMessage `json:"blocks,omitempty"`
}

type CreateTripInput struct {
	Title     string `json:"title"`
	StartDate string `json:"startDate,omitempty"`
	EndDate   string `json:"endDate,omitempty"`
	Type      string `json:"type,omitempty"`
	Language  string `json:"language,omitempty"`
}

func (c *Client) ListTrips() ([]TripPlan, error) {
	var out []TripPlan
	err := c.JSON(Request{Path: "/api/tripPlans"}, &out)
	return out, err
}

func (c *Client) CreateTrip(in CreateTripInput) (*TripPlan, error) {
	if in.Language == "" {
		in.Language = c.Language
	}
	if in.Type == "" {
		in.Type = "tripPlan"
	}
	out := &TripPlan{}
	err := c.JSON(Request{Method: http.MethodPost, Path: "/api/tripPlans", Body: in}, out)
	return out, err
}

// GetTrip returns the raw trip document. The itinerary schema is large and
// changes often, so it is passed through verbatim rather than modelled.
func (c *Client) GetTrip(key string) (json.RawMessage, error) {
	resp, err := c.Do(Request{Path: "/api/tripPlans/" + url.PathEscape(key)})
	if err != nil {
		return nil, err
	}
	return resp.Data, nil
}

func (c *Client) DeleteTrip(key string) error {
	_, err := c.Do(Request{Method: http.MethodDelete, Path: "/api/tripPlans/" + url.PathEscape(key)})
	return err
}

func (c *Client) Sections(key string) ([]Section, error) {
	var out []Section
	err := c.JSON(Request{Path: "/api/tripPlans/" + url.PathEscape(key) + "/sections"}, &out)
	return out, err
}

// PlaceWithNote is one entry in an add-places call. Place is the full Google
// Places object as returned by GetPlaceDetails — the server keys off its
// place_id, so a hand-built stub will not work.
type PlaceWithNote struct {
	Place json.RawMessage `json:"place"`
	Text  *string         `json:"text"`
}

type addPlacesBody struct {
	Places        []PlaceWithNote `json:"places"`
	AddDuplicates bool            `json:"addDuplicates"`
}

type AddPlacesResult struct {
	AddedPlaceIDs []json.RawMessage `json:"addedPlaceIds"`
}

// AddPlaces appends places to a trip. An empty sectionId adds them to the
// trip's unscheduled list rather than to a specific day.
func (c *Client) AddPlaces(key, sectionID string, places []PlaceWithNote, addDuplicates bool) (*AddPlacesResult, error) {
	out := &AddPlacesResult{}
	err := c.JSON(Request{
		Method: http.MethodPost,
		Path:   placesPath(key, sectionID),
		Body:   addPlacesBody{Places: places, AddDuplicates: addDuplicates},
	}, out)
	return out, err
}

func (c *Client) RemovePlaces(key, sectionID string, placeIDs []string) error {
	_, err := c.Do(Request{
		Method: http.MethodDelete,
		Path:   placesPath(key, sectionID),
		Body:   map[string][]string{"placeIds": placeIDs},
	})
	return err
}

func placesPath(key, sectionID string) string {
	p := "/api/tripPlans/" + url.PathEscape(key) + "/sections"
	if sectionID != "" {
		p += "/" + url.PathEscape(sectionID)
	}
	return p + "/places"
}

func (c *Client) Invite(key string, invitees []string, message string) (json.RawMessage, error) {
	resp, err := c.Do(Request{
		Method: http.MethodPost,
		Path:   "/api/tripPlans/" + url.PathEscape(key) + "/invite",
		Body:   map[string]any{"invitees": invitees, "message": message},
	})
	if err != nil {
		return nil, err
	}
	return resp.Data, nil
}

// TripURL is the human-facing link for a trip key.
func TripURL(key string) string {
	return fmt.Sprintf("https://wanderlog.com/plan/%s", key)
}
