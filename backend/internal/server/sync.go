package server

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"net/http"
	"strings"
	"time"

	"github.com/Shik3i/KoalaTrade/backend/internal/marketdata"
	"github.com/Shik3i/KoalaTrade/backend/internal/storage"
)

const (
	clientIDHeader      = "X-Koala-Client-ID"
	maxSyncBodyBytes    = 1 << 20
	maxSyncPositions    = 200
	maxSyncTransactions = 1000
	quantityScale       = 1_000_000
)

type portfolioSyncRequest struct {
	ID                string            `json:"id"`
	SchemaVersion     int64             `json:"schemaVersion"`
	StartingCashCents int64             `json:"startingCashCents"`
	CashCents         int64             `json:"cashCents"`
	Positions         []syncPosition    `json:"positions"`
	Transactions      []syncTransaction `json:"transactions"`
	CreatedAt         string            `json:"createdAt"`
	UpdatedAt         string            `json:"updatedAt"`
}

type portfolioSyncResponse struct {
	Portfolio portfolioSyncRequest `json:"portfolio"`
	SyncedAt  time.Time            `json:"syncedAt"`
}

type syncPosition struct {
	AssetID          string               `json:"assetId"`
	Symbol           string               `json:"symbol"`
	Name             string               `json:"name"`
	Kind             marketdata.AssetKind `json:"kind"`
	Quantity         float64              `json:"quantity"`
	AverageCostCents int64                `json:"averageCostCents"`
	LastPriceCents   int64                `json:"lastPriceCents"`
	UpdatedAt        string               `json:"updatedAt"`
}

type syncTransaction struct {
	ID         string  `json:"id"`
	AssetID    string  `json:"assetId"`
	Symbol     string  `json:"symbol"`
	Side       string  `json:"side"`
	Quantity   float64 `json:"quantity"`
	PriceCents int64   `json:"priceCents"`
	FeeCents   int64   `json:"feeCents"`
	Status     string  `json:"status"`
	CreatedAt  string  `json:"createdAt"`
}

func (s *Server) handlePutPortfolioSync(w http.ResponseWriter, r *http.Request) {
	clientID, ok := validToken(r.Header.Get(clientIDHeader))
	if !ok {
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: "valid client id is required"})
		return
	}
	user, hasUser := s.currentUser(r)

	var payload portfolioSyncRequest
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxSyncBodyBytes))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&payload); err != nil {
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: "invalid portfolio payload"})
		return
	}

	portfolio, err := s.syncPayloadToPortfolio(r, clientID, payload)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: err.Error()})
		return
	}
	if hasUser {
		portfolio.UserID = user.ID
	}

	if err := s.db.UpsertPortfolio(r.Context(), portfolio); err != nil {
		writeJSON(w, http.StatusServiceUnavailable, errorResponse{Error: "portfolio sync unavailable"})
		return
	}

	responsePayload, err := portfolioToSyncPayload(portfolio)
	if err != nil {
		writeJSON(w, http.StatusServiceUnavailable, errorResponse{Error: "portfolio sync unavailable"})
		return
	}
	writeJSON(w, http.StatusOK, portfolioSyncResponse{Portfolio: responsePayload, SyncedAt: time.Now().UTC()})
}

func (s *Server) handleGetPortfolioSync(w http.ResponseWriter, r *http.Request) {
	clientPortfolioID, ok := validToken(r.URL.Query().Get("id"))
	if !ok {
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: "valid portfolio id is required"})
		return
	}

	var portfolio storage.Portfolio
	var err error
	if user, ok := s.currentUser(r); ok {
		portfolio, err = s.db.PortfolioByUser(r.Context(), user.ID, clientPortfolioID)
	} else {
		clientID, ok := validToken(r.Header.Get(clientIDHeader))
		if !ok {
			writeJSON(w, http.StatusBadRequest, errorResponse{Error: "valid client id is required"})
			return
		}
		portfolio, err = s.db.PortfolioByClient(r.Context(), clientID, clientPortfolioID)
	}
	if err != nil {
		writeJSON(w, http.StatusNotFound, errorResponse{Error: "portfolio not found"})
		return
	}

	payload, err := portfolioToSyncPayload(portfolio)
	if err != nil {
		writeJSON(w, http.StatusServiceUnavailable, errorResponse{Error: "portfolio sync unavailable"})
		return
	}
	writeJSON(w, http.StatusOK, portfolioSyncResponse{Portfolio: payload, SyncedAt: time.Now().UTC()})
}

