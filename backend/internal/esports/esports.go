// Package esports turns the public LoL Esports schedule into tradeable
// prediction markets by attaching live Polymarket "match winner" odds.
package esports

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"net/http"
	"net/url"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/Shik3i/KoalaTrade/backend/internal/storage"
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

type MatchDetails struct {
	MatchID    string       `json:"matchId"`
	State      string       `json:"state"`
	Team1Code  string       `json:"team1Code"`
	Team2Code  string       `json:"team2Code"`
	Team1Score int          `json:"team1Score"`
	Team2Score int          `json:"team2Score"`
	Games      []MatchGame  `json:"games"`
	Videos     []MatchVideo `json:"videos"`
	FetchedAt  time.Time    `json:"fetchedAt"`
}

type MatchGame struct {
	GameID     string `json:"gameId"`
	GameNumber int    `json:"gameNumber"`
	State      string `json:"state"`
}

type MatchVideo struct {
	GameID   string `json:"gameId,omitempty"`
	Kind     string `json:"kind"`
	URL      string `json:"url"`
	Provider string `json:"provider,omitempty"`
	Locale   string `json:"locale,omitempty"`
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

type TeamStore interface {
	LoadEsportsTeams(ctx context.Context) ([]storage.EsportsTeam, error)
	UpsertEsportsTeams(ctx context.Context, teams []storage.EsportsTeam) error
	EsportsTeamLogo(ctx context.Context, code string) ([]byte, string, bool, error)
}

type MatchDetailsStore interface {
	LoadEsportsMatchDetails(ctx context.Context, matchID string) (storage.EsportsMatchDetail, []storage.EsportsMatchGame, []storage.EsportsMatchVideo, bool, error)
	UpsertEsportsMatchDetails(ctx context.Context, detail storage.EsportsMatchDetail, games []storage.EsportsMatchGame, videos []storage.EsportsMatchVideo) error
}

const resultsMetaKey = "esports_results"

const (
	teamSnapshotTTL = 7 * 24 * time.Hour
	maxTeamLogoSize = 8 << 20
	matchDetailsTTL = 5 * time.Minute
	teamLogoWorkers = 32
	teamLogoTimeout = 5 * time.Second
)

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

var teamCodePattern = regexp.MustCompile(`^[A-Z0-9_-]{1,32}$`)

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
	teamsSyncAt      time.Time
	teamsLoaded      bool
	teamsAttemptAt   time.Time
	teamLogoCodes    map[string]bool
	teamSyncMu       sync.Mutex
	results          map[string]Result
	resultsLoaded    bool
}

