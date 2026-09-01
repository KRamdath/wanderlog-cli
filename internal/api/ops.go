package api

import (
	"encoding/json"
	"net/http"
	"net/url"
	"sort"
)

// Op is a ShareDB json0 operation. Wanderlog's itinerary is an OT document and
// applyOps is the only channel that can edit block fields or reorder a day.
//
// The subset used here:
//
//	{p: path, oi: value}            set an object key
//	{p: path, od: old, oi: new}     replace an object key
//	{p: path, lm: index}            move a list element
//
// Ops in one batch apply sequentially, so a move shifts the indices seen by
// every op after it.
type Op struct {
	P  []any `json:"p"`
	OI any   `json:"oi,omitempty"`
	OD any   `json:"od,omitempty"`
	LM *int  `json:"lm,omitempty"`
}

func (c *Client) ApplyOps(key string, ops []Op) error {
	if len(ops) == 0 {
		return nil
	}
	_, err := c.Do(Request{
		Method: http.MethodPost,
		Path:   "/api/tripPlans/" + url.PathEscape(key) + "/applyOps",
		Body:   map[string]any{"ops": ops},
	})
	return err
}

// Block is one itinerary entry, with the indices needed to address it in an op
// path. Times are "HH:MM" or empty.
type Block struct {
	SectionIndex int
	SectionID    int64
	BlockIndex   int
	BlockID      int64
	Type         string
	Name         string
	PlaceID      string
	StartTime    string
	EndTime      string
}

// Itinerary reads a trip and flattens its blocks, preserving the array indices
// that op paths are expressed in.
func (c *Client) Itinerary(key string) ([]Block, error) {
	raw, err := c.GetTrip(key)
	if err != nil {
		return nil, err
	}

	var doc struct {
		TripPlan struct {
			Itinerary struct {
				Sections []struct {
					ID     int64 `json:"id"`
					Blocks []struct {
						ID        int64  `json:"id"`
						Type      string `json:"type"`
						StartTime string `json:"startTime"`
						EndTime   string `json:"endTime"`
						Place     struct {
							Name    string `json:"name"`
							PlaceID string `json:"place_id"`
						} `json:"place"`
					} `json:"blocks"`
				} `json:"sections"`
			} `json:"itinerary"`
		} `json:"tripPlan"`
	}
	if err := json.Unmarshal(raw, &doc); err != nil {
		return nil, err
	}

	var out []Block
	for si, s := range doc.TripPlan.Itinerary.Sections {
		for bi, b := range s.Blocks {
			out = append(out, Block{
				SectionIndex: si,
				SectionID:    s.ID,
				BlockIndex:   bi,
				BlockID:      b.ID,
				Type:         b.Type,
				Name:         b.Place.Name,
				PlaceID:      b.Place.PlaceID,
				StartTime:    b.StartTime,
				EndTime:      b.EndTime,
			})
		}
	}
	return out, nil
}

// SetTimeOps builds the ops to write a block's start and end time. An empty
// value clears the field.
func SetTimeOps(b Block, start, end string) []Op {
	var ops []Op
	add := func(field, old, val string) {
		p := []any{"itinerary", "sections", b.SectionIndex, "blocks", b.BlockIndex, field}
		op := Op{P: p}
		if old != "" {
			op.OD = old
		}
		if val != "" {
			op.OI = val
		}
		// A no-op write would be rejected; skip when nothing changes.
		if old == val {
			return
		}
		ops = append(ops, op)
	}
	add("startTime", b.StartTime, start)
	add("endTime", b.EndTime, end)
	return ops
}

// ScheduleOps orders one section chronologically.
//
// Untimed blocks are anchored to the timed block above them rather than being
// swept to the end, so a group like "Astronomical Clock" followed by an untimed
// "Old Town Square" travels together. Only whole groups are sorted, by the
// leading block's start time; groups without a time keep their relative order
// and stay in front.
//
// The returned ops are list moves that apply sequentially, so they are computed
// against a simulated array rather than the original indices.
func ScheduleOps(sectionIndex int, blocks []Block) []Op {
	type group struct {
		start string
		ids   []int64
	}
	var groups []group
	for _, b := range blocks {
		if b.StartTime != "" || len(groups) == 0 {
			groups = append(groups, group{start: b.StartTime, ids: []int64{b.BlockID}})
			continue
		}
		g := &groups[len(groups)-1]
		g.ids = append(g.ids, b.BlockID)
	}

	// Stable sort keeps equal times, and untimed groups, in their existing order.
	idx := make([]int, len(groups))
	for i := range idx {
		idx[i] = i
	}
	sort.SliceStable(idx, func(a, b int) bool {
		ga, gb := groups[idx[a]], groups[idx[b]]
		if (ga.start == "") != (gb.start == "") {
			return ga.start == "" // untimed groups sort before timed ones
		}
		return ga.start < gb.start
	})

	var want []int64
	for _, i := range idx {
		want = append(want, groups[i].ids...)
	}

	cur := make([]int64, 0, len(blocks))
	for _, b := range blocks {
		cur = append(cur, b.BlockID)
	}

	var ops []Op
	for target, id := range want {
		from := -1
		for i := target; i < len(cur); i++ {
			if cur[i] == id {
				from = i
				break
			}
		}
		if from < 0 || from == target {
			continue
		}
		to := target
		ops = append(ops, Op{
			P:  []any{"itinerary", "sections", sectionIndex, "blocks", from},
			LM: &to,
		})
		moved := cur[from]
		cur = append(cur[:from], cur[from+1:]...)
		rest := append([]int64{moved}, cur[target:]...)
		cur = append(cur[:target], rest...)
	}
	return ops
}