func (s *Server) syncPayloadToPortfolio(r *http.Request, clientID string, payload portfolioSyncRequest) (storage.Portfolio, error) {
	if err := payload.validate(); err != nil {
		return storage.Portfolio{}, err
	}

	markets, err := s.marketData.Markets(r.Context())
	if err != nil {
		return storage.Portfolio{}, errors.New("market catalog unavailable")
	}
	if err := s.db.UpsertMarkets(r.Context(), markets); err != nil {
		return storage.Portfolio{}, errors.New("market catalog unavailable")
	}

	knownAssets := make(map[string]marketdata.Market, len(markets))
	for _, market := range markets {
		knownAssets[market.AssetID] = market
	}

	createdAt, err := parseSyncTime(payload.CreatedAt, "createdAt")
	if err != nil {
		return storage.Portfolio{}, err
	}
	updatedAt, err := parseSyncTime(payload.UpdatedAt, "updatedAt")
	if err != nil {
		return storage.Portfolio{}, err
	}

	portfolio := storage.Portfolio{
		ID:                syncPortfolioID(clientID, payload.ID),
		ClientID:          clientID,
		ClientPortfolioID: payload.ID,
		SchemaVersion:     payload.SchemaVersion,
		StartingCashCents: payload.StartingCashCents,
		CashCents:         payload.CashCents,
		CreatedAt:         createdAt,
		UpdatedAt:         updatedAt,
		Positions:         make([]storage.PortfolioPosition, 0, len(payload.Positions)),
		Transactions:      make([]storage.PortfolioTransaction, 0, len(payload.Transactions)),
	}

	for _, item := range payload.Positions {
		if _, ok := knownAssets[item.AssetID]; !ok {
			return storage.Portfolio{}, fmt.Errorf("unknown asset id %q", item.AssetID)
		}
		position, err := item.storagePosition()
		if err != nil {
			return storage.Portfolio{}, err
		}
		portfolio.Positions = append(portfolio.Positions, position)
	}

	for _, item := range payload.Transactions {
		if _, ok := knownAssets[item.AssetID]; !ok {
			return storage.Portfolio{}, fmt.Errorf("unknown asset id %q", item.AssetID)
		}
		transaction, err := item.storageTransaction()
		if err != nil {
			return storage.Portfolio{}, err
		}
		transaction.Status = "synced"
		portfolio.Transactions = append(portfolio.Transactions, transaction)
	}

	return portfolio, nil
}

func (p portfolioSyncRequest) validate() error {
	if _, ok := validToken(p.ID); !ok {
		return errors.New("valid portfolio id is required")
	}
	if p.SchemaVersion != 1 {
		return errors.New("unsupported portfolio schema version")
	}
	if p.StartingCashCents < 0 || p.CashCents < 0 {
		return errors.New("portfolio cash values must be non-negative")
	}
	if len(p.Positions) > maxSyncPositions {
		return errors.New("too many portfolio positions")
	}
	if len(p.Transactions) > maxSyncTransactions {
		return errors.New("too many portfolio transactions")
	}
	return nil
}

func (p syncPosition) storagePosition() (storage.PortfolioPosition, error) {
	if err := validateAsset(p.AssetID, p.Symbol); err != nil {
		return storage.PortfolioPosition{}, err
	}
	if p.Name == "" {
		return storage.PortfolioPosition{}, errors.New("position name is required")
	}
	if !validKind(p.Kind) {
		return storage.PortfolioPosition{}, errors.New("invalid position kind")
	}
	quantityMicro, err := quantityToMicro(p.Quantity)
	if err != nil {
		return storage.PortfolioPosition{}, fmt.Errorf("invalid position quantity: %w", err)
	}
	if p.AverageCostCents < 0 || p.LastPriceCents < 0 {
		return storage.PortfolioPosition{}, errors.New("position prices must be non-negative")
	}
	updatedAt, err := parseSyncTime(p.UpdatedAt, "position updatedAt")
	if err != nil {
		return storage.PortfolioPosition{}, err
	}
	return storage.PortfolioPosition{
		AssetID:          p.AssetID,
		Symbol:           p.Symbol,
		Name:             p.Name,
		Kind:             p.Kind,
		QuantityMicro:    quantityMicro,
		AverageCostCents: p.AverageCostCents,
		LastPriceCents:   p.LastPriceCents,
		UpdatedAt:        updatedAt,
	}, nil
}

