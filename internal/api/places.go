package api

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/url"
)

// Prediction is one Google Places autocomplete suggestion.
type Prediction struct {
	PlaceID     string `json:"place_id"`
	Description string `json:"description"`
	Structured  struct {
		MainText      string `json:"main_text"`
		SecondaryText string `json:"secondary_text"`
	} `json:"structured_formatting"`
}

// Geo is a Wanderlog destination (city, region, country) — distinct from a
// Google place. Geos carry the bounds used to bias place searches.
type Geo struct {
	ID          int     `json:"id"`
	Name        string  `json:"name"`
	StateName   string  `json:"stateName"`
	CountryName string  `json:"countryName"`
	Latitude    float64 `json:"latitude"`
	Longitude   float64 `json:"longitude"`
	Subcategory string  `json:"subcategory"`
	MapsPlaceID int     `json:"mapsPlaceId"`
}

func (g Geo) Label() string {
	s := g.Name
	if g.StateName != "" {
		s += ", " + g.StateName
	}
	if g.CountryName != "" {
		s += ", " + g.CountryName
	}
	return s
}

// SearchGeos resolves a destination name. This route is unauthenticated.
func (c *Client) SearchGeos(query string) ([]Geo, error) {
	var out []Geo
	err := c.JSON(Request{Path: "/api/geo/autocomplete/" + url.PathEscape(query)}, &out)
	return out, err
}

// sessionToken mimics the Google Places session token the web client sends.
// The server rejects autocomplete requests that omit it.
func sessionToken() string {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "00000000000000000000000000000000"
	}
	b[6] = (b[6] & 0x0f) | 0x40
	b[8] = (b[8] & 0x3f) | 0x80
	h := hex.EncodeToString(b)
	return fmt.Sprintf("%s-%s-%s-%s-%s", h[0:8], h[8:12], h[12:16], h[16:20], h[20:32])
}

// SearchPlaces runs Google Places autocomplete through Wanderlog's proxy.
// lat/lng/radius bias results toward a destination; the server requires all of
// input, sessiontoken, location and radius to be present.
func (c *Client) SearchPlaces(input string, lat, lng float64, radiusMeters int) ([]Prediction, error) {
	if radiusMeters <= 0 {
		radiusMeters = 50000
	}
	req := map[string]any{
		"input":        input,
		"sessiontoken": sessionToken(),
		"location":     fmt.Sprintf("%f,%f", lat, lng),
		"radius":       radiusMeters,
		"language":     c.Language,
	}
	blob, err := json.Marshal(req)
	if err != nil {
		return nil, err
	}
	q := url.Values{"request": {string(blob)}}

	var out []Prediction
	err = c.JSON(Request{Path: "/api/placesAPI/autocomplete/v2", Query: q}, &out)
	return out, err
}

// GetPlaceDetails returns the full Google Places object for a place_id. This is
// exactly the payload AddPlaces expects to receive back under "place".
func (c *Client) GetPlaceDetails(placeID string) (json.RawMessage, error) {
	q := url.Values{"placeId": {placeID}, "language": {c.Language}}
	resp, err := c.Do(Request{Path: "/api/placesAPI/getPlaceDetails/v2", Query: q})
	if err != nil {
		return nil, err
	}
	return resp.Data, nil
}

// PlaceName pulls the display name out of a raw place object.
func PlaceName(raw json.RawMessage) string {
	var p struct {
		Name string `json:"name"`
	}
	if err := json.Unmarshal(raw, &p); err != nil {
		return ""
	}
	return p.Name
}
