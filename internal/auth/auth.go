package auth

import (
	"context"
	"strings"
	"time"

	"github.com/abdul-hamid-achik/file.cheap/internal/apperror"
	"github.com/abdul-hamid-achik/file.cheap/internal/db"
	"github.com/abdul-hamid-achik/file.cheap/internal/logger"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
)

const (
	// PasswordResetExpiry is how long a password reset token is valid
	PasswordResetExpiry = 1 * time.Hour
	// EmailVerificationExpiry is how long an email verification token is valid
	EmailVerificationExpiry = 24 * time.Hour
)

// API Token Permissions
const (
	PermFilesRead     = "files:read"
	PermFilesWrite    = "files:write"
	PermFilesDelete   = "files:delete"
	PermTransform     = "transform"
	PermSharesRead    = "shares:read"
	PermSharesWrite   = "shares:write"
	PermWebhooksRead  = "webhooks:read"
	PermWebhooksWrite = "webhooks:write"
)

// AllPermissions contains all available permissions
var AllPermissions = []string{
	PermFilesRead,
	PermFilesWrite,
	PermFilesDelete,
	PermTransform,
	PermSharesRead,
	PermSharesWrite,
	PermWebhooksRead,
	PermWebhooksWrite,
}

// PermissionPresets defines common permission combinations
var PermissionPresets = map[string][]string{
	"read_only": {PermFilesRead, PermSharesRead},
	"standard":  {PermFilesRead, PermFilesWrite, PermTransform, PermSharesRead, PermSharesWrite},
	"full":      AllPermissions,
}

// CreateAPITokenInput contains parameters for creating an API token
type CreateAPITokenInput struct {
	Name        string
	Permissions []string
	ExpiresAt   *time.Time
}

// Service provides authentication operations.
type Service struct {
	queries *db.Queries
}

// NewService creates a new auth service.
func NewService(queries *db.Queries) *Service {
	return &Service{queries: queries}
}

// RegisterInput contains the data needed to register a new user.
type RegisterInput struct {
	Email    string
	Password string
	Name     string
}

// RegisterResult contains the result of a registration.
type RegisterResult struct {
	User                   *db.User
	EmailVerificationToken string // Raw token to send to user
}

// Register creates a new user account.
func (s *Service) Register(ctx context.Context, input RegisterInput) (*RegisterResult, error) {
	log := logger.FromContext(ctx)

	if err := ValidatePassword(input.Password); err != nil {
		log.Debug("registration failed: invalid password", "email", input.Email)
		return nil, apperror.Wrap(err, apperror.ErrWeakPassword)
	}

	passwordHash, err := HashPassword(input.Password)
	if err != nil {
		return nil, apperror.Wrap(err, apperror.ErrInternal)
	}

	user, err := s.queries.CreateUser(ctx, db.CreateUserParams{
		Email:        input.Email,
		PasswordHash: &passwordHash,
		Name:         input.Name,
		AvatarUrl:    nil,
		Role:         db.UserRoleUser,
	})
	if err != nil {
		log.Warn("registration failed: could not create user", "email", input.Email, "error", err)
		if strings.Contains(err.Error(), "duplicate") || strings.Contains(err.Error(), "unique") {
			return nil, apperror.Wrap(err, apperror.ErrEmailTaken)
		}
		return nil, apperror.Wrap(err, apperror.ErrInternal)
	}

	token, tokenHash, err := GenerateToken()
	if err != nil {
		return nil, apperror.Wrap(err, apperror.ErrInternal)
	}

	_, err = s.queries.CreateEmailVerification(ctx, db.CreateEmailVerificationParams{
		UserID:    user.ID,
		TokenHash: tokenHash,
		ExpiresAt: pgtype.Timestamptz{Time: time.Now().Add(EmailVerificationExpiry), Valid: true},
	})
	if err != nil {
		return nil, apperror.Wrap(err, apperror.ErrInternal)
	}

	log.Info("user registered", "email", input.Email)

	return &RegisterResult{
		User:                   &user,
		EmailVerificationToken: token,
	}, nil
}

// LoginInput contains the data needed to log in.
type LoginInput struct {
	Email    string
	Password string
}