func NewService(apiKey, lolBaseURL, polyBaseURL string, timeout, ttl time.Duration, store Store) *Service {
	return &Service{
		apiKey:        apiKey,
		lolBase:       strings.TrimRight(lolBaseURL, "/"),
		polyBase:      strings.TrimRight(polyBaseURL, "/"),
		http:          &http.Client{Timeout: timeout},
		ttl:           ttl,
		store:         store,
		results:       make(map[string]Result),
		teamLogoCodes: make(map[string]bool),
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
	out := make([]Result, 0, len(matchIDs))
	missing := make([]string, 0, len(matchIDs))
	seen := make(map[string]struct{}, len(matchIDs))
	for _, id := range matchIDs {
		if result, ok := s.results[id]; ok {
			out = append(out, result)
		} else if id != "" {
			if _, exists := seen[id]; !exists {
				missing = append(missing, id)
				seen[id] = struct{}{}
			}
		}
	}
	s.mu.Unlock()

	if len(missing) == 0 {
		return out
	}

	fresh := make([]Result, 0, len(missing))
	for _, matchID := range missing {
		details, err := s.MatchDetails(ctx, matchID)
		if err != nil {
			continue
		}
		if result, ok := resultFromMatchDetails(details); ok {
			fresh = append(fresh, result)
			out = append(out, result)
		}
	}
	s.recordResults(ctx, fresh)
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
	// Hydrate the logo map from SQLite before consulting the match cache. Never
	// block this request on the weekly upstream catalogue/logo refresh.
	s.ensureTeamsLoaded(ctx)

	s.mu.Lock()
	if s.cache != nil && time.Since(s.cachedAt) < s.ttl {
		s.attachTeamImagesLocked(s.cache)
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
	s.attachTeamImagesLocked(matches)
	s.cache = matches
	s.cachedAt = time.Now()
	s.mu.Unlock()
	return matches, nil
}

func (s *Service) attachTeamImagesLocked(matches []Match) {
	for i := range matches {
		matches[i].Team1.Image = teamImageURL(matches[i].Team1.Code, s.teamLogoCodes[strings.ToUpper(strings.TrimSpace(matches[i].Team1.Code))])
		matches[i].Team2.Image = teamImageURL(matches[i].Team2.Code, s.teamLogoCodes[strings.ToUpper(strings.TrimSpace(matches[i].Team2.Code))])
	}
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

type eventDetailsResponse struct {
	Data struct {
		Event struct {
			State   string `json:"state"`
			Streams []struct {
				Parameter string `json:"parameter"`
				Locale    string `json:"locale"`
				Provider  string `json:"provider"`
			} `json:"streams"`
			Match struct {
				Teams []struct {
					Code   string `json:"code"`
					Result struct {
						GameWins int    `json:"gameWins"`
						Outcome  string `json:"outcome"`
					} `json:"result"`
				} `json:"teams"`
				Games []struct {
					ID     string `json:"id"`
					State  string `json:"state"`
					Number int    `json:"number"`
					VODs   []struct {
						Parameter string `json:"parameter"`
						Locale    string `json:"locale"`
						Provider  string `json:"provider"`
					} `json:"vods"`
				} `json:"games"`
			} `json:"match"`
		} `json:"event"`
	} `json:"data"`
}

// MatchDetails loads detailed games and VODs on demand. Persisted details are
// returned while fresh so expanding a card does not repeatedly hit lolesports.
func (s *Service) MatchDetails(ctx context.Context, matchID string) (MatchDetails, error) {
	matchID = strings.TrimSpace(matchID)
	if matchID == "" {
		return MatchDetails{}, ErrMatchNotFound
	}
	var stale MatchDetails
	hasStale := false
	if store, ok := s.store.(MatchDetailsStore); ok {
		stored, games, videos, found, err := store.LoadEsportsMatchDetails(ctx, matchID)
		if err != nil {
			return MatchDetails{}, err
		}
		if found {
			cached := fromStoredMatchDetails(stored, games, videos)
			if detailsFresh(cached) {
				return cached, nil
			}
			stale = cached
			hasStale = true
		}
	}
	fallback := func(err error) (MatchDetails, error) {
		if hasStale {
			return stale, nil
		}
		return MatchDetails{}, err
	}

	endpoint := s.lolBase + "/persisted/gw/getEventDetails?hl=en-GB&id=" + url.QueryEscape(matchID)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return fallback(err)
	}
	req.Header.Set("x-api-key", s.apiKey)
	resp, err := s.http.Do(req)
	if err != nil {
		return fallback(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusNotFound {
		return fallback(ErrMatchNotFound)
	}
	if resp.StatusCode != http.StatusOK {
		return fallback(fmt.Errorf("lolesports event details status %d", resp.StatusCode))
	}

	var payload eventDetailsResponse
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return fallback(err)
	}
	details := parseMatchDetails(matchID, payload)
	if len(details.Team1Code) == 0 && len(details.Team2Code) == 0 && len(details.Games) == 0 {
		return fallback(errors.New("lolesports event details response was empty"))
	}

	if store, ok := s.store.(MatchDetailsStore); ok {
		stored, games, videos := toStoredMatchDetails(details)
		if err := store.UpsertEsportsMatchDetails(ctx, stored, games, videos); err != nil {
			return MatchDetails{}, err
		}
	}
	return details, nil
}

func detailsFresh(details MatchDetails) bool {
	if details.FetchedAt.IsZero() {
		return false
	}
	ttl := matchDetailsTTL
	if details.State == "inProgress" {
		ttl = 30 * time.Second
	}
	return time.Since(details.FetchedAt) < ttl
}

func parseMatchDetails(matchID string, payload eventDetailsResponse) MatchDetails {
	event := payload.Data.Event
	details := MatchDetails{
		MatchID:   matchID,
		State:     strings.TrimSpace(event.State),
		Games:     make([]MatchGame, 0, len(event.Match.Games)),
		Videos:    make([]MatchVideo, 0),
		FetchedAt: time.Now().UTC(),
	}
	if len(event.Match.Teams) > 0 {
		details.Team1Code = strings.ToUpper(strings.TrimSpace(event.Match.Teams[0].Code))
		details.Team1Score = event.Match.Teams[0].Result.GameWins
	}
	if len(event.Match.Teams) > 1 {
		details.Team2Code = strings.ToUpper(strings.TrimSpace(event.Match.Teams[1].Code))
		details.Team2Score = event.Match.Teams[1].Result.GameWins
	}
	for index, game := range event.Match.Games {
		number := game.Number
		if number <= 0 {
			number = index + 1
		}
		details.Games = append(details.Games, MatchGame{
			GameID: game.ID, GameNumber: number, State: strings.TrimSpace(game.State),
		})
		for _, vod := range game.VODs {
			if video, ok := makeMatchVideo("vod", game.ID, vod.Provider, vod.Locale, vod.Parameter); ok {
				details.Videos = appendUniqueVideo(details.Videos, video)
			}
		}
	}
	for _, stream := range event.Streams {
		if video, ok := makeMatchVideo("stream", "", stream.Provider, stream.Locale, stream.Parameter); ok {
			details.Videos = appendUniqueVideo(details.Videos, video)
		}
	}
	if len(details.Games) > 0 {
		allTerminal := true
		for _, game := range details.Games {
			switch strings.ToLower(game.State) {
			case "completed", "unneeded", "cancelled", "canceled":
			default:
				allTerminal = false
			}
		}
		// getEventDetails occasionally omits event.state and reports a scheduled
		// event even after every game has finished. The game states are the more
		// reliable signal for the details view.
		if allTerminal {
			details.State = "completed"
		}
	}
	if details.State == "" {
		details.State = "scheduled"
	}
	return details
}

func makeMatchVideo(kind, gameID, provider, locale, parameter string) (MatchVideo, bool) {
	parameter = strings.TrimSpace(parameter)
	provider = strings.TrimSpace(provider)
	if parameter == "" {
		return MatchVideo{}, false
	}
	videoURL := parameter
	if parsed, err := url.Parse(parameter); err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") {
		switch strings.ToLower(provider) {
		case "youtube", "youtube.com":
			videoURL = "https://www.youtube.com/watch?v=" + url.QueryEscape(parameter)
		case "twitch":
			if kind == "vod" {
				videoURL = "https://www.twitch.tv/videos/" + url.PathEscape(parameter)
			} else {
				videoURL = "https://www.twitch.tv/" + url.PathEscape(parameter)
			}
		default:
			return MatchVideo{}, false
		}
	}
	return MatchVideo{GameID: gameID, Kind: kind, URL: videoURL, Provider: provider, Locale: locale}, true
}

func appendUniqueVideo(videos []MatchVideo, candidate MatchVideo) []MatchVideo {
	for _, video := range videos {
		if video.Kind == candidate.Kind && video.GameID == candidate.GameID && video.URL == candidate.URL {
			return videos
		}
	}
	return append(videos, candidate)
}

func fromStoredMatchDetails(detail storage.EsportsMatchDetail, games []storage.EsportsMatchGame, videos []storage.EsportsMatchVideo) MatchDetails {
	out := MatchDetails{
		MatchID: detail.MatchID, State: detail.State, Team1Code: detail.Team1Code, Team2Code: detail.Team2Code,
		Team1Score: detail.Team1Score, Team2Score: detail.Team2Score, Games: make([]MatchGame, 0, len(games)), Videos: make([]MatchVideo, 0, len(videos)),
	}
	out.FetchedAt, _ = time.Parse(time.RFC3339Nano, detail.FetchedAt)
	for _, game := range games {
		out.Games = append(out.Games, MatchGame{GameID: game.GameID, GameNumber: game.GameNumber, State: game.State})
	}
	for _, video := range videos {
		out.Videos = append(out.Videos, MatchVideo{GameID: video.GameID, Kind: video.Kind, URL: video.URL, Provider: video.Provider, Locale: video.Locale})
	}
	return out
}

func resultFromMatchDetails(details MatchDetails) (Result, bool) {
	if !strings.EqualFold(details.State, "completed") || details.Team1Code == "" || details.Team2Code == "" || details.Team1Score == details.Team2Score {
		return Result{}, false
	}
	winner := details.Team1Code
	if details.Team2Score > details.Team1Score {
		winner = details.Team2Code
	}
	return Result{
		MatchID:     details.MatchID,
		WinnerCode:  strings.ToUpper(winner),
		Team1Code:   strings.ToUpper(details.Team1Code),
		Team2Code:   strings.ToUpper(details.Team2Code),
		CompletedAt: details.FetchedAt,
	}, true
}

func toStoredMatchDetails(details MatchDetails) (storage.EsportsMatchDetail, []storage.EsportsMatchGame, []storage.EsportsMatchVideo) {
	stored := storage.EsportsMatchDetail{
		MatchID: details.MatchID, State: details.State, Team1Code: details.Team1Code, Team2Code: details.Team2Code,
		Team1Score: details.Team1Score, Team2Score: details.Team2Score, FetchedAt: details.FetchedAt.UTC().Format(time.RFC3339Nano),
	}
	games := make([]storage.EsportsMatchGame, 0, len(details.Games))
	for _, game := range details.Games {
		games = append(games, storage.EsportsMatchGame{MatchID: details.MatchID, GameID: game.GameID, GameNumber: game.GameNumber, State: game.State})
	}
	videos := make([]storage.EsportsMatchVideo, 0, len(details.Videos))
	for _, video := range details.Videos {
		videos = append(videos, storage.EsportsMatchVideo{MatchID: details.MatchID, GameID: video.GameID, Kind: video.Kind, URL: video.URL, Provider: video.Provider, Locale: video.Locale})
	}
	return stored, games, videos
}

type teamsResponse struct {
	Data struct {
		Teams []struct {
			Name             string `json:"name"`
			Code             string `json:"code"`
			Image            string `json:"image"`
			AlternativeImage string `json:"alternativeImage"`
			Status           string `json:"status"`
			HomeLeague       struct {
				Name string `json:"name"`
			} `json:"homeLeague"`
		} `json:"teams"`
	} `json:"data"`
}

func (s *Service) ensureTeamsLoaded(ctx context.Context) {
	s.teamSyncMu.Lock()
	defer s.teamSyncMu.Unlock()

	s.mu.Lock()
	if s.teamsLoaded {
		s.mu.Unlock()
		return
	}
	s.mu.Unlock()

	teamStore, ok := s.store.(TeamStore)
	if !ok {
		s.mu.Lock()
		s.teamsLoaded = true
		s.mu.Unlock()
		return
	}

	stored, err := teamStore.LoadEsportsTeams(ctx)
	s.mu.Lock()
	defer s.mu.Unlock()
	s.teamsLoaded = true
	if err != nil {
		return
	}
	s.teamsCache = storedTeamInfo(stored)
	if len(s.teamsCache) > 0 {
		s.teamsCachedAt = time.Now()
	} else {
		s.teamsCachedAt = time.Time{}
	}
	s.teamLogoCodes = make(map[string]bool, len(stored))
	for _, team := range stored {
		s.teamLogoCodes[team.Code] = len(team.Logo) > 0
		if parsed, parseErr := time.Parse(time.RFC3339Nano, team.SyncedAt); parseErr == nil && parsed.After(s.teamsSyncAt) {
			s.teamsSyncAt = parsed
		}
	}
}

func (s *Service) getFallbackTeams() []TeamInfo {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.teamsCache == nil {
		s.teamsCache = append([]TeamInfo(nil), fallbackTeams...)
		s.teamsCachedAt = time.Now()
	}
	return s.teamsCache
}

// Teams returns the catalogue of active eSports teams (for the favorites picker),
// trimmed and deduplicated, served from a weekly database snapshot.
func (s *Service) Teams(ctx context.Context) ([]TeamInfo, error) {
	s.ensureTeamsLoaded(ctx)

	s.mu.Lock()
	attemptedRecently := !s.teamsAttemptAt.IsZero() && time.Since(s.teamsAttemptAt) < 15*time.Minute
	if len(s.teamsCache) > 0 {
		// Persisted rows are the request-path source of truth. The background
		// scheduler refreshes stale metadata and missing logos without delaying UI.
		cached := s.teamsCache
		s.mu.Unlock()
		return cached, nil
	}
	if attemptedRecently {
		s.mu.Unlock()
		return s.getFallbackTeams(), nil
	}
	s.mu.Unlock()

	teams, err := s.syncTeams(ctx)
	if err == nil {
		return teams, nil
	}
	if cached := s.cachedTeams(); cached != nil {
		return cached, nil
	}
	return s.getFallbackTeams(), nil
}

// SyncTeamsIfDue is called by the weekly background scheduler. It returns the
// upstream error so operators can see failed refreshes in the server log.
func (s *Service) SyncTeamsIfDue(ctx context.Context) error {
	s.ensureTeamsLoaded(ctx)
	s.mu.Lock()
	fresh := !s.teamsSyncAt.IsZero() && time.Since(s.teamsSyncAt) < teamSnapshotTTL
	s.mu.Unlock()
	if fresh {
		return nil
	}
	_, err := s.syncTeams(ctx)
	return err
}

func (s *Service) cachedTeams() []TeamInfo {
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(s.teamsCache) == 0 {
		return nil
	}
	return s.teamsCache
}

func (s *Service) syncTeams(ctx context.Context) ([]TeamInfo, error) {
	s.teamSyncMu.Lock()
	defer s.teamSyncMu.Unlock()

	s.mu.Lock()
	if !s.teamsSyncAt.IsZero() && time.Since(s.teamsSyncAt) < teamSnapshotTTL {
		cached := s.teamsCache
		s.mu.Unlock()
		return cached, nil
	}
	s.teamsAttemptAt = time.Now()
	s.mu.Unlock()

	teamStore, hasTeamStore := s.store.(TeamStore)
	existing := map[string]storage.EsportsTeam{}
	if hasTeamStore {
		if stored, err := teamStore.LoadEsportsTeams(ctx); err == nil {
			for _, team := range stored {
				existing[team.Code] = team
			}
		}
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, s.lolBase+"/persisted/gw/getTeams?hl=en-GB", nil)
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
		return nil, fmt.Errorf("lolesports teams status %d", resp.StatusCode)
	}

	var payload teamsResponse
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return nil, err
	}

	seen := map[string]struct{}{}
	teams := make([]TeamInfo, 0, len(payload.Data.Teams))
	storedTeams := make([]storage.EsportsTeam, 0, len(payload.Data.Teams))
	logoJobs := make([]int, 0, len(payload.Data.Teams))
	logoURLs := make([]string, 0, len(payload.Data.Teams))
	syncedAt := time.Now().UTC().Format(time.RFC3339Nano)
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

		imageURL := normalizeImageURL(t.Image)
		if imageURL == "" {
			imageURL = normalizeImageURL(t.AlternativeImage)
		}
		storedTeam := storage.EsportsTeam{
			Code:          code,
			Name:          strings.TrimSpace(t.Name),
			League:        league,
			LogoSourceURL: imageURL,
			SyncedAt:      syncedAt,
		}
		needsLogo := imageURL != ""
		if previous, ok := existing[code]; ok {
			storedTeam.Logo = previous.Logo
			storedTeam.LogoContentType = previous.LogoContentType
			if imageURL != "" && imageURL == previous.LogoSourceURL && len(previous.Logo) > 0 {
				needsLogo = false
			} else if len(previous.Logo) > 0 {
				// Keep the old source until the replacement has downloaded. This
				// makes a failed CDN update retry on the next weekly sync.
				storedTeam.LogoSourceURL = previous.LogoSourceURL
			}
		}
		storedTeams = append(storedTeams, storedTeam)
		logoURLs = append(logoURLs, imageURL)
		if needsLogo {
			logoJobs = append(logoJobs, len(storedTeams)-1)
		}
		teams = append(teams, TeamInfo{Code: code, Name: storedTeam.Name, League: league, Image: teamImageURL(code, len(storedTeam.Logo) > 0)})
	}
	if len(teams) == 0 {
		return nil, errors.New("lolesports teams response was empty")
	}
	s.downloadTeamLogos(ctx, storedTeams, logoJobs, logoURLs)
	sort.Slice(teams, func(i, j int) bool { return teams[i].Name < teams[j].Name })
	logos := make(map[string]bool, len(storedTeams))
	for _, team := range storedTeams {
		logos[team.Code] = len(team.Logo) > 0
	}
	for i := range teams {
		teams[i].Image = teamImageURL(teams[i].Code, logos[teams[i].Code])
	}

	if hasTeamStore {
		if err := teamStore.UpsertEsportsTeams(ctx, storedTeams); err != nil {
			return nil, err
		}
	}
	s.mu.Lock()
	s.teamsCache = teams
	s.teamsCachedAt = time.Now()
	// Metadata is a complete weekly snapshot even when individual upstream logo
	// downloads fail. Those teams keep their previous bytes and retry next week.
	s.teamsSyncAt = time.Now()
	s.teamsLoaded = true
	s.teamLogoCodes = logos
	s.mu.Unlock()

	return teams, nil
}

