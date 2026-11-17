package open5e

// Monster is the subset of an Open5e /v1/monsters/ entry that Grimhold uses.
type Monster struct {
	Slug            string         `json:"slug"`
	Name            string         `json:"name"`
	Size            string         `json:"size"`
	Type            string         `json:"type"`
	Subtype         string         `json:"subtype"`
	Alignment       string         `json:"alignment"`
	ArmorClass      int            `json:"armor_class"`
	ArmorDesc       string         `json:"armor_desc"`
	HitPoints       int            `json:"hit_points"`
	HitDice         string         `json:"hit_dice"`
	Speed           map[string]any `json:"speed"`
	Strength        int            `json:"strength"`
	Dexterity       int            `json:"dexterity"`
	Constitution    int            `json:"constitution"`
	Intelligence    int            `json:"intelligence"`
	Wisdom          int            `json:"wisdom"`
	Charisma        int            `json:"charisma"`
	Senses          string         `json:"senses"`
	Languages       string         `json:"languages"`
	ChallengeRating string         `json:"challenge_rating"`
	CR              float64        `json:"cr"`
	Actions         []Action       `json:"actions"`
	SpecialAbility  []Ability      `json:"special_abilities"`
	Document        string         `json:"document__slug"`
}

// Action is one entry of a monster's "actions" list. Attack-related fields
// are pointers because many actions (Multiattack, breath weapons…) omit them.
type Action struct {
	Name        string `json:"name"`
	Desc        string `json:"desc"`
	AttackBonus *int   `json:"attack_bonus"`
	DamageDice  string `json:"damage_dice"`
	DamageBonus *int   `json:"damage_bonus"`
}

// Ability is a passive trait such as "Nimble Escape".
type Ability struct {
	Name string `json:"name"`
	Desc string `json:"desc"`
}

// MonsterPage is one page of the paginated monster listing.
type MonsterPage struct {
	Count    int       `json:"count"`
	Next     *string   `json:"next"`
	Previous *string   `json:"previous"`
	Results  []Monster `json:"results"`
}
