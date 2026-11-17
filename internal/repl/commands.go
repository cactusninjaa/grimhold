package repl

import (
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/cactusninjaa/grimhold/internal/game"
	"github.com/cactusninjaa/grimhold/internal/open5e"
	"github.com/cactusninjaa/grimhold/internal/world"
)

// ---- world ----------------------------------------------------------------

func cmdMap(s *Session, _ []string) error {
	cur := s.game.State.Location
	s.printf("%s\n", s.ui.Bold("Known regions"))
	for _, r := range s.game.World.Sorted() {
		marker := "  "
		if r.ID == cur {
			marker = s.ui.Cyan("➤ ")
		}
		gate := ""
		if r.MinLevel > 0 {
			gate = s.ui.Dim(fmt.Sprintf(" (level %d+)", r.MinLevel))
		}
		s.printf("%s%-16s %-22s %s%s\n", marker, r.ID, r.Name, s.ui.Danger(r.Danger), gate)
		s.printf("     exits: %s\n", strings.Join(r.Exits, ", "))
	}
	return nil
}

func dangerHint(level int) string {
	switch {
	case level <= 0:
		return "It feels almost safe here. Almost."
	case level == 1:
		return "Something small and hungry watches from the edges."
	case level == 2:
		return "The silence here has teeth."
	case level == 3:
		return "Every shadow seems to lean toward you."
	default:
		return "Death keeps a ledger here, and your name is pencilled in."
	}
}

func cmdLook(s *Session, _ []string) error {
	r := s.game.Region()
	s.printf("%s  %s\n", s.ui.Bold(r.Name), s.ui.Danger(r.Danger))
	s.printf("%s\n", r.Description)
	s.printf("%s\n", s.ui.Dim(dangerHint(r.Danger)))
	if r.Boss != nil {
		s.printf("%s\n", s.ui.Magenta("Something ancient is waiting for you here: "+r.Boss.Title+"."))
	}
	exits := make([]string, 0, len(r.Exits))
	for _, id := range r.Exits {
		label := id
		if dest, ok := s.game.World.Region(id); ok && dest.MinLevel > s.game.State.Character.Level {
			label += s.ui.Dim(fmt.Sprintf(" (sealed: level %d)", dest.MinLevel))
		}
		exits = append(exits, label)
	}
	s.printf("Exits: %s\n", strings.Join(exits, ", "))
	if f := s.game.Foe; f != nil {
		s.printf("%s %s blocks your way.\n", s.ui.Red("⚔"), f.Name)
	}
	return nil
}

func cmdTravel(s *Session, args []string) error {
	id := open5e.Slugify(strings.Join(args, " "))
	r, err := s.game.Travel(id)
	if err != nil {
		return fmt.Errorf("you cannot travel there: %w", err)
	}
	if r.Arrival != "" {
		s.printf("%s\n", r.Arrival)
	} else {
		s.printf("You make your way to %s.\n", r.Name)
	}
	s.prefetch(r)
	return nil
}

// prefetch warms the cache with every foe of the region concurrently, so
// that explore is instant. Failures are not fatal: explore will retry.
func (s *Session) prefetch(r *world.Region) {
	if s.client == nil {
		return
	}
	slugs := append([]string(nil), r.Foes...)
	if r.Boss != nil {
		slugs = append(slugs, r.Boss.Slug)
	}
	ctx, cancel := s.ctx()
	defer cancel()
	start := time.Now()
	got, err := s.client.PrefetchMonsters(ctx, slugs)
	if err != nil {
		s.printf("%s\n", s.ui.Dim("(the scouts did not all return: "+err.Error()+")"))
		return
	}
	s.printf("%s\n", s.ui.Dim(fmt.Sprintf("(scouted %d kinds of foe in %s)", len(got), time.Since(start).Round(time.Millisecond))))
}

// Prefetch warms the cache for the current region. Called once at start-up.
func (s *Session) Prefetch() { s.prefetch(s.game.Region()) }

// ---- encounters -------------------------------------------------------------

