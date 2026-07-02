// Package esports turns the public LoL Esports schedule into tradeable
// prediction markets by attaching live Polymarket "match winner" odds.
package esports

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

// ErrMatchNotFound is returned when a per-match odds refresh targets an unknown id.
var ErrMatchNotFound = errors.New("match not found")

type Team struct {
	Name       string `json:"name"`
	Code       string `json:"code"`
	Image      string `json:"image"`
	ProbBps    int64  `json:"probBps"`    // implied win probability in basis points (0-10000)
	PriceCents int64  `json:"priceCents"` // Yes contract price in cents (0-100)
}

type Match struct {
	ID            string    `json:"id"`
	StartTime     time.Time `json:"startTime"`
	State         string    `json:"state"`
	League        string    `json:"league"`
	BlockName     string    `json:"blockName"`
	BestOf        int       `json:"bestOf"`
	Team1         Team      `json:"team1"`
	Team2         Team      `json:"team2"`
	HasOdds       bool      `json:"hasOdds"`
	PolymarketURL string    `json:"polymarketUrl"`
}

type TeamInfo struct {
	Code   string `json:"code"`
	Name   string `json:"name"`
	League string `json:"league"`
	Image  string `json:"image"`
}

// Result is the settled outcome of a completed match, used to resolve bets.
type Result struct {
	MatchID     string    `json:"matchId"`
	WinnerCode  string    `json:"winnerCode"`
	Team1Code   string    `json:"team1Code"`
	Team2Code   string    `json:"team2Code"`
	CompletedAt time.Time `json:"completedAt"`
}

// Store persists results (so they survive restarts / the schedule window aging
// out) and supplies team code mappings for Polymarket slug building.
type Store interface {
	GetMeta(ctx context.Context, key string) (string, bool, error)
	SetMeta(ctx context.Context, key, value string) error
	TeamMappingsMap(ctx context.Context) (map[string]string, error)
}

const resultsMetaKey = "esports_results"
const teamsMetaKey = "esports_teams"

var fallbackTeams = []TeamInfo{
	{Code: "G2", Name: "G2 Esports", League: "LEC", Image: ""},
	{Code: "FNC", Name: "Fnatic", League: "LEC", Image: ""},
	{Code: "VIT", Name: "Team Vitality", League: "LEC", Image: ""},
	{Code: "KC", Name: "Karmine Corp", League: "LEC", Image: ""},
	{Code: "T1", Name: "T1", League: "LCK", Image: ""},
	{Code: "GEN", Name: "Gen.G", League: "LCK", Image: ""},
	{Code: "EINS", Name: "Eintracht Spandau", League: "Prime League", Image: ""},
	{Code: "MOUZ", Name: "MOUZ", League: "Prime League", Image: ""},
	{Code: "NNO", Name: "NNO Prime", League: "Prime League", Image: ""},
}

type Service struct {
	apiKey   string
	lolBase  string
	polyBase string
	http     *http.Client
	ttl      time.Duration
	store    Store

	mu               sync.Mutex
	cache            []Match
	cachedAt         time.Time
	scheduleCache    []Match
	scheduleCachedAt time.Time
	teamsCache       []TeamInfo
	teamsCachedAt    time.Time
	results          map[string]Result
	resultsLoaded    bool
}

func NewService(apiKey, lolBaseURL, polyBaseURL string, timeout, ttl time.Duration, store Store) *Service {
	return &Service{
		apiKey:   apiKey,
		lolBase:  strings.TrimRight(lolBaseURL, "/"),
		polyBase: strings.TrimRight(polyBaseURL, "/"),
		http:     &http.Client{Timeout: timeout},
		ttl:      ttl,
		store:    store,
		results:  make(map[string]Result),
	}
}

type Status struct {
	ScheduleCached     bool `json:"scheduleCached"`
	ScheduleAgeSeconds int  `json:"scheduleAgeSeconds"`
	MatchCount         int  `json:"matchCount"`
	MatchesWithOdds    int  `json:"matchesWithOdds"`
	ResultsCount       int  `json:"resultsCount"`
	TeamsCached        bool `json:"teamsCached"`
	TeamCount          int  `json:"teamCount"`
}

type SlugDiagnostic struct {
	Match         Match    `json:"match"`
	Slugs         []string `json:"slugs"`
	Found         bool     `json:"found"`
	EventSlug     string   `json:"eventSlug"`
	PolymarketURL string   `json:"polymarketUrl"`
}

