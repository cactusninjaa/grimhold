package game

import (
	"context"
	"errors"
	"math/rand/v2"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/cactusninjaa/grimhold/internal/open5e"
	"github.com/cactusninjaa/grimhold/internal/world"
)

const testWorld = `{
  "start": "camp",
  "regions": [
    {"id": "camp", "name": "Camp", "description": "Safe.", "danger": 0,
     "foes": ["rat"], "exits": ["woods"]},
    {"id": "woods", "name": "Woods", "description": "Dark.", "danger": 3,
     "foes": ["rat", "ogre"], "exits": ["camp", "lair"]},
    {"id": "lair", "name": "Lair", "description": "Doom.", "danger": 4, "min_level": 3,
     "foes": [], "boss": {"slug": "ogre", "title": "Big Ogre", "hp_bonus": 10},
     "exits": ["woods"]}
  ]
}`

func intp(i int) *int { return &i }

// fakeSource serves monsters from memory: no network in tests.
type fakeSource map[string]open5e.Monster

func (f fakeSource) Monster(_ context.Context, slug string) (open5e.Monster, error) {
	m, ok := f[slug]
	if !ok {
		return open5e.Monster{}, open5e.ErrNotFound
	}
	return m, nil
}

var monsters = fakeSource{
	"rat": {Slug: "rat", Name: "Rat", HitPoints: 3, ArmorClass: 10, CR: 0, Dexterity: 14,
		Actions: []open5e.Action{{Name: "Bite", AttackBonus: intp(2), DamageDice: "1d2"}}},
	"ogre": {Slug: "ogre", Name: "Ogre", HitPoints: 59, ArmorClass: 11, CR: 2, Dexterity: 8,
		Actions: []open5e.Action{{Name: "Greatclub", AttackBonus: intp(6), DamageDice: "2d8", DamageBonus: intp(4)}}},
}

func newTestGame(t *testing.T, seed uint64) *Game {
	t.Helper()
	w, err := world.Parse([]byte(testWorld))
	if err != nil {
		t.Fatalf("parse world: %v", err)
	}
	return New(w, monsters, rand.New(rand.NewPCG(seed, seed)), "Tester")
}

func TestParseDice(t *testing.T) {
	tests := []struct {
		in      string
		want    Dice
		wantErr bool
	}{
		{in: "1d8", want: Dice{Groups: []Die{{1, 8}}}},
		{in: "2d6+5", want: Dice{Groups: []Die{{2, 6}}, Modifier: 5}},
		{in: "1d6 + 1d4 - 1", want: Dice{Groups: []Die{{1, 6}, {1, 4}}, Modifier: -1}},
		{in: "d20", want: Dice{Groups: []Die{{1, 20}}}},
		{in: "3", want: Dice{Modifier: 3}},
		{in: "", wantErr: true},
		{in: "2d", wantErr: true},
		{in: "0d6", wantErr: true},
		{in: "abc", wantErr: true},
	}
	for _, tt := range tests {
		got, err := ParseDice(tt.in)
		if (err != nil) != tt.wantErr {
			t.Errorf("ParseDice(%q) err = %v, wantErr %v", tt.in, err, tt.wantErr)
			continue
		}
		if !tt.wantErr && !reflect.DeepEqual(got, tt.want) {
			t.Errorf("ParseDice(%q) = %+v, want %+v", tt.in, got, tt.want)
		}
	}
}

func TestDiceRollBounds(t *testing.T) {
	r := rand.New(rand.NewPCG(1, 2))
	d := MustDice("2d6+3")
	for i := 0; i < 1000; i++ {
		if v := d.Roll(r); v < 5 || v > 15 {
			t.Fatalf("2d6+3 rolled %d", v)
		}
	}
}

