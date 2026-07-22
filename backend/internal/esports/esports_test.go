package esports

import (
	"context"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/Shik3i/KoalaTrade/backend/internal/storage"
)

func TestSlugDiagnosticUsesTemporaryMapping(t *testing.T) {
	service := NewService("", "", "https://polymarket.test", time.Second, time.Minute, &slugTestStore{})
	service.http = &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		if !strings.Contains(r.URL.RawQuery, "lol-es1-g2-2026-07-02") {
			return jsonResponse(`[]`), nil
		}
		return jsonResponse(`[{
			"slug":"lol-es1-g2-2026-07-02",
			"markets":[{
				"question":"Who will win?",
				"groupItemTitle":"Match Winner",
				"sportsMarketType":"moneyline",
				"outcomes":"[\"Eintracht Spandau\",\"G2 Esports\"]",
				"outcomePrices":"[\"0.42\",\"0.58\"]"
			}]
		}]`), nil
	})}
	service.cache = []Match{{
		ID:        "match-1",
		StartTime: time.Date(2026, 7, 2, 18, 0, 0, 0, time.UTC),
		League:    "LEC",
		Team1:     Team{Name: "Eintracht Spandau", Code: "EINS"},
		Team2:     Team{Name: "G2 Esports", Code: "G2"},
	}}

	diag, err := service.SlugDiagnostic(context.Background(), "match-1", "EINS", "ES1", true)
	if err != nil {
		t.Fatalf("slug diagnostic: %v", err)
	}
	if !diag.Found {
		t.Fatal("expected diagnostic to find mapped Polymarket event")
	}
	if diag.EventSlug != "lol-es1-g2-2026-07-02" {
		t.Fatalf("expected mapped event slug, got %q", diag.EventSlug)
	}
}

func TestTeamsPersistAndServeLocalLogos(t *testing.T) {
	store := &teamTestStore{}
	service := NewService("test-key", "https://lolesports.test", "", time.Second, time.Minute, store)
	requests := 0
	service.http = &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		requests++
		switch r.URL.Path {
		case "/persisted/gw/getTeams":
			return jsonResponse(`{"data":{"teams":[{"name":"G2 Esports","code":"G2","image":"http://logo.test/g2.png","status":"active","homeLeague":{"name":"LEC"}}]}}`), nil
		case "/g2.png":
			return &http.Response{
				StatusCode: http.StatusOK,
				Header:     http.Header{"Content-Type": []string{"image/png"}},
				Body:       io.NopCloser(strings.NewReader("png-bytes")),
			}, nil
		default:
			return nil, errors.New("unexpected request: " + r.URL.String())
		}
	})}

	teams, err := service.Teams(context.Background())
	if err != nil {
		t.Fatalf("teams: %v", err)
	}
	if len(teams) != 1 || teams[0].Image != "/api/esports/teams/G2/logo" {
		t.Fatalf("expected local team logo URL, got %+v", teams)
	}
	if requests != 2 {
		t.Fatalf("expected team and logo requests, got %d", requests)
	}
	if _, _, ok, err := service.TeamLogo(context.Background(), "g2"); err != nil || !ok {
		t.Fatalf("expected stored logo, ok=%v err=%v", ok, err)
	}

	if _, err := service.Teams(context.Background()); err != nil {
		t.Fatalf("cached teams: %v", err)
	}
	if requests != 2 {
		t.Fatalf("expected weekly cache to avoid another upstream fetch, got %d requests", requests)
	}
}

func TestTeamsFallsBackAfterEmptySnapshotFailure(t *testing.T) {
	store := &teamTestStore{}
	service := NewService("test-key", "https://lolesports.test", "", time.Second, time.Minute, store)
	requests := 0
	service.http = &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
		requests++
		return nil, errors.New("teams upstream unavailable")
	})}

	teams, err := service.Teams(context.Background())
	if err != nil || len(teams) == 0 {
		t.Fatalf("expected fallback teams, got len=%d err=%v", len(teams), err)
	}
	if _, err := service.Teams(context.Background()); err != nil {
		t.Fatalf("fallback teams second read: %v", err)
	}
	if requests != 1 {
		t.Fatalf("expected recent failed snapshot to be suppressed, got %d upstream requests", requests)
	}
}

func TestTeamsDoNotMarkIncompleteLogoSnapshotFresh(t *testing.T) {
	store := &teamTestStore{}
	service := NewService("test-key", "https://lolesports.test", "", time.Second, time.Minute, store)
	service.http = &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		switch r.URL.Path {
		case "/persisted/gw/getTeams":
			return jsonResponse(`{"data":{"teams":[{"name":"G2 Esports","code":"G2","image":"http://logo.test/g2.png","status":"active","homeLeague":{"name":"LEC"}},{"name":"FNC","code":"FNC","image":"http://logo.test/fnc.png","status":"active","homeLeague":{"name":"LEC"}}]}}`), nil
		case "/g2.png":
			return &http.Response{StatusCode: http.StatusOK, Header: http.Header{"Content-Type": []string{"image/png"}}, Body: io.NopCloser(strings.NewReader("png-bytes"))}, nil
		default:
			return nil, errors.New("logo unavailable")
		}
	})}

	teams, err := service.Teams(context.Background())
	if err != nil || len(teams) != 2 {
		t.Fatalf("expected metadata with partial logos, got teams=%+v err=%v", teams, err)
	}
	if !service.teamsSyncAt.IsZero() {
		t.Fatal("expected incomplete logo snapshot to remain due for retry")
	}
}

