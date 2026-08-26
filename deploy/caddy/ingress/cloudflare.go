package ingress

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"strings"
	"time"
)

// CloudflareClient manages DNS records via the Cloudflare API.
type CloudflareClient struct {
	token      string
	zoneID     string
	baseDomain string
	log        *slog.Logger
	httpClient *http.Client
}

// NewCloudflareClient creates a client for the given token and base domain.
// It looks up the zone ID automatically from the base domain.
func NewCloudflareClient(token, baseDomain string, logger *slog.Logger) (*CloudflareClient, error) {
	c := &CloudflareClient{
		token:      token,
		baseDomain: baseDomain,
		log:        logger,
		httpClient: &http.Client{Timeout: 15 * time.Second},
	}

	zoneID, err := c.findZoneID()
	if err != nil {
		return nil, fmt.Errorf("finding zone ID for %s: %w", baseDomain, err)
	}
	c.zoneID = zoneID
	c.log.Info("cloudflare zone resolved", "domain", baseDomain, "zone_id", zoneID)
	return c, nil
}

// EnsureDNS ensures the infrastructure A records and the apps wildcard A record
// exist, pointing to the given server IP. Records are created if missing, updated
// if the IP has changed, and left alone if already correct.
func (c *CloudflareClient) EnsureDNS(serverIP, authDomain, _, appsDomain string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	records := []struct {
		name    string
		comment string
	}{
		{
			name:    authDomain,
			comment: "styrman auth portal (auto-managed by styrman ingress)",
		},
		// NOTE: devbox also created a "shell" record here; styrman has no
		// shell app, so no record. shellDomain stays in the signature only
		// for the generator's hostname override (unused unless a container
		// is labeled hostname=shell).
		{
			name:    fmt.Sprintf("*.%s", appsDomain),
			comment: "styrman apps wildcard (auto-managed by styrman ingress)",
		},
	}

	for _, r := range records {
		if err := c.ensureRecord(ctx, r.name, serverIP, r.comment); err != nil {
			return fmt.Errorf("ensuring DNS record %s: %w", r.name, err)
		}
	}
	return nil
}

// ensureRecord checks if an A record exists and creates or updates it as needed.
func (c *CloudflareClient) ensureRecord(ctx context.Context, name, ip, comment string) error {
	existing, err := c.getRecord(ctx, name)
	if err != nil {
		return err
	}

	if existing != nil {
		if existing.Content == ip {
			c.log.Info("dns record already correct", "name", name, "ip", ip)
			return nil
		}
		c.log.Info("updating dns record", "name", name, "old_ip", existing.Content, "new_ip", ip)
		return c.updateRecord(ctx, existing.ID, name, ip, comment)
	}

	c.log.Info("creating dns record", "name", name, "ip", ip)
	return c.createRecord(ctx, name, ip, comment)
}

// cfRecord represents a Cloudflare DNS record.
type cfRecord struct {
	ID      string `json:"id"`
	Type    string `json:"type"`
	Name    string `json:"name"`
	Content string `json:"content"`
	Proxied bool   `json:"proxied"`
	TTL     int    `json:"ttl"`
}

// cfResponse wraps the Cloudflare API response.
type cfResponse struct {
	Success bool            `json:"success"`
	Result  json.RawMessage `json:"result"`
	Errors  []struct {
		Message string `json:"message"`
	} `json:"errors"`
}

// findZoneID looks up the Cloudflare zone ID for the base domain by trying
// progressively shorter domain suffixes (e.g., sub.example.com → example.com).
func (c *CloudflareClient) findZoneID() (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	parts := strings.Split(c.baseDomain, ".")

	// Try from the full domain down to the last two parts
	for i := 0; i < len(parts)-1; i++ {
		candidate := strings.Join(parts[i:], ".")
		url := fmt.Sprintf("https://api.cloudflare.com/client/v4/zones?name=%s&status=active", candidate)

		body, err := c.doRequest(ctx, "GET", url, nil)
		if err != nil {
			return "", err
		}

		var resp cfResponse
		if err := json.Unmarshal(body, &resp); err != nil {
			return "", fmt.Errorf("parsing zones response: %w", err)
		}

		var zones []struct {
			ID   string `json:"id"`
			Name string `json:"name"`
		}
		if err := json.Unmarshal(resp.Result, &zones); err != nil {
			return "", fmt.Errorf("parsing zones result: %w", err)
		}

		if len(zones) > 0 {
			return zones[0].ID, nil
		}
	}

	return "", fmt.Errorf("no Cloudflare zone found for domain %s", c.baseDomain)
}