// Status reports cache state for the admin panel.
func (s *Service) Status(ctx context.Context) Status {
	s.ensureResultsLoaded(ctx)

	s.mu.Lock()
	defer s.mu.Unlock()
	status := Status{
		ResultsCount: len(s.results),
		TeamsCached:  s.teamsCache != nil,
		TeamCount:    len(s.teamsCache),
	}
	if s.cache != nil {
		status.ScheduleCached = true
		status.ScheduleAgeSeconds = int(time.Since(s.cachedAt).Seconds())
		status.MatchCount = len(s.cache)
		for _, m := range s.cache {
			if m.HasOdds {
				status.MatchesWithOdds++
			}
		}
	}
	return status
}

// ForceRefresh invalidates the schedule cache and re-fetches schedule + odds now.
func (s *Service) ForceRefresh(ctx context.Context) ([]Match, error) {
	s.mu.Lock()
	s.cachedAt = time.Time{}
	s.mu.Unlock()
	return s.Matches(ctx)
}

// Results returns the stored outcomes for the requested match ids (only those
// that have completed).
func (s *Service) Results(ctx context.Context, matchIDs []string) []Result {
	s.ensureResultsLoaded(ctx)

	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]Result, 0, len(matchIDs))
	for _, id := range matchIDs {
		if result, ok := s.results[id]; ok {
			out = append(out, result)
		}
	}
	return out
}

func (s *Service) ensureResultsLoaded(ctx context.Context) {
	s.mu.Lock()
	if s.resultsLoaded || s.store == nil {
		s.resultsLoaded = true
		s.mu.Unlock()
		return
	}
	s.mu.Unlock()

	raw, ok, err := s.store.GetMeta(ctx, resultsMetaKey)
	s.mu.Lock()
	defer s.mu.Unlock()
	if err == nil && ok {
		var loaded map[string]Result
		if json.Unmarshal([]byte(raw), &loaded) == nil && loaded != nil {
			for id, result := range loaded {
				if _, exists := s.results[id]; !exists {
					s.results[id] = result
				}
			}
		}
	}
	s.resultsLoaded = true
}

// recordResults merges newly seen completed results and persists them.
func (s *Service) recordResults(ctx context.Context, fresh []Result) {
	if len(fresh) == 0 {
		return
	}
	s.ensureResultsLoaded(ctx)

	s.mu.Lock()
	changed := false
	for _, result := range fresh {
		if _, exists := s.results[result.MatchID]; !exists {
			s.results[result.MatchID] = result
			changed = true
		}
	}
	snapshot := make(map[string]Result, len(s.results))
	for id, result := range s.results {
		snapshot[id] = result
	}
	s.mu.Unlock()

	if changed && s.store != nil {
		if encoded, err := json.Marshal(snapshot); err == nil {
			_ = s.store.SetMeta(ctx, resultsMetaKey, string(encoded))
		}
	}
}

// Matches returns upcoming/in-progress LoL matches with odds, served from a
// short-lived in-memory cache. On upstream failure it falls back to the last
// good snapshot rather than erroring out.
func (s *Service) Matches(ctx context.Context) ([]Match, error) {
	s.mu.Lock()
	if s.cache != nil && time.Since(s.cachedAt) < s.ttl {
		cached := s.cache
		s.mu.Unlock()
		return cached, nil
	}
	s.mu.Unlock()

	var matches []Match
	var err error

	s.mu.Lock()
	useCachedSchedule := s.scheduleCache != nil && time.Since(s.scheduleCachedAt) < 6*time.Hour
	if useCachedSchedule {
		matches = make([]Match, len(s.scheduleCache))
		copy(matches, s.scheduleCache)
	}
	s.mu.Unlock()

	if !useCachedSchedule {
		matches, err = s.fetchSchedule(ctx)
		if err != nil {
			s.mu.Lock()
			defer s.mu.Unlock()
			if s.cache != nil {
				return s.cache, nil
			}
			return nil, err
		}

		s.mu.Lock()
		s.scheduleCache = make([]Match, len(matches))
		copy(s.scheduleCache, matches)
		s.scheduleCachedAt = time.Now()
		s.mu.Unlock()
	}

	s.attachOdds(ctx, matches)

	s.mu.Lock()
	s.cache = matches
	s.cachedAt = time.Now()
	s.mu.Unlock()
	return matches, nil
}