func storedTeamInfo(stored []storage.EsportsTeam) []TeamInfo {
	if len(stored) == 0 {
		return nil
	}
	teams := make([]TeamInfo, 0, len(stored))
	for _, team := range stored {
		teams = append(teams, TeamInfo{
			Code: team.Code, Name: team.Name, League: team.League,
			Image: teamImageURL(team.Code, len(team.Logo) > 0),
		})
	}
	return teams
}

func normalizeImageURL(raw string) string {
	return strings.Replace(strings.TrimSpace(raw), "http://", "https://", 1)
}

func teamImageURL(code string, hasLogo bool) string {
	code = strings.ToUpper(strings.TrimSpace(code))
	if !hasLogo || code == "" || code == "TBD" || code == "TBDD" {
		return ""
	}
	return "/api/esports/teams/" + url.PathEscape(code) + "/logo"
}

func (s *Service) fetchTeamLogo(ctx context.Context, imageURL string) ([]byte, string, error) {
	parsed, err := url.Parse(imageURL)
	if err != nil || parsed.Host == "" || (parsed.Scheme != "https" && parsed.Scheme != "http") {
		return nil, "", errors.New("invalid team logo URL")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, imageURL, nil)
	if err != nil {
		return nil, "", err
	}
	req.Header.Set("Accept", "image/avif,image/webp,image/png,image/jpeg,image/svg+xml;q=0.9,*/*;q=0.1")
	resp, err := s.http.Do(req)
	if err != nil {
		return nil, "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, "", fmt.Errorf("team logo status %d", resp.StatusCode)
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, maxTeamLogoSize+1))
	if err != nil {
		return nil, "", err
	}
	if len(data) == 0 || len(data) > maxTeamLogoSize {
		return nil, "", errors.New("team logo exceeds size limit")
	}
	contentType := strings.TrimSpace(strings.Split(resp.Header.Get("Content-Type"), ";")[0])
	if strings.HasSuffix(strings.ToLower(parsed.Path), ".svg") {
		contentType = "image/svg+xml"
	}
	if !strings.HasPrefix(contentType, "image/") {
		contentType = http.DetectContentType(data)
	}
	if !strings.HasPrefix(contentType, "image/") {
		return nil, "", errors.New("team logo is not an image")
	}
	return bytes.Clone(data), contentType, nil
}

