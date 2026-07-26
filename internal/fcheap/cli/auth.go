package cli

import (
	"errors"
	"fmt"
	"net/http"
	"os"
	"time"

	"github.com/abdul-hamid-achik/file.cheap/internal/cloudauth"
	"github.com/spf13/cobra"
)

var (
	authServiceURL string
	authClientName string
)

var authCmd = &cobra.Command{
	Use:   "auth",
	Short: "Authenticate this device with the private artifact console",
}

var authLoginCmd = &cobra.Command{
	Use:   "login",
	Short: "Pair this CLI through a browser and owner email",
	RunE: func(cmd *cobra.Command, args []string) error {
		serviceURL := authServiceURL
		if serviceURL == "" {
			serviceURL = os.Getenv("FILECHEAP_ARTIFACT_SERVICE_URL")
		}
		if serviceURL == "" {
			return fmt.Errorf("--service-url or FILECHEAP_ARTIFACT_SERVICE_URL is required")
		}
		name := authClientName
		if name == "" {
			hostname, err := os.Hostname()
			if err != nil || hostname == "" {
				hostname = "file.cheap CLI"
			}
			name = hostname
		}
		client := cloudauth.NewClient(nil)
		authorization, err := client.Start(GetContext(), serviceURL, name)
		if err != nil {
			return err
		}
		printer.Header("Pair this device")
		printer.KeyValue("URL", authorization.VerificationURI)
		printer.KeyValue("Code", authorization.UserCode)
		printer.Info("Verify the owner email and explicitly approve this device.")
		token, err := client.Poll(GetContext(), serviceURL, authorization)
		if err != nil {
			return err
		}
		credentials, err := cloudauth.FromToken(serviceURL, token, time.Now())
		if err != nil {
			return err
		}
		if err := cloudauth.Save(credentials); err != nil {
			return err
		}
		printer.Success("This device is authenticated")
		printer.Info("The token is stored in the XDG config directory with mode 0600.")
		return nil
	},
}

var authStatusCmd = &cobra.Command{
	Use:   "status",
	Short: "Check the stored console session",
	RunE: func(cmd *cobra.Command, args []string) error {
		credentials, err := cloudauth.Load()
		if err != nil {
			return err
		}
		client := cloudauth.NewClient(nil)
		session, credentials, err := sessionWithRefresh(client, credentials)
		if err != nil {
			return err
		}
		if printer.IsJSON() {
			return printer.JSON(map[string]string{"email": session.Email, "service_url": credentials.ServiceURL, "status": "authenticated"})
		}
		printer.Success("Authenticated as %s", session.Email)
		printer.KeyValue("Service", credentials.ServiceURL)
		return nil
	},
}

var authRefreshCmd = &cobra.Command{
	Use:   "refresh",
	Short: "Rotate the stored refresh token and issue a new access token",
	RunE: func(cmd *cobra.Command, args []string) error {
		credentials, err := cloudauth.Load()
		if err != nil {
			return err
		}
		credentials, err = refreshCredentials(cloudauth.NewClient(nil), credentials)
		if err != nil {
			return err
		}
		printer.Success("Device credential rotated")
		printer.KeyValue("Access expires", credentials.AccessExpiresAt.Format(time.RFC3339))
		return nil
	},
}

var authLogoutCmd = &cobra.Command{
	Use:   "logout",
	Short: "Remove the console credential from this device",
	RunE: func(cmd *cobra.Command, args []string) error {
		credentials, err := cloudauth.Load()
		if err != nil {
			return err
		}
		client := cloudauth.NewClient(nil)
		_, credentials, err = sessionWithRefresh(client, credentials)
		if err != nil {
			return err
		}
		if err := client.Logout(GetContext(), credentials.ServiceURL, credentials.Token); err != nil {
			return err
		}
		if err := cloudauth.Drop(); err != nil {
			return err
		}
		printer.Success("Local console credential removed")
		return nil
	},
}

func init() {
	authLoginCmd.Flags().StringVar(&authServiceURL, "service-url", "", "file.cheap service origin")
	authLoginCmd.Flags().StringVar(&authClientName, "client-name", "", "Device label shown on the approval page")
	authLoginCmd.PreRunE = func(cmd *cobra.Command, args []string) error {
		if jsonOutput || quietMode {
			return fmt.Errorf("fcheap auth login is interactive and does not support --json or --quiet")
		}
		return nil
	}
	authCmd.AddCommand(authLoginCmd, authStatusCmd, authRefreshCmd, authLogoutCmd)
}

func sessionWithRefresh(client *cloudauth.Client, credentials cloudauth.Credentials) (cloudauth.Session, cloudauth.Credentials, error) {
	session, err := client.Session(GetContext(), credentials.ServiceURL, credentials.Token)
	if err == nil {
		return session, credentials, nil
	}
	var remote *cloudauth.RemoteError
	if !errors.As(err, &remote) || remote.Status != http.StatusUnauthorized || credentials.RefreshToken == "" {
		return cloudauth.Session{}, credentials, err
	}
	credentials, err = refreshCredentials(client, credentials)
	if err != nil {
		return cloudauth.Session{}, credentials, err
	}
	session, err = client.Session(GetContext(), credentials.ServiceURL, credentials.Token)
	return session, credentials, err
}

func refreshCredentials(client *cloudauth.Client, credentials cloudauth.Credentials) (cloudauth.Credentials, error) {
	pending, err := cloudauth.BeginRefresh(credentials)
	if err != nil {
		return credentials, err
	}
	// Persist the candidate and rotation id before the network call. A retry after
	// an ambiguous response therefore sends the exact same idempotent rotation.
	if err := cloudauth.Save(pending); err != nil {
		return credentials, err
	}
	token, err := client.Refresh(
		GetContext(),
		pending.ServiceURL,
		pending.RefreshToken,
		pending.PendingRefreshToken,
		pending.PendingRotationID,
	)
	if err != nil {
		return pending, err
	}
	completed, err := cloudauth.CompleteRefresh(pending, token, time.Now())
	if err != nil {
		return pending, err
	}
	if err := cloudauth.Save(completed); err != nil {
		return pending, err
	}
	return completed, nil
}