// RefreshMatchOdds re-queries Polymarket for a single match, bypassing the
// schedule cache TTL. Polymarket has no rate limit, so this is called on demand
// right before a user places a bet to show the freshest odds.
func (s *Service) RefreshMatchOdds(ctx context.Context, matchID string) (Match, error) {
	s.mu.Lock()
	idx := -1
	for i := range s.cache {
		if s.cache[i].ID == matchID {
			idx = i
			break
		}
	}
	if idx == -1 {
		s.mu.Unlock()
		return Match{}, ErrMatchNotFound
	}
	match := s.cache[idx]
	s.mu.Unlock()

	match.HasOdds = false
	match.Team1.ProbBps, match.Team1.PriceCents = 0, 0
	match.Team2.ProbBps, match.Team2.PriceCents = 0, 0

	if match.Team1.Code != "TBD" && match.Team2.Code != "TBD" {
		slugs := generateSlugs(&match, s.mappingDict(ctx))
		for start := 0; start < len(slugs); start += 50 {
			end := start + 50
			if end > len(slugs) {
				end = len(slugs)
			}
			events, err := s.fetchPolymarketEvents(ctx, slugs[start:end])
			if err != nil {
				continue
			}
			for _, event := range events {
				applyOdds(&match, event)
			}
		}
	}

	s.mu.Lock()
	for i := range s.cache {
		if s.cache[i].ID == matchID {
			s.cache[i] = match
			break
		}
	}
	s.mu.Unlock()
	return match, nil
}

func (s *Service) SlugDiagnostic(ctx context.Context, matchID, originalCode, polymarketCode string, liveTest bool) (SlugDiagnostic, error) {
	match, err := s.matchByID(ctx, matchID)
	if err != nil {
		return SlugDiagnostic{}, err
	}
	dict := s.mappingDict(ctx)
	if dict == nil {
		dict = map[string]string{}
	}
	if strings.TrimSpace(originalCode) != "" && strings.TrimSpace(polymarketCode) != "" {
		dict[strings.ToLower(strings.TrimSpace(originalCode))] = strings.TrimSpace(polymarketCode)
	}

	slugs := generateSlugs(&match, dict)
	out := SlugDiagnostic{Match: match, Slugs: slugs}
	if !liveTest || len(slugs) == 0 {
		return out, nil
	}

	testMatch := match
	testMatch.HasOdds = false
	testMatch.PolymarketURL = ""
	testMatch.Team1.ProbBps, testMatch.Team1.PriceCents = 0, 0
	testMatch.Team2.ProbBps, testMatch.Team2.PriceCents = 0, 0
	var lastErr error
	for start := 0; start < len(slugs); start += 50 {
		end := start + 50
		if end > len(slugs) {
			end = len(slugs)
		}
		events, err := s.fetchPolymarketEvents(ctx, slugs[start:end])
		if err != nil {
			lastErr = err
			continue
		}
		for _, event := range events {
			applyOdds(&testMatch, event)
			if testMatch.HasOdds {
				out.Found = true
				out.EventSlug = event.Slug
				out.PolymarketURL = testMatch.PolymarketURL
				return out, nil
			}
		}
	}
	if lastErr != nil {
		return out, lastErr
	}
	return out, nil
}

func (s *Service) matchByID(ctx context.Context, matchID string) (Match, error) {
	s.mu.Lock()
	for _, match := range s.cache {
		if match.ID == matchID {
			s.mu.Unlock()
			return match, nil
		}
	}
	s.mu.Unlock()

	matches, err := s.Matches(ctx)
	if err != nil {
		return Match{}, err
	}
	for _, match := range matches {
		if match.ID == matchID {
			return match, nil
		}
	}
	return Match{}, ErrMatchNotFound
}

type teamsResponse struct {
	Data struct {
		Teams []struct {
			Name       string `json:"name"`
			Code       string `json:"code"`
			Image      string `json:"image"`
			Status     string `json:"status"`
			HomeLeague struct {
				Name string `json:"name"`
			} `json:"homeLeague"`
		} `json:"teams"`
	} `json:"data"`
}

func (s *Service) ensureTeamsLoaded(ctx context.Context) {
	s.mu.Lock()
	if s.teamsCache != nil || s.store == nil {
		s.mu.Unlock()
		return
	}
	s.mu.Unlock()

	raw, ok, err := s.store.GetMeta(ctx, teamsMetaKey)
	if err == nil && ok {
		var loaded []TeamInfo
		s.mu.Lock()
		if json.Unmarshal([]byte(raw), &loaded) == nil && loaded != nil {
			s.teamsCache = loaded
			s.teamsCachedAt = time.Now()
		}
		s.mu.Unlock()
	}
}