func TestMatchDetailsParseGamesAndVideos(t *testing.T) {
	service := NewService("test-key", "https://lolesports.test", "", time.Second, time.Minute, &slugTestStore{})
	service.http = &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		if r.URL.Path != "/persisted/gw/getEventDetails" || r.URL.Query().Get("id") != "match-42" {
			return nil, errors.New("unexpected request: " + r.URL.String())
		}
		return jsonResponse(`{"data":{"event":{"state":"scheduled","streams":[{"parameter":"lolesports","provider":"twitch","locale":"en-US"}],"match":{"teams":[{"code":"G2","result":{"gameWins":2,"outcome":"win"}},{"code":"FNC","result":{"gameWins":1,"outcome":"loss"}}],"games":[{"id":"game-1","number":1,"state":"completed","vods":[{"parameter":"abc123","provider":"youtube","locale":"en-US"}]}]}}}}`), nil
	})}

	details, err := service.MatchDetails(context.Background(), "match-42")
	if err != nil {
		t.Fatalf("match details: %v", err)
	}
	if details.State != "completed" || details.Team1Score != 2 || details.Team2Score != 1 || len(details.Games) != 1 || len(details.Videos) != 2 {
		t.Fatalf("unexpected match details: %+v", details)
	}
	if details.Videos[0].URL != "https://www.youtube.com/watch?v=abc123" {
		t.Fatalf("unexpected VOD URL: %+v", details.Videos)
	}
}

func TestResultsFallsBackToCompletedMatchDetails(t *testing.T) {
	service := NewService("test-key", "https://lolesports.test", "", time.Second, time.Minute, &slugTestStore{})
	service.http = &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		return jsonResponse(`{"data":{"event":{"state":null,"match":{"teams":[{"code":"VER","result":{"gameWins":2}},{"code":"ABV","result":{"gameWins":0}}],"games":[{"id":"game-1","number":1,"state":"completed"},{"id":"game-2","number":2,"state":"completed"},{"id":"game-3","number":3,"state":"unneeded"}]}}}}`), nil
	})}

	results := service.Results(context.Background(), []string{"match-42"})
	if len(results) != 1 || results[0].WinnerCode != "VER" {
		t.Fatalf("expected details fallback winner VER, got %+v", results)
	}
}

func TestResultFromEventUsesGameWinsWhenOutcomeMissing(t *testing.T) {
	result, ok := resultFromEvent("match-42", time.Now().UTC(), scheduleTeam{
		Code: "VER", Result: struct {
			Outcome  string `json:"outcome"`
			GameWins int    `json:"gameWins"`
		}{GameWins: 2},
	}, scheduleTeam{
		Code: "ABV", Result: struct {
			Outcome  string `json:"outcome"`
			GameWins int    `json:"gameWins"`
		}{GameWins: 0},
	})
	if !ok || result.WinnerCode != "VER" {
		t.Fatalf("expected game-score winner VER, got %+v ok=%v", result, ok)
	}
}

func TestMatchDetailsUsesStaleCacheOnUpstreamFailure(t *testing.T) {
	store, err := storage.OpenSQLite(t.TempDir() + "/koalatrade.db")
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })
	if err := store.UpsertEsportsMatchDetails(context.Background(), storage.EsportsMatchDetail{
		MatchID: "match-42", State: "completed", Team1Code: "VER", Team2Code: "ABV",
		Team1Score: 2, Team2Score: 0, FetchedAt: "2026-07-20T12:00:00Z",
	}, nil, nil); err != nil {
		t.Fatalf("store stale details: %v", err)
	}

	service := NewService("test-key", "https://lolesports.test", "", time.Second, time.Minute, store)
	service.http = &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
		return nil, errors.New("upstream unavailable")
	})}
	details, err := service.MatchDetails(context.Background(), "match-42")
	if err != nil || details.Team1Score != 2 || details.State != "completed" {
		t.Fatalf("expected stale details fallback, got %+v err=%v", details, err)
	}
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) {
	return f(r)
}

func jsonResponse(body string) *http.Response {
	return &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"application/json"}},
		Body:       io.NopCloser(strings.NewReader(body)),
	}
}

type slugTestStore struct{}

func (s *slugTestStore) GetMeta(ctx context.Context, key string) (string, bool, error) {
	return "", false, ctx.Err()
}

func (s *slugTestStore) SetMeta(ctx context.Context, key, value string) error {
	return ctx.Err()
}

func (s *slugTestStore) TeamMappingsMap(ctx context.Context) (map[string]string, error) {
	return nil, ctx.Err()
}

type teamTestStore struct {
	teams []storage.EsportsTeam
}

func (s *teamTestStore) GetMeta(ctx context.Context, key string) (string, bool, error) {
	return "", false, nil
}

func (s *teamTestStore) SetMeta(ctx context.Context, key, value string) error {
	return nil
}

func (s *teamTestStore) TeamMappingsMap(ctx context.Context) (map[string]string, error) {
	return nil, nil
}

func (s *teamTestStore) LoadEsportsTeams(ctx context.Context) ([]storage.EsportsTeam, error) {
	return append([]storage.EsportsTeam(nil), s.teams...), nil
}

func (s *teamTestStore) UpsertEsportsTeams(ctx context.Context, teams []storage.EsportsTeam) error {
	s.teams = append([]storage.EsportsTeam(nil), teams...)
	return nil
}

func (s *teamTestStore) EsportsTeamLogo(ctx context.Context, code string) ([]byte, string, bool, error) {
	for _, team := range s.teams {
		if team.Code == strings.ToUpper(code) {
			return team.Logo, team.LogoContentType, len(team.Logo) > 0, nil
		}
	}
	return nil, "", false, nil
}
