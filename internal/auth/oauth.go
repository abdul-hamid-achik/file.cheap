package auth

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	"github.com/abdul-hamid-achik/file.cheap/internal/apperror"
	"github.com/abdul-hamid-achik/file.cheap/internal/db"
	"github.com/abdul-hamid-achik/file.cheap/internal/metrics"
	"github.com/jackc/pgx/v5/pgtype"
	"golang.org/x/oauth2"
	"golang.org/x/oauth2/github"
	"golang.org/x/oauth2/google"
)

// OAuthConfig holds configuration for OAuth providers.
type OAuthConfig struct {
	GoogleClientID     string
	GoogleClientSecret string
	GitHubClientID     string
	GitHubClientSecret string
	BaseURL            string // e.g., "https://example.com"
}

// OAuthService handles OAuth authentication.
type OAuthService struct {
	queries      *db.Queries
	googleConfig *oauth2.Config
	githubConfig *oauth2.Config
}

// NewOAuthService creates a new OAuth service.
func NewOAuthService(queries *db.Queries, cfg OAuthConfig) *OAuthService {
	svc := &OAuthService{
		queries: queries,
	}

	if cfg.GoogleClientID != "" && cfg.GoogleClientSecret != "" {
		svc.googleConfig = &oauth2.Config{
			ClientID:     cfg.GoogleClientID,
			ClientSecret: cfg.GoogleClientSecret,
			RedirectURL:  cfg.BaseURL + "/auth/google/callback",
			Scopes:       []string{"openid", "email", "profile"},
			Endpoint:     google.Endpoint,
		}
	}

	if cfg.GitHubClientID != "" && cfg.GitHubClientSecret != "" {
		svc.githubConfig = &oauth2.Config{
			ClientID:     cfg.GitHubClientID,
			ClientSecret: cfg.GitHubClientSecret,
			RedirectURL:  cfg.BaseURL + "/auth/github/callback",
			Scopes:       []string{"user:email", "read:user"},
			Endpoint:     github.Endpoint,
		}
	}

	return svc
}

// OAuthUserInfo contains user information from an OAuth provider.
type OAuthUserInfo struct {
	Provider   db.OauthProvider
	ProviderID string
	Email      string
	Name       string
	AvatarURL  string
}

// GetGoogleAuthURL returns the Google OAuth authorization URL.
func (s *OAuthService) GetGoogleAuthURL(state string) (string, error) {
	if s.googleConfig == nil {
		return "", apperror.ErrServiceUnavailable
	}
	return s.googleConfig.AuthCodeURL(state, oauth2.AccessTypeOffline), nil
}

// GetGitHubAuthURL returns the GitHub OAuth authorization URL.
func (s *OAuthService) GetGitHubAuthURL(state string) (string, error) {
	if s.githubConfig == nil {
		return "", apperror.ErrServiceUnavailable
	}
	return s.githubConfig.AuthCodeURL(state), nil
}

// ExchangeGoogleCode exchanges a Google OAuth code for tokens and user info.
func (s *OAuthService) ExchangeGoogleCode(ctx context.Context, code string) (*OAuthUserInfo, *oauth2.Token, error) {
	if s.googleConfig == nil {
		return nil, nil, apperror.ErrServiceUnavailable
	}

	token, err := s.googleConfig.Exchange(ctx, code)
	if err != nil {
		metrics.RecordAuthOperation("oauth_google_exchange", "error")
		return nil, nil, apperror.Wrap(err, apperror.ErrUnauthorized)
	}

	userInfo, err := s.getGoogleUserInfo(ctx, token)
	if err != nil {
		metrics.RecordAuthOperation("oauth_google_exchange", "error")
		return nil, nil, err
	}

	metrics.RecordAuthOperation("oauth_google_exchange", "success")
	return userInfo, token, nil
}