func cmdExplore(s *Session, _ []string) error {
	ctx, cancel := s.ctx()
	defer cancel()
	res, err := s.game.Explore(ctx)
	if err != nil {
		return fmt.Errorf("explore: %w", err)
	}
	if res.Foe == nil {
		s.printf("You wander for a while. Nothing but wind and old bones.\n")
		if res.FoundGold > 0 {
			s.printf("Among the bones you find %s.\n", s.ui.Yellow(fmt.Sprintf("%d gold", res.FoundGold)))
		}
		return nil
	}
	f := res.Foe
	if f.Boss {
		s.printf("%s\n", s.ui.Magenta("The air turns to frost. "+f.Name+" rises to meet you!"))
	} else {
		if len(res.Rolls) > 1 {
			s.printf("%s\n", s.ui.Dim("You hear "+strings.Join(res.Rolls, ", ")+"…"))
		}
		s.printf("%s A %s lurches out of hiding!\n", s.ui.Red("⚔"), s.ui.Bold(f.Name))
	}
	s.printf("  HP %s  AC %d  CR %s\n", s.ui.HPBar(f.HP, f.MaxHP, 10), f.AC, formatCR(f.CR))
	s.printf("Type %s or %s.\n", s.ui.Bold("fight"), s.ui.Bold("flee"))
	return nil
}

func (s *Session) printSwing(sw game.Swing, playerAttacking bool) {
	verb := "misses"
	if sw.Hit {
		verb = "hits"
	}
	roll := fmt.Sprintf("d20 %d%+d = %d vs AC %d", sw.Roll, sw.Bonus, sw.Total(), sw.AC)
	switch {
	case sw.Crit:
		roll = s.ui.Yellow("NATURAL 20!") + " " + roll
	case sw.Roll == 1:
		roll = s.ui.Dim("natural 1…") + " " + roll
	}
	line := fmt.Sprintf("%s (%s) %s", sw.Attack, roll, verb)
	if sw.Hit {
		dmg := fmt.Sprintf("%d damage", sw.Damage)
		if playerAttacking {
			dmg = s.ui.Green(dmg)
		} else {
			dmg = s.ui.Red(dmg)
		}
		line += " for " + dmg
	}
	who := "You"
	if !playerAttacking {
		who = sw.Attacker
	}
	s.printf("  %s: %s.\n", who, line)
}

func cmdFight(s *Session, _ []string) error {
	foe := s.game.Foe
	res, err := s.game.Fight()
	if err != nil {
		return err
	}
	s.printSwing(res.Player, true)
	for _, sw := range res.Foe {
		s.printSwing(sw, false)
	}
	switch {
	case res.Victory != nil:
		s.printVictory(res.Victory)
	case res.Defeat != nil:
		s.printDefeat(res.Defeat)
	default:
		c := s.game.State.Character
		s.printf("  %-14s %s\n", "You", s.ui.HPBar(c.HP, c.MaxHP, 20))
		s.printf("  %-14s %s\n", truncate(foe.Name, 14), s.ui.HPBar(foe.HP, foe.MaxHP, 20))
	}
	return nil
}

func cmdFlee(s *Session, _ []string) error {
	foeName := ""
	if s.game.Foe != nil {
		foeName = s.game.Foe.Name
	}
	res, err := s.game.Flee()
	if err != nil {
		return err
	}
	s.printf("  You run (d20 %d%+d = %d vs DC %d)", res.Roll, res.Bonus, res.Roll+res.Bonus, res.DC)
	if res.Escaped {
		s.printf(" and lose %s in the fog.\n", foeName)
		return nil
	}
	s.printf(" but stumble!\n")
	if res.FreeHit != nil {
		s.printSwing(*res.FreeHit, false)
	}
	if res.Defeat != nil {
		s.printDefeat(res.Defeat)
		return nil
	}
	c := s.game.State.Character
	s.printf("  %-14s %s\n", "You", s.ui.HPBar(c.HP, c.MaxHP, 20))
	return nil
}

func (s *Session) printVictory(v *game.Victory) {
	s.printf("%s %s falls. (%s)\n", s.ui.Green("✔"), v.Foe, ordinal(v.Kills)+" slain")
	s.printf("  +%s", s.ui.Cyan(fmt.Sprintf("%d XP", v.XP)))
	if v.Gold > 0 {
		s.printf("  +%s", s.ui.Yellow(fmt.Sprintf("%d gold", v.Gold)))
	}
	s.printf("\n")
	if v.LevelsGained > 0 {
		c := s.game.State.Character
		s.printf("%s You are now level %d! Max HP %d, attack %+d.\n",
			s.ui.Yellow("★"), c.Level, c.MaxHP, c.AttackBonus)
	}
}

