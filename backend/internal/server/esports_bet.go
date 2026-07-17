package server

import (
	"context"
	"encoding/json"
	"log/slog"
	"math"
	"net/http"
	"strings"
	"time"

	"github.com/Shik3i/KoalaTrade/backend/internal/esports"
	"github.com/Shik3i/KoalaTrade/backend/internal/marketdata"
	"github.com/Shik3i/KoalaTrade/backend/internal/storage"
)

// betSettleInterval is how often the background settler pays out resolved bets
// for every holder, so a competition portfolio settles even if its owner never
// reopens the app.
const betSettleInterval = 60 * time.Second

const maxContractsPerBet = 1_000_000

type esportsBetRequest struct {
	PortfolioID string `json:"portfolioId"`
	MatchID     string `json:"matchId"`
	TeamCode    string `json:"teamCode"`
	Side        string `json:"side"`
	Contracts   int64  `json:"contracts"`
}

// handleEsportsBet buys/sells "Yes" contracts on a match winner, server-side:
// it prices the fill from the server's own (freshly refreshed) Polymarket odds,
// validates against the server-held portfolio, settles any already-resolved
// bets first, and returns the authoritative portfolio. A client can no longer
// self-report the odds or the payout.
func (s *Server) handleEsportsBet(w http.ResponseWriter, r *http.Request) {
	clientID, ok := validToken(r.Header.Get(clientIDHeader))
	if !ok {
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: "valid client id is required"})
		return
	}
	user, hasUser := s.currentUser(r)

	var req esportsBetRequest
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxOrderBodyBytes))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: "invalid bet payload"})
		return
	}
	if _, ok := validToken(req.PortfolioID); !ok {
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: "valid portfolio id is required"})
		return
	}
	if req.Side != "buy" && req.Side != "sell" {
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: "side must be buy or sell"})
		return
	}
	if req.Contracts <= 0 || req.Contracts > maxContractsPerBet {
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: "invalid contract count"})
		return
	}

	// Freshly refresh this match's odds so the fill is priced from the current
	// server-side quote (Polymarket has no rate limit, matching the client's
	// refresh-before-bet flow).
	match, err := s.esports.RefreshMatchOdds(r.Context(), req.MatchID)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: "match not available"})
		return
	}
	var team, other esports.Team
	switch req.TeamCode {
	case match.Team1.Code:
		team, other = match.Team1, match.Team2
	case match.Team2.Code:
		team, other = match.Team2, match.Team1
	default:
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: "team is not part of this match"})
		return
	}
	if team.PriceCents <= 0 {
		writeJSON(w, http.StatusConflict, errorResponse{Error: "no live odds for this team right now"})
		return
	}

	asset := marketdata.Market{
		AssetID:   esportsAssetID(match.ID, team.Code),
		Symbol:    team.Code,
		Name:      team.Name + " schlägt " + other.Code + " · " + match.League,
		Kind:      marketdata.AssetKindEvent,
		Source:    "polymarket",
		UpdatedAt: time.Now().UTC(),
	}
	// Register the dynamic event market so the positions/transactions FKs hold.
	if err := s.db.UpsertMarkets(r.Context(), []marketdata.Market{asset}); err != nil {
		writeJSON(w, http.StatusServiceUnavailable, errorResponse{Error: "market catalog unavailable"})
		return
	}

	s.tradeMu.Lock()
	defer s.tradeMu.Unlock()

	portfolio, err := s.loadOrCreatePortfolio(r, clientID, user, hasUser, req.PortfolioID)
	if err != nil {
		writeJSON(w, http.StatusServiceUnavailable, errorResponse{Error: "portfolio unavailable"})
		return
	}
	// Settle any resolved bets before applying a new one (frees cash/position).
	portfolio, _ = s.settlePortfolioBets(r.Context(), portfolio)

	updated, _, err := applyMarketTrade(portfolio, asset, req.Side, req.Contracts*quantityScale, team.PriceCents, time.Now().UTC())
	if err != nil {
		writeJSON(w, http.StatusUnprocessableEntity, errorResponse{Error: err.Error()})
		return
	}
	if err := s.db.UpsertPortfolio(r.Context(), updated); err != nil {
		writeJSON(w, http.StatusServiceUnavailable, errorResponse{Error: "could not save bet"})
		return
	}
	s.writeOrderResponse(w, r, updated)
}

