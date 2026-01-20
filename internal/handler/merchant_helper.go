package handler

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
)

// Merchant represents the merchant data from the external customer API
type Merchant struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	Phone    string `json:"phone"`
	Email    string `json:"email"`
	Username string `json:"username"`
	Active   bool   `json:"active"`
}

// fetchMerchantDetails fetches merchant information from the external customer API.
func fetchMerchantDetails(customerServiceURL string, merchantID string, authHeader string) (*Merchant, error) {
	if customerServiceURL == "" {
		return nil, fmt.Errorf("customer service URL is not configured")
	}

	merchantURL := fmt.Sprintf("%s/merchants/%s", customerServiceURL, merchantID)

	slog.Info("Fetching merchant details from external API", "url", merchantURL)

	req, err := http.NewRequest("GET", merchantURL, nil)
	if err != nil {
		slog.Error("Failed to create request", "error", err, "url", merchantURL)
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	if authHeader != "" {
		req.Header.Set("Authorization", authHeader)
	}

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		slog.Error("Failed to fetch merchant from API", "error", err, "url", merchantURL)
		return nil, fmt.Errorf("failed to fetch merchant: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		slog.Error("Customer API returned non-200 status", "status", resp.StatusCode, "url", merchantURL)
		return nil, fmt.Errorf("customer API returned status %d", resp.StatusCode)
	}

	var apiResponse struct {
		Data            Merchant `json:"data"`
		ResponseCode    string   `json:"responseCode"`
		ResponseMessage string   `json:"responseMessage"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&apiResponse); err != nil {
		slog.Error("Failed to decode merchant data", "error", err)
		return nil, fmt.Errorf("failed to decode merchant: %w", err)
	}

	return &apiResponse.Data, nil
}