func (s *Session) printDefeat(d *game.Defeat) {
	start, _ := s.game.World.Region(d.WokeUpIn)
	s.printf("%s Darkness takes you.\n", s.ui.Red("☠"))
	name := d.WokeUpIn
	if start != nil {
		name = start.Name
	}
	s.printf("You wake up at %s with 1 HP and your pride wounded.", name)
	if d.GoldLost > 0 {
		s.printf(" Someone helped themselves to %d gold.", d.GoldLost)
	}
	s.printf("\n")
}

func cmdRest(s *Session, _ []string) error {
	healed, err := s.game.Rest()
	if err != nil {
		return err
	}
	c := s.game.State.Character
	s.printf("You make a cold camp and sleep. (+%d HP)\n", healed)
	s.printf("  HP %s\n", s.ui.HPBar(c.HP, c.MaxHP, 20))
	return nil
}

// ---- character ----------------------------------------------------------------

func cmdCharacter(s *Session, _ []string) error {
	st := s.game.State
	c := st.Character
	s.printf("%s, level %d\n", s.ui.Bold(c.Name), c.Level)
	s.printf("  HP      %s\n", s.ui.HPBar(c.HP, c.MaxHP, 20))
	s.printf("  XP      %d / %d\n", c.XP, game.XPForNextLevel(c.Level))
	s.printf("  AC      %d\n", c.AC)
	s.printf("  Attack  %+d\n", c.AttackBonus)
	s.printf("  Weapon  %s (%s%+d)\n", c.Weapon.Name, c.Weapon.Damage, c.DamageBonus)
	s.printf("  Gold    %s\n", s.ui.Yellow(fmt.Sprint(st.Gold)))
	r := s.game.Region()
	rest := "available"
	if st.Rested {
		rest = "used for this visit"
	}
	s.printf("  Where   %s (rest %s)\n", r.Name, rest)
	return nil
}

func cmdSlain(s *Session, _ []string) error {
	slain := s.game.State.Slain
	if len(slain) == 0 {
		s.printf("Your slain bestiary is empty. The monsters are not.\n")
		return nil
	}
	names := make([]string, 0, len(slain))
	total := 0
	for n, k := range slain {
		names = append(names, n)
		total += k
	}
	sort.Slice(names, func(i, j int) bool {
		if slain[names[i]] != slain[names[j]] {
			return slain[names[i]] > slain[names[j]]
		}
		return names[i] < names[j]
	})
	plural := "s"
	if total == 1 {
		plural = ""
	}
	s.printf("%s (%d kill%s)\n", s.ui.Bold("Slain bestiary"), total, plural)
	for _, n := range names {
		s.printf("  %-36s ×%d\n", n, slain[n])
	}
	return nil
}

// ---- bestiary ----------------------------------------------------------------

func (s *Session) showPage(url string, num int) error {
	ctx, cancel := s.ctx()
	defer cancel()
	page, err := s.client.MonsterPage(ctx, url)
	if err != nil {
		return fmt.Errorf("bestiary: %w", err)
	}
	s.pageNext, s.pagePrev, s.pageLoaded, s.pageNum = page.Next, page.Previous, true, num
	s.printf("%s — page %d (%d monsters)\n", s.ui.Bold("Bestiary"), num, page.Count)
	for _, m := range page.Results {
		s.printf("  %-26s CR %-4s %-12s HP %-4d AC %d\n", m.Name, m.ChallengeRating, m.Type, m.HitPoints, m.ArmorClass)
	}
	var nav []string
	if page.Previous != nil {
		nav = append(nav, "prev")
	}
	if page.Next != nil {
		nav = append(nav, "next")
	}
	if len(nav) > 0 {
		s.printf("%s\n", s.ui.Dim("("+strings.Join(nav, " / ")+", or inspect <monster>)"))
	}
	return nil
}

func cmdBestiary(s *Session, _ []string) error {
	return s.showPage(s.client.FirstMonsterPageURL(), 1)
}

func cmdNext(s *Session, _ []string) error {
	if !s.pageLoaded {
		return cmdBestiary(s, nil)
	}
	if s.pageNext == nil {
		return errors.New("this is the last page of the bestiary")
	}
	return s.showPage(*s.pageNext, s.pageNum+1)
}

func cmdPrev(s *Session, _ []string) error {
	if !s.pageLoaded {
		return errors.New("open the bestiary first")
	}
	if s.pagePrev == nil {
		return errors.New("this is the first page of the bestiary")
	}
	return s.showPage(*s.pagePrev, s.pageNum-1)
}

