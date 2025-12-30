package handler

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
)

// Product represents the product data from external API
type Product struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description"`
	Price       int    `json:"price"`
	CreatedAt   string `json:"created_at"`
	UpdatedAt   string `json:"updated_at"`
	CreatedBy   string `json:"created_by"`
	URL         string `json:"url"`
	Currency    string `json:"currency"`
	Stock       int    `json:"stock"`
}

// fetchProductDetailsInternal fetches product information from the external product API.
// This is a shared internal function used by handlers.
func fetchProductDetailsInternal(productServiceURL string, authHeader string) (map[string]Product, error) {
	productURL := productServiceURL + "/products"

	slog.Info("Fetching product details from external API", "url", productURL)

	req, err := http.NewRequest("GET", productURL, nil)
	if err != nil {
		slog.Error("Failed to create request", "error", err, "url", productURL)
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	// Add Content-Type and Accept headers
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")

	// Add Authorization header if provided
	if authHeader != "" {
		req.Header.Set("Authorization", authHeader)
	}

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		slog.Error("Failed to fetch products from API", "error", err, "url", productURL)
		return nil, fmt.Errorf("failed to fetch products: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		slog.Error("Product API returned non-200 status", "status", resp.StatusCode)
		return nil, fmt.Errorf("product API returned status %d", resp.StatusCode)
	}

	var products []Product
	if err := json.NewDecoder(resp.Body).Decode(&products); err != nil {
		slog.Error("Failed to decode product data", "error", err)
		return nil, fmt.Errorf("failed to decode products: %w", err)
	}

	// Create a map for quick lookup by product ID
	productMap := make(map[string]Product)
	for _, product := range products {
		productMap[product.ID] = product
	}

	slog.Info("Successfully fetched products", "count", len(products))
	return productMap, nil
}
