// Package repl implements Grimhold's read-eval-print loop.
package repl

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"sort"
	"strings"
	"time"

	"github.com/cactusninjaa/grimhold/internal/game"
	"github.com/cactusninjaa/grimhold/internal/open5e"
	"github.com/cactusninjaa/grimhold/internal/ui"
)

// errQuit is returned by the quit command to stop the loop.
var errQuit = errors.New("quit")

// cliCommand is one entry of the command registry. help is generated from
// these entries.
type cliCommand struct {
	name        string
	usage       string // argument synopsis, e.g. "<region>"
	description string
	minArgs     int
	maxArgs     int // -1 means unlimited
	callback    func(s *Session, args []string) error
}

// Session holds everything the commands need.
type Session struct {
	game     *game.Game
	client   *open5e.Client
	out      io.Writer
	ui       ui.Palette
	savePath string
	timeout  time.Duration

	commands map[string]cliCommand

	// bestiary pagination state
	pageNext, pagePrev *string
	pageLoaded         bool
	pageNum            int
}

// Config wires a session together.
type Config struct {
	Game     *game.Game
	Client   *open5e.Client
	Out      io.Writer
	UI       ui.Palette
	SavePath string
}

// New creates a session.
func New(cfg Config) *Session {
	s := &Session{
		game:     cfg.Game,
		client:   cfg.Client,
		out:      cfg.Out,
		ui:       cfg.UI,
		savePath: cfg.SavePath,
		timeout:  20 * time.Second,
	}
	s.commands = commands()
	return s
}

// commands is the single source of truth for what the REPL understands.
func commands() map[string]cliCommand {
	list := []cliCommand{
		{name: "help", description: "Show this message", maxArgs: 0, callback: cmdHelp},
		{name: "map", description: "List the known regions", maxArgs: 0, callback: cmdMap},
		{name: "travel", usage: "<region>", description: "Travel to a connected region", minArgs: 1, maxArgs: -1, callback: cmdTravel},
		{name: "look", description: "Describe your surroundings", maxArgs: 0, callback: cmdLook},
		{name: "explore", description: "Wander the region (risk an encounter)", maxArgs: 0, callback: cmdExplore},
		{name: "fight", description: "Trade blows with the current foe (one round)", maxArgs: 0, callback: cmdFight},
		{name: "flee", description: "Try to escape the current foe", maxArgs: 0, callback: cmdFlee},
		{name: "rest", description: "Heal fully (once per region visit)", maxArgs: 0, callback: cmdRest},
		{name: "character", description: "Show your character sheet", maxArgs: 0, callback: cmdCharacter},
		{name: "slain", description: "Show your slain bestiary", maxArgs: 0, callback: cmdSlain},
		{name: "bestiary", description: "Browse SRD monsters (first page)", maxArgs: 0, callback: cmdBestiary},
		{name: "next", description: "Next bestiary page", maxArgs: 0, callback: cmdNext},
		{name: "prev", description: "Previous bestiary page", maxArgs: 0, callback: cmdPrev},
		{name: "inspect", usage: "<monster>", description: "Show a monster's stat block", minArgs: 1, maxArgs: -1, callback: cmdInspect},
		{name: "save", description: "Save the game to disk", maxArgs: 0, callback: cmdSave},
		{name: "load", description: "Load the last saved game", maxArgs: 0, callback: cmdLoad},
		{name: "quit", description: "Leave Grimhold", maxArgs: 0, callback: cmdQuit},
	}
	m := make(map[string]cliCommand, len(list))
	for _, c := range list {
		m[c.name] = c
	}
	return m
}

// Run reads commands from in until EOF or quit.
func (s *Session) Run(in io.Reader) error {
	scanner := bufio.NewScanner(in)
	for {
		fmt.Fprint(s.out, s.prompt())
		if !scanner.Scan() {
			fmt.Fprintln(s.out)
			if err := scanner.Err(); err != nil {
				return fmt.Errorf("read input: %w", err)
			}
			fmt.Fprintln(s.out, "The fog swallows you. Farewell.")
			return nil
		}
		if err := s.Exec(scanner.Text()); errors.Is(err, errQuit) {
			return nil
		}
	}
}

func (s *Session) prompt() string {
	if f := s.game.Foe; f != nil {
		return fmt.Sprintf("grimhold %s > ", s.ui.Red(fmt.Sprintf("[⚔ %s %d/%d]", f.Name, f.HP, f.MaxHP)))
	}
	return "grimhold > "
}

// Exec parses and runs one input line. Errors from commands are printed;
// only errQuit is returned so the caller can stop.
func (s *Session) Exec(line string) (err error) {
	words := strings.Fields(strings.ToLower(line))
	if len(words) == 0 {
		return nil
	}
	name, args := words[0], words[1:]
	if name == "exit" {
		name = "quit"
	}
	cmd, ok := s.commands[name]
	if !ok {
		fmt.Fprintf(s.out, "%s Unknown command %q. Type %s for a list.\n", s.ui.Red("✗"), name, s.ui.Bold("help"))
		return nil
	}
	if len(args) < cmd.minArgs || (cmd.maxArgs >= 0 && len(args) > cmd.maxArgs) {
		fmt.Fprintf(s.out, "%s Usage: %s\n", s.ui.Red("✗"), strings.TrimSpace(cmd.name+" "+cmd.usage))
		return nil
	}

	// A bug in one command must never take the whole game down.
	defer func() {
		if r := recover(); r != nil {
			fmt.Fprintf(s.out, "%s Something went terribly wrong: %v\n", s.ui.Red("✗"), r)
			err = nil
		}
	}()

	if err := cmd.callback(s, args); err != nil {
		if errors.Is(err, errQuit) {
			return err
		}
		fmt.Fprintf(s.out, "%s %s\n", s.ui.Red("✗"), capitalize(err.Error()))
	}
	return nil
}

func (s *Session) ctx() (context.Context, context.CancelFunc) {
	return context.WithTimeout(context.Background(), s.timeout)
}

func (s *Session) printf(format string, a ...any) {
	fmt.Fprintf(s.out, format, a...)
}

func capitalize(str string) string {
	if str == "" {
		return str
	}
	return strings.ToUpper(str[:1]) + str[1:]
}

func cmdHelp(s *Session, _ []string) error {
	names := make([]string, 0, len(s.commands))
	for n := range s.commands {
		names = append(names, n)
	}
	sort.Strings(names)
	s.printf("%s — a dungeon crawler\n\nCommands:\n", s.ui.Bold("Grimhold"))
	for _, n := range names {
		c := s.commands[n]
		s.printf("  %-24s %s\n", strings.TrimSpace(c.name+" "+c.usage), c.description)
	}
	return nil
}

func cmdQuit(s *Session, _ []string) error {
	s.printf("You turn your back on Grimhold. Unsaved deeds fade like breath on cold glass.\n")
	return errQuit
}