// Login authenticates a user by email and password.
func (s *Service) Login(ctx context.Context, input LoginInput) (*db.User, error) {
	log := logger.FromContext(ctx)

	user, err := s.queries.GetUserByEmail(ctx, input.Email)
	if err != nil {
		log.Debug("login failed: user not found", "email", input.Email)
		return nil, apperror.ErrInvalidCredentials
	}

	if user.PasswordHash == nil {
		log.Debug("login failed: no password set", "email", input.Email)
		return nil, apperror.ErrOAuthOnly
	}

	if err := CheckPassword(input.Password, *user.PasswordHash); err != nil {
		log.Warn("login failed: invalid password", "email", input.Email)
		return nil, apperror.ErrInvalidCredentials
	}

	log.Info("user logged in", "email", input.Email)
	return &user, nil
}

// VerifyEmail verifies a user's email using the token.
func (s *Service) VerifyEmail(ctx context.Context, token string) error {
	log := logger.FromContext(ctx)
	tokenHash := HashToken(token)

	verification, err := s.queries.GetEmailVerificationByTokenHash(ctx, tokenHash)
	if err != nil {
		log.Debug("email verification failed: invalid token")
		return apperror.ErrInvalidToken
	}

	if err := s.queries.MarkEmailVerified(ctx, verification.ID); err != nil {
		return apperror.Wrap(err, apperror.ErrInternal)
	}

	if err := s.queries.VerifyUserEmail(ctx, verification.UserID); err != nil {
		return apperror.Wrap(err, apperror.ErrInternal)
	}

	log.Info("email verified")
	return nil
}

// RequestPasswordResetResult contains the result of a password reset request.
type RequestPasswordResetResult struct {
	Token string // Raw token to send to user
	Email string // User's email for sending
}

// RequestPasswordReset generates a password reset token.
func (s *Service) RequestPasswordReset(ctx context.Context, email string) (*RequestPasswordResetResult, error) {
	log := logger.FromContext(ctx)

	user, err := s.queries.GetUserByEmail(ctx, email)
	if err != nil {
		log.Debug("password reset requested for unknown email", "email", email)
		return nil, nil
	}

	if err := s.queries.DeleteUserPasswordResets(ctx, user.ID); err != nil {
		return nil, apperror.Wrap(err, apperror.ErrInternal)
	}

	token, tokenHash, err := GenerateToken()
	if err != nil {
		return nil, apperror.Wrap(err, apperror.ErrInternal)
	}

	_, err = s.queries.CreatePasswordReset(ctx, db.CreatePasswordResetParams{
		UserID:    user.ID,
		TokenHash: tokenHash,
		ExpiresAt: pgtype.Timestamptz{Time: time.Now().Add(PasswordResetExpiry), Valid: true},
	})
	if err != nil {
		return nil, apperror.Wrap(err, apperror.ErrInternal)
	}

	log.Info("password reset requested", "email", email)

	return &RequestPasswordResetResult{
		Token: token,
		Email: user.Email,
	}, nil
}

// ResetPassword resets a user's password using the token.
func (s *Service) ResetPassword(ctx context.Context, token, newPassword string) error {
	log := logger.FromContext(ctx)

	if err := ValidatePassword(newPassword); err != nil {
		return apperror.Wrap(err, apperror.ErrWeakPassword)
	}

	tokenHash := HashToken(token)

	reset, err := s.queries.GetPasswordResetByTokenHash(ctx, tokenHash)
	if err != nil {
		log.Debug("password reset failed: invalid token")
		return apperror.ErrInvalidToken
	}

	passwordHash, err := HashPassword(newPassword)
	if err != nil {
		return apperror.Wrap(err, apperror.ErrInternal)
	}

	if err := s.queries.UpdateUserPassword(ctx, db.UpdateUserPasswordParams{
		ID:           reset.UserID,
		PasswordHash: &passwordHash,
	}); err != nil {
		return apperror.Wrap(err, apperror.ErrInternal)
	}

	if err := s.queries.MarkPasswordResetUsed(ctx, reset.ID); err != nil {
		return apperror.Wrap(err, apperror.ErrInternal)
	}

	log.Info("password reset completed")
	return nil
}

// ValidatePasswordResetToken checks if a password reset token is valid.
func (s *Service) ValidatePasswordResetToken(ctx context.Context, token string) (bool, error) {
	tokenHash := HashToken(token)
	_, err := s.queries.GetPasswordResetByTokenHash(ctx, tokenHash)
	if err != nil {
		return false, nil
	}
	return true, nil
}

// CreateVerificationTokenResult contains the result of creating a verification token.
type CreateVerificationTokenResult struct {
	Token string
	Email string
	Name  string
}