// ExchangeGitHubCode exchanges a GitHub OAuth code for tokens and user info.
func (s *OAuthService) ExchangeGitHubCode(ctx context.Context, code string) (*OAuthUserInfo, *oauth2.Token, error) {
	if s.githubConfig == nil {
		return nil, nil, apperror.ErrServiceUnavailable
	}

	token, err := s.githubConfig.Exchange(ctx, code)
	if err != nil {
		metrics.RecordAuthOperation("oauth_github_exchange", "error")
		return nil, nil, apperror.Wrap(err, apperror.ErrUnauthorized)
	}

	userInfo, err := s.getGitHubUserInfo(ctx, token)
	if err != nil {
		metrics.RecordAuthOperation("oauth_github_exchange", "error")
		return nil, nil, err
	}

	metrics.RecordAuthOperation("oauth_github_exchange", "success")
	return userInfo, token, nil
}

func (s *OAuthService) getGoogleUserInfo(ctx context.Context, token *oauth2.Token) (*OAuthUserInfo, error) {
	client := s.googleConfig.Client(ctx, token)
	resp, err := client.Get("https://www.googleapis.com/oauth2/v2/userinfo")
	if err != nil {
		return nil, apperror.Wrap(err, apperror.ErrInternal)
	}
	defer func() { _ = resp.Body.Close() }()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, apperror.Wrap(err, apperror.ErrInternal)
	}

	var data struct {
		ID      string `json:"id"`
		Email   string `json:"email"`
		Name    string `json:"name"`
		Picture string `json:"picture"`
	}
	if err := json.Unmarshal(body, &data); err != nil {
		return nil, apperror.Wrap(err, apperror.ErrInternal)
	}

	return &OAuthUserInfo{
		Provider:   db.OauthProviderGoogle,
		ProviderID: data.ID,
		Email:      data.Email,
		Name:       data.Name,
		AvatarURL:  data.Picture,
	}, nil
}

func (s *OAuthService) getGitHubUserInfo(ctx context.Context, token *oauth2.Token) (*OAuthUserInfo, error) {
	client := s.githubConfig.Client(ctx, token)

	resp, err := client.Get("https://api.github.com/user")
	if err != nil {
		return nil, apperror.Wrap(err, apperror.ErrInternal)
	}
	defer func() { _ = resp.Body.Close() }()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, apperror.Wrap(err, apperror.ErrInternal)
	}

	var userData struct {
		ID        int64  `json:"id"`
		Login     string `json:"login"`
		Name      string `json:"name"`
		Email     string `json:"email"`
		AvatarURL string `json:"avatar_url"`
	}
	if err := json.Unmarshal(body, &userData); err != nil {
		return nil, apperror.Wrap(err, apperror.ErrInternal)
	}

	email := userData.Email
	if email == "" {
		email, _ = s.getGitHubPrimaryEmail(ctx, client)
	}

	name := userData.Name
	if name == "" {
		name = userData.Login
	}

	return &OAuthUserInfo{
		Provider:   db.OauthProviderGithub,
		ProviderID: fmt.Sprintf("%d", userData.ID),
		Email:      email,
		Name:       name,
		AvatarURL:  userData.AvatarURL,
	}, nil
}

func (s *OAuthService) getGitHubPrimaryEmail(ctx context.Context, client *http.Client) (string, error) {
	resp, err := client.Get("https://api.github.com/user/emails")
	if err != nil {
		return "", apperror.Wrap(err, apperror.ErrInternal)
	}
	defer func() { _ = resp.Body.Close() }()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", apperror.Wrap(err, apperror.ErrInternal)
	}

	var emails []struct {
		Email    string `json:"email"`
		Primary  bool   `json:"primary"`
		Verified bool   `json:"verified"`
	}
	if err := json.Unmarshal(body, &emails); err != nil {
		return "", apperror.Wrap(err, apperror.ErrInternal)
	}

	for _, e := range emails {
		if e.Primary && e.Verified {
			return e.Email, nil
		}
	}

	return "", apperror.ErrNotFound
}

// OAuthLoginResult contains the result of an OAuth login.
type OAuthLoginResult struct {
	User       *db.User
	IsNewUser  bool
	OAuthToken *oauth2.Token
}

