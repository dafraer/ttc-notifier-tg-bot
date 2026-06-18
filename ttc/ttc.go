package ttc

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"
)

const (
	baseURL       = "https://transit.ttc.com.ge/pis-gateway/api/v2"
	defaultAPIKey = "c0a2f304-551a-4d08-b8df-2c53ecd57f9f"
)

// Client is a TTC API client. It mirrors the `ttc` object from the original
// TypeScript library. A zero value is not usable; create one with New.
type Client struct {
	baseURL    string
	apiKey     string
	httpClient *http.Client

	mu     sync.RWMutex
	locale Locale
}

// New creates a TTC API client with sensible defaults. The default locale is
// "en", matching the original library's defaults.
func New() *Client {
	return &Client{
		baseURL:    baseURL,
		apiKey:     defaultAPIKey,
		httpClient: &http.Client{Timeout: 20 * time.Second},
		locale:     LocaleEn,
	}
}

// SetLocale sets the preferred language for responses.
func (c *Client) SetLocale(locale Locale) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.locale = locale
}

func (c *Client) currentLocale() Locale {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.locale
}

// get performs a GET request against the gateway, decoding the JSON body into
// out. params are added as query parameters; locale defaults to the client's
// configured locale unless explicitly provided.
func (c *Client) get(ctx context.Context, path string, params map[string]string, out any) error {
	u, err := url.Parse(c.baseURL + path)
	if err != nil {
		return fmt.Errorf("ttc: invalid url: %w", err)
	}

	q := u.Query()
	if _, ok := params["locale"]; !ok {
		q.Set("locale", c.currentLocale())
	}
	for k, v := range params {
		q.Set(k, v)
	}
	u.RawQuery = q.Encode()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return fmt.Errorf("ttc: build request: %w", err)
	}
	req.Header.Set("X-Api-Key", c.apiKey)
	req.Header.Set("Accept", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("ttc: request failed: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("ttc: read body: %w", err)
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("ttc: request failed with status code %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}

	if out == nil {
		return nil
	}
	if err := json.Unmarshal(body, out); err != nil {
		return fmt.Errorf("ttc: decode response: %w", err)
	}
	return nil
}

// Stops returns all stops in the network. Pass an empty locale to use the
// client default.
func (c *Client) Stops(ctx context.Context, locale Locale) ([]BusStop, error) {
	params := map[string]string{}
	if locale != "" {
		params["locale"] = locale
	}
	var data []BusStop
	if err := c.get(ctx, "/stops", params, &data); err != nil {
		return nil, err
	}
	return data, nil
}

// Stop returns detailed information about a single stop.
func (c *Client) Stop(ctx context.Context, stopID string) (*BusStop, error) {
	var data BusStop
	if err := c.get(ctx, fmt.Sprintf("/stops/1:%s", stopID), nil, &data); err != nil {
		return nil, err
	}
	return &data, nil
}

// Routes returns all bus routes.
func (c *Client) Routes(ctx context.Context, locale Locale) ([]Bus, error) {
	params := map[string]string{"modes": "BUS"}
	if locale != "" {
		params["locale"] = locale
	}
	var data []Bus
	if err := c.get(ctx, "/routes", params, &data); err != nil {
		return nil, err
	}
	return data, nil
}

// PlanOptions are the parameters for the Plan method.
type PlanOptions struct {
	From   LatLng
	To     LatLng
	Locale Locale
}

// Plan plans a journey between two points.
func (c *Client) Plan(ctx context.Context, opts PlanOptions) (*BusPlan, error) {
	locale := opts.Locale
	if locale == "" {
		locale = LocaleEn
	}
	params := map[string]string{
		"fromPlace":  joinLatLng(opts.From),
		"toPlace":    joinLatLng(opts.To),
		"departMode": "leaveNow",
		"modes":      "WALK,BUS",
		"optimize":   "quick",
		"locale":     locale,
	}
	var data BusPlan
	if err := c.get(ctx, "/plan", params, &data); err != nil {
		return nil, err
	}
	return &data, nil
}

// BusPolyline returns the encoded polyline for a route in the given direction.
func (c *Client) BusPolyline(ctx context.Context, busID string, forward bool) (*Polyline, error) {
	params := map[string]string{"forward": strconv.FormatBool(forward)}
	var data Polyline
	if err := c.get(ctx, fmt.Sprintf("/routes/1:%s/polyline", busID), params, &data); err != nil {
		return nil, err
	}
	return &data, nil
}

// Locations returns real-time positions of buses on a route.
func (c *Client) Locations(ctx context.Context, busID string, forward bool) ([]BusLocation, error) {
	params := map[string]string{"forward": strconv.FormatBool(forward)}
	var data []BusLocation
	if err := c.get(ctx, fmt.Sprintf("/routes/1:%s/positions", busID), params, &data); err != nil {
		return nil, err
	}
	return data, nil
}

// StopRoutes returns the routes serving a given stop.
func (c *Client) StopRoutes(ctx context.Context, stopID string, locale Locale) ([]Bus, error) {
	if locale == "" {
		locale = LocaleEn
	}
	params := map[string]string{"locale": locale}
	var data []Bus
	if err := c.get(ctx, fmt.Sprintf("/stops/1:%s/routes", stopID), params, &data); err != nil {
		return nil, err
	}
	return data, nil
}

// BusRoutes returns the stops along a route in the given direction.
func (c *Client) BusRoutes(ctx context.Context, busID string, forward bool, locale Locale) ([]BusStop, error) {
	if locale == "" {
		locale = LocaleEn
	}
	params := map[string]string{
		"locale":  locale,
		"forward": strconv.FormatBool(forward),
	}
	var data []BusStop
	if err := c.get(ctx, fmt.Sprintf("/routes/1:%s/stops", busID), params, &data); err != nil {
		return nil, err
	}
	return data, nil
}

// ArrivalOptions are the parameters for the ArrivalTimes method.
type ArrivalOptions struct {
	StopID                      string
	Locale                      Locale
	IgnoreScheduledArrivalTimes bool
}

// ArrivalTimes returns arrival predictions for a stop.
func (c *Client) ArrivalTimes(ctx context.Context, opts ArrivalOptions) ([]BusArrival, error) {
	locale := opts.Locale
	if locale == "" {
		locale = LocaleEn
	}
	params := map[string]string{
		"locale":                      locale,
		"ignoreScheduledArrivalTimes": strconv.FormatBool(opts.IgnoreScheduledArrivalTimes),
	}
	var data []BusArrival
	if err := c.get(ctx, fmt.Sprintf("/stops/1:%s/arrival-times", opts.StopID), params, &data); err != nil {
		return nil, err
	}
	return data, nil
}

func joinLatLng(p LatLng) string {
	parts := make([]string, len(p))
	for i, v := range p {
		parts[i] = strconv.FormatFloat(v, 'f', -1, 64)
	}
	return strings.Join(parts, ",")
}
