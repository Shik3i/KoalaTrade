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
	PortfolioID       string  `json:"portfolioId"`
	AssetID           string  `json:"assetId"`
	Side              string  `json:"side"`
	Quantity          float64 `json:"quantity"`
	OrderType         string  `json:"orderType"`         // "market" fills now; "limit"/"stop" queue a server-side open order
	TriggerPriceCents int64   `json:"triggerPriceCents"` // required for limit/stop
}

// orderResponse is the unified reply for the orders endpoint: the (possibly
// updated) portfolio plus the caller's current open-order queue.
type orderResponse struct {
	Portfolio  portfolioSyncRequest `json:"portfolio"`
	OpenOrders []openOrderPayload   `json:"openOrders"`
	SyncedAt   time.Time            `json:"syncedAt"`
}

type openOrderPayload struct {
	ID                string               `json:"id"`
	AssetID           string               `json:"assetId"`
	Symbol            string               `json:"symbol"`
	Name              string               `json:"name"`
	Kind              marketdata.AssetKind `json:"kind"`
	Side              string               `json:"side"`
	OrderType         string               `json:"orderType"`
	Quantity          float64              `json:"quantity"`
	TriggerPriceCents int64                `json:"triggerPriceCents"`
	CreatedAt         string               `json:"createdAt"`
}

func openOrderPayloads(orders []storage.OpenOrder) []openOrderPayload {
	out := make([]openOrderPayload, 0, len(orders))
	for _, o := range orders {
		out = append(out, openOrderPayload{
			ID:                o.ID,
			AssetID:           o.AssetID,
			Symbol:            o.Symbol,
			Name:              o.Name,
			Kind:              o.Kind,
			Side:              o.Side,
			OrderType:         o.OrderType,
			Quantity:          float64(o.QuantityMicro) / quantityScale,
			TriggerPriceCents: o.TriggerPriceCents,
			CreatedAt:         o.CreatedAt.Format(time.RFC3339Nano),
		})
	}
	return out
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
	orderType := req.OrderType
	if orderType == "" {
		orderType = "market"
	}
	if orderType != "market" && orderType != "limit" && orderType != "stop" {
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: "invalid order type"})
		return
	}
	quantityMicro, err := quantityToMicro(req.Quantity)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: "quantity must be greater than zero"})
		return
	}

	// Resolve the tradable asset from the catalogue (server owns symbol/name/kind).
	// This also upserts the catalogue into the assets table, satisfying the
	// foreign keys used by positions and open orders.
	asset, err := s.marketAsset(r, req.AssetID)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: err.Error()})
		return
	}

	if orderType == "market" {
		s.executeMarketOrder(w, r, clientID, user, hasUser, req.PortfolioID, asset, req.Side, quantityMicro)
		return
	}
	s.queueOpenOrder(w, r, clientID, user, hasUser, req.PortfolioID, asset, req.Side, orderType, quantityMicro, req.TriggerPriceCents)
}

func (s *Server) executeMarketOrder(w http.ResponseWriter, r *http.Request, clientID string, user sessionUser, hasUser bool, portfolioID string, asset marketdata.Market, side string, quantityMicro int64) {
	// Fill price comes from the server's own quote store. If the poller hasn't
	// refreshed this asset yet, do a single bounded live fetch (one symbol, on
	// explicit user action — never a stampede) so a trade always fills at a real
	// current price rather than a stale or zero one.
	priceCents := s.serverQuotePrice(r, asset.AssetID)
	if priceCents <= 0 {
		writeJSON(w, http.StatusConflict, errorResponse{Error: "no live price available for this asset right now"})
		return
	}

	s.tradeMu.Lock()
	defer s.tradeMu.Unlock()

	portfolio, err := s.loadOrCreatePortfolio(r, clientID, user, hasUser, portfolioID)
	if err != nil {
		writeJSON(w, http.StatusServiceUnavailable, errorResponse{Error: "portfolio unavailable"})
		return
	}

	updated, _, err := applyMarketTrade(portfolio, asset, side, quantityMicro, priceCents, time.Now().UTC())
	if err != nil {
		writeJSON(w, http.StatusUnprocessableEntity, errorResponse{Error: err.Error()})
		return
	}
	if err := s.db.UpsertPortfolio(r.Context(), updated); err != nil {
		writeJSON(w, http.StatusServiceUnavailable, errorResponse{Error: "could not save order"})
		return
	}
	s.writeOrderResponse(w, r, updated)
}

func (s *Server) queueOpenOrder(w http.ResponseWriter, r *http.Request, clientID string, user sessionUser, hasUser bool, portfolioID string, asset marketdata.Market, side, orderType string, quantityMicro, triggerPriceCents int64) {
	if triggerPriceCents <= 0 {
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: "trigger price must be greater than zero"})
		return
	}

	s.tradeMu.Lock()
	defer s.tradeMu.Unlock()

	portfolio, err := s.loadOrCreatePortfolio(r, clientID, user, hasUser, portfolioID)
	if err != nil {
		writeJSON(w, http.StatusServiceUnavailable, errorResponse{Error: "portfolio unavailable"})
		return
	}
	// Persist the portfolio row so the open_orders foreign key holds (a freshly
	// created portfolio isn't in the DB yet).
	if err := s.db.UpsertPortfolio(r.Context(), portfolio); err != nil {
		writeJSON(w, http.StatusServiceUnavailable, errorResponse{Error: "portfolio unavailable"})
		return
	}

	order := storage.OpenOrder{
		ID:                newOpenOrderID(),
		PortfolioID:       portfolio.ID,
		AssetID:           asset.AssetID,
		Symbol:            asset.Symbol,
		Name:              asset.Name,
		Kind:              asset.Kind,
		Side:              side,
		OrderType:         orderType,
		QuantityMicro:     quantityMicro,
		TriggerPriceCents: triggerPriceCents,
		CreatedAt:         time.Now().UTC(),
	}
	if err := s.db.CreateOpenOrder(r.Context(), order); err != nil {
		writeJSON(w, http.StatusServiceUnavailable, errorResponse{Error: "could not save order"})
		return
	}
	s.writeOrderResponse(w, r, portfolio)
}

// writeOrderResponse returns the portfolio plus the caller's current open-order
// queue, so a single round-trip keeps the client fully in sync.
func (s *Server) writeOrderResponse(w http.ResponseWriter, r *http.Request, portfolio storage.Portfolio) {
	payload, err := portfolioToSyncPayload(portfolio)
	if err != nil {
		writeJSON(w, http.StatusServiceUnavailable, errorResponse{Error: "could not serialize portfolio"})
		return
	}
	orders, _ := s.db.OpenOrdersByPortfolio(r.Context(), portfolio.ID)
	writeJSON(w, http.StatusOK, orderResponse{
		Portfolio:  payload,
		OpenOrders: openOrderPayloads(orders),
		SyncedAt:   time.Now().UTC(),
	})
}

func newOpenOrderID() string {
	var b [16]byte
	_, _ = rand.Read(b[:])
	return "oo-" + hex.EncodeToString(b[:])
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