func (s *Service) getFallbackTeams(ctx context.Context) []TeamInfo {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.teamsCache == nil {
		s.teamsCache = fallbackTeams
		s.teamsCachedAt = time.Now()
		if s.store != nil {
			if encoded, err := json.Marshal(fallbackTeams); err == nil {
				_ = s.store.SetMeta(ctx, teamsMetaKey, string(encoded))
			}
		}
	}
	return s.teamsCache
}

// Teams returns the catalogue of active eSports teams (for the favorites picker),
// trimmed and deduplicated, served from a TTL cache.
func (s *Service) Teams(ctx context.Context) ([]TeamInfo, error) {
	s.ensureTeamsLoaded(ctx)

	s.mu.Lock()
	if s.teamsCache != nil && time.Since(s.teamsCachedAt) < 24*time.Hour {
		cached := s.teamsCache
		s.mu.Unlock()
		return cached, nil
	}
	s.mu.Unlock()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, s.lolBase+"/persisted/gw/getTeams?hl=en-GB", nil)
	if err != nil {
		return s.getFallbackTeams(ctx), nil
	}
	req.Header.Set("x-api-key", s.apiKey)

	resp, err := s.http.Do(req)
	if err != nil {
		return s.getFallbackTeams(ctx), nil
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return s.getFallbackTeams(ctx), nil
	}

	var payload teamsResponse
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return s.getFallbackTeams(ctx), nil
	}

	seen := map[string]struct{}{}
	teams := make([]TeamInfo, 0, len(payload.Data.Teams))
	for _, t := range payload.Data.Teams {
		code := strings.ToUpper(strings.TrimSpace(t.Code))
		if code == "" || code == "TBD" || code == "TBDD" || t.Status == "archived" {
			continue
		}
		if _, ok := seen[code]; ok {
			continue
		}
		seen[code] = struct{}{}
		league := t.HomeLeague.Name
		if league == "" {
			league = "Unknown"
		}
		teams = append(teams, TeamInfo{
			Code:   code,
			Name:   t.Name,
			League: league,
			Image:  strings.Replace(t.Image, "http://", "https://", 1),
		})
	}
	sort.Slice(teams, func(i, j int) bool { return teams[i].Name < teams[j].Name })

	s.mu.Lock()
	s.teamsCache = teams
	s.teamsCachedAt = time.Now()
	s.mu.Unlock()

	if s.store != nil {
		if encoded, err := json.Marshal(teams); err == nil {
			_ = s.store.SetMeta(ctx, teamsMetaKey, string(encoded))
		}
	}

	return teams, nil
}

type scheduleResponse struct {
	Data struct {
		Schedule struct {
			Events []struct {
				ID        string    `json:"id"`
				StartTime time.Time `json:"startTime"`
				State     string    `json:"state"`
				BlockName string    `json:"blockName"`
				League    struct {
					Name string `json:"name"`
				} `json:"league"`
				Match struct {
					ID       string `json:"id"`
					Strategy struct {
						Count int `json:"count"`
					} `json:"strategy"`
					Teams []struct {
						Name   string `json:"name"`
						Code   string `json:"code"`
						Image  string `json:"image"`
						Result struct {
							Outcome  string `json:"outcome"`
							GameWins int    `json:"gameWins"`
						} `json:"result"`
					} `json:"teams"`
				} `json:"match"`
			} `json:"events"`
		} `json:"schedule"`
	} `json:"data"`
}