func TestNewFoeUsesDamageBonus(t *testing.T) {
	f := NewFoe(monsters["ogre"])
	if len(f.Attacks) != 1 || f.Attacks[0].Damage.String() != "2d8+4" || f.DexMod != -1 {
		t.Fatalf("unexpected foe: %+v", f)
	}
	// A monster with no usable attack gets a generic strike.
	f = NewFoe(open5e.Monster{Name: "Blob", HitPoints: 5, CR: 1})
	if len(f.Attacks) != 1 || f.Attacks[0].Name != "Strike" {
		t.Fatalf("expected fallback strike, got %+v", f.Attacks)
	}
}

func TestLevelUp(t *testing.T) {
	c := NewCharacter("x")
	if got := c.GainXP(49); got != 0 {
		t.Fatalf("levelled up early")
	}
	if got := c.GainXP(1 + 150); got != 2 { // 200 XP total → level 3
		t.Fatalf("levels gained = %d, want 2", got)
	}
	if c.Level != 3 || c.MaxHP != startMaxHP+2*hpPerLevel || c.AttackBonus != startAttackBonus+2 || c.HP != c.MaxHP {
		t.Fatalf("bad character after level up: %+v", c)
	}
}

// A seeded game must be fully deterministic: same seed, same fight.
func TestCombatIsDeterministic(t *testing.T) {
	play := func() []RoundResult {
		g := newTestGame(t, 42)
		foe := NewFoe(monsters["ogre"])
		g.Foe = &foe
		var log []RoundResult
		for g.InCombat() {
			r, err := g.Fight()
			if err != nil {
				t.Fatalf("Fight: %v", err)
			}
			log = append(log, r)
		}
		return log
	}
	a, b := play(), play()
	if !reflect.DeepEqual(a, b) {
		t.Fatal("same seed produced different fights")
	}
	if len(a) == 0 {
		t.Fatal("no rounds fought")
	}
}

func TestVictoryGrantsXPAndBestiary(t *testing.T) {
	for seed := uint64(0); seed < 20; seed++ {
		g := newTestGame(t, seed)
		foe := NewFoe(monsters["rat"])
		foe.Attacks[0].Bonus = -100 // the rat can never hit
		g.Foe = &foe
		for g.InCombat() {
			res, err := g.Fight()
			if err != nil {
				t.Fatal(err)
			}
			if res.Defeat != nil {
				t.Fatal("lost to a harmless rat")
			}
			if res.Victory != nil {
				if res.Victory.XP != XPForCR(0) || res.Victory.Kills != 1 {
					t.Fatalf("bad victory: %+v", res.Victory)
				}
			}
		}
		if g.State.Slain["Rat"] != 1 || g.State.Character.XP != 10 {
			t.Fatalf("seed %d: slain=%v xp=%d", seed, g.State.Slain, g.State.Character.XP)
		}
	}
}

func TestDefeatSendsPlayerHomeWithoutTouchingSave(t *testing.T) {
	g := newTestGame(t, 7)
	path := filepath.Join(t.TempDir(), "save.json")
	if err := g.Save(path); err != nil {
		t.Fatalf("Save: %v", err)
	}
	before, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}

	if _, err := g.Travel("woods"); err != nil {
		t.Fatal(err)
	}
	g.State.Gold = 10
	g.State.Character.HP = 1
	foe := NewFoe(monsters["ogre"])
	foe.AC = 100 // player never hits (except on a natural 20)
	foe.HP = 1000
	foe.MaxHP = 1000
	foe.Attacks[0].Bonus = 100 // ogre always hits (except on a natural 1)
	g.Foe = &foe

	var defeat *Defeat
	for i := 0; i < 50 && defeat == nil; i++ {
		res, err := g.Fight()
		if err != nil {
			t.Fatal(err)
		}
		defeat = res.Defeat
	}
	if defeat == nil {
		t.Fatal("player never fell")
	}
	if g.State.Location != "camp" || g.State.Character.HP != 1 || g.InCombat() || g.State.Gold != 5 {
		t.Fatalf("bad respawn state: %+v foe=%v", g.State, g.Foe)
	}
	after, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(before) != string(after) {
		t.Fatal("dying modified the save file")
	}
}