// FindOrCreateUser finds an existing user by OAuth account or creates a new one.
func (s *OAuthService) FindOrCreateUser(ctx context.Context, info *OAuthUserInfo, token *oauth2.Token) (*OAuthLoginResult, error) {
	oauthAccount, err := s.queries.GetOAuthAccountWithUser(ctx, db.GetOAuthAccountWithUserParams{
		Provider:       info.Provider,
		ProviderUserID: info.ProviderID,
	})
	if err == nil {
		var expiresAt pgtype.Timestamptz
		if token.Expiry.IsZero() {
			expiresAt = pgtype.Timestamptz{Valid: false}
		} else {
			expiresAt = pgtype.Timestamptz{Time: token.Expiry, Valid: true}
		}

		_ = s.queries.UpdateOAuthTokens(ctx, db.UpdateOAuthTokensParams{
			Provider:       info.Provider,
			ProviderUserID: info.ProviderID,
			AccessToken:    &token.AccessToken,
			RefreshToken:   &token.RefreshToken,
			ExpiresAt:      expiresAt,
		})

		user, err := s.queries.GetUserByID(ctx, oauthAccount.UserID)
		if err != nil {
			metrics.RecordAuthOperation("oauth_login", "error")
			return nil, apperror.Wrap(err, apperror.ErrInternal)
		}

		metrics.RecordAuthOperation("oauth_login", "success")
		return &OAuthLoginResult{
			User:       &user,
			IsNewUser:  false,
			OAuthToken: token,
		}, nil
	}

	existingUser, err := s.queries.GetUserByEmail(ctx, info.Email)
	if err == nil {
		var expiresAt pgtype.Timestamptz
		if token.Expiry.IsZero() {
			expiresAt = pgtype.Timestamptz{Valid: false}
		} else {
			expiresAt = pgtype.Timestamptz{Time: token.Expiry, Valid: true}
		}

		_, err := s.queries.CreateOAuthAccount(ctx, db.CreateOAuthAccountParams{
			UserID:         existingUser.ID,
			Provider:       info.Provider,
			ProviderUserID: info.ProviderID,
			AccessToken:    &token.AccessToken,
			RefreshToken:   &token.RefreshToken,
			ExpiresAt:      expiresAt,
		})
		if err != nil {
			metrics.RecordAuthOperation("oauth_login", "error")
			return nil, apperror.Wrap(err, apperror.ErrInternal)
		}

		metrics.RecordAuthOperation("oauth_login", "success")
		return &OAuthLoginResult{
			User:       &existingUser,
			IsNewUser:  false,
			OAuthToken: token,
		}, nil
	}

	var avatarURL *string
	if info.AvatarURL != "" {
		avatarURL = &info.AvatarURL
	}

	newUser, err := s.queries.CreateUser(ctx, db.CreateUserParams{
		Email:        info.Email,
		PasswordHash: nil,
		Name:         info.Name,
		AvatarUrl:    avatarURL,
		Role:         db.UserRoleUser,
	})
	if err != nil {
		metrics.RecordAuthOperation("oauth_login", "error")
		return nil, apperror.Wrap(err, apperror.ErrInternal)
	}

	if err := s.queries.VerifyUserEmail(ctx, newUser.ID); err != nil {
		metrics.RecordAuthOperation("oauth_login", "error")
		return nil, apperror.Wrap(err, apperror.ErrInternal)
	}

	var expiresAt pgtype.Timestamptz
	if token.Expiry.IsZero() {
		expiresAt = pgtype.Timestamptz{Valid: false}
	} else {
		expiresAt = pgtype.Timestamptz{Time: token.Expiry, Valid: true}
	}

	_, err = s.queries.CreateOAuthAccount(ctx, db.CreateOAuthAccountParams{
		UserID:         newUser.ID,
		Provider:       info.Provider,
		ProviderUserID: info.ProviderID,
		AccessToken:    &token.AccessToken,
		RefreshToken:   &token.RefreshToken,
		ExpiresAt:      expiresAt,
	})
	if err != nil {
		metrics.RecordAuthOperation("oauth_login", "error")
		return nil, apperror.Wrap(err, apperror.ErrInternal)
	}

	newUser, err = s.queries.GetUserByID(ctx, newUser.ID)
	if err != nil {
		metrics.RecordAuthOperation("oauth_login", "error")
		return nil, apperror.Wrap(err, apperror.ErrInternal)
	}

	metrics.RecordAuthOperation("oauth_login", "success")
	return &OAuthLoginResult{
		User:       &newUser,
		IsNewUser:  true,
		OAuthToken: token,
	}, nil
}

