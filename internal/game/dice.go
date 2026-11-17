package game

import (
	"fmt"
	"math/rand/v2"
	"strconv"
	"strings"
)

// Die is a group of identical dice, e.g. 2d6.
type Die struct {
	Count int
	Sides int
}

// Dice is a dice expression such as "2d6+1d4+3".
type Dice struct {
	Groups   []Die
	Modifier int
}

// ParseDice parses expressions like "1d8", "2d6+5", "1d6 + 1d4 - 1" or "3".
func ParseDice(s string) (Dice, error) {
	expr := strings.ReplaceAll(strings.ToLower(s), " ", "")
	if expr == "" {
		return Dice{}, fmt.Errorf("parse dice %q: empty expression", s)
	}
	var d Dice
	// Normalise "-" into "+-" so we can split on "+".
	expr = strings.ReplaceAll(expr, "-", "+-")
	for _, term := range strings.Split(expr, "+") {
		if term == "" {
			continue
		}
		sign := 1
		if strings.HasPrefix(term, "-") {
			sign = -1
			term = term[1:]
		}
		if count, sides, ok := strings.Cut(term, "d"); ok {
			n := 1
			if count != "" {
				var err error
				if n, err = strconv.Atoi(count); err != nil {
					return Dice{}, fmt.Errorf("parse dice %q: bad count %q: %w", s, count, err)
				}
			}
			m, err := strconv.Atoi(sides)
			if err != nil {
				return Dice{}, fmt.Errorf("parse dice %q: bad sides %q: %w", s, sides, err)
			}
			if n <= 0 || m <= 0 || sign < 0 {
				return Dice{}, fmt.Errorf("parse dice %q: invalid term %q", s, term)
			}
			d.Groups = append(d.Groups, Die{Count: n, Sides: m})
			continue
		}
		v, err := strconv.Atoi(term)
		if err != nil {
			return Dice{}, fmt.Errorf("parse dice %q: bad modifier %q: %w", s, term, err)
		}
		d.Modifier += sign * v
	}
	if len(d.Groups) == 0 && d.Modifier == 0 {
		return Dice{}, fmt.Errorf("parse dice %q: no dice", s)
	}
	return d, nil
}

// MustDice is ParseDice for literals known to be valid.
func MustDice(s string) Dice {
	d, err := ParseDice(s)
	if err != nil {
		panic(err)
	}
	return d
}

// Roll rolls the expression. The result is never below 1 if any dice
// were involved (a hit always hurts a little).
func (d Dice) Roll(r *rand.Rand) int {
	total := d.Modifier
	for _, g := range d.Groups {
		for i := 0; i < g.Count; i++ {
			total += r.IntN(g.Sides) + 1
		}
	}
	if len(d.Groups) > 0 && total < 1 {
		total = 1
	}
	return total
}

// HasModifier reports whether the expression has a flat bonus.
func (d Dice) HasModifier() bool { return d.Modifier != 0 }

// String renders the expression in standard notation.
func (d Dice) String() string {
	var b strings.Builder
	for i, g := range d.Groups {
		if i > 0 {
			b.WriteString("+")
		}
		fmt.Fprintf(&b, "%dd%d", g.Count, g.Sides)
	}
	switch {
	case d.Modifier > 0 && len(d.Groups) > 0:
		fmt.Fprintf(&b, "+%d", d.Modifier)
	case d.Modifier != 0 || len(d.Groups) == 0:
		fmt.Fprintf(&b, "%d", d.Modifier)
	}
	return b.String()
}

// D20 rolls a single twenty-sided die.
func D20(r *rand.Rand) int { return r.IntN(20) + 1 }
