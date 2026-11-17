package game

import (
	"math"

	"github.com/cactusninjaa/grimhold/internal/open5e"
	"github.com/cactusninjaa/grimhold/internal/world"
)

// Attack is one way a foe can hurt you.
type Attack struct {
	Name   string
	Bonus  int
	Damage Dice
}

// Foe is a monster engaged in combat.
type Foe struct {
	Slug           string
	Name           string
	HP             int
	MaxHP          int
	AC             int
	CR             float64
	DexMod         int
	Attacks        []Attack
	AttacksPerTurn int
	Boss           bool
}

// Alive reports whether the foe still stands.
func (f *Foe) Alive() bool { return f.HP > 0 }

// abilityMod converts a 5e ability score into its modifier.
func abilityMod(score int) int {
	return int(math.Floor(float64(score-10) / 2))
}

// NewFoe converts an Open5e monster into a combat-ready foe. Actions that
// cannot be resolved as a simple attack roll (Multiattack, breath weapons…)
// are skipped; if nothing usable remains, a generic strike scaled to the
// challenge rating is used instead.
func NewFoe(m open5e.Monster) Foe {
	f := Foe{
		Slug:           m.Slug,
		Name:           m.Name,
		HP:             max(1, m.HitPoints),
		MaxHP:          max(1, m.HitPoints),
		AC:             m.ArmorClass,
		CR:             m.CR,
		DexMod:         abilityMod(m.Dexterity),
		AttacksPerTurn: 1,
	}
	for _, a := range m.Actions {
		if a.AttackBonus == nil || a.DamageDice == "" {
			continue
		}
		d, err := ParseDice(a.DamageDice)
		if err != nil {
			continue // exotic notation: not an attack we can model
		}
		if a.DamageBonus != nil && !d.HasModifier() {
			d.Modifier = *a.DamageBonus
		}
		f.Attacks = append(f.Attacks, Attack{Name: a.Name, Bonus: *a.AttackBonus, Damage: d})
	}
	if len(f.Attacks) == 0 {
		bonus := 2 + int(math.Ceil(m.CR/2))
		f.Attacks = []Attack{{
			Name:   "Strike",
			Bonus:  bonus,
			Damage: Dice{Groups: []Die{{Count: 1, Sides: 6}}, Modifier: int(math.Ceil(m.CR))},
		}}
	}
	return f
}

// NewBoss builds the unique guardian of a region: renamed, tougher, and
// able to make several attacks per turn.
func NewBoss(m open5e.Monster, b world.Boss) Foe {
	f := NewFoe(m)
	f.Boss = true
	if b.Title != "" {
		f.Name = b.Title + " (" + m.Name + ")"
	}
	f.MaxHP += b.HPBonus
	f.HP = f.MaxHP
	f.AttacksPerTurn = 2
	return f
}