// CreateVerificationToken creates a new email verification token for a user by email.
func (s *Service) CreateVerificationToken(ctx context.Context, email string) (*CreateVerificationTokenResult, error) {
	log := logger.FromContext(ctx)

	user, err := s.queries.GetUserByEmail(ctx, email)
	if err != nil {
		log.Debug("verification token requested for unknown email", "email", email)
		return nil, nil
	}

	pgID := pgtype.UUID{Bytes: user.ID.Bytes, Valid: true}
	_ = s.queries.DeleteUserEmailVerifications(ctx, pgID)

	token, tokenHash, err := GenerateToken()
	if err != nil {
		return nil, apperror.Wrap(err, apperror.ErrInternal)
	}

	_, err = s.queries.CreateEmailVerification(ctx, db.CreateEmailVerificationParams{
		UserID:    user.ID,
		TokenHash: tokenHash,
		ExpiresAt: pgtype.Timestamptz{Time: time.Now().Add(EmailVerificationExpiry), Valid: true},
	})
	if err != nil {
		return nil, apperror.Wrap(err, apperror.ErrInternal)
	}

	return &CreateVerificationTokenResult{
		Token: token,
		Email: user.Email,
		Name:  user.Name,
	}, nil
}

// GetUserByID retrieves a user by their ID.
func (s *Service) GetUserByID(ctx context.Context, id uuid.UUID) (*db.User, error) {
	pgID := pgtype.UUID{Bytes: id, Valid: true}
	user, err := s.queries.GetUserByID(ctx, pgID)
	if err != nil {
		return nil, apperror.Wrap(err, apperror.ErrNotFound)
	}
	return &user, nil
}

// VerifyPassword verifies a user's password by user ID.
func (s *Service) VerifyPassword(ctx context.Context, userID uuid.UUID, password string) error {
	pgID := pgtype.UUID{Bytes: userID, Valid: true}
	user, err := s.queries.GetUserByID(ctx, pgID)
	if err != nil {
		return apperror.Wrap(err, apperror.ErrNotFound)
	}

	if user.PasswordHash == nil {
		return apperror.ErrOAuthOnly
	}

	if err := CheckPassword(password, *user.PasswordHash); err != nil {
		return apperror.ErrInvalidCredentials
	}

	return nil
}

// UpdateUserInput contains the data for updating a user.
type UpdateUserInput struct {
	Name      string
	AvatarURL *string
}

// UpdateUser updates a user's profile.
func (s *Service) UpdateUser(ctx context.Context, id uuid.UUID, input UpdateUserInput) (*db.User, error) {
	pgID := pgtype.UUID{Bytes: id, Valid: true}
	user, err := s.queries.UpdateUser(ctx, db.UpdateUserParams{
		ID:        pgID,
		Name:      input.Name,
		AvatarUrl: input.AvatarURL,
	})
	if err != nil {
		return nil, apperror.Wrap(err, apperror.ErrInternal)
	}
	return &user, nil
}

// ChangePassword changes a user's password.
func (s *Service) ChangePassword(ctx context.Context, userID uuid.UUID, currentPassword, newPassword string) error {
	pgID := pgtype.UUID{Bytes: userID, Valid: true}

	user, err := s.queries.GetUserByID(ctx, pgID)
	if err != nil {
		return apperror.Wrap(err, apperror.ErrNotFound)
	}

	if user.PasswordHash == nil {
		return apperror.ErrOAuthOnly
	}

	if err := CheckPassword(currentPassword, *user.PasswordHash); err != nil {
		return apperror.ErrInvalidCredentials
	}

	if err := ValidatePassword(newPassword); err != nil {
		return apperror.Wrap(err, apperror.ErrWeakPassword)
	}

	passwordHash, err := HashPassword(newPassword)
	if err != nil {
		return apperror.Wrap(err, apperror.ErrInternal)
	}

	if err := s.queries.UpdateUserPassword(ctx, db.UpdateUserPasswordParams{
		ID:           pgID,
		PasswordHash: &passwordHash,
	}); err != nil {
		return apperror.Wrap(err, apperror.ErrInternal)
	}

	return nil
}

