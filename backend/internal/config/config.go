package config

import (
	"fmt"
	"os"
	"strconv"
)

const defaultStartingCashCents int64 = 1_000_000
const defaultMarketDataCacheSeconds = 60
const defaultMarketDataPollSeconds = 60

type Config struct {
	AppName                string
	DatabasePath           string
	Port                   int
	Environment            string
	StartingCashCents      int64
	MarketDataProvider     string
	MarketDataCacheSeconds int
	MarketDataPollSeconds  int
	MarketDataHTTPTimeout  int
	CoinGeckoBaseURL       string
	CoinGeckoAPIKey        string
	FinnhubBaseURL         string
	FinnhubAPIKey          string
}

func Load() Config {
	return Config{
		AppName:                getEnv("APP_NAME", "KoalaTrade"),
		DatabasePath:           getEnv("DB_PATH", "data/koalatrade.db"),
		Port:                   getEnvInt("PORT", 8080),
		Environment:            getEnv("APP_ENV", "development"),
		StartingCashCents:      getEnvInt64("STARTING_CASH_CENTS", defaultStartingCashCents),
		MarketDataProvider:     getEnv("MARKET_DATA_PROVIDER", "mock"),
		MarketDataCacheSeconds: getEnvInt("MARKET_DATA_CACHE_SECONDS", defaultMarketDataCacheSeconds),
		MarketDataPollSeconds:  getEnvInt("MARKET_DATA_POLL_SECONDS", defaultMarketDataPollSeconds),
		MarketDataHTTPTimeout:  getEnvInt("MARKET_DATA_HTTP_TIMEOUT_SECONDS", 5),
		CoinGeckoBaseURL:       getEnv("COINGECKO_BASE_URL", ""),
		CoinGeckoAPIKey:        getEnv("COINGECKO_API_KEY", ""),
		FinnhubBaseURL:         getEnv("FINNHUB_BASE_URL", ""),
		FinnhubAPIKey:          getEnv("FINNHUB_API_KEY", ""),
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