func (t syncTransaction) storageTransaction() (storage.PortfolioTransaction, error) {
	if _, ok := validToken(t.ID); !ok {
		return storage.PortfolioTransaction{}, errors.New("valid transaction id is required")
	}
	if err := validateAsset(t.AssetID, t.Symbol); err != nil {
		return storage.PortfolioTransaction{}, err
	}
	if t.Side != "buy" && t.Side != "sell" {
		return storage.PortfolioTransaction{}, errors.New("invalid transaction side")
	}
	if t.Status != "local" && t.Status != "synced" {
		return storage.PortfolioTransaction{}, errors.New("invalid transaction status")
	}
	quantityMicro, err := quantityToMicro(t.Quantity)
	if err != nil {
		return storage.PortfolioTransaction{}, fmt.Errorf("invalid transaction quantity: %w", err)
	}
	if t.PriceCents <= 0 || t.FeeCents < 0 {
		return storage.PortfolioTransaction{}, errors.New("invalid transaction price or fee")
	}
	createdAt, err := parseSyncTime(t.CreatedAt, "transaction createdAt")
	if err != nil {
		return storage.PortfolioTransaction{}, err
	}
	return storage.PortfolioTransaction{
		ID:            t.ID,
		AssetID:       t.AssetID,
		Symbol:        t.Symbol,
		Side:          t.Side,
		QuantityMicro: quantityMicro,
		PriceCents:    t.PriceCents,
		FeeCents:      t.FeeCents,
		Status:        t.Status,
		CreatedAt:     createdAt,
	}, nil
}

func portfolioToSyncPayload(portfolio storage.Portfolio) (portfolioSyncRequest, error) {
	payload := portfolioSyncRequest{
		ID:                portfolio.ClientPortfolioID,
		SchemaVersion:     portfolio.SchemaVersion,
		StartingCashCents: portfolio.StartingCashCents,
		CashCents:         portfolio.CashCents,
		CreatedAt:         portfolio.CreatedAt.Format(time.RFC3339Nano),
		UpdatedAt:         portfolio.UpdatedAt.Format(time.RFC3339Nano),
		Positions:         make([]syncPosition, 0, len(portfolio.Positions)),
		Transactions:      make([]syncTransaction, 0, len(portfolio.Transactions)),
	}
	for _, position := range portfolio.Positions {
		payload.Positions = append(payload.Positions, syncPosition{
			AssetID:          position.AssetID,
			Symbol:           position.Symbol,
			Name:             position.Name,
			Kind:             position.Kind,
			Quantity:         float64(position.QuantityMicro) / quantityScale,
			AverageCostCents: position.AverageCostCents,
			LastPriceCents:   position.LastPriceCents,
			UpdatedAt:        position.UpdatedAt.Format(time.RFC3339Nano),
		})
	}
	for _, transaction := range portfolio.Transactions {
		payload.Transactions = append(payload.Transactions, syncTransaction{
			ID:         transaction.ID,
			AssetID:    transaction.AssetID,
			Symbol:     transaction.Symbol,
			Side:       transaction.Side,
			Quantity:   float64(transaction.QuantityMicro) / quantityScale,
			PriceCents: transaction.PriceCents,
			FeeCents:   transaction.FeeCents,
			Status:     transaction.Status,
			CreatedAt:  transaction.CreatedAt.Format(time.RFC3339Nano),
		})
	}
	return payload, nil
}

func validateAsset(assetID, symbol string) error {
	if assetID == "" || len(assetID) > 128 || strings.TrimSpace(symbol) == "" || len(symbol) > 24 {
		return errors.New("valid asset id and symbol are required")
	}
	return nil
}

func validKind(kind marketdata.AssetKind) bool {
	switch kind {
	case marketdata.AssetKindStock, marketdata.AssetKindETF, marketdata.AssetKindCrypto, marketdata.AssetKindCommodity, marketdata.AssetKindEvent:
		return true
	default:
		return false
	}
}

func validToken(value string) (string, bool) {
	value = strings.TrimSpace(value)
	if len(value) < 8 || len(value) > 128 {
		return "", false
	}
	for _, char := range value {
		if (char >= 'a' && char <= 'z') || (char >= 'A' && char <= 'Z') || (char >= '0' && char <= '9') || char == '-' || char == '_' {
			continue
		}
		return "", false
	}
	return value, true
}

func quantityToMicro(value float64) (int64, error) {
	if math.IsNaN(value) || math.IsInf(value, 0) || value <= 0 || value > 1_000_000_000 {
		return 0, errors.New("quantity must be greater than zero")
	}
	return int64(math.Round(value * quantityScale)), nil
}

func parseSyncTime(value, field string) (time.Time, error) {
	parsed, err := time.Parse(time.RFC3339Nano, value)
	if err != nil {
		return time.Time{}, fmt.Errorf("invalid %s", field)
	}
	return parsed.UTC(), nil
}

func syncPortfolioID(clientID, clientPortfolioID string) string {
	sum := sha256.Sum256([]byte(clientID + ":" + clientPortfolioID))
	return "portfolio-" + hex.EncodeToString(sum[:16])
}
