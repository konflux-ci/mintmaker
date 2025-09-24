// Copyright 2025 Red Hat, Inc.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package clients

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"time"
)

type KiteClient struct {
	baseURL    string
	httpClient *http.Client
}

// NewKiteClient creates a new Kite API client
func NewKiteClient(baseURL string) (*KiteClient, error) {
	if baseURL == "" {
		return nil, errors.New("KITE API base URL cannot be empty")
	}

	return &KiteClient{
		baseURL: baseURL,
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
	}, nil
}

// sendRequest sends the given request to KITE API and stores
// the decoded response body in the value pointed to by out
func (c *KiteClient) sendRequest(req *http.Request, out any) error {
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("KITE API returned status code %d", resp.StatusCode)
	}

	if out != nil {
		if err := json.NewDecoder(resp.Body).Decode(out); err != nil {
			return fmt.Errorf("failed to decode response: %w", err)
		}
	}

	return nil
}

// GetVersion returns the KITE API version information
func (c *KiteClient) GetVersion(ctx context.Context) (map[string]any, error) {
	url := fmt.Sprintf("%s/api/v1/version", c.baseURL)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}

	var respBody map[string]any
	if err := c.sendRequest(req, &respBody); err != nil {
		return nil, err
	}

	return respBody, nil
}
