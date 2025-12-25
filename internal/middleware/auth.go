package middleware

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
)

// AuthValidationResponse represents the response from the auth validation endpoint
type AuthValidationResponse struct {
	ResponseCode    string                 `json:"responseCode"`
	ResponseMessage string                 `json:"responseMessage"`
	Data            map[string]interface{} `json:"data"`
}

// AuthMiddleware validates JWT tokens by calling an external auth service
func AuthMiddleware(authValidateURL string) gin.HandlerFunc {
	return func(c *gin.Context) {
		// Extract Authorization header
		authHeader := c.GetHeader("Authorization")

		if authHeader == "" {
			slog.Warn("Missing Authorization header", "path", c.Request.URL.Path)
			c.JSON(http.StatusUnauthorized, gin.H{
				"responseCode":    "401",
				"responseMessage": "Authorization header is required",
			})
			c.Abort()
			return
		}

		// Validate Bearer token format
		parts := strings.Split(authHeader, " ")
		if len(parts) != 2 || parts[0] != "Bearer" {
			slog.Warn("Invalid Authorization header format", "path", c.Request.URL.Path)
			c.JSON(http.StatusUnauthorized, gin.H{
				"responseCode":    "401",
				"responseMessage": "Invalid Authorization header format. Expected: Bearer <token>",
			})
			c.Abort()
			return
		}

		token := parts[1]

		// Create HTTP request to validate token
		req, err := http.NewRequestWithContext(c.Request.Context(), "POST", authValidateURL, nil)
		if err != nil {
			slog.Error("Failed to create auth validation request", "error", err)
			c.JSON(http.StatusInternalServerError, gin.H{
				"responseCode":    "500",
				"responseMessage": "Internal server error",
			})
			c.Abort()
			return
		}

		// Set Authorization header for validation request
		req.Header.Set("Authorization", authHeader)
		req.Header.Set("Content-Type", "application/json")

		// Make the validation request
		client := &http.Client{}
		resp, err := client.Do(req)
		if err != nil {
			slog.Error("Failed to validate token", "error", err, "auth_url", authValidateURL)
			c.JSON(http.StatusUnauthorized, gin.H{
				"responseCode":    "401",
				"responseMessage": "Token validation failed",
			})
			c.Abort()
			return
		}
		defer resp.Body.Close()

		// Check validation response status
		if resp.StatusCode != http.StatusOK {
			slog.Warn("Token validation failed", "status", resp.StatusCode, "token_preview", token[:10]+"...")

			var validationResp AuthValidationResponse
			if err := json.NewDecoder(resp.Body).Decode(&validationResp); err == nil {
				c.JSON(http.StatusUnauthorized, gin.H{
					"responseCode":    validationResp.ResponseCode,
					"responseMessage": validationResp.ResponseMessage,
				})
			} else {
				c.JSON(http.StatusUnauthorized, gin.H{
					"responseCode":    "401",
					"responseMessage": "Unauthorized",
				})
			}
			c.Abort()
			return
		}

		// Token is valid, decode response
		var validationResp AuthValidationResponse
		if err := json.NewDecoder(resp.Body).Decode(&validationResp); err != nil {
			slog.Error("Failed to decode validation response", "error", err)
			c.JSON(http.StatusInternalServerError, gin.H{
				"responseCode":    "500",
				"responseMessage": "Internal server error",
			})
			c.Abort()
			return
		}

		slog.Info("Token validated successfully", "user_data", validationResp.Data)

		// Store user data in context for handlers to use
		if validationResp.Data != nil {
			for key, value := range validationResp.Data {
				c.Set(key, value)
			}
		}

		// Continue to the next handler
		c.Next()
	}
}