// GetOrCreateUserSettings retrieves user settings, creating default ones if they don't exist.
func (s *Service) GetOrCreateUserSettings(ctx context.Context, userID uuid.UUID) (*db.UserSetting, error) {
	pgID := pgtype.UUID{Bytes: userID, Valid: true}

	settings, err := s.queries.GetUserSettings(ctx, pgID)
	if err != nil {
		settings, err = s.queries.CreateUserSettings(ctx, pgID)
		if err != nil {
			return nil, apperror.Wrap(err, apperror.ErrInternal)
		}
	}
	return &settings, nil
}

// UpdateNotificationSettings updates a user's notification preferences.
func (s *Service) UpdateNotificationSettings(ctx context.Context, userID uuid.UUID, emailNotifications, processingAlerts, marketingEmails bool) error {
	pgID := pgtype.UUID{Bytes: userID, Valid: true}

	_, err := s.queries.UpsertUserSettings(ctx, pgID)
	if err != nil {
		return apperror.Wrap(err, apperror.ErrInternal)
	}

	_, err = s.queries.UpdateNotificationSettings(ctx, db.UpdateNotificationSettingsParams{
		UserID:             pgID,
		EmailNotifications: emailNotifications,
		ProcessingAlerts:   processingAlerts,
		MarketingEmails:    marketingEmails,
	})
	if err != nil {
		return apperror.Wrap(err, apperror.ErrInternal)
	}
	return nil
}

// UpdateFileSettings updates a user's file preferences.
func (s *Service) UpdateFileSettings(ctx context.Context, userID uuid.UUID, retentionDays int32, autoDelete bool) error {
	pgID := pgtype.UUID{Bytes: userID, Valid: true}

	_, err := s.queries.UpsertUserSettings(ctx, pgID)
	if err != nil {
		return apperror.Wrap(err, apperror.ErrInternal)
	}

	_, err = s.queries.UpdateFileSettings(ctx, db.UpdateFileSettingsParams{
		UserID:               pgID,
		DefaultRetentionDays: retentionDays,
		AutoDeleteOriginals:  autoDelete,
	})
	if err != nil {
		return apperror.Wrap(err, apperror.ErrInternal)
	}
	return nil
}

// ListAPITokens retrieves all API tokens for a user.
func (s *Service) ListAPITokens(ctx context.Context, userID uuid.UUID) ([]db.ApiToken, error) {
	pgID := pgtype.UUID{Bytes: userID, Valid: true}
	tokens, err := s.queries.ListAPITokensByUser(ctx, pgID)
	if err != nil {
		return nil, apperror.Wrap(err, apperror.ErrInternal)
	}
	return tokens, nil
}

const APITokenPrefix = "fp_"

// CreateAPIToken creates a new API token for a user.
// The returned token is prefixed with "fp_" for easy identification.
func (s *Service) CreateAPIToken(ctx context.Context, userID uuid.UUID, input CreateAPITokenInput) (string, *db.ApiToken, error) {
	pgID := pgtype.UUID{Bytes: userID, Valid: true}

	rawToken, tokenHash, err := GenerateToken()
	if err != nil {
		return "", nil, apperror.Wrap(err, apperror.ErrInternal)
	}

	prefixedToken := APITokenPrefix + rawToken
	prefix := prefixedToken[:10]

	// Default to full permissions if none specified
	permissions := input.Permissions
	if len(permissions) == 0 {
		permissions = AllPermissions
	}

	var expiresAt pgtype.Timestamptz
	if input.ExpiresAt != nil {
		expiresAt = pgtype.Timestamptz{Time: *input.ExpiresAt, Valid: true}
	}

	token, err := s.queries.CreateAPIToken(ctx, db.CreateAPITokenParams{
		UserID:      pgID,
		Name:        input.Name,
		TokenHash:   tokenHash,
		TokenPrefix: prefix,
		Permissions: permissions,
		ExpiresAt:   expiresAt,
	})
	if err != nil {
		return "", nil, apperror.Wrap(err, apperror.ErrInternal)
	}

	return prefixedToken, &token, nil
}

// DeleteAPIToken deletes an API token by ID.
func (s *Service) DeleteAPIToken(ctx context.Context, userID, tokenID uuid.UUID) error {
	pgUserID := pgtype.UUID{Bytes: userID, Valid: true}
	pgTokenID := pgtype.UUID{Bytes: tokenID, Valid: true}

	err := s.queries.DeleteAPIToken(ctx, db.DeleteAPITokenParams{
		ID:     pgTokenID,
		UserID: pgUserID,
	})
	if err != nil {
		return apperror.Wrap(err, apperror.ErrInternal)
	}
	return nil
}
