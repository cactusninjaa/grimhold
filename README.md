# Grimhold

A text-based dungeon crawler for the terminal. Explore cursed regions,
stumble into random encounters, fight foes in dice-based combat and record
your kills in a bestiary. Monster data comes live from the
[Open5e API](https://open5e.com) (D&D 5e SRD, no API key); the world map is
`world.json`.

Go 1.22+, standard library only.

## Build & run

```sh
go build -o grimhold ./cmd/grimhold
./grimhold                 # new game (run from the repo root so world.json is found)
./grimhold --continue      # resume from save.json (starts fresh if there is none)
```

| Flag | Default | Meaning |
|---|---|---|
| `--continue` | `false` | load `save.json` on start-up |
| `--world` | `world.json` | world file |
| `--save` | `save.json` | save file used by `save` / `load` / `--continue` |
| `--name` | `Wanderer` | character name |
| `--seed` | random | seed the dice (same seed + same commands = same game) |
| `--no-color` | `false` | disable ANSI colours (also honours `NO_COLOR`) |
| `--api` | `https://api.open5e.com` | Open5e base URL |

Tests (no network access needed):

```sh
go test -race ./...
gofmt -l . && go vet ./...
```

## Commands

```text
bestiary                 Browse SRD monsters (first page)
character                Show your character sheet
explore                  Wander the region (risk an encounter)
fight                    Trade blows with the current foe (one round)
flee                     Try to escape the current foe
help                     Show this message
inspect <monster>        Show a monster's stat block
load                     Load the last saved game
look                     Describe your surroundings
map                      List the known regions
next                     Next bestiary page
prev                     Previous bestiary page
quit                     Leave Grimhold (alias: exit)
rest                     Heal fully (once per region visit)
save                     Save the game to disk
slain                    Show your slain bestiary
travel <region>          Travel to a connected region
```

`help` is generated from the command registry (`map[string]cliCommand` in
`internal/repl/repl.go`). Input is case-insensitive; multi-word names work
(`inspect giant spider`, `travel ashen marsh`). Unknown commands, wrong
argument counts and empty lines get a friendly message. A `recover` around
each command means even a bug cannot crash the loop.

## Rules

- **Character**: starts at level 1 with 20 HP, AC 14, attack +4 and a
  longsword (1d8+2).
- **Explore**: some chance of a quiet walk (sometimes with a few gold
  coins). Otherwise you roll a foe from the region's `foes` list. A region
  with danger *n* rolls *n* times and keeps the one with the highest CR.
- **Fight**: each `fight` is one round. You roll `d20 + attack bonus` vs the
  foe's AC, and on a hit you roll weapon dice + damage bonus. Then the foe
  strikes back with one of its Open5e actions, using that action's
  `attack_bonus` and `damage_dice`. A natural 1 always misses. A natural 20
  always hits and doubles the dice.
- **Flee**: `d20 + DEX mod` vs `10 + foe's DEX mod`. If you fail, the foe
  gets a free hit and the fight goes on.
- **Victory**: you gain `100 × CR` XP (minimum 10), get an entry in the
  slain bestiary and have a 60% chance of finding gold.
- **Levels**: leaving level *L* takes `L² × 50` total XP (50, 200, 450,
  800…). Each level gives +8 max HP, +1 attack, +1 damage and a full heal.
  You also gain +1 AC every 4 levels.
- **Rest**: full heal, once per region visit. Travelling resets it.
- **Death**: you wake up at the start region with 1 HP and lose half your
  gold. Nothing is written to disk when you die, so the last save is
  untouched.
- **Save/load**: `save.json` holds the location, character, slain bestiary,
  gold and rest state. You cannot save mid-combat. Saves are written
  atomically (temp file + `rename`), so a crash can never leave a
  half-written save. A save that fails to load (corrupt file, unknown
  region…) leaves the running game unchanged.

## Architecture

```text
cmd/grimhold/main.go   flags + wiring (thin)
internal/repl          REPL loop, command registry, output formatting
internal/game          rules: dice, character, foes, combat, explore, save/load
internal/open5e        typed API client, pagination, concurrent prefetch
internal/cache         TTL cache: map + sync.Mutex + reaper goroutine
internal/world         world.json loading and validation
internal/ui            hand-rolled ANSI colours, HP bars, danger skulls
```

- **Open5e client**: decodes JSON into typed structs (`Monster`, `Action`,
  `MonsterPage`). The bestiary follows the API's own `next` / `previous`
  URLs. HTTP 404 maps to `open5e.ErrNotFound`.
- **Cache**: each raw response is cached by URL for 5 minutes. A reaper
  goroutine purges expired entries every minute. The clock can be injected
  for tests.
- **Errors**: wrapped with `%w` all the way up. The game exposes sentinel
  errors (`ErrInCombat`, `ErrNoExit`, `ErrNoSave`, …) that callers check
  with `errors.Is`.
- `game` depends on a small `MonsterSource` interface rather than the HTTP
  client, so tests can use an in-memory fake.

## World

`world.json` has 7 regions. It keeps the format from the brief and adds
three optional fields:
`arrival` (text shown when you travel there), `min_level` (level gate) and
`boss`.

```text
crossroads ── ashen-marsh ── drowned-chapel ── barrow-throne (boss, level 4+)
    │                              │
whispering-pines ────────── bone-quarry ── mire-of-teeth
```

## Bonus features

| Bonus | Where |
|---|---|
| **Test suite** (+6): 26 test functions, none of which touch the network. Combat uses a seeded `rand` and is checked for determinism, defeat/respawn, flee and XP. The cache is tested for expiry and the reaper using an injected clock. The Open5e client is tested for decoding, 404s, pagination, caching and prefetch against an `httptest.Server`. Also covered: dice parsing (table-driven), world validation, save/load round trip and corrupt saves, and REPL robustness with garbage input. | `*_test.go` |
| **Faster dungeon** (+4): on `travel` (and at start-up and `load`), every foe of the new region is fetched concurrently: one goroutine per slug, a `sync.WaitGroup`, and results gathered on a channel. Errors are combined with `errors.Join`. `explore` then reads from the cache. | `open5e.Client.PrefetchMonsters`, `repl.(*Session).prefetch` |
| **ANSI colours** (+2): hand-rolled escape codes for HP bars (green/yellow/red), danger levels (☠ coloured by severity), hits and misses. Turned off automatically when stdout is not a terminal or `NO_COLOR` is set. | `internal/ui` |
| **Boss region** (+3, past the +12 cap, for fun): *The Barrow Throne* requires level 4. *The Barrow King*, a souped-up wight, gets +25 HP, attacks twice per turn, is harder to flee and gives double XP plus a large pile of gold. | `world.json`, `game.NewBoss` |

## Sample session

```text
$ ./grimhold --no-color --seed 3 --name Mara
GRIMHOLD — the cursed marches await. Type help.
(scouted 3 kinds of foe in 92ms)
Gallows Crossroads  safe-ish
Four muddy roads meet beneath an empty gibbet. A lantern burns in the waystone, and no one remembers lighting it.
It feels almost safe here. Almost.
Exits: ashen-marsh, whispering-pines
grimhold > travel bone-quarry
✗ You cannot travel there: no path leads there from here: Bone Quarry is not adjacent to Gallows Crossroads
grimhold > travel whispering-pines
The pines close behind you. Something in the needles whispers your name, then laughs.
(scouted 3 kinds of foe in 32ms)
grimhold > explore
⚔ A Wolf lurches out of hiding!
  HP [██████████] 11/11  AC 13  CR 1/4
Type fight or flee.
grimhold [⚔ Wolf 11/11] > fight
  You: Longsword (d20 7+4 = 11 vs AC 13) misses.
  Wolf: Bite (d20 6+4 = 10 vs AC 14) misses.
  You            [████████████████████] 20/20
  Wolf           [████████████████████] 11/11
grimhold [⚔ Wolf 11/11] > fight
  You: Longsword (d20 12+4 = 16 vs AC 13) hits for 3 damage.
  Wolf: Bite (d20 19+4 = 23 vs AC 14) hits for 5 damage.
  You            [███████████████·····] 15/20
  Wolf           [██████████████······] 8/11
grimhold [⚔ Wolf 8/11] > save
✗ Save failed: you are in combat: you cannot save with Wolf at your throat
grimhold [⚔ Wolf 8/11] > fight
  You: Longsword (d20 15+4 = 19 vs AC 13) hits for 5 damage.
  Wolf: Bite (d20 9+4 = 13 vs AC 14) misses.
  You            [███████████████·····] 15/20
  Wolf           [█████···············] 3/11
grimhold [⚔ Wolf 3/11] > fight
  You: Longsword (d20 16+4 = 20 vs AC 13) hits for 4 damage.
✔ Wolf falls. (1st slain)
  +25 XP  +6 gold
grimhold > character
Mara, level 1
  HP      [███████████████·····] 15/20
  XP      25 / 50
  AC      14
  Attack  +4
  Weapon  Longsword (1d8+2)
  Gold    6
  Where   Whispering Pines (rest available)
grimhold > slain
Slain bestiary (1 kill)
  Wolf                                 ×1
grimhold > save
Your deeds are etched into save.json.
grimhold > quit
You turn your back on Grimhold. Unsaved deeds fade like breath on cold glass.

$ ./grimhold --continue
GRIMHOLD — the cursed marches await. Type help.
Welcome back, Mara.
...
```
