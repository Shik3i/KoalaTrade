package server

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"net/http"
	"time"

	"github.com/Shik3i/KoalaTrade/backend/internal/marketdata"
	"github.com/Shik3i/KoalaTrade/backend/internal/storage"
)

// orderFeeBPS is the simulated per-trade commission, in basis points. It is the
// authoritative fee: the server, not the client, decides what a trade costs.
const orderFeeBPS = 8

const maxOrderBodyBytes = 4 << 10

type orderRequest struct {
	PortfolioID string  `json:"portfolioId"`
	AssetID     string  `json:"assetId"`
	Side        string  `json:"side"`
	Quantity    float64 `json:"quantity"`
	OrderType   string  `json:"orderType"` // currently only "market"; limit/stop are server-side open orders (phase 2)
}

// handleCreateOrder executes a market order server-side: it loads the caller's
// server-held portfolio, fills at the *server's* quote price (never a
// client-supplied price), validates cash/position, persists the result, and
// returns the updated portfolio. This is the anti-cheat core of competitive
// mode — a client can no longer fabricate prices, cash, or positions.
func (s *Server) handleCreateOrder(w http.ResponseWriter, r *http.Request) {
	clientID, ok := validToken(r.Header.Get(clientIDHeader))
	if !ok {
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: "valid client id is required"})
		return
	}
	user, hasUser := s.currentUser(r)

	var req orderRequest
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxOrderBodyBytes))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: "invalid order payload"})
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
	if req.OrderType != "" && req.OrderType != "market" {
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: "only market orders are supported here"})
		return
	}
	quantityMicro, err := quantityToMicro(req.Quantity)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: "quantity must be greater than zero"})
		return
	}

	// Resolve the tradable asset from the catalogue (server owns symbol/name/kind).
	asset, err := s.marketAsset(r, req.AssetID)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: err.Error()})
		return
	}

	// Fill price comes from the server's own quote store. If the poller hasn't
	// refreshed this asset yet, do a single bounded live fetch (one symbol, on
	// explicit user action — never a stampede) so a trade always fills at a real
	// current price rather than a stale or zero one.
	priceCents := s.serverQuotePrice(r, req.AssetID)
	if priceCents <= 0 {
		writeJSON(w, http.StatusConflict, errorResponse{Error: "no live price available for this asset right now"})
		return
	}

	portfolio, err := s.loadOrCreatePortfolio(r, clientID, user, hasUser, req.PortfolioID)
	if err != nil {
		writeJSON(w, http.StatusServiceUnavailable, errorResponse{Error: "portfolio unavailable"})
		return
	}

	now := time.Now().UTC()
	updated, _, err := applyMarketTrade(portfolio, asset, req.Side, quantityMicro, priceCents, now)
	if err != nil {
		writeJSON(w, http.StatusUnprocessableEntity, errorResponse{Error: err.Error()})
		return
	}

	if err := s.db.UpsertPortfolio(r.Context(), updated); err != nil {
		writeJSON(w, http.StatusServiceUnavailable, errorResponse{Error: "could not save order"})
		return
	}

	payload, err := portfolioToSyncPayload(updated)
	if err != nil {
		writeJSON(w, http.StatusServiceUnavailable, errorResponse{Error: "could not serialize portfolio"})
		return
	}
	writeJSON(w, http.StatusOK, portfolioSyncResponse{Portfolio: payload, SyncedAt: now})
}

// marketAsset returns the catalogue entry for a tradable asset id, or an error
// if it is unknown. eSports event markets are handled by a separate flow.
func (s *Server) marketAsset(r *http.Request, assetID string) (marketdata.Market, error) {
	markets, err := s.marketData.Markets(r.Context())
	if err != nil {
		return marketdata.Market{}, errors.New("market catalog unavailable")
	}
	for _, m := range markets {
		if m.AssetID == assetID {
			return m, nil
		}
	}
	return marketdata.Market{}, fmt.Errorf("unknown asset id %q", assetID)
}

// serverQuotePrice returns the current server-side price for one asset, doing a
// single bounded live refresh if the stored quote is missing/zero.
func (s *Server) serverQuotePrice(r *http.Request, assetID string) int64 {
	if quotes, err := s.marketData.Quotes(r.Context(), []string{assetID}); err == nil {
		for _, q := range quotes {
			if q.AssetID == assetID && q.PriceCents > 0 {
				return q.PriceCents
			}
		}
	}
	if fresh, err := s.marketData.Refresh(r.Context(), []string{assetID}); err == nil {
		for _, q := range fresh {
			if q.AssetID == assetID && q.PriceCents > 0 {
				return q.PriceCents
			}
		}
	}
	return 0
}