func (s *Service) downloadTeamLogos(ctx context.Context, teams []storage.EsportsTeam, jobs []int, logoURLs []string) []int {
	if len(jobs) == 0 {
		return nil
	}
	workerCount := len(jobs)
	if workerCount > teamLogoWorkers {
		workerCount = teamLogoWorkers
	}
	type result struct {
		index       int
		sourceURL   string
		logo        []byte
		contentType string
		success     bool
	}
	pending := append([]int(nil), jobs...)
	for attempt := 0; attempt < 2 && len(pending) > 0; attempt++ {
		queue := make(chan int)
		results := make(chan result, len(pending))
		var workers sync.WaitGroup
		for i := 0; i < workerCount; i++ {
			workers.Add(1)
			go func() {
				defer workers.Done()
				for index := range queue {
					logoCtx, cancel := context.WithTimeout(ctx, teamLogoTimeout)
					logo, contentType, err := s.fetchTeamLogo(logoCtx, logoURLs[index])
					cancel()
					results <- result{
						index: index, sourceURL: logoURLs[index], logo: logo,
						contentType: contentType, success: err == nil,
					}
				}
			}()
		}
		for _, index := range pending {
			queue <- index
		}
		close(queue)
		workers.Wait()
		close(results)
		failed := make([]int, 0)
		for result := range results {
			if !result.success {
				failed = append(failed, result.index)
				continue
			}
			teams[result.index].LogoSourceURL = result.sourceURL
			teams[result.index].Logo = result.logo
			teams[result.index].LogoContentType = result.contentType
		}
		pending = failed
		if ctx.Err() != nil {
			break
		}
	}
	return pending
}

