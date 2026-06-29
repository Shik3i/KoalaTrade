package config

import (
	"fmt"
	"os"
	"strconv"
)

const defaultStartingCashCents int64 = 1_000_000
const defaultMarketDataCacheSeconds = 60
const defaultMarketDataPollSeconds = 60

// Full cycle over which every asset is refreshed exactly once. The poller
// staggers asset refreshes evenly across this window so the per-minute provider
// rate limit (free tier) is never exceeded, instead of fetching everything at once.
const defaultMarketDataRefreshWindowSeconds = 900

// Public LoL Esports API key (same one the lolesports.com web client ships).
const defaultLolesportsAPIKey = "0TvQnueqKa5mxJntVWt0w4LpLfEkrV1Ta8rQBb9Z"
const defaultLolesportsBaseURL = "https://esports-api.lolesports.com"
const defaultPolymarketBaseURL = "https://gamma-api.polymarket.com"
const defaultEsportsCacheSeconds = 300

type Config struct {
	AppName                     string
	DatabasePath                string
	Port                        int
	Environment                 string
	StartingCashCents           int64
	MarketDataProvider          string
	MarketDataCacheSeconds      int
	MarketDataPollSeconds       int
	MarketDataRefreshWindowSecs int
	MarketDataHTTPTimeout       int
	CoinGeckoBaseURL            string
	CoinGeckoAPIKey             string
	FinnhubBaseURL              string
	FinnhubAPIKey               string
	LolesportsAPIKey            string
	LolesportsBaseURL           string
	PolymarketBaseURL           string
	EsportsCacheSeconds         int
	AdminUsername               string
	AdminPassword               string
	AuthSecret                  string
}

func Load() Config {
	return Config{
		AppName:                     getEnv("APP_NAME", "KoalaTrade"),
		DatabasePath:                getEnv("DB_PATH", "data/koalatrade.db"),
		Port:                        getEnvInt("PORT", 8080),
		Environment:                 getEnv("APP_ENV", "development"),
		StartingCashCents:           getEnvInt64("STARTING_CASH_CENTS", defaultStartingCashCents),
		MarketDataProvider:          getEnv("MARKET_DATA_PROVIDER", "mock"),
		MarketDataCacheSeconds:      getEnvInt("MARKET_DATA_CACHE_SECONDS", defaultMarketDataCacheSeconds),
		MarketDataPollSeconds:       getEnvInt("MARKET_DATA_POLL_SECONDS", defaultMarketDataPollSeconds),
		MarketDataRefreshWindowSecs: getEnvInt("MARKET_DATA_REFRESH_WINDOW_SECONDS", defaultMarketDataRefreshWindowSeconds),
		MarketDataHTTPTimeout:       getEnvInt("MARKET_DATA_HTTP_TIMEOUT_SECONDS", 5),
		CoinGeckoBaseURL:            getEnv("COINGECKO_BASE_URL", ""),
		CoinGeckoAPIKey:             getEnv("COINGECKO_API_KEY", ""),
		FinnhubBaseURL:              getEnv("FINNHUB_BASE_URL", ""),
		FinnhubAPIKey:               getEnv("FINNHUB_API_KEY", ""),
		LolesportsAPIKey:            getEnv("LOLESPORTS_API_KEY", defaultLolesportsAPIKey),
		LolesportsBaseURL:           getEnv("LOLESPORTS_BASE_URL", defaultLolesportsBaseURL),
		PolymarketBaseURL:           getEnv("POLYMARKET_BASE_URL", defaultPolymarketBaseURL),
		EsportsCacheSeconds:         getEnvInt("ESPORTS_CACHE_SECONDS", defaultEsportsCacheSeconds),
		AdminUsername:               getEnv("ADMIN_USERNAME", "admin"),
		AdminPassword:               getEnv("ADMIN_PASSWORD", ""),
		AuthSecret:                  getEnv("AUTH_SECRET", ""),
	}
}

func (c Config) ListenAddr() string {
	return fmt.Sprintf(":%d", c.Port)
}

func getEnv(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}

func getEnvInt(key string, fallback int) int {
	value := os.Getenv(key)
	if value == "" {
		return fallback
	}

	parsed, err := strconv.Atoi(value)
	if err != nil {
		return fallback
	}
	return parsed
}

func getEnvInt64(key string, fallback int64) int64 {
	value := os.Getenv(key)
	if value == "" {
		return fallback
	}

	parsed, err := strconv.ParseInt(value, 10, 64)
	if err != nil {
		return fallback
	}
	return parsed
}
