package game

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
)

// Save writes the game state to path atomically: the JSON goes to a
// temporary file in the same directory which is then renamed over the
// target, so a crash mid-write can never leave a half-written save.
func (g *Game) Save(path string) error {
	if g.InCombat() {
		return fmt.Errorf("%w: you cannot save with %s at your throat", ErrInCombat, g.Foe.Name)
	}
	data, err := json.MarshalIndent(g.State, "", "  ")
	if err != nil {
		return fmt.Errorf("encode save: %w", err)
	}
	dir := filepath.Dir(path)
	tmp, err := os.CreateTemp(dir, ".grimhold-save-*.tmp")
	if err != nil {
		return fmt.Errorf("create temp save: %w", err)
	}
	tmpName := tmp.Name()
	cleanup := func() { _ = os.Remove(tmpName) } // best effort; the real error is returned

	if _, err := tmp.Write(append(data, '\n')); err != nil {
		tmp.Close()
		cleanup()
		return fmt.Errorf("write temp save: %w", err)
	}
	if err := tmp.Sync(); err != nil {
		tmp.Close()
		cleanup()
		return fmt.Errorf("sync temp save: %w", err)
	}
	if err := tmp.Close(); err != nil {
		cleanup()
		return fmt.Errorf("close temp save: %w", err)
	}
	if err := os.Rename(tmpName, path); err != nil {
		cleanup()
		return fmt.Errorf("replace save: %w", err)
	}
	return nil
}

// Load replaces the current state with the one stored at path. On any
// error the running game is left untouched.
func (g *Game) Load(path string) error {
	data, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return fmt.Errorf("%w at %s", ErrNoSave, path)
	}
	if err != nil {
		return fmt.Errorf("read save: %w", err)
	}
	var s State
	if err := json.Unmarshal(data, &s); err != nil {
		return fmt.Errorf("%w: %w", ErrBadSave, err)
	}
	if err := g.validate(s); err != nil {
		return fmt.Errorf("%w: %w", ErrBadSave, err)
	}
	if s.Slain == nil {
		s.Slain = map[string]int{}
	}
	g.State = s
	g.Foe = nil
	return nil
}

func (g *Game) validate(s State) error {
	if s.Version != stateVersion {
		return fmt.Errorf("unsupported version %d", s.Version)
	}
	if _, ok := g.World.Region(s.Location); !ok {
		return fmt.Errorf("unknown location %q", s.Location)
	}
	c := s.Character
	if c.Level < 1 || c.MaxHP < 1 || c.HP < 1 || c.HP > c.MaxHP {
		return fmt.Errorf("character stats out of range (level %d, hp %d/%d)", c.Level, c.HP, c.MaxHP)
	}
	if _, err := ParseDice(c.Weapon.Damage); err != nil {
		return fmt.Errorf("weapon: %w", err)
	}
	if s.Gold < 0 {
		return fmt.Errorf("negative gold")
	}
	return nil
}
