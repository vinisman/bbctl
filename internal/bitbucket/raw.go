package bitbucket

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"

	"github.com/vinisman/bbctl/internal/config"
)

// doRaw performs a raw HTTP request to Bitbucket, bypassing the generated SDK.
// Used for APIs not covered by the SDK: ScriptRunner, default-tasks, etc.
// The path is relative to BaseURL (BaseURL usually ends with /rest).
// The client transport contains the retry layer, so retries on 5xx/network
// errors happen automatically.
func (c *Client) doRaw(method, path string, query url.Values, body any) (int, []byte, error) {
	base := config.GlobalCfg.BaseURL
	u := base + path
	if len(query) > 0 {
		u += "?" + query.Encode()
	}

	var reader io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return 0, nil, fmt.Errorf("failed to marshal request body: %w", err)
		}
		reader = bytes.NewReader(b)
	}

	req, err := http.NewRequest(method, u, reader)
	if err != nil {
		return 0, nil, fmt.Errorf("failed to build request %s %s: %w", method, u, err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", "bbctl/1.0")
	req.Header.Set("X-Atlassian-Token", "no-check")

	// Auth: token → Bearer, otherwise basic (same as NewClient for the SDK)
	if config.GlobalCfg.Token != "" {
		req.Header.Set("Authorization", "Bearer "+config.GlobalCfg.Token)
	} else if config.GlobalCfg.Username != "" && config.GlobalCfg.Password != "" {
		req.SetBasicAuth(config.GlobalCfg.Username, config.GlobalCfg.Password)
	}

	httpResp, err := c.client.Do(req)
	if err != nil {
		return 0, nil, err
	}
	defer httpResp.Body.Close()

	respBody, err := io.ReadAll(httpResp.Body)
	if err != nil {
		return httpResp.StatusCode, nil, fmt.Errorf("failed to read response body: %w", err)
	}
	return httpResp.StatusCode, respBody, nil
}
