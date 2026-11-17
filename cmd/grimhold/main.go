// Command grimhold is a text-based dungeon crawler for the terminal.
package main

import (
	"errors"
	"flag"
	"fmt"
	"math/rand/v2"
	"os"
	"time"

	"github.com/cactusninjaa/grimhold/internal/cache"
	"github.com/cactusninjaa/grimhold/internal/game"
	"github.com/cactusninjaa/grimhold/internal/open5e"
	"github.com/cactusninjaa/grimhold/internal/repl"
	"github.com/cactusninjaa/grimhold/internal/ui"
	"github.com/cactusninjaa/grimhold/internal/world"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "grimhold:", err)
		os.Exit(1)
	}
}

func run() error {
	var (
		worldPath = flag.String("world", "world.json", "path to the world file")
		savePath  = flag.String("save", "save.json", "path to the save file")
		cont      = flag.Bool("continue", false, "resume from the save file")
		name      = flag.String("name", "Wanderer", "your character's name")
		seed      = flag.Uint64("seed", 0, "random seed (0 = random)")
		noColor   = flag.Bool("no-color", false, "disable ANSI colours")
		apiURL    = flag.String("api", open5e.DefaultBaseURL, "Open5e API base URL")
	)
	flag.Parse()

	w, err := world.Load(*worldPath)
	if err != nil {
		return fmt.Errorf("load world: %w", err)
	}

	c := cache.New(5*time.Minute, time.Minute)
	defer c.Stop()
	client := open5e.NewClient(*apiURL, nil, c)

	s := *seed
	if s == 0 {
		s = uint64(time.Now().UnixNano())
	}
	rng := rand.New(rand.NewPCG(s, s^0x9e3779b97f4a7c15))

	g := game.New(w, client, rng, *name)

	palette := ui.Auto()
	if *noColor {
		palette = ui.Palette{}
	}

	session := repl.New(repl.Config{
		Game:     g,
		Client:   client,
		Out:      os.Stdout,
		UI:       palette,
		SavePath: *savePath,
	})

	fmt.Println(palette.Bold("GRIMHOLD") + " — the cursed marches await. Type " + palette.Bold("help") + ".")
	if *cont {
		switch err := g.Load(*savePath); {
		case errors.Is(err, game.ErrNoSave):
			fmt.Println("No save found — starting a new adventure.")
		case err != nil:
			return fmt.Errorf("continue: %w", err)
		default:
			fmt.Printf("Welcome back, %s.\n", g.State.Character.Name)
		}
	}
	session.Prefetch()
	if err := session.Exec("look"); err != nil {
		return fmt.Errorf("look: %w", err)
	}

	return session.Run(os.Stdin)
}
