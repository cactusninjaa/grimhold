// Package world loads and validates the region graph described in world.json.
package world

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"slices"
	"sort"
)

// Boss describes a unique foe guarding a region.
type Boss struct {
	Slug    string `json:"slug"`
	Title   string `json:"title"`
	HPBonus int    `json:"hp_bonus,omitempty"`
}

// Region is one node of the world graph.
type Region struct {
	ID          string   `json:"id"`
	Name        string   `json:"name"`
	Description string   `json:"description"`
	Arrival     string   `json:"arrival,omitempty"`
	Danger      int      `json:"danger"`
	Foes        []string `json:"foes"`
	Exits       []string `json:"exits"`
	MinLevel    int      `json:"min_level,omitempty"`
	Boss        *Boss    `json:"boss,omitempty"`
}

// HasExit reports whether id is reachable directly from r.
func (r *Region) HasExit(id string) bool {
	return slices.Contains(r.Exits, id)
}

// World is the whole map.
type World struct {
	Start   string    `json:"start"`
	Regions []*Region `json:"regions"`

	byID map[string]*Region
}

// Load reads and validates a world file.
func Load(path string) (*World, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read world file: %w", err)
	}
	w, err := Parse(data)
	if err != nil {
		return nil, fmt.Errorf("world file %s: %w", path, err)
	}
	return w, nil
}

// Parse decodes and validates world JSON.
func Parse(data []byte) (*World, error) {
	var w World
	if err := json.Unmarshal(data, &w); err != nil {
		return nil, fmt.Errorf("decode world: %w", err)
	}
	if err := w.index(); err != nil {
		return nil, err
	}
	return &w, nil
}

func (w *World) index() error {
	if len(w.Regions) == 0 {
		return errors.New("world has no regions")
	}
	w.byID = make(map[string]*Region, len(w.Regions))
	for _, r := range w.Regions {
		if r.ID == "" {
			return errors.New("region with empty id")
		}
		if _, dup := w.byID[r.ID]; dup {
			return fmt.Errorf("duplicate region id %q", r.ID)
		}
		if len(r.Foes) == 0 && r.Boss == nil {
			return fmt.Errorf("region %q has no foes", r.ID)
		}
		w.byID[r.ID] = r
	}
	if _, ok := w.byID[w.Start]; !ok {
		return fmt.Errorf("start region %q does not exist", w.Start)
	}
	for _, r := range w.Regions {
		for _, e := range r.Exits {
			if _, ok := w.byID[e]; !ok {
				return fmt.Errorf("region %q has exit to unknown region %q", r.ID, e)
			}
		}
	}
	return nil
}

// Region returns the region with the given id.
func (w *World) Region(id string) (*Region, bool) {
	r, ok := w.byID[id]
	return r, ok
}

// Sorted returns regions ordered by danger, then name.
func (w *World) Sorted() []*Region {
	out := make([]*Region, len(w.Regions))
	copy(out, w.Regions)
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Danger != out[j].Danger {
			return out[i].Danger < out[j].Danger
		}
		return out[i].Name < out[j].Name
	})
	return out
}
