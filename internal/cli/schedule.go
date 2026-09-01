package cli

import (
	"errors"
	"flag"
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"github.com/KRamdath/wanderlog-cli/internal/api"
)

var timePattern = regexp.MustCompile(`^([01]?\d|2[0-3]):[0-5]\d$`)

func validTime(s string) error {
	if s == "" || timePattern.MatchString(s) {
		return nil
	}
	return fmt.Errorf("time %q is not HH:MM", s)
}

// tripSetTime writes start/end times onto one itinerary block.
var tripSetTime = wrap(func(args []string) error {
	fs := flag.NewFlagSet("trip set-time", flag.ContinueOnError)
	section := fs.String("section", "", "section/day id (required)")
	place := fs.String("place", "", "Google place id of the block (required)")
	start := fs.String("start", "", "start time HH:MM; empty clears it")
	end := fs.String("end", "", "end time HH:MM; empty clears it")
	if err := parseFlags(fs, args); err != nil {
		return err
	}
	key, err := requireArg(fs, "key")
	if err != nil {
		return err
	}
	if *section == "" || *place == "" {
		return errors.New("--section and --place are both required")
	}
	if err := validTime(*start); err != nil {
		return err
	}
	if err := validTime(*end); err != nil {
		return err
	}

	c, err := authedClient()
	if err != nil {
		return err
	}
	blocks, err := c.Itinerary(key)
	if err != nil {
		return err
	}

	sid, err := strconv.ParseInt(*section, 10, 64)
	if err != nil {
		return fmt.Errorf("--section must be a numeric section id: %w", err)
	}

	for _, b := range blocks {
		if b.SectionID == sid && b.PlaceID == *place {
			ops := api.SetTimeOps(b, *start, *end)
			if len(ops) == 0 {
				return emit(map[string]any{"changed": false, "place": b.Name})
			}
			if err := c.ApplyOps(key, ops); err != nil {
				return err
			}
			return emit(map[string]any{
				"changed": true, "place": b.Name,
				"start": *start, "end": *end,
			})
		}
	}
	return fmt.Errorf("no block with place id %s in section %s", *place, *section)
})

// tripScheduleDay reorders a day so its blocks run chronologically.
var tripScheduleDay = wrap(func(args []string) error {
	fs := flag.NewFlagSet("trip schedule-day", flag.ContinueOnError)
	section := fs.String("section", "", "section/day id; omit to do every dated day")
	dry := fs.Bool("dry-run", false, "show the resulting order without writing")
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

	var only int64 = -1
	if *section != "" {
		if only, err = strconv.ParseInt(*section, 10, 64); err != nil {
			return fmt.Errorf("--section must be a numeric section id: %w", err)
		}
	}

	// Each write shifts indices, so sections are re-read between them.
	sectionIDs, err := targetSections(c, key, only)
	if err != nil {
		return err
	}

	results := []map[string]any{}
	for _, sid := range sectionIDs {
		blocks, err := c.Itinerary(key)
		if err != nil {
			return err
		}
		var inSection []api.Block
		sectionIndex := -1
		for _, b := range blocks {
			if b.SectionID == sid {
				inSection = append(inSection, b)
				sectionIndex = b.SectionIndex
			}
		}
		if len(inSection) < 2 {
			continue
		}

		ops := api.ScheduleOps(sectionIndex, inSection)
		order := plannedOrder(inSection)

		if !*dry && len(ops) > 0 {
			if err := c.ApplyOps(key, ops); err != nil {
				return fmt.Errorf("reordering section %d: %w", sid, err)
			}
		}
		results = append(results, map[string]any{
			"section": sid,
			"moves":   len(ops),
			"order":   order,
		})
	}

	return emit(map[string]any{"dryRun": *dry, "sections": results})
})

// plannedOrder renders the sequence ScheduleOps will produce, for reporting.
func plannedOrder(blocks []api.Block) []string {
	type group struct {
		start string
		names []string
	}
	var groups []group
	for _, b := range blocks {
		label := b.Name
		if label == "" {
			label = b.Type
		}
		if b.StartTime != "" || len(groups) == 0 {
			groups = append(groups, group{start: b.StartTime, names: []string{label}})
			continue
		}
		g := &groups[len(groups)-1]
		g.names = append(g.names, label)
	}

	timed, untimed := []group{}, []group{}
	for _, g := range groups {
		if g.start == "" {
			untimed = append(untimed, g)
		} else {
			timed = append(timed, g)
		}
	}
	for i := 0; i < len(timed); i++ {
		for j := i + 1; j < len(timed); j++ {
			if timed[j].start < timed[i].start {
				timed[i], timed[j] = timed[j], timed[i]
			}
		}
	}

	out := []string{}
	for _, g := range append(untimed, timed...) {
		stamp := g.start
		if stamp == "" {
			stamp = "--:--"
		}
		out = append(out, stamp+"  "+strings.Join(g.names, " + "))
	}
	return out
}

func targetSections(c *api.Client, key string, only int64) ([]int64, error) {
	secs, err := c.Sections(key)
	if err != nil {
		return nil, err
	}
	var ids []int64
	for _, s := range secs {
		if only >= 0 {
			if s.ID == only {
				ids = append(ids, s.ID)
			}
			continue
		}
		// Day sections are the ones headed with a weekday date.
		if s.Type == "normal" && isDayHeading(s.DisplayHeading) {
			ids = append(ids, s.ID)
		}
	}
	if only >= 0 && len(ids) == 0 {
		return nil, fmt.Errorf("no section with id %d", only)
	}
	return ids, nil
}

var weekdays = []string{"Monday", "Tuesday", "Wednesday", "Thursday", "Friday", "Saturday", "Sunday"}

func isDayHeading(h string) bool {
	for _, d := range weekdays {
		if strings.HasPrefix(h, d) {
			return true
		}
	}
	return false
}
