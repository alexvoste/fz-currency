package rates

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"sync"
	"time"
)

type cacheEntry struct {
	value  any
	expiry time.Time
}

type Cache struct {
	mu    sync.RWMutex
	items map[string]cacheEntry
}

func NewCache() *Cache {
	return &Cache{items: make(map[string]cacheEntry)}
}

func (c *Cache) Get(key string) (any, bool) {
	c.mu.RLock()
	entry, ok := c.items[key]
	c.mu.RUnlock()
	if !ok {
		return nil, false
	}
	if time.Now().After(entry.expiry) {
		c.mu.Lock()
		delete(c.items, key)
		c.mu.Unlock()
		return nil, false
	}
	return entry.value, true
}

func (c *Cache) Set(key string, value any, ttl time.Duration) {
	c.mu.Lock()
	c.items[key] = cacheEntry{value: value, expiry: time.Now().Add(ttl)}
	c.mu.Unlock()
}

func (c *Cache) Clear() {
	c.mu.Lock()
	c.items = make(map[string]cacheEntry)
	c.mu.Unlock()
}

type RatesClient struct {
	client *http.Client
	cache  *Cache
}

type CryptoRate struct {
	Price     float64
	Change24h float64
}

type FiatRates struct {
	USDEUR float64
	USDRUB float64
	EURRUB float64
}

func NewRatesClient(client *http.Client, cache *Cache) *RatesClient {
	return &RatesClient{client: client, cache: cache}
}

func (c *RatesClient) GetCryptoRates(ctx context.Context) (map[string]CryptoRate, error) {
	cacheKey := "crypto_rates"
	if cached, ok := c.cache.Get(cacheKey); ok {
		return cached.(map[string]CryptoRate), nil
	}

	endpoint := "https://api.coingecko.com/api/v3/simple/price?ids=bitcoin,ethereum,solana,toncoin&vs_currencies=usd&include_24hr_change=true"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, err
	}

	resp, err := c.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("crypto endpoint returned %d", resp.StatusCode)
	}

	var payload map[string]struct {
		Usd          float64 `json:"usd"`
		Usd24hChange float64 `json:"usd_24h_change"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return nil, err
	}

	rates := map[string]CryptoRate{}
	for key, item := range payload {
		switch strings.ToLower(key) {
		case "bitcoin":
			rates["BTC"] = CryptoRate{Price: item.Usd, Change24h: item.Usd24hChange}
		case "ethereum":
			rates["ETH"] = CryptoRate{Price: item.Usd, Change24h: item.Usd24hChange}
		case "solana":
			rates["SOL"] = CryptoRate{Price: item.Usd, Change24h: item.Usd24hChange}
		case "toncoin":
			rates["TON"] = CryptoRate{Price: item.Usd, Change24h: item.Usd24hChange}
		}
	}

	c.cache.Set(cacheKey, rates, 5*time.Minute)
	return rates, nil
}

func (c *RatesClient) GetFiatRates(ctx context.Context) (FiatRates, error) {
	cacheKey := "fiat_rates"
	if cached, ok := c.cache.Get(cacheKey); ok {
		return cached.(FiatRates), nil
	}

	endpoint := "https://open.er-api.com/v6/latest/USD"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return FiatRates{}, err
	}

	resp, err := c.client.Do(req)
	if err != nil {
		return FiatRates{}, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return FiatRates{}, fmt.Errorf("fiat endpoint returned %d", resp.StatusCode)
	}

	var payload struct {
		Result string             `json:"result"`
		Rates  map[string]float64 `json:"rates"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return FiatRates{}, err
	}
	if payload.Result != "success" {
		return FiatRates{}, fmt.Errorf("fiat endpoint returned invalid result")
	}

	usdEur := payload.Rates["EUR"]
	usdRub := payload.Rates["RUB"]
	rates := FiatRates{USDEUR: usdEur, USDRUB: usdRub, EURRUB: usdRub / usdEur}

	c.cache.Set(cacheKey, rates, 5*time.Minute)
	return rates, nil
}

func (c *RatesClient) GetCryptoHistory(ctx context.Context, id string, days int) ([]float64, error) {
	cacheKey := fmt.Sprintf("crypto_history:%s:%d", id, days)
	if cached, ok := c.cache.Get(cacheKey); ok {
		return cached.([]float64), nil
	}

	endpoint := fmt.Sprintf("https://api.coingecko.com/api/v3/coins/%s/market_chart?vs_currency=usd&days=%d&interval=daily", url.PathEscape(id), days)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, err
	}

	resp, err := c.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("crypto history endpoint returned %d", resp.StatusCode)
	}

	var payload struct {
		Prices [][]float64 `json:"prices"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return nil, err
	}

	history := make([]float64, 0, len(payload.Prices))
	for _, row := range payload.Prices {
		if len(row) >= 2 {
			history = append(history, row[1])
		}
	}

	c.cache.Set(cacheKey, history, 5*time.Minute)
	return history, nil
}

func (c *RatesClient) GetFiatHistory(ctx context.Context, base, quote string, days int) ([]float64, error) {
	cacheKey := fmt.Sprintf("fiat_history:%s:%s:%d", base, quote, days)
	if cached, ok := c.cache.Get(cacheKey); ok {
		return cached.([]float64), nil
	}

	end := time.Now().UTC()
	start := end.AddDate(0, 0, -days+1)

	endpoint := fmt.Sprintf(
		"https://api.exchangerate.host/timeseries?start_date=%s&end_date=%s&base=%s&symbols=%s",
		start.Format("2006-01-02"),
		end.Format("2006-01-02"),
		url.QueryEscape(base),
		url.QueryEscape(quote),
	)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, err
	}

	resp, err := c.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("fiat history endpoint returned %d", resp.StatusCode)
	}

	var payload struct {
		Rates map[string]map[string]float64 `json:"rates"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return nil, err
	}

	keys := make([]string, 0, len(payload.Rates))
	for date := range payload.Rates {
		keys = append(keys, date)
	}
	sort.Strings(keys)

	history := make([]float64, 0, len(keys))
	for _, date := range keys {
		if value, ok := payload.Rates[date][quote]; ok {
			history = append(history, value)
		}
	}

	c.cache.Set(cacheKey, history, 5*time.Minute)
	return history, nil
}
