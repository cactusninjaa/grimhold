// Package open5e is a small, cached client for the Open5e REST API
// (https://api.open5e.com), which serves D&D 5e SRD content.
package open5e

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/cactusninjaa/grimhold/internal/cache"
)

// DefaultBaseURL is the public Open5e API root.
const DefaultBaseURL = "https://api.open5e.com"

// ErrNotFound is returned when the API answers 404.
var ErrNotFound = errors.New("not found")

// Client fetches and decodes Open5e resources. Raw responses are cached
// by URL so repeated lookups never hit the network twice within the TTL.
type Client struct {
	baseURL    string
	httpClient *http.Client
	cache      *cache.Cache
}

// NewClient builds a client against baseURL (use DefaultBaseURL in
// production, an httptest.Server URL in tests).
func NewClient(baseURL string, httpClient *http.Client, c *cache.Cache) *Client {
	if httpClient == nil {
		httpClient = &http.Client{Timeout: 15 * time.Second}
	}
	return &Client{
		baseURL:    strings.TrimRight(baseURL, "/"),
		httpClient: httpClient,
		cache:      c,
	}
}

// FirstMonsterPageURL is where the bestiary starts: SRD monsters only,
// 20 per page.
func (c *Client) FirstMonsterPageURL() string {
	return c.baseURL + "/v1/monsters/?document__slug=wotc-srd&ordering=name&limit=20"
}

// Monster fetches a single monster by slug (e.g. "giant-spider").
func (c *Client) Monster(ctx context.Context, slug string) (Monster, error) {
	slug = Slugify(slug)
	if slug == "" {
		return Monster{}, errors.New("empty monster name")
	}
	var m Monster
	u := c.baseURL + "/v1/monsters/" + url.PathEscape(slug) + "/"
	if err := c.getJSON(ctx, u, &m); err != nil {
		return Monster{}, fmt.Errorf("fetch monster %q: %w", slug, err)
	}
	return m, nil
}

// MonsterPage fetches one page of the monster listing. pageURL is either
// FirstMonsterPageURL or a next/previous URL returned by a previous page.
func (c *Client) MonsterPage(ctx context.Context, pageURL string) (MonsterPage, error) {
	var p MonsterPage
	if err := c.getJSON(ctx, pageURL, &p); err != nil {
		return MonsterPage{}, fmt.Errorf("fetch monster page: %w", err)
	}
	return p, nil
}

// PrefetchMonsters fetches every slug concurrently (fan-out), waits for all
// of them (fan-in) and returns the monsters that succeeded together with
// any errors joined into one.
func (c *Client) PrefetchMonsters(ctx context.Context, slugs []string) (map[string]Monster, error) {
	type result struct {
		slug string
		m    Monster
		err  error
	}
	results := make(chan result, len(slugs))
	var wg sync.WaitGroup
	for _, s := range slugs {
		wg.Add(1)
		go func(slug string) {
			defer wg.Done()
			m, err := c.Monster(ctx, slug)
			results <- result{slug: slug, m: m, err: err}
		}(s)
	}
	wg.Wait()
	close(results)

	out := make(map[string]Monster, len(slugs))
	var errs []error
	for r := range results {
		if r.err != nil {
			errs = append(errs, r.err)
			continue
		}
		out[r.slug] = r.m
	}
	return out, errors.Join(errs...)
}

func (c *Client) getJSON(ctx context.Context, u string, v any) error {
	body, err := c.get(ctx, u)
	if err != nil {
		return err
	}
	if err := json.Unmarshal(body, v); err != nil {
		return fmt.Errorf("decode %s: %w", u, err)
	}
	return nil
}

func (c *Client) get(ctx context.Context, u string) ([]byte, error) {
	if c.cache != nil {
		if b, ok := c.cache.Get(u); ok {
			return b, nil
		}
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return nil, fmt.Errorf("build request: %w", err)
	}
	req.Header.Set("Accept", "application/json")
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("GET %s: %w", u, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		return nil, ErrNotFound
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("GET %s: unexpected status %s", u, resp.Status)
	}
	b, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read body: %w", err)
	}
	if c.cache != nil {
		c.cache.Add(u, b)
	}
	return b, nil
}

// Slugify turns user input such as "Giant Spider" into "giant-spider".
func Slugify(s string) string {
	fields := strings.Fields(strings.ToLower(strings.TrimSpace(s)))
	return strings.Join(fields, "-")
}
