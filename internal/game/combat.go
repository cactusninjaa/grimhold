package game

import (
	"fmt"
	"math"
)

// Swing is a single attack roll and its outcome.
type Swing struct {
	Attacker string
	Attack   string
	Roll     int // the natural d20
	Bonus    int
	AC       int
	Hit      bool
	Crit     bool
	Damage   int
}

// Total is the d20 plus bonus.
func (s Swing) Total() int { return s.Roll + s.Bonus }

// Victory is the spoils of a won fight.
type Victory struct {
	Foe          string
	XP           int
	Gold         int
	LevelsGained int
	Kills        int // total kills of this foe type, including this one
}

// Defeat describes what happened when the character fell.
type Defeat struct {
	GoldLost int
	WokeUpIn string
}

// RoundResult is one exchange of blows.
type RoundResult struct {
	Player  Swing
	Foe     []Swing
	Victory *Victory
	Defeat  *Defeat
}

// FleeResult is the outcome of trying to run.
type FleeResult struct {
	Roll    int
	Bonus   int
	DC      int
	Escaped bool
	FreeHit *Swing
	Defeat  *Defeat
}

// swing resolves one attack: natural 1 always misses, natural 20 always
// hits and doubles the dice.
func (g *Game) swing(attacker, attack string, bonus, ac int, dmg Dice) Swing {
	s := Swing{Attacker: attacker, Attack: attack, Roll: D20(g.rng), Bonus: bonus, AC: ac}
	switch {
	case s.Roll == 1:
		s.Hit = false
	case s.Roll == 20:
		s.Hit, s.Crit = true, true
	default:
		s.Hit = s.Total() >= ac
	}
	if s.Hit {
		s.Damage = dmg.Roll(g.rng)
		if s.Crit {
			s.Damage += Dice{Groups: dmg.Groups}.Roll(g.rng)
		}
	}
	return s
}

func (g *Game) playerDamage() Dice {
	c := g.State.Character
	d, err := ParseDice(c.Weapon.Damage)
	if err != nil {
		d = Dice{Groups: []Die{{Count: 1, Sides: 4}}}
	}
	d.Modifier += c.DamageBonus
	return d
}

// foeTurn lets the foe attack AttacksPerTurn times (stopping early if the
// player drops).
func (g *Game) foeTurn() []Swing {
	c := &g.State.Character
	var swings []Swing
	for i := 0; i < g.Foe.AttacksPerTurn && c.Alive(); i++ {
		a := g.Foe.Attacks[g.rng.IntN(len(g.Foe.Attacks))]
		s := g.swing(g.Foe.Name, a.Name, a.Bonus, c.AC, a.Damage)
		c.TakeDamage(s.Damage)
		swings = append(swings, s)
	}
	return swings
}

// Fight resolves one round: the player strikes, then the foe answers if
// it still stands.
func (g *Game) Fight() (RoundResult, error) {
	if !g.InCombat() {
		return RoundResult{}, fmt.Errorf("%w: try exploring", ErrNoCombat)
	}
	c := &g.State.Character
	var res RoundResult

	res.Player = g.swing(c.Name, c.Weapon.Name, c.AttackBonus, g.Foe.AC, g.playerDamage())
	g.Foe.HP = max(0, g.Foe.HP-res.Player.Damage)
	if !g.Foe.Alive() {
		res.Victory = g.win()
		return res, nil
	}

	res.Foe = g.foeTurn()
	if !c.Alive() {
		res.Defeat = g.lose()
	}
	return res, nil
}

// Flee attempts to escape: d20 + DEX modifier against 10 + the foe's DEX
// modifier. Failure grants the foe a free attack.
func (g *Game) Flee() (FleeResult, error) {
	if !g.InCombat() {
		return FleeResult{}, fmt.Errorf("%w: there is nothing to run from", ErrNoCombat)
	}
	c := &g.State.Character
	res := FleeResult{Roll: D20(g.rng), Bonus: c.DexMod, DC: 10 + g.Foe.DexMod}
	if g.Foe.Boss {
		res.DC += 3
	}
	if res.Roll+res.Bonus >= res.DC {
		res.Escaped = true
		g.Foe = nil
		return res, nil
	}
	a := g.Foe.Attacks[g.rng.IntN(len(g.Foe.Attacks))]
	s := g.swing(g.Foe.Name, a.Name, a.Bonus, c.AC, a.Damage)
	c.TakeDamage(s.Damage)
	res.FreeHit = &s
	if !c.Alive() {
		res.Defeat = g.lose()
	}
	return res, nil
}

func (g *Game) win() *Victory {
	foe := g.Foe
	g.Foe = nil

	v := &Victory{Foe: foe.Name, XP: XPForCR(foe.CR)}
	if foe.Boss {
		v.XP *= 2
		v.Gold = 50 + g.rng.IntN(51)
	} else if g.rng.IntN(100) < 60 {
		v.Gold = (g.rng.IntN(6) + 1) * max(1, int(math.Ceil(foe.CR)))
	}
	g.State.Gold += v.Gold
	if g.State.Slain == nil {
		g.State.Slain = map[string]int{}
	}
	g.State.Slain[foe.Name]++
	v.Kills = g.State.Slain[foe.Name]
	v.LevelsGained = g.State.Character.GainXP(v.XP)
	return v
}

// lose sends the character back to the start with 1 HP and half their
// gold. Nothing is written to disk: the last save stays intact.
func (g *Game) lose() *Defeat {
	g.Foe = nil
	d := &Defeat{GoldLost: g.State.Gold / 2, WokeUpIn: g.World.Start}
	g.State.Gold -= d.GoldLost
	g.State.Location = g.World.Start
	g.State.Character.HP = 1
	g.State.Rested = false
	return d
}
