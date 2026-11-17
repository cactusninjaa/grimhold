package repl

import (
	"bytes"
	"math/rand/v2"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/cactusninjaa/grimhold/internal/game"
	"github.com/cactusninjaa/grimhold/internal/open5e"
	"github.com/cactusninjaa/grimhold/internal/ui"
	"github.com/cactusninjaa/grimhold/internal/world"
)

const testWorld = `{
  "start": "camp",
  "regions": [
    {"id": "camp", "name": "Camp", "description": "Safe.", "danger": 0, "foes": ["rat"], "exits": ["woods"]},
    {"id": "woods", "name": "Woods", "description": "Dark.", "danger": 1, "foes": ["rat"], "exits": ["camp"]}
  ]
}`

const ratJSON = `{"slug":"rat","name":"Rat","hit_points":3,"armor_class":10,"cr":0,"challenge_rating":"0",
  "dexterity":11,"speed":{"walk":20},"actions":[{"name":"Bite","attack_bonus":1,"damage_dice":"1d1"}]}`

func newTestSession(t *testing.T) (*Session, *bytes.Buffer) {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v1/monsters/rat/":
			_, _ = w.Write([]byte(ratJSON))
		case "/v1/monsters/":
			_, _ = w.Write([]byte(`{"count":1,"next":null,"previous":null,"results":[` + ratJSON + `]}`))
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(srv.Close)

	w, err := world.Parse([]byte(testWorld))
	if err != nil {
		t.Fatal(err)
	}
	client := open5e.NewClient(srv.URL, srv.Client(), nil)
	g := game.New(w, client, rand.New(rand.NewPCG(1, 1)), "Tester")
	out := &bytes.Buffer{}
	s := New(Config{
		Game: g, Client: client, Out: out, UI: ui.Palette{},
		SavePath: filepath.Join(t.TempDir(), "save.json"),
	})
	return s, out
}

func TestHelpListsEveryCommand(t *testing.T) {
	s, out := newTestSession(t)
	if err := s.Exec("help"); err != nil {
		t.Fatal(err)
	}
	for name, c := range s.commands {
		if !strings.Contains(out.String(), name) || !strings.Contains(out.String(), c.description) {
			t.Errorf("help is missing %q", name)
		}
	}
}

// Garbage input must never crash the REPL or stop the loop.
func TestRobustToBadInput(t *testing.T) {
	s, out := newTestSession(t)
	inputs := []string{
		"", "   ", "\t", "xyzzy", "HELP", "travel", "travel nowhere",
		"travel camp", "look look", "fight", "flee", "next", "prev",
		"inspect", "inspect not a monster", "save now", "load",
		"rest", "rest", "💀", strings.Repeat("a", 10000),
	}
	for _, in := range inputs {
		if err := s.Exec(in); err != nil {
			t.Fatalf("Exec(%q) returned %v", in, err)
		}
	}
	if !strings.Contains(out.String(), "Unknown command") || !strings.Contains(out.String(), "Usage: travel <region>") {
		t.Fatalf("expected friendly errors, got:\n%s", out.String())
	}
}

func TestFullLoopPlaysAndQuits(t *testing.T) {
	s, out := newTestSession(t)
	script := strings.Join([]string{
		"travel woods", "explore", "explore", "explore", "explore",
		"fight", "fight", "fight", "fight", "fight", "fight",
		"character", "slain", "bestiary", "inspect rat", "save", "load", "quit",
		"this line must never run",
	}, "\n")
	if err := s.Run(strings.NewReader(script)); err != nil {
		t.Fatal(err)
	}
	got := out.String()
	for _, want := range []string{"Woods", "Tester, level", "Bestiary", "RAT", "etched", "Memories return"} {
		if !strings.Contains(got, want) {
			t.Errorf("output missing %q", want)
		}
	}
	if strings.Contains(got, "Unknown command \"this\"") {
		t.Fatal("loop kept running after quit")
	}
}