func esportsAssetID(matchID, teamCode string) string {
	return "event:lol:" + matchID + ":" + teamCode
}

// parseEventAsset extracts the match id and team code from an event asset id of
// the form event:lol:<matchId>:<teamCode>.
func parseEventAsset(assetID string) (matchID, teamCode string, ok bool) {
	parts := strings.Split(assetID, ":")
	if len(parts) < 4 || parts[0] != "event" || parts[1] != "lol" {
		return "", "", false
	}
	return parts[2], parts[3], true
}

// settleEventPositions pays out any event position whose match has resolved: a
// winning "Yes" contract pays 100¢ each, a loser expires at 0¢. Returns the
// updated portfolio and whether anything settled.
func settleEventPositions(p storage.Portfolio, results map[string]esports.Result, now time.Time) (storage.Portfolio, bool) {
	kept := make([]storage.PortfolioPosition, 0, len(p.Positions))
	txns := p.Transactions
	changed := false

	for _, pos := range p.Positions {
		matchID, teamCode, ok := parseEventAsset(pos.AssetID)
		if !ok {
			kept = append(kept, pos)
			continue
		}
		result, resolved := results[matchID]
		if !resolved {
			kept = append(kept, pos)
			continue
		}
		payoutCents := int64(0)
		if strings.EqualFold(result.WinnerCode, teamCode) {
			payoutCents = 100
		}
		contracts := float64(pos.QuantityMicro) / quantityScale
		proceeds := int64(math.Round(contracts * float64(payoutCents)))
		p.CashCents += proceeds
		txns = append([]storage.PortfolioTransaction{{
			ID:            newTransactionID(),
			AssetID:       pos.AssetID,
			Symbol:        pos.Symbol,
			Side:          "sell",
			QuantityMicro: pos.QuantityMicro,
			PriceCents:    payoutCents,
			FeeCents:      0,
			Status:        "synced",
			CreatedAt:     now,
		}}, txns...)
		changed = true
	}

	if !changed {
		return p, false
	}
	p.Positions = kept
	p.Transactions = txns
	p.UpdatedAt = now
	return p, true
}

// settlePortfolioBets settles a single portfolio against the currently known
// match results.
func (s *Server) settlePortfolioBets(ctx context.Context, portfolio storage.Portfolio) (storage.Portfolio, bool) {
	matchIDs := eventMatchIDs(portfolio)
	if len(matchIDs) == 0 {
		return portfolio, false
	}
	results := s.esports.Results(ctx, matchIDs)
	if len(results) == 0 {
		return portfolio, false
	}
	byMatch := make(map[string]esports.Result, len(results))
	for _, result := range results {
		byMatch[result.MatchID] = result
	}
	return settleEventPositions(portfolio, byMatch, time.Now().UTC())
}

func eventMatchIDs(portfolio storage.Portfolio) []string {
	seen := make(map[string]struct{})
	ids := make([]string, 0)
	for _, pos := range portfolio.Positions {
		if matchID, _, ok := parseEventAsset(pos.AssetID); ok {
			if _, dup := seen[matchID]; !dup {
				seen[matchID] = struct{}{}
				ids = append(ids, matchID)
			}
		}
	}
	return ids
}

// StartBetSettler periodically settles resolved bets for every holder so a
// competition portfolio's equity stays correct even when its owner is offline.
func (s *Server) StartBetSettler(ctx context.Context, logger *slog.Logger) {
	go func() {
		ticker := time.NewTicker(betSettleInterval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				s.settleAllBets(ctx, logger)
			}
		}
	}()
}

func (s *Server) settleAllBets(ctx context.Context, logger *slog.Logger) {
	ids, err := s.db.PortfolioIDsWithEventPositions(ctx)
	if err != nil || len(ids) == 0 {
		return
	}
	for _, id := range ids {
		s.tradeMu.Lock()
		portfolio, err := s.db.Portfolio(ctx, id)
		if err == nil {
			if updated, changed := s.settlePortfolioBets(ctx, portfolio); changed {
				if err := s.db.UpsertPortfolio(ctx, updated); err != nil {
					logger.Warn("bet settle: save failed", "portfolio", id, "error", err)
				} else {
					logger.Info("bets settled", "portfolio", id)
				}
			}
		}
		s.tradeMu.Unlock()
	}
}