// getRecord retrieves an existing A record by name, or nil if not found.
func (c *CloudflareClient) getRecord(ctx context.Context, name string) (*cfRecord, error) {
	url := fmt.Sprintf(
		"https://api.cloudflare.com/client/v4/zones/%s/dns_records?type=A&name=%s",
		c.zoneID, name,
	)

	body, err := c.doRequest(ctx, "GET", url, nil)
	if err != nil {
		return nil, err
	}

	var resp cfResponse
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, fmt.Errorf("parsing record response: %w", err)
	}

	var records []cfRecord
	if err := json.Unmarshal(resp.Result, &records); err != nil {
		return nil, fmt.Errorf("parsing records: %w", err)
	}

	if len(records) == 0 {
		return nil, nil
	}
	return &records[0], nil
}

// createRecord creates a new A record.
func (c *CloudflareClient) createRecord(ctx context.Context, name, ip, comment string) error {
	url := fmt.Sprintf("https://api.cloudflare.com/client/v4/zones/%s/dns_records", c.zoneID)

	payload := map[string]interface{}{
		"type":    "A",
		"name":    name,
		"content": ip,
		"ttl":     1, // auto
		"proxied": false,
		"comment": comment,
	}

	body, err := c.doRequest(ctx, "POST", url, payload)
	if err != nil {
		return err
	}

	var resp cfResponse
	if err := json.Unmarshal(body, &resp); err != nil {
		return fmt.Errorf("parsing create response: %w", err)
	}
	if !resp.Success {
		return fmt.Errorf("cloudflare API error: %v", resp.Errors)
	}

	c.log.Info("dns record created", "name", name, "ip", ip)
	return nil
}

// updateRecord updates an existing A record.
func (c *CloudflareClient) updateRecord(ctx context.Context, recordID, name, ip, comment string) error {
	url := fmt.Sprintf(
		"https://api.cloudflare.com/client/v4/zones/%s/dns_records/%s",
		c.zoneID, recordID,
	)

	payload := map[string]interface{}{
		"type":    "A",
		"name":    name,
		"content": ip,
		"ttl":     1,
		"proxied": false,
		"comment": comment,
	}

	body, err := c.doRequest(ctx, "PUT", url, payload)
	if err != nil {
		return err
	}

	var resp cfResponse
	if err := json.Unmarshal(body, &resp); err != nil {
		return fmt.Errorf("parsing update response: %w", err)
	}
	if !resp.Success {
		return fmt.Errorf("cloudflare API error: %v", resp.Errors)
	}

	c.log.Info("dns record updated", "name", name, "ip", ip)
	return nil
}

// doRequest executes an authenticated Cloudflare API request.
func (c *CloudflareClient) doRequest(ctx context.Context, method, url string, payload interface{}) ([]byte, error) {
	var bodyReader io.Reader
	if payload != nil {
		data, err := json.Marshal(payload)
		if err != nil {
			return nil, fmt.Errorf("marshaling payload: %w", err)
		}
		bodyReader = bytes.NewReader(data)
	}

	req, err := http.NewRequestWithContext(ctx, method, url, bodyReader)
	if err != nil {
		return nil, fmt.Errorf("creating request: %w", err)
	}

	req.Header.Set("Authorization", "Bearer "+c.token)
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("cloudflare API request failed: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("reading response: %w", err)
	}

	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("cloudflare API returned %d: %s", resp.StatusCode, string(body))
	}

	return body, nil
}

// DetectPublicIP discovers the server's public IP address.
func DetectPublicIP() (string, error) {
	httpClient := &http.Client{Timeout: 5 * time.Second}

	// Try multiple services for resilience (URLs centralized in config.go)
	for _, svcURL := range IPDetectionURLs {
		ip, err := fetchIP(httpClient, svcURL)
		if err != nil {
			continue
		}
		if ip != "" {
			return ip, nil
		}
	}

	return "", fmt.Errorf("failed to detect public IP from any service")
}

// fetchIP makes a single HTTP GET and returns the trimmed response body.
func fetchIP(httpClient *http.Client, url string) (string, error) {
	resp, err := httpClient.Get(url)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}

	ip := strings.TrimSpace(string(body))
	parsed := net.ParseIP(ip)
	if parsed == nil || parsed.To4() == nil {
		return "", fmt.Errorf("invalid or non-IPv4 address from %s: %q", url, ip)
	}
	return ip, nil
}
