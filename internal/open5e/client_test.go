package open5e

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/cactusninjaa/grimhold/internal/cache"
)

const goblinJSON = `{
  "slug": "goblin", "name": "Goblin", "size": "Small", "type": "Humanoid",
  "subtype": "goblinoid", "alignment": "neutral evil",
  "armor_class": 15, "armor_desc": "leather armor, shield",
  "hit_points": 7, "hit_dice": "2d6", "speed": {"walk": 30},
  "strength": 8, "dexterity": 14, "constitution": 10,
  "intelligence": 10, "wisdom": 8, "charisma": 8,
  "challenge_rating": "1/4", "cr": 0.25,
  "actions": [
    {"name": "Scimitar", "desc": "Melee", "attack_bonus": 4, "damage_dice": "1d6", "damage_bonus": 2},
    {"name": "Taunt", "desc": "Rude gestures."}
  ],
  "special_abilities": [{"name": "Nimble Escape", "desc": "Disengage or Hide as a bonus action."}],
  "some_field_we_ignore": [1, 2, 3]
}`

// newTestServer serves a goblin, a two-page listing and 404 for the rest,
// counting requests so tests can assert on cache behaviour.
func newTestServer(t *testing.T) (*httptest.Server, *atomic.Int32) {
	t.Helper()
	var hits atomic.Int32
	mux := http.NewServeMux()
	var srv *httptest.Server
	mux.HandleFunc("/v1/monsters/goblin/", func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(goblinJSON))
	})
	mux.HandleFunc("/v1/monsters/", func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		if r.URL.Path != "/v1/monsters/" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Query().Get("page") == "2" {
			_, _ = w.Write([]byte(`{"count": 2, "next": null, "previous": "` + srv.URL + `/v1/monsters/?page=1",
				"results": [{"slug": "zombie", "name": "Zombie", "cr": 0.25, "hit_points": 22}]}`))
			return
		}
		_, _ = w.Write([]byte(`{"count": 2, "next": "` + srv.URL + `/v1/monsters/?page=2", "previous": null,
			"results": [` + goblinJSON + `]}`))
	})
	srv = httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv, &hits
}

func TestMonsterDecoding(t *testing.T) {
	srv, _ := newTestServer(t)
	c := NewClient(srv.URL, srv.Client(), nil)

	m, err := c.Monster(context.Background(), "Goblin")
	if err != nil {
		t.Fatalf("Monster: %v", err)
	}
	if m.Name != "Goblin" || m.HitPoints != 7 || m.ArmorClass != 15 || m.CR != 0.25 {
		t.Fatalf("unexpected monster: %+v", m)
	}
	if len(m.Actions) != 2 {
		t.Fatalf("got %d actions, want 2", len(m.Actions))
	}
	a := m.Actions[0]
	if a.AttackBonus == nil || *a.AttackBonus != 4 || a.DamageDice != "1d6" || a.DamageBonus == nil || *a.DamageBonus != 2 {
		t.Fatalf("scimitar decoded wrong: %+v", a)
	}
	if m.Actions[1].AttackBonus != nil {
		t.Fatal("Taunt should have no attack bonus")
	}
	if m.Speed["walk"] != float64(30) {
		t.Fatalf("speed = %v", m.Speed)
	}
}

func TestMonsterNotFound(t *testing.T) {
	srv, _ := newTestServer(t)
	c := NewClient(srv.URL, srv.Client(), nil)
	_, err := c.Monster(context.Background(), "tarrasque-junior")
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("err = %v, want ErrNotFound", err)
	}
}

func TestResponsesAreCached(t *testing.T) {
	srv, hits := newTestServer(t)
	ch := cache.New(time.Minute, 0)
	defer ch.Stop()
	c := NewClient(srv.URL, srv.Client(), ch)

	for i := 0; i < 3; i++ {
		if _, err := c.Monster(context.Background(), "goblin"); err != nil {
			t.Fatalf("Monster: %v", err)
		}
	}
	if got := hits.Load(); got != 1 {
		t.Fatalf("server hit %d times, want 1", got)
	}
}

func TestPagination(t *testing.T) {
	srv, _ := newTestServer(t)
	c := NewClient(srv.URL, srv.Client(), nil)
	ctx := context.Background()

	p1, err := c.MonsterPage(ctx, srv.URL+"/v1/monsters/")
	if err != nil {
		t.Fatalf("page 1: %v", err)
	}
	if p1.Previous != nil || p1.Next == nil || len(p1.Results) != 1 || p1.Results[0].Name != "Goblin" {
		t.Fatalf("bad page 1: %+v", p1)
	}
	p2, err := c.MonsterPage(ctx, *p1.Next)
	if err != nil {
		t.Fatalf("page 2: %v", err)
	}
	if p2.Next != nil || p2.Previous == nil || p2.Results[0].Name != "Zombie" {
		t.Fatalf("bad page 2: %+v", p2)
	}
}

func TestPrefetchMonsters(t *testing.T) {
	srv, _ := newTestServer(t)
	c := NewClient(srv.URL, srv.Client(), nil)

	got, err := c.PrefetchMonsters(context.Background(), []string{"goblin", "nope"})
	if err == nil || !errors.Is(err, ErrNotFound) {
		t.Fatalf("expected joined ErrNotFound, got %v", err)
	}
	if _, ok := got["goblin"]; !ok || len(got) != 1 {
		t.Fatalf("got %v, want only goblin", got)
	}
}

func TestSlugify(t *testing.T) {
	tests := map[string]string{
		"Giant Spider":          "giant-spider",
		"  young   RED dragon ": "young-red-dragon",
		"goblin":                "goblin",
		"":                      "",
	}
	for in, want := range tests {
		if got := Slugify(in); got != want {
			t.Errorf("Slugify(%q) = %q, want %q", in, got, want)
		}
	}
}
