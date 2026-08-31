package cli

import (
	"errors"
	"flag"
	"fmt"
	"strings"

	"github.com/KRamdath/wanderlog-cli/internal/api"
)

var tripList = wrap(func(args []string) error {
	c, err := authedClient()
	if err != nil {
		return err
	}
	trips, err := c.ListTrips()
	if err != nil {
		return err
	}
	out := make([]map[string]any, 0, len(trips))
	for _, t := range trips {
		out = append(out, map[string]any{
			"key":       t.Key,
			"title":     t.Title,
			"startDate": t.StartDate,
			"endDate":   t.EndDate,
			"url":       api.TripURL(t.Key),
		})
	}
	return emit(out)
})

var tripCreate = wrap(func(args []string) error {
	fs := flag.NewFlagSet("trip create", flag.ContinueOnError)
	geo := fs.String("geo", "", "destination name, e.g. \"kyoto\" (required unless --geo-id is given)")
	geoID := fs.Int("geo-id", 0, "destination geo id from `wlog geo search`")
	title := fs.String("title", "", "trip title; Wanderlog names it after the destination if omitted")
	start := fs.String("start", "", "start date, YYYY-MM-DD")
	end := fs.String("end", "", "end date, YYYY-MM-DD")
	typ := fs.String("type", api.TypePlan, "trip type: plan, journal, recommendations, story")
	privacy := fs.String("privacy", api.PrivacyFriends, "privacy: private, friends, public")
	if err := parseFlags(fs, args); err != nil {
		return err
	}

	c, err := authedClient()
	if err != nil {
		return err
	}

	// A trip must have a destination; the server rejects an empty geo list.
	ids := []int{}
	switch {
	case *geoID != 0:
		ids = append(ids, *geoID)
	case *geo != "":
		geos, gerr := c.SearchGeos(*geo)
		if gerr != nil {
			return gerr
		}
		if len(geos) == 0 {
			return fmt.Errorf("no destination matched %q", *geo)
		}
		ids = append(ids, geos[0].ID)
	default:
		return errors.New("a destination is required: pass --geo NAME or --geo-id N")
	}

	in := api.CreateTripInput{
		GeoIDs:  ids,
		Type:    *typ,
		Privacy: *privacy,
	}
	// These are distinct from empty strings: null tells the server to
	// autogenerate a title and to leave the trip undated.
	if *title != "" {
		in.Title = title
	}
	if *start != "" {
		in.StartDate = start
	}
	if *end != "" {
		in.EndDate = end
	}

	trip, err := c.CreateTrip(in)
	if err != nil {
		return err
	}
	return emit(map[string]any{
		"key":   trip.Key,
		"title": trip.Title,
		"url":   api.TripURL(trip.Key),
	})
})

var tripGet = wrap(func(args []string) error {
	fs := flag.NewFlagSet("trip get", flag.ContinueOnError)
	if err := parseFlags(fs, args); err != nil {
		return err
	}
	key, err := requireArg(fs, "key")
	if err != nil {
		return err
	}
	c, err := authedClient()
	if err != nil {
		return err
	}
	raw, err := c.GetTrip(key)
	if err != nil {
		return err
	}
	return emitRaw(raw)
})

var tripDelete = wrap(func(args []string) error {
	fs := flag.NewFlagSet("trip delete", flag.ContinueOnError)
	yes := fs.Bool("yes", false, "confirm deletion")
	if err := parseFlags(fs, args); err != nil {
		return err
	}
	key, err := requireArg(fs, "key")
	if err != nil {
		return err
	}
	// Deleting a trip is destructive and not obviously reversible, so it needs
	// an explicit confirmation rather than defaulting to proceed.
	if !*yes {
		return errors.New("refusing to delete without --yes")
	}
	c, err := authedClient()
	if err != nil {
		return err
	}
	if err := c.DeleteTrip(key); err != nil {
		return err
	}
	return emit(map[string]any{"deleted": true, "key": key})
})

var tripSections = wrap(func(args []string) error {
	fs := flag.NewFlagSet("trip sections", flag.ContinueOnError)
	if err := parseFlags(fs, args); err != nil {
		return err
	}
	key, err := requireArg(fs, "key")
	if err != nil {
		return err
	}
	c, err := authedClient()
	if err != nil {
		return err
	}
	sections, err := c.Sections(key)
	if err != nil {
		return err
	}
	out := make([]map[string]any, 0, len(sections))
	for _, s := range sections {
		out = append(out, map[string]any{
			"id":      s.ID,
			"heading": s.DisplayHeading,
			"type":    s.Type,
		})
	}
	return emit(out)
})

var tripAddPlace = wrap(func(args []string) error {
	fs := flag.NewFlagSet("trip add-place", flag.ContinueOnError)
	place := fs.String("place", "", "Google place id, or a place name to search for (required)")
	section := fs.String("section", "", "section/day id from `wlog trip sections`; omit to add to \"Places to visit\"")
	note := fs.String("note", "", "note to attach to the place")
	geo := fs.String("geo", "", "destination name used to disambiguate a place name")
	near := fs.String("near", "", "LAT,LNG used to disambiguate a place name")
	dup := fs.Bool("allow-duplicates", false, "add even if the place is already in the trip")
	if err := parseFlags(fs, args); err != nil {
		return err
	}
	key, err := requireArg(fs, "key")
	if err != nil {
		return err
	}
	if *place == "" {
		return errors.New("--place is required")
	}

	c, err := authedClient()
	if err != nil {
		return err
	}
	raw, err := resolvePlace(c, *place, *geo, *near)
	if err != nil {
		return err
	}

	entry := api.PlaceWithNote{Place: raw, Text: api.NoteDelta(*note)}

	if _, err := c.AddPlaces(key, *section, []api.PlaceWithNote{entry}, *dup); err != nil {
		return err
	}
	return emit(map[string]any{
		"added":   true,
		"trip":    key,
		"section": *section,
		"place":   api.PlaceName(raw),
		"url":     api.TripURL(key),
	})
})

var tripRemovePlace = wrap(func(args []string) error {
	fs := flag.NewFlagSet("trip remove-place", flag.ContinueOnError)
	placeIDs := fs.String("place-id", "", "comma-separated place ids to remove (required)")
	section := fs.String("section", "", "section/day id")
	if err := parseFlags(fs, args); err != nil {
		return err
	}
	key, err := requireArg(fs, "key")
	if err != nil {
		return err
	}
	if *placeIDs == "" {
		return errors.New("--place-id is required")
	}

	ids := make([]string, 0)
	for _, id := range strings.Split(*placeIDs, ",") {
		if id = strings.TrimSpace(id); id != "" {
			ids = append(ids, id)
		}
	}
	if len(ids) == 0 {
		return errors.New("--place-id did not contain any ids")
	}

	c, err := authedClient()
	if err != nil {
		return err
	}
	if err := c.RemovePlaces(key, *section, ids); err != nil {
		return err
	}
	return emit(map[string]any{"removed": true, "trip": key, "placeIds": ids})
})
