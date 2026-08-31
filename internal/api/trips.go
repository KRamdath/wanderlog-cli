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

// Section is one day (or standing bucket such as "Places to visit") within a
// trip's itinerary. Section ids are numeric, and the heading arrives as
// displayHeading rather than a name/date pair.
type Section struct {
	ID               int64  `json:"id"`
	DisplayHeading   string `json:"displayHeading"`
	Type             string `json:"type"`
	PlaceMarkerColor string `json:"placeMarkerColor,omitempty"`
}

// Trip plan types, as used by the create endpoint.
const (
	TypePlan            = "plan"
	TypeJournal         = "journal"
	TypeRecommendations = "recommendations"
	TypeStory           = "story"
)

// Privacy levels. Wanderlog defaults a new plan to "friends".
const (
	PrivacyPrivate = "private"
	PrivacyFriends = "friends"
	PrivacyPublic  = "public"
)

// CreateTripInput mirrors the payload the web client sends. The server rejects
// partial bodies with a generic "unexpectedError", so every field is sent
// explicitly — including the nulls — rather than omitted.
type CreateTripInput struct {
	GeoIDs                       []int   `json:"geoIds"`
	InitialMapsPlaceIDs          []int   `json:"initialMapsPlaceIds"`
	InitialSections              any     `json:"initialSections"`
	InitialEmailID               any     `json:"initialEmailId"`
	Type                         string  `json:"type"`
	Privacy                      string  `json:"privacy"`
	IsMapEmbed                   bool    `json:"isMapEmbed"`
	Title                        *string `json:"title"`
	StartDate                    *string `json:"startDate"`
	EndDate                      *string `json:"endDate"`
	AutogenerateItineraryOptions any     `json:"autogenerateItineraryOptions"`
	Language                     string  `json:"language"`
}

// CreateTripResult is what the create endpoint returns. Note it carries id and
// viewKey too, and createdSectionIds is empty even when day sections were
// generated from the date range.
type CreateTripResult struct {
	ID                int64   `json:"id"`
	Key               string  `json:"key"`
	Title             string  `json:"title"`
	ViewKey           string  `json:"viewKey"`
	CreatedSectionIDs []int64 `json:"createdSectionIds"`
}

func (c *Client) ListTrips() ([]TripPlan, error) {
	var out []TripPlan
	err := c.JSON(Request{Path: "/api/tripPlans"}, &out)
	return out, err
}

// CreateTrip makes a new trip. At least one geo id is required — the server
// refuses a destination-less trip with errType "noGeosForTripPlan".
func (c *Client) CreateTrip(in CreateTripInput) (*CreateTripResult, error) {
	if in.Language == "" {
		in.Language = c.Language
	}
	if in.Type == "" {
		in.Type = TypePlan
	}
	if in.Privacy == "" {
		in.Privacy = PrivacyFriends
	}
	// Nil slices would marshal to null; the server expects arrays.
	if in.GeoIDs == nil {
		in.GeoIDs = []int{}
	}
	if in.InitialMapsPlaceIDs == nil {
		in.InitialMapsPlaceIDs = []int{}
	}
	out := &CreateTripResult{}
	err := c.JSON(Request{Method: http.MethodPost, Path: "/api/tripPlans", Body: in}, out)
	return out, err
}

// ClientSchemaVersion is the itinerary schema version the web client sends.
// Without it the server refuses the read with "incompatibleItineraryConversion"
// ("your app is too old"). Bump this if that error reappears.
const ClientSchemaVersion = "2"

// GetTrip returns the raw trip document. The itinerary schema is large and
// changes often, so it is passed through verbatim rather than modelled.
//
// Unlike most routes, this one puts its payload at the top level of the
// envelope (tripPlan, resources, guideResources, settings) instead of nesting
// it under "data", so the whole body is returned with "success" stripped.
func (c *Client) GetTrip(key string) (json.RawMessage, error) {
	resp, err := c.Do(Request{
		Path:  "/api/tripPlans/" + url.PathEscape(key),
		Query: url.Values{"clientSchemaVersion": {ClientSchemaVersion}},
	})
	if err != nil {
		return nil, err
	}

	var body map[string]json.RawMessage
	if err := json.Unmarshal(resp.Raw, &body); err != nil {
		return resp.Raw, nil
	}
	delete(body, "success")
	out, err := json.Marshal(body)
	if err != nil {
		return resp.Raw, nil
	}
	return out, nil
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

// AddPlaces appends places to a trip. An empty sectionId lets the server pick
// the destination, which in practice is the trip's "Places to visit" section
// rather than a specific day.
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
