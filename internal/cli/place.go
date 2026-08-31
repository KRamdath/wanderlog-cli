package cli

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"strconv"
	"strings"

	"github.com/KRamdath/wanderlog-cli/internal/api"
)

var geoSearch = wrap(func(args []string) error {
	fs := flag.NewFlagSet("geo search", flag.ContinueOnError)
	limit := fs.Int("limit", 10, "maximum results")
	if err := parseFlags(fs, args); err != nil {
		return err
	}
	query := strings.Join(fs.Args(), " ")
	if query == "" {
		return errors.New("missing required argument <query>")
	}

	c, err := client()
	if err != nil {
		return err
	}
	geos, err := c.SearchGeos(query)
	if err != nil {
		return err
	}
	if len(geos) > *limit {
		geos = geos[:*limit]
	}

	out := make([]map[string]any, 0, len(geos))
	for _, g := range geos {
		out = append(out, map[string]any{
			"id":        g.ID,
			"label":     g.Label(),
			"latitude":  g.Latitude,
			"longitude": g.Longitude,
			"type":      g.Subcategory,
		})
	}
	return emit(out)
})

// parseNear accepts "lat,lng" as produced by `wlog geo search`.
func parseNear(s string) (lat, lng float64, err error) {
	parts := strings.Split(s, ",")
	if len(parts) != 2 {
		return 0, 0, fmt.Errorf("--near must be LAT,LNG (got %q)", s)
	}
	lat, err = strconv.ParseFloat(strings.TrimSpace(parts[0]), 64)
	if err != nil {
		return 0, 0, fmt.Errorf("invalid latitude in --near: %w", err)
	}
	lng, err = strconv.ParseFloat(strings.TrimSpace(parts[1]), 64)
	if err != nil {
		return 0, 0, fmt.Errorf("invalid longitude in --near: %w", err)
	}
	return lat, lng, nil
}

var placeSearch = wrap(func(args []string) error {
	fs := flag.NewFlagSet("place search", flag.ContinueOnError)
	near := fs.String("near", "", "bias results toward LAT,LNG")
	geo := fs.String("geo", "", "bias results toward a destination name (resolved via geo search)")
	radius := fs.Int("radius", 50000, "search radius in meters")
	limit := fs.Int("limit", 10, "maximum results")
	if err := parseFlags(fs, args); err != nil {
		return err
	}
	query := strings.Join(fs.Args(), " ")
	if query == "" {
		return errors.New("missing required argument <query>")
	}

	c, err := client()
	if err != nil {
		return err
	}

	var lat, lng float64
	switch {
	case *near != "":
		if lat, lng, err = parseNear(*near); err != nil {
			return err
		}
	case *geo != "":
		geos, gerr := c.SearchGeos(*geo)
		if gerr != nil {
			return gerr
		}
		if len(geos) == 0 {
			return fmt.Errorf("no destination matched %q", *geo)
		}
		lat, lng = geos[0].Latitude, geos[0].Longitude
	default:
		return errors.New("place search needs a location bias: pass --near LAT,LNG or --geo NAME")
	}

	preds, err := c.SearchPlaces(query, lat, lng, *radius)
	if err != nil {
		return err
	}
	if len(preds) > *limit {
		preds = preds[:*limit]
	}

	out := make([]map[string]any, 0, len(preds))
	for _, p := range preds {
		out = append(out, map[string]any{
			"placeId":     p.PlaceID,
			"name":        p.Structured.MainText,
			"address":     p.Structured.SecondaryText,
			"description": p.Description,
		})
	}
	return emit(out)
})

var placeGet = wrap(func(args []string) error {
	fs := flag.NewFlagSet("place get", flag.ContinueOnError)
	if err := parseFlags(fs, args); err != nil {
		return err
	}
	placeID, err := requireArg(fs, "googlePlaceId")
	if err != nil {
		return err
	}
	c, err := client()
	if err != nil {
		return err
	}
	raw, err := c.GetPlaceDetails(placeID)
	if err != nil {
		return err
	}
	return emitRaw(raw)
})

// resolvePlace turns either a Google place id or a free-text query into the
// full place object AddPlaces requires.
func resolvePlace(c *api.Client, spec, geoHint, near string) (json.RawMessage, error) {
	// Google place ids are opaque but consistently prefixed.
	if strings.HasPrefix(spec, "ChI") || strings.HasPrefix(spec, "Ei") || strings.HasPrefix(spec, "Gh") {
		return c.GetPlaceDetails(spec)
	}

	var lat, lng float64
	var err error
	switch {
	case near != "":
		if lat, lng, err = parseNear(near); err != nil {
			return nil, err
		}
	case geoHint != "":
		geos, gerr := c.SearchGeos(geoHint)
		if gerr != nil {
			return nil, gerr
		}
		if len(geos) == 0 {
			return nil, fmt.Errorf("no destination matched %q", geoHint)
		}
		lat, lng = geos[0].Latitude, geos[0].Longitude
	default:
		return nil, fmt.Errorf("%q is not a Google place id; add --geo NAME or --near LAT,LNG "+
			"so the name can be resolved", spec)
	}

	preds, err := c.SearchPlaces(spec, lat, lng, 50000)
	if err != nil {
		return nil, err
	}
	if len(preds) == 0 {
		return nil, fmt.Errorf("no place matched %q", spec)
	}
	return c.GetPlaceDetails(preds[0].PlaceID)
}
