package cli

import (
	"context"
	"fmt"
	"time"

	"github.com/pkg/browser"
	"github.com/spf13/cobra"
)

var authCmd = &cobra.Command{
	Use:   "auth",
	Short: "Authenticate with file.cheap",
	Long:  `Manage authentication for the file.cheap CLI.`,
}

var authLoginCmd = &cobra.Command{
	Use:   "login",
	Short: "Login to file.cheap",
	Long: `Authenticate with file.cheap using device flow (browser) or API key.

Examples:
  fc auth login                        # Interactive browser login
  fc auth login --api-key fp_xxxxx     # Login with API key`,
	RunE: runAuthLogin,
}

var authStatusCmd = &cobra.Command{
	Use:   "status",
	Short: "Show authentication status",
	RunE:  runAuthStatus,
}

var authLogoutCmd = &cobra.Command{
	Use:   "logout",
	Short: "Logout from file.cheap",
	RunE:  runAuthLogout,
}

var apiKeyFlag string

func init() {
	authCmd.AddCommand(authLoginCmd)
	authCmd.AddCommand(authStatusCmd)
	authCmd.AddCommand(authLogoutCmd)

	authLoginCmd.Flags().StringVar(&apiKeyFlag, "api-key", "", "API key (skips browser flow)")
}

func runAuthLogin(cmd *cobra.Command, args []string) error {
	if apiKeyFlag != "" {
		cfg.APIKey = apiKeyFlag
		if err := cfg.Save(); err != nil {
			return fmt.Errorf("failed to save config: %w", err)
		}
		printer.Success("Logged in with API key")
		return nil
	}

	ctx := context.Background()
	printer.Info("Starting device authentication...")

	deviceResp, err := apiClient.DeviceAuth(ctx)
	if err != nil {
		return fmt.Errorf("failed to start device auth: %w", err)
	}

	verifyURL := fmt.Sprintf("%s?code=%s", deviceResp.VerificationURI, deviceResp.UserCode)

	printer.Println()
	printer.Printf("Opening browser to: %s\n", verifyURL)
	printer.Printf("Your code: %s\n", deviceResp.UserCode)
	printer.Println()
	printer.Info("Waiting for authorization...")

	if err := browser.OpenURL(verifyURL); err != nil {
		printer.Warn("Could not open browser automatically")
		printer.Printf("Please open this URL manually: %s\n", verifyURL)
	}

	pollInterval := time.Duration(deviceResp.Interval) * time.Second
	if pollInterval < time.Second {
		pollInterval = 5 * time.Second
	}

	timeout := time.Duration(deviceResp.ExpiresIn) * time.Second
	if timeout == 0 {
		timeout = 15 * time.Minute
	}

	deadline := time.Now().Add(timeout)

	for time.Now().Before(deadline) {
		time.Sleep(pollInterval)

		tokenResp, err := apiClient.DeviceToken(ctx, deviceResp.DeviceCode)
		if err != nil {
			continue
		}

		if tokenResp.Error != "" {
			if tokenResp.Error == "authorization_pending" {
				continue
			}
			if tokenResp.Error == "slow_down" {
				pollInterval += 5 * time.Second
				continue
			}
			if tokenResp.Error == "expired_token" {
				return fmt.Errorf("authorization expired, please try again")
			}
			if tokenResp.Error == "access_denied" {
				return fmt.Errorf("authorization denied")
			}
			return fmt.Errorf("authorization failed: %s", tokenResp.ErrorDescription)
		}

		if tokenResp.APIKey != "" {
			cfg.APIKey = tokenResp.APIKey
			if err := cfg.Save(); err != nil {
				return fmt.Errorf("failed to save config: %w", err)
			}
			printer.Println()
			printer.Success("Logged in successfully!")
			return nil
		}
	}

	return fmt.Errorf("authorization timed out, please try again")
}

func runAuthStatus(cmd *cobra.Command, args []string) error {
	if jsonOutput {
		return printer.JSON(map[string]interface{}{
			"authenticated": cfg.IsAuthenticated(),
			"base_url":      cfg.BaseURL,
		})
	}

	printer.Section("Authentication Status")
	if cfg.IsAuthenticated() {
		printer.KeyValue("Status", "Authenticated")
		maskedKey := maskAPIKey(cfg.APIKey)
		printer.KeyValue("API Key", maskedKey)
	} else {
		printer.KeyValue("Status", "Not authenticated")
	}
	printer.KeyValue("Base URL", cfg.BaseURL)
	printer.Println()

	return nil
}

func runAuthLogout(cmd *cobra.Command, args []string) error {
	if !cfg.IsAuthenticated() {
		printer.Warn("Not currently logged in")
		return nil
	}

	if err := cfg.ClearAuth(); err != nil {
		return fmt.Errorf("failed to clear auth: %w", err)
	}

	printer.Success("Logged out successfully")
	return nil
}

func maskAPIKey(key string) string {
	if len(key) <= 10 {
		if len(key) <= 4 {
			return "****"
		}
		return key[:2] + "..." + key[len(key)-2:]
	}
	return key[:6] + "..." + key[len(key)-4:]
}