func cmdInspect(s *Session, args []string) error {
	ctx, cancel := s.ctx()
	defer cancel()
	name := strings.Join(args, " ")
	m, err := s.client.Monster(ctx, name)
	if errors.Is(err, open5e.ErrNotFound) {
		return fmt.Errorf("no monster called %q in the archives", name)
	}
	if err != nil {
		return fmt.Errorf("inspect: %w", err)
	}
	s.printStatBlock(m)
	return nil
}

func (s *Session) printStatBlock(m open5e.Monster) {
	rule := s.ui.Red(strings.Repeat("─", 48))
	s.printf("%s\n%s\n", rule, s.ui.Bold(strings.ToUpper(m.Name)))
	kind := m.Type
	if m.Subtype != "" {
		kind += " (" + m.Subtype + ")"
	}
	s.printf("%s\n", s.ui.Dim(fmt.Sprintf("%s %s, %s", m.Size, strings.ToLower(kind), m.Alignment)))
	s.printf("%s\n", rule)
	ac := fmt.Sprint(m.ArmorClass)
	if m.ArmorDesc != "" {
		ac += " (" + m.ArmorDesc + ")"
	}
	s.printf("Armor Class  %s\n", ac)
	s.printf("Hit Points   %d (%s)\n", m.HitPoints, m.HitDice)
	s.printf("Speed        %s\n", formatSpeed(m.Speed))
	s.printf("%s\n", rule)
	s.printf(" STR     DEX     CON     INT     WIS     CHA\n")
	scores := []int{m.Strength, m.Dexterity, m.Constitution, m.Intelligence, m.Wisdom, m.Charisma}
	for _, sc := range scores {
		s.printf("%-8s", fmt.Sprintf("%d(%+d)", sc, (sc/2)-5))
	}
	s.printf("\n%s\n", rule)
	if m.Senses != "" {
		s.printf("Senses     %s\n", m.Senses)
	}
	if m.Languages != "" {
		s.printf("Languages  %s\n", m.Languages)
	}
	s.printf("Challenge  %s (%d XP in Grimhold)\n", m.ChallengeRating, game.XPForCR(m.CR))
	for _, a := range m.SpecialAbility {
		s.printf("\n%s %s\n", s.ui.Bold(a.Name+"."), a.Desc)
	}
	if len(m.Actions) > 0 {
		s.printf("\n%s\n", s.ui.Red("ACTIONS"))
		for _, a := range m.Actions {
			s.printf("%s %s\n", s.ui.Bold(a.Name+"."), a.Desc)
		}
	}
	s.printf("%s\n", rule)
}

func formatSpeed(sp map[string]any) string {
	if len(sp) == 0 {
		return "—"
	}
	keys := make([]string, 0, len(sp))
	for k := range sp {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	var parts []string
	for _, k := range keys {
		switch v := sp[k].(type) {
		case float64:
			if v > 0 {
				parts = append(parts, fmt.Sprintf("%s %g ft.", k, v))
			}
		case bool:
			if v {
				parts = append(parts, "("+k+")")
			}
		}
	}
	return strings.Join(parts, ", ")
}

// ---- persistence --------------------------------------------------------------

func cmdSave(s *Session, _ []string) error {
	if err := s.game.Save(s.savePath); err != nil {
		return fmt.Errorf("save failed: %w", err)
	}
	s.printf("Your deeds are etched into %s.\n", s.savePath)
	return nil
}

func cmdLoad(s *Session, _ []string) error {
	if err := s.game.Load(s.savePath); err != nil {
		if errors.Is(err, game.ErrNoSave) {
			return errors.New("there is no save to load yet — use save first")
		}
		return fmt.Errorf("load failed: %w", err)
	}
	c := s.game.State.Character
	s.printf("Memories return. %s, level %d, stands in %s.\n", c.Name, c.Level, s.game.Region().Name)
	s.prefetch(s.game.Region())
	return nil
}

// ---- helpers ----------------------------------------------------------------

func formatCR(cr float64) string {
	switch cr {
	case 0.125:
		return "1/8"
	case 0.25:
		return "1/4"
	case 0.5:
		return "1/2"
	}
	return fmt.Sprintf("%g", cr)
}

func ordinal(n int) string {
	suffix := "th"
	if n%100 < 11 || n%100 > 13 {
		switch n % 10 {
		case 1:
			suffix = "st"
		case 2:
			suffix = "nd"
		case 3:
			suffix = "rd"
		}
	}
	return fmt.Sprintf("%d%s", n, suffix)
}

func truncate(str string, n int) string {
	r := []rune(str)
	if len(r) <= n {
		return str
	}
	return string(r[:n-1]) + "…"
}