// TeamLogo returns the locally persisted logo for a team code.
func (s *Service) TeamLogo(ctx context.Context, code string) ([]byte, string, bool, error) {
	code = strings.ToUpper(strings.TrimSpace(code))
	if !teamCodePattern.MatchString(code) {
		return nil, "", false, nil
	}
	teamStore, ok := s.store.(TeamStore)
	if !ok {
		return nil, "", false, nil
	}
	return teamStore.EsportsTeamLogo(ctx, code)
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
			Team1:     s.team(event.Match.Teams[0].Name, event.Match.Teams[0].Code),
			Team2:     s.team(event.Match.Teams[1].Name, event.Match.Teams[1].Code),
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
	if winner == "" && t1.Result.GameWins != t2.Result.GameWins {
		if t1.Result.GameWins > t2.Result.GameWins {
			winner = strings.ToUpper(t1.Code)
		} else {
			winner = strings.ToUpper(t2.Code)
		}
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

func (s *Service) team(name, code string) Team {
	if name == "" {
		name = "TBD"
	}
	if code == "" {
		code = "TBD"
	}
	code = strings.ToUpper(strings.TrimSpace(code))
	s.mu.Lock()
	hasLogo := s.teamLogoCodes[code]
	s.mu.Unlock()
	return Team{Name: name, Code: code, Image: teamImageURL(code, hasLogo)}
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

	parsedPrices := make([]float64, 2)
	for i, raw := range prices {
		price, err := strconv.ParseFloat(raw, 64)
		if err != nil || math.IsNaN(price) || math.IsInf(price, 0) || price < 0 || price > 1 {
			return
		}
		parsedPrices[i] = price
	}
	if math.Abs((parsedPrices[0]+parsedPrices[1])-1) > 0.05 {
		return
	}

	// A two-outcome moneyline is only usable when both prices can be assigned
	// atomically. Polymarket and lolesports occasionally use different names;
	// one unique match is sufficient because the other outcome must be the
	// opposing team. Ties stay unavailable instead of exposing a fake 0% side.
	directScore := boolScore(teamMatches(match.Team1, names[0])) + boolScore(teamMatches(match.Team2, names[1]))
	reverseScore := boolScore(teamMatches(match.Team2, names[0])) + boolScore(teamMatches(match.Team1, names[1]))
	if directScore == reverseScore || (directScore == 0 && reverseScore == 0) {
		return
	}
	team1Index, team2Index := 0, 1
	if reverseScore > directScore {
		team1Index, team2Index = 1, 0
	}

	// KoalaTrade is a paper exchange with integer-cent fills. Normalise the
	// paired Polymarket values together, then round them as one complementary
	// pair so displayed chances and executable prices both total exactly 100.
	total := parsedPrices[team1Index] + parsedPrices[team2Index]
	team1ProbBps := int64((parsedPrices[team1Index]/total)*10000 + 0.5)
	if team1ProbBps < 0 {
		team1ProbBps = 0
	} else if team1ProbBps > 10000 {
		team1ProbBps = 10000
	}
	team1PriceCents := int64(float64(team1ProbBps)/100 + 0.5)
	match.Team1.ProbBps = team1ProbBps
	match.Team1.PriceCents = team1PriceCents
	match.Team2.ProbBps = 10000 - team1ProbBps
	match.Team2.PriceCents = 100 - team1PriceCents
	match.HasOdds = true
	match.PolymarketURL = "https://polymarket.com/event/" + event.Slug
}

func boolScore(value bool) int {
	if value {
		return 1
	}
	return 0
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