// GenerateState generates a random state token for OAuth CSRF protection.
func GenerateState() (string, error) {
	token, _, err := GenerateToken()
	return token, err
}

// IsGoogleConfigured returns whether Google OAuth is configured.
func (s *OAuthService) IsGoogleConfigured() bool {
	return s.googleConfig != nil
}

// IsGitHubConfigured returns whether GitHub OAuth is configured.
func (s *OAuthService) IsGitHubConfigured() bool {
	return s.githubConfig != nil
}

var (
	ErrOAuthAlreadyLinked = apperror.New("already_linked", "This account is already connected", http.StatusConflict)
	ErrOAuthLinkedToOther = apperror.New("linked_to_other", "This account is linked to another user", http.StatusConflict)
	ErrLastAuthMethod     = apperror.New("last_auth_method", "Cannot disconnect your only sign-in method", http.StatusBadRequest)
)

// LinkOAuthAccount links an OAuth provider to an existing user.
func (s *OAuthService) LinkOAuthAccount(ctx context.Context, userID pgtype.UUID, info *OAuthUserInfo, token *oauth2.Token) error {
	existing, err := s.queries.GetOAuthAccount(ctx, db.GetOAuthAccountParams{
		Provider:       info.Provider,
		ProviderUserID: info.ProviderID,
	})
	if err == nil {
		if existing.UserID == userID {
			metrics.RecordAuthOperation("link_oauth", "error")
			return ErrOAuthAlreadyLinked
		}
		metrics.RecordAuthOperation("link_oauth", "error")
		return ErrOAuthLinkedToOther
	}

	var expiresAt pgtype.Timestamptz
	if !token.Expiry.IsZero() {
		expiresAt = pgtype.Timestamptz{Time: token.Expiry, Valid: true}
	}

	_, err = s.queries.CreateOAuthAccount(ctx, db.CreateOAuthAccountParams{
		UserID:         userID,
		Provider:       info.Provider,
		ProviderUserID: info.ProviderID,
		AccessToken:    &token.AccessToken,
		RefreshToken:   &token.RefreshToken,
		ExpiresAt:      expiresAt,
	})
	if err != nil {
		metrics.RecordAuthOperation("link_oauth", "error")
		return apperror.Wrap(err, apperror.ErrInternal)
	}

	metrics.RecordAuthOperation("link_oauth", "success")
	return nil
}

// ListUserOAuthAccounts returns all OAuth accounts linked to a user.
func (s *OAuthService) ListUserOAuthAccounts(ctx context.Context, userID pgtype.UUID) ([]db.OauthAccount, error) {
	accounts, err := s.queries.ListUserOAuthAccounts(ctx, userID)
	if err != nil {
		return nil, apperror.Wrap(err, apperror.ErrInternal)
	}
	return accounts, nil
}

// DeleteOAuthAccount removes an OAuth account link from a user.
func (s *OAuthService) DeleteOAuthAccount(ctx context.Context, userID pgtype.UUID, provider db.OauthProvider) error {
	err := s.queries.DeleteOAuthAccount(ctx, db.DeleteOAuthAccountParams{
		UserID:   userID,
		Provider: provider,
	})
	if err != nil {
		metrics.RecordAuthOperation("unlink_oauth", "error")
		return err
	}
	metrics.RecordAuthOperation("unlink_oauth", "success")
	return nil
}

// CanDisconnectOAuth checks if a user can disconnect an OAuth provider.
func (s *OAuthService) CanDisconnectOAuth(ctx context.Context, userID pgtype.UUID, hasPassword bool) (bool, error) {
	if hasPassword {
		return true, nil
	}

	accounts, err := s.ListUserOAuthAccounts(ctx, userID)
	if err != nil {
		return false, err
	}

	return len(accounts) > 1, nil
}