func (s *Service) fetchSchedule(ctx context.Context) ([]Match, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, s.lolBase+"/persisted/gw/getSchedule?hl=en-GB", nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("x-api-key", s.apiKey)

	resp, err := s.http.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("lolesports schedule status %d", resp.StatusCode)
	}

	var payload scheduleResponse
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return nil, err
	}

	matches := make([]Match, 0, 40)
	results := make([]Result, 0)
	for _, event := range payload.Data.Schedule.Events {
		if len(event.Match.Teams) < 2 {
			continue
		}
		id := event.Match.ID
		if id == "" {
			id = event.ID
		}

		if event.State == "completed" {
			if result, ok := resultFromEvent(id, event.StartTime, event.Match.Teams[0], event.Match.Teams[1]); ok {
				results = append(results, result)
			}
			continue
		}

		if len(matches) >= 40 {
			continue
		}
		matches = append(matches, Match{
			ID:        id,
			StartTime: event.StartTime,
			State:     event.State,
			League:    event.League.Name,
			BlockName: event.BlockName,
			BestOf:    event.Match.Strategy.Count,
			Team1:     team(event.Match.Teams[0].Name, event.Match.Teams[0].Code, event.Match.Teams[0].Image),
			Team2:     team(event.Match.Teams[1].Name, event.Match.Teams[1].Code, event.Match.Teams[1].Image),
		})
	}

	s.recordResults(ctx, results)
	return matches, nil
}

type scheduleTeam = struct {
	Name   string `json:"name"`
	Code   string `json:"code"`
	Image  string `json:"image"`
	Result struct {
		Outcome  string `json:"outcome"`
		GameWins int    `json:"gameWins"`
	} `json:"result"`
}

func resultFromEvent(matchID string, completedAt time.Time, t1, t2 scheduleTeam) (Result, bool) {
	winner := ""
	if t1.Result.Outcome == "win" {
		winner = strings.ToUpper(t1.Code)
	} else if t2.Result.Outcome == "win" {
		winner = strings.ToUpper(t2.Code)
	}
	if winner == "" {
		return Result{}, false
	}
	return Result{
		MatchID:     matchID,
		WinnerCode:  winner,
		Team1Code:   strings.ToUpper(t1.Code),
		Team2Code:   strings.ToUpper(t2.Code),
		CompletedAt: completedAt,
	}, true
}

func team(name, code, image string) Team {
	if name == "" {
		name = "TBD"
	}
	if code == "" {
		code = "TBD"
	}
	return Team{Name: name, Code: code, Image: strings.Replace(image, "http://", "https://", 1)}
}

type polymarketMarket struct {
	Question         string `json:"question"`
	GroupItemTitle   string `json:"groupItemTitle"`
	SportsMarketType string `json:"sportsMarketType"`
	Outcomes         string `json:"outcomes"`
	OutcomePrices    string `json:"outcomePrices"`
}

type polymarketEvent struct {
	ID      string             `json:"id"`
	Slug    string             `json:"slug"`
	Title   string             `json:"title"`
	Markets []polymarketMarket `json:"markets"`
}

// attachOdds queries Polymarket for each match by guessed slugs, then maps the
// returned "match winner" market back onto the schedule.
func (s *Service) attachOdds(ctx context.Context, matches []Match) {
	dict := s.mappingDict(ctx)
	slugToIndex := map[string]int{}
	allSlugs := make([]string, 0, len(matches)*8)
	for i := range matches {
		m := &matches[i]
		if m.Team1.Code == "TBD" || m.Team2.Code == "TBD" {
			continue
		}
		for _, slug := range generateSlugs(m, dict) {
			if _, seen := slugToIndex[slug]; !seen {
				slugToIndex[slug] = i
				allSlugs = append(allSlugs, slug)
			}
		}
	}
	if len(allSlugs) == 0 {
		return
	}

	for start := 0; start < len(allSlugs); start += 50 {
		end := start + 50
		if end > len(allSlugs) {
			end = len(allSlugs)
		}
		events, err := s.fetchPolymarketEvents(ctx, allSlugs[start:end])
		if err != nil {
			continue
		}
		for _, event := range events {
			idx, ok := slugToIndex[event.Slug]
			if !ok {
				continue
			}
			applyOdds(&matches[idx], event)
		}
	}
}

func (s *Service) fetchPolymarketEvents(ctx context.Context, slugs []string) ([]polymarketEvent, error) {
	query := url.Values{}
	for _, slug := range slugs {
		query.Add("slug", slug)
	}
	endpoint := s.polyBase + "/events?" + query.Encode()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, err
	}
	resp, err := s.http.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("polymarket status %d", resp.StatusCode)
	}

	var events []polymarketEvent
	if err := json.NewDecoder(resp.Body).Decode(&events); err != nil {
		return nil, err
	}
	return events, nil
}

