package server

import (
	"math"
	"net/http"
	"sort"

	"github.com/Shik3i/KoalaTrade/backend/internal/storage"
)

const leaderboardLimit = 100

type leaderboardEntry struct {
	Rank             int    `json:"rank"`
	DisplayName      string `json:"displayName"`
	TotalEquityCents int64  `json:"totalEquityCents"`
	TotalReturnBps   int64  `json:"totalReturnBps"`
	IsYou            bool   `json:"isYou"`
}

// handleLeaderboard ranks every account by the current value of its competition
// portfolio, valued at the server's own quotes — so standings can't be gamed by
// a client. Anonymous practice portfolios are excluded (accounts only).
func (s *Server) handleLeaderboard(w http.ResponseWriter, r *http.Request) {
	portfolios, err := s.db.LeaderboardPortfolios(r.Context())
	if err != nil {
		writeJSON(w, http.StatusServiceUnavailable, errorResponse{Error: "leaderboard unavailable"})
		return
	}

	// One quote lookup for every distinct tradable asset held across all ranked
	// portfolios (event positions are valued at their stored odds price).
	assetSet := make(map[string]struct{})
	for _, lp := range portfolios {
		for _, pos := range lp.Positions {
			assetSet[pos.AssetID] = struct{}{}
		}
	}
	priceByAsset := make(map[string]int64, len(assetSet))
	if len(assetSet) > 0 {
		assetIDs := make([]string, 0, len(assetSet))
		for id := range assetSet {
			assetIDs = append(assetIDs, id)
		}
		if quotes, qErr := s.marketData.Quotes(r.Context(), assetIDs); qErr == nil {
			for _, q := range quotes {
				priceByAsset[q.AssetID] = q.PriceCents
			}
		}
	}

	var myID string
	if user, ok := s.currentUser(r); ok {
		myID = user.ID
	}

	entries := make([]leaderboardEntry, 0, len(portfolios))
	for _, lp := range portfolios {
		equity := lp.CashCents
		for _, pos := range lp.Positions {
			price := pos.LastPriceCents
			if live, ok := priceByAsset[pos.AssetID]; ok && live > 0 {
				price = live
			}
			equity += int64(math.Round(float64(pos.QuantityMicro) / quantityScale * float64(price)))
		}
		var returnBps int64
		if lp.StartingCashCents > 0 {
			returnBps = (equity - lp.StartingCashCents) * 10_000 / lp.StartingCashCents
		}
		entries = append(entries, leaderboardEntry{
			DisplayName:      leaderboardName(lp),
			TotalEquityCents: equity,
			TotalReturnBps:   returnBps,
			IsYou:            myID != "" && lp.UserID == myID,
		})
	}

	sort.SliceStable(entries, func(i, j int) bool {
		if entries[i].TotalReturnBps != entries[j].TotalReturnBps {
			return entries[i].TotalReturnBps > entries[j].TotalReturnBps
		}
		return entries[i].TotalEquityCents > entries[j].TotalEquityCents
	})

	for i := range entries {
		entries[i].Rank = i + 1
	}
	if len(entries) > leaderboardLimit {
		entries = entries[:leaderboardLimit]
	}

	writeJSON(w, http.StatusOK, map[string]any{"leaderboard": entries})
}

func leaderboardName(lp storage.LeaderboardPortfolio) string {
	if lp.DisplayName != "" {
		return lp.DisplayName
	}
	if lp.Username != "" {
		return lp.Username
	}
	return "Anonym"
}