func TestFleeFailureGivesFreeHit(t *testing.T) {
	sawEscape, sawFreeHit := false, false
	for seed := uint64(0); seed < 50; seed++ {
		g := newTestGame(t, seed)
		foe := NewFoe(monsters["ogre"])
		g.Foe = &foe
		res, err := g.Flee()
		if err != nil {
			t.Fatal(err)
		}
		if res.Escaped {
			sawEscape = true
			if g.InCombat() || res.FreeHit != nil {
				t.Fatal("escaped but still in combat or got hit")
			}
		} else {
			sawFreeHit = true
			if res.FreeHit == nil {
				t.Fatal("failed flee without a free hit")
			}
		}
	}
	if !sawEscape || !sawFreeHit {
		t.Fatalf("escape=%v freeHit=%v: expected both outcomes across seeds", sawEscape, sawFreeHit)
	}
}

func TestTravelRules(t *testing.T) {
	g := newTestGame(t, 1)
	if _, err := g.Travel("lair"); !errors.Is(err, ErrNoExit) {
		t.Fatalf("travel to non-adjacent: %v", err)
	}
	if _, err := g.Travel("narnia"); !errors.Is(err, ErrUnknown) {
		t.Fatalf("travel to unknown: %v", err)
	}
	if _, err := g.Travel("woods"); err != nil {
		t.Fatal(err)
	}
	if _, err := g.Travel("lair"); !errors.Is(err, ErrTooWeak) {
		t.Fatalf("level gate not enforced: %v", err)
	}
	g.State.Character.Level = 3
	if _, err := g.Travel("lair"); err != nil {
		t.Fatal(err)
	}
	res, err := g.Explore(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if res.Foe == nil || !res.Foe.Boss || res.Foe.MaxHP != 69 || res.Foe.AttacksPerTurn != 2 {
		t.Fatalf("expected boss, got %+v", res.Foe)
	}
	if _, err := g.Travel("woods"); !errors.Is(err, ErrInCombat) {
		t.Fatalf("travel during combat: %v", err)
	}
}

func TestRestOncePerVisit(t *testing.T) {
	g := newTestGame(t, 1)
	g.State.Character.HP = 3
	if _, err := g.Rest(); err != nil {
		t.Fatal(err)
	}
	if g.State.Character.HP != g.State.Character.MaxHP {
		t.Fatal("rest did not heal")
	}
	if _, err := g.Rest(); !errors.Is(err, ErrRested) {
		t.Fatalf("second rest: %v", err)
	}
	if _, err := g.Travel("woods"); err != nil {
		t.Fatal(err)
	}
	if _, err := g.Rest(); err != nil {
		t.Fatalf("rest after travel: %v", err)
	}
}

func TestSaveLoadRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "save.json")
	g := newTestGame(t, 3)

	if err := g.Load(path); !errors.Is(err, ErrNoSave) {
		t.Fatalf("load without save: %v", err)
	}

	if _, err := g.Travel("woods"); err != nil {
		t.Fatal(err)
	}
	g.State.Gold = 17
	g.State.Slain["Rat"] = 4
	g.State.Character.GainXP(60)
	g.State.Rested = true
	want := g.State
	if err := g.Save(path); err != nil {
		t.Fatal(err)
	}

	g2 := newTestGame(t, 9)
	if err := g2.Load(path); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(g2.State, want) {
		t.Fatalf("round trip mismatch:\n got %+v\nwant %+v", g2.State, want)
	}

	foe := NewFoe(monsters["rat"])
	g2.Foe = &foe
	if err := g2.Save(path); !errors.Is(err, ErrInCombat) {
		t.Fatalf("save during combat: %v", err)
	}

	if err := os.WriteFile(path, []byte(`{"version":1,"location":"atlantis"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := g2.Load(path); !errors.Is(err, ErrBadSave) {
		t.Fatalf("corrupt save: %v", err)
	}
	if !reflect.DeepEqual(g2.State, want) {
		t.Fatal("failed load modified state")
	}
}