func applyOdds(match *Match, event polymarketEvent) {
	if match.HasOdds {
		return
	}
	winner := pickMoneylineMarket(event)
	if winner == nil {
		return
	}

	var names, prices []string
	if err := json.Unmarshal([]byte(winner.Outcomes), &names); err != nil {
		return
	}
	if err := json.Unmarshal([]byte(winner.OutcomePrices), &prices); err != nil {
		return
	}
	if len(names) != 2 || len(prices) != 2 {
		return
	}

	for i, name := range names {
		price, _ := strconv.ParseFloat(prices[i], 64)
		probBps := int64(price*10000 + 0.5)
		priceCents := int64(price*100 + 0.5)
		switch {
		case teamMatches(match.Team1, name):
			match.Team1.ProbBps = probBps
			match.Team1.PriceCents = priceCents
		case teamMatches(match.Team2, name):
			match.Team2.ProbBps = probBps
			match.Team2.PriceCents = priceCents
		}
	}

	if match.Team1.PriceCents > 0 || match.Team2.PriceCents > 0 {
		match.HasOdds = true
		match.PolymarketURL = "https://polymarket.com/event/" + event.Slug
	}
}

func pickMoneylineMarket(event polymarketEvent) *polymarketMarket {
	for i := range event.Markets {
		m := &event.Markets[i]
		if m.SportsMarketType == "moneyline" || m.GroupItemTitle == "Match Winner" {
			return m
		}
	}
	// Fallback: a two-team market that is not a per-game / prop sub-market.
	for i := range event.Markets {
		m := &event.Markets[i]
		var names []string
		if err := json.Unmarshal([]byte(m.Outcomes), &names); err != nil || len(names) != 2 {
			continue
		}
		if containsAny(names, "Yes", "No", "Over", "Under") {
			continue
		}
		q := strings.ToLower(m.Question)
		if strings.Contains(q, "first blood") || strings.Contains(q, "game") || strings.Contains(q, "handicap") || strings.Contains(q, "map") {
			continue
		}
		return m
	}
	return nil
}

var nonAlnum = regexp.MustCompile(`[^a-z0-9]`)

func normalize(s string) string {
	return nonAlnum.ReplaceAllString(strings.ToLower(s), "")
}

func teamMatches(t Team, outcome string) bool {
	o := normalize(outcome)
	return o != "" && (o == normalize(t.Name) || o == normalize(t.Code) ||
		strings.Contains(normalize(t.Name), o) || strings.Contains(o, normalize(t.Name)))
}

func containsAny(list []string, candidates ...string) bool {
	set := map[string]struct{}{}
	for _, item := range list {
		set[item] = struct{}{}
	}
	for _, c := range candidates {
		if _, ok := set[c]; ok {
			return true
		}
	}
	return false
}

func (s *Service) mappingDict(ctx context.Context) map[string]string {
	if s.store == nil {
		return nil
	}
	dict, err := s.store.TeamMappingsMap(ctx)
	if err != nil {
		return nil
	}
	return dict
}

// generateSlugs mirrors Polymarket's LoL slug convention:
// lol-<team1>-<team2>-<YYYY-MM-DD>, trying both team orders and nearby dates
// (timezone skew) plus code/name identifiers. Admin-defined mappings let an
// lolesports code resolve to Polymarket's differing code (e.g. EINS -> ES1).
func generateSlugs(m *Match, mapping map[string]string) []string {
	ids1 := identifiers(m.Team1, mapping)
	ids2 := identifiers(m.Team2, mapping)
	seen := map[string]struct{}{}
	out := make([]string, 0, 16)

	for offset := -2; offset <= 1; offset++ {
		date := m.StartTime.AddDate(0, 0, offset).UTC().Format("2006-01-02")
		for _, a := range ids1 {
			for _, b := range ids2 {
				for _, slug := range []string{
					"lol-" + a + "-" + b + "-" + date,
					"lol-" + b + "-" + a + "-" + date,
				} {
					if _, ok := seen[slug]; !ok {
						seen[slug] = struct{}{}
						out = append(out, slug)
					}
				}
			}
		}
	}
	return out
}

func identifiers(t Team, mapping map[string]string) []string {
	seen := map[string]struct{}{}
	out := make([]string, 0, 3)
	mapped := ""
	if mapping != nil {
		mapped = normalize(mapping[strings.ToLower(t.Code)])
	}
	for _, candidate := range []string{mapped, normalize(t.Code), normalize(t.Name)} {
		if candidate == "" {
			continue
		}
		if _, ok := seen[candidate]; !ok {
			seen[candidate] = struct{}{}
			out = append(out, candidate)
		}
	}
	sort.Strings(out)
	return out
}
