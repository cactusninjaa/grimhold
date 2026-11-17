// Package game holds Grimhold's rules: the character, exploration, combat
// and persistence. It knows nothing about terminals; the REPL formats the
// structured results it returns.
package game

import (
	"context"
	"errors"
	"fmt"
	"math/rand/v2"

	"github.com/cactusninjaa/grimhold/internal/open5e"
	"github.com/cactusninjaa/grimhold/internal/world"
)

// Sentinel errors callers can test with errors.Is.
var (
	ErrInCombat    = errors.New("you are in combat")
	ErrNoCombat    = errors.New("you are not in combat")
	ErrNoExit      = errors.New("no path leads there from here")
	ErrUnknown     = errors.New("no such region")
	ErrTooWeak     = errors.New("you are not strong enough to enter")
	ErrRested      = errors.New("you have already rested here")
	ErrNoSave      = errors.New("no save file found")
	ErrBadSave     = errors.New("save file is invalid")
	ErrAlreadyHere = errors.New("you are already there")
)

// MonsterSource provides monster data. *open5e.Client satisfies it; tests
// substitute a map-backed fake.
type MonsterSource interface {
	Monster(ctx context.Context, slug string) (open5e.Monster, error)
}

// State is everything persisted in save.json.
type State struct {
	Version   int            `json:"version"`
	Location  string         `json:"location"`
	Character Character      `json:"character"`
	Slain     map[string]int `json:"slain"`
	Gold      int            `json:"gold"`
	Rested    bool           `json:"rested"`
}

const stateVersion = 1

// Game is a running session.
type Game struct {
	World *world.World
	State State
	Foe   *Foe // current encounter, never persisted

	rng *rand.Rand
	src MonsterSource
}

// New starts a fresh game at the world's start region.
func New(w *world.World, src MonsterSource, rng *rand.Rand, name string) *Game {
	return &Game{
		World: w,
		State: State{
			Version:   stateVersion,
			Location:  w.Start,
			Character: NewCharacter(name),
			Slain:     map[string]int{},
		},
		rng: rng,
		src: src,
	}
}

// Region returns the region the player is standing in.
func (g *Game) Region() *world.Region {
	r, ok := g.World.Region(g.State.Location)
	if !ok {
		// State is validated on load and Travel only accepts known ids,
		// so this is a programming error.
		panic(fmt.Sprintf("game: current location %q not in world", g.State.Location))
	}
	return r
}

// InCombat reports whether a foe is engaged.
func (g *Game) InCombat() bool { return g.Foe != nil }

// Travel moves the player along an exit of the current region.
func (g *Game) Travel(id string) (*world.Region, error) {
	if g.InCombat() {
		return nil, fmt.Errorf("%w: fight or flee first", ErrInCombat)
	}
	dest, ok := g.World.Region(id)
	if !ok {
		return nil, fmt.Errorf("%w: %q", ErrUnknown, id)
	}
	if dest.ID == g.State.Location {
		return nil, fmt.Errorf("%w: %s", ErrAlreadyHere, dest.Name)
	}
	if !g.Region().HasExit(id) {
		return nil, fmt.Errorf("%w: %s is not adjacent to %s", ErrNoExit, dest.Name, g.Region().Name)
	}
	if g.State.Character.Level < dest.MinLevel {
		return nil, fmt.Errorf("%w %s (requires level %d)", ErrTooWeak, dest.Name, dest.MinLevel)
	}
	g.State.Location = dest.ID
	g.State.Rested = false
	return dest, nil
}

// Rest fully heals the character, once per region visit.
func (g *Game) Rest() (healed int, err error) {
	if g.InCombat() {
		return 0, fmt.Errorf("%w: no one sleeps with a blade at their throat", ErrInCombat)
	}
	if g.State.Rested {
		return 0, fmt.Errorf("%w: travel elsewhere before resting again", ErrRested)
	}
	c := &g.State.Character
	healed = c.MaxHP - c.HP
	c.HP = c.MaxHP
	g.State.Rested = true
	return healed, nil
}

// ExploreResult describes what wandering turned up.
type ExploreResult struct {
	Foe       *Foe // nil when nothing attacked
	FoundGold int
	Rolls     []string // names of candidates rolled, for flavour
}

// Explore wanders the current region. It may start an encounter.
func (g *Game) Explore(ctx context.Context) (ExploreResult, error) {
	if g.InCombat() {
		return ExploreResult{}, fmt.Errorf("%w with %s", ErrInCombat, g.Foe.Name)
	}
	region := g.Region()

	if region.Boss != nil {
		m, err := g.src.Monster(ctx, region.Boss.Slug)
		if err != nil {
			return ExploreResult{}, fmt.Errorf("summon boss: %w", err)
		}
		foe := NewBoss(m, *region.Boss)
		g.Foe = &foe
		return ExploreResult{Foe: g.Foe}, nil
	}

	quietChance := max(10, 35-5*region.Danger)
	if g.rng.IntN(100) < quietChance {
		res := ExploreResult{}
		if g.rng.IntN(2) == 0 {
			res.FoundGold = g.rng.IntN(4) + 1
			g.State.Gold += res.FoundGold
		}
		return res, nil
	}

	// Dangerous regions roll several times and keep the worst foe.
	rolls := max(1, region.Danger)
	var worst *open5e.Monster
	var names []string
	for i := 0; i < rolls; i++ {
		slug := region.Foes[g.rng.IntN(len(region.Foes))]
		m, err := g.src.Monster(ctx, slug)
		if err != nil {
			return ExploreResult{}, fmt.Errorf("roll encounter: %w", err)
		}
		names = append(names, m.Name)
		if worst == nil || m.CR > worst.CR {
			worst = &m
		}
	}
	foe := NewFoe(*worst)
	g.Foe = &foe
	return ExploreResult{Foe: g.Foe, Rolls: names}, nil
}
