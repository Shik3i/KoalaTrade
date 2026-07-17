package server

import (
	"context"
	"log/slog"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/Shik3i/KoalaTrade/backend/internal/marketdata"
	"github.com/Shik3i/KoalaTrade/backend/internal/storage"
)

// openOrderEvalInterval is how often the engine re-checks pending Limit/Stop
// orders against the latest server quotes. Reads are cheap (DB-only), so a
// tight cadence keeps fills prompt without touching any provider.
const openOrderEvalInterval = 10 * time.Second

func (s *Server) handleListOpenOrders(w http.ResponseWriter, r *http.Request) {
	portfolioID, ok := s.resolvePortfolioID(r)
	if !ok {
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: "valid client id is required"})
		return
	}
	orders, err := s.db.OpenOrdersByPortfolio(r.Context(), portfolioID)
	if err != nil {
		writeJSON(w, http.StatusServiceUnavailable, errorResponse{Error: "open orders unavailable"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"openOrders": openOrderPayloads(orders)})
}

func (s *Server) handleCancelOpenOrder(w http.ResponseWriter, r *http.Request) {
	portfolioID, ok := s.resolvePortfolioID(r)
	if !ok {
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: "valid client id is required"})
		return
	}
	id := chi.URLParam(r, "id")
	if _, ok := validToken(id); !ok {
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: "valid order id is required"})
		return
	}

	s.tradeMu.Lock()
	deleted, err := s.db.DeleteOpenOrder(r.Context(), id, portfolioID)
	s.tradeMu.Unlock()
	if err != nil {
		writeJSON(w, http.StatusServiceUnavailable, errorResponse{Error: "could not cancel order"})
		return
	}
	if !deleted {
		writeJSON(w, http.StatusNotFound, errorResponse{Error: "order not found"})
		return
	}

	orders, _ := s.db.OpenOrdersByPortfolio(r.Context(), portfolioID)
	writeJSON(w, http.StatusOK, map[string]any{"openOrders": openOrderPayloads(orders)})
}

// resolvePortfolioID derives the portfolio id for a read/cancel request from the
// authenticated user (if any) or the client id header + the ?id= portfolio.
func (s *Server) resolvePortfolioID(r *http.Request) (string, bool) {
	clientPortfolioID, ok := validToken(r.URL.Query().Get("id"))
	if !ok {
		return "", false
	}
	if user, ok := s.currentUser(r); ok {
		if p, err := s.db.PortfolioByUser(r.Context(), user.ID, clientPortfolioID); err == nil {
			return p.ID, true
		}
	}
	clientID, ok := validToken(r.Header.Get(clientIDHeader))
	if !ok {
		return "", false
	}
	return syncPortfolioID(clientID, clientPortfolioID), true
}

// shouldTriggerOrder reports whether a pending order fills at the given price.
// Mirrors the client engine exactly:
//   - Limit buy  fills when price drops to/through the limit (buy at or better).
//   - Limit sell fills when price rises to/through the limit (sell at or better).
//   - Stop buy   fires when price rises to/through the stop (breakout/cover).
//   - Stop sell  fires when price falls to/through the stop (classic stop-loss).
func shouldTriggerOrder(order storage.OpenOrder, priceCents int64) bool {
	if priceCents <= 0 {
		return false
	}
	if order.OrderType == "limit" {
		if order.Side == "buy" {
			return priceCents <= order.TriggerPriceCents
		}
		return priceCents >= order.TriggerPriceCents
	}
	// stop
	if order.Side == "buy" {
		return priceCents >= order.TriggerPriceCents
	}
	return priceCents <= order.TriggerPriceCents
}

// StartOpenOrderEngine runs the background evaluator: on every tick it fills any
// pending Limit/Stop order whose trigger is met at the latest server price.
// This is what makes open orders execute even when the user's browser is closed.
func (s *Server) StartOpenOrderEngine(ctx context.Context, logger *slog.Logger) {
	go func() {
		ticker := time.NewTicker(openOrderEvalInterval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				s.evaluateOpenOrders(ctx, logger)
			}
		}
	}()
}

func (s *Server) evaluateOpenOrders(ctx context.Context, logger *slog.Logger) {
	orders, err := s.db.AllOpenOrders(ctx)
	if err != nil || len(orders) == 0 {
		return
	}

	// One quote lookup for the distinct assets referenced by pending orders.
	assetSet := make(map[string]struct{}, len(orders))
	for _, o := range orders {
		assetSet[o.AssetID] = struct{}{}
	}
	assetIDs := make([]string, 0, len(assetSet))
	for id := range assetSet {
		assetIDs = append(assetIDs, id)
	}
	quotes, err := s.marketData.Quotes(ctx, assetIDs)
	if err != nil {
		return
	}
	priceByAsset := make(map[string]int64, len(quotes))
	for _, q := range quotes {
		priceByAsset[q.AssetID] = q.PriceCents
	}

	for _, order := range orders {
		price := priceByAsset[order.AssetID]
		if !shouldTriggerOrder(order, price) {
			continue
		}
		s.fillOpenOrder(ctx, logger, order, price)
	}
}

// fillOpenOrder executes one triggered order against its portfolio, then removes
// it. An order that can't fill (e.g. insufficient cash/position) is dropped —
// not retried forever — so a saturated queue can't spin.
func (s *Server) fillOpenOrder(ctx context.Context, logger *slog.Logger, order storage.OpenOrder, priceCents int64) {
	s.tradeMu.Lock()
	defer s.tradeMu.Unlock()

	portfolio, err := s.db.Portfolio(ctx, order.PortfolioID)
	if err != nil {
		// Portfolio gone (deleted/reset) → discard the orphaned order.
		_ = s.db.DeleteOpenOrderByID(ctx, order.ID)
		return
	}

	asset := marketdata.Market{
		AssetID: order.AssetID,
		Symbol:  order.Symbol,
		Name:    order.Name,
		Kind:    order.Kind,
	}
	updated, _, tradeErr := applyMarketTrade(portfolio, asset, order.Side, order.QuantityMicro, priceCents, time.Now().UTC())
	if tradeErr != nil {
		logger.Info("open order dropped (could not fill)", "id", order.ID, "asset", order.AssetID, "reason", tradeErr.Error())
		_ = s.db.DeleteOpenOrderByID(ctx, order.ID)
		return
	}
	if err := s.db.UpsertPortfolio(ctx, updated); err != nil {
		logger.Warn("open order fill: save failed", "id", order.ID, "error", err)
		return // keep the order; retry next tick
	}
	_ = s.db.DeleteOpenOrderByID(ctx, order.ID)
	logger.Info("open order filled", "id", order.ID, "asset", order.AssetID, "side", order.Side, "price_cents", priceCents)
}
