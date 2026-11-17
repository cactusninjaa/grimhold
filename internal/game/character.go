package game

import "math"

// Weapon is what the character swings.
type Weapon struct {
	Name   string `json:"name"`
	Damage string `json:"damage"` // dice notation, e.g. "1d8"
}

// Character is the player.
type Character struct {
	Name        string `json:"name"`
	Level       int    `json:"level"`
	XP          int    `json:"xp"`
	HP          int    `json:"hp"`
	MaxHP       int    `json:"max_hp"`
	AC          int    `json:"ac"`
	AttackBonus int    `json:"attack_bonus"`
	DamageBonus int    `json:"damage_bonus"`
	DexMod      int    `json:"dex_mod"`
	Weapon      Weapon `json:"weapon"`
}

// Tunables for progression.
const (
	startMaxHP       = 20
	startAC          = 14
	startAttackBonus = 4
	startDamageBonus = 2
	startDexMod      = 2

	hpPerLevel = 8
)

// NewCharacter returns a fresh level-1 character.
func NewCharacter(name string) Character {
	if name == "" {
		name = "Wanderer"
	}
	return Character{
		Name:        name,
		Level:       1,
		HP:          startMaxHP,
		MaxHP:       startMaxHP,
		AC:          startAC,
		AttackBonus: startAttackBonus,
		DamageBonus: startDamageBonus,
		DexMod:      startDexMod,
		Weapon:      Weapon{Name: "Longsword", Damage: "1d8"},
	}
}

// XPForNextLevel is the cumulative XP needed to leave the given level:
// level² × 50 (50, 200, 450, 800, …).
func XPForNextLevel(level int) int { return level * level * 50 }

// XPForCR converts a challenge rating into experience: 100 XP per CR,
// with a floor of 10 for CR 0 vermin.
func XPForCR(cr float64) int {
	return max(10, int(math.Round(cr*100)))
}

// GainXP adds experience and applies any level-ups. It returns the number
// of levels gained.
func (c *Character) GainXP(xp int) int {
	c.XP += xp
	gained := 0
	for c.XP >= XPForNextLevel(c.Level) {
		c.Level++
		c.MaxHP += hpPerLevel
		c.AttackBonus++
		c.DamageBonus++
		if c.Level%4 == 0 {
			c.AC++
		}
		c.HP = c.MaxHP
		gained++
	}
	return gained
}

// Alive reports whether the character is still standing.
func (c *Character) Alive() bool { return c.HP > 0 }

// TakeDamage lowers HP, never below zero.
func (c *Character) TakeDamage(n int) {
	c.HP = max(0, c.HP-n)
}