// loadOrCreatePortfolio fetches the caller's server portfolio (by user when
// authenticated, else by client id), creating a fresh one seeded with the
// configured starting cash if none exists yet.
func (s *Server) loadOrCreatePortfolio(r *http.Request, clientID string, user sessionUser, hasUser bool, portfolioID string) (storage.Portfolio, error) {
	var (
		portfolio storage.Portfolio
		err       error
	)
	if hasUser {
		portfolio, err = s.db.PortfolioByUser(r.Context(), user.ID, portfolioID)
	} else {
		portfolio, err = s.db.PortfolioByClient(r.Context(), clientID, portfolioID)
	}
	if err == nil {
		if hasUser && portfolio.UserID == "" {
			portfolio.UserID = user.ID
		}
		return portfolio, nil
	}

	now := time.Now().UTC()
	fresh := storage.Portfolio{
		ID:                syncPortfolioID(clientID, portfolioID),
		ClientID:          clientID,
		ClientPortfolioID: portfolioID,
		SchemaVersion:     1,
		StartingCashCents: s.cfg.StartingCashCents,
		CashCents:         s.cfg.StartingCashCents,
		CreatedAt:         now,
		UpdatedAt:         now,
	}
	if hasUser {
		fresh.UserID = user.ID
	}
	return fresh, nil
}

// applyMarketTrade is the server-authoritative port of the client's applyTrade:
// it validates and applies a filled market order to a portfolio, returning the
// updated portfolio and the recorded transaction. Quantities are in micro-units
// (1e6); money is in integer cents. Mirrors the client's arithmetic exactly so
// the migration produces identical numbers.
func applyMarketTrade(
	p storage.Portfolio,
	asset marketdata.Market,
	side string,
	quantityMicro, priceCents int64,
	now time.Time,
) (storage.Portfolio, storage.PortfolioTransaction, error) {
	if quantityMicro <= 0 {
		return p, storage.PortfolioTransaction{}, errors.New("quantity must be greater than zero")
	}
	if priceCents <= 0 {
		return p, storage.PortfolioTransaction{}, errors.New("price must be greater than zero")
	}

	quantity := float64(quantityMicro) / quantityScale
	grossCents := int64(math.Round(quantity * float64(priceCents)))
	feeCents := grossCents * orderFeeBPS / 10_000
	if feeCents < 0 {
		feeCents = 0
	}

	positions := make([]storage.PortfolioPosition, len(p.Positions))
	copy(positions, p.Positions)
	idx := -1
	for i := range positions {
		if positions[i].AssetID == asset.AssetID {
			idx = i
			break
		}
	}

	switch side {
	case "buy":
		total := grossCents + feeCents
		if total > p.CashCents {
			return p, storage.PortfolioTransaction{}, errors.New("not enough cash for this order")
		}
		p.CashCents -= total
		if idx >= 0 {
			existing := positions[idx]
			oldCost := float64(existing.QuantityMicro) / quantityScale * float64(existing.AverageCostCents)
			newQtyMicro := existing.QuantityMicro + quantityMicro
			newQty := float64(newQtyMicro) / quantityScale
			existing.QuantityMicro = newQtyMicro
			existing.AverageCostCents = int64(math.Round((oldCost + float64(grossCents)) / newQty))
			existing.LastPriceCents = priceCents
			existing.UpdatedAt = now
			positions[idx] = existing
		} else {
			positions = append(positions, storage.PortfolioPosition{
				AssetID:          asset.AssetID,
				Symbol:           asset.Symbol,
				Name:             asset.Name,
				Kind:             asset.Kind,
				QuantityMicro:    quantityMicro,
				AverageCostCents: priceCents,
				LastPriceCents:   priceCents,
				UpdatedAt:        now,
			})
		}
	case "sell":
		if idx < 0 || positions[idx].QuantityMicro < quantityMicro {
			return p, storage.PortfolioTransaction{}, errors.New("not enough position size for this sell")
		}
		p.CashCents += grossCents - feeCents
		newQtyMicro := positions[idx].QuantityMicro - quantityMicro
		if newQtyMicro <= 0 {
			positions = append(positions[:idx], positions[idx+1:]...)
		} else {
			positions[idx].QuantityMicro = newQtyMicro
			positions[idx].LastPriceCents = priceCents
			positions[idx].UpdatedAt = now
		}
	default:
		return p, storage.PortfolioTransaction{}, errors.New("invalid order side")
	}

	txn := storage.PortfolioTransaction{
		ID:            newTransactionID(),
		AssetID:       asset.AssetID,
		Symbol:        asset.Symbol,
		Side:          side,
		QuantityMicro: quantityMicro,
		PriceCents:    priceCents,
		FeeCents:      feeCents,
		Status:        "synced",
		CreatedAt:     now,
	}

	p.Positions = positions
	p.Transactions = append([]storage.PortfolioTransaction{txn}, p.Transactions...)
	p.UpdatedAt = now
	return p, txn, nil
}

func newTransactionID() string {
	var b [16]byte
	_, _ = rand.Read(b[:])
	return "txn-" + hex.EncodeToString(b[:])
}
