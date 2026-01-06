package auth

import (
	"context"
	"net"
	"net/http"
	"net/netip"
	"time"

	"github.com/abdul-hamid-achik/file-processor/internal/apperror"
	"github.com/abdul-hamid-achik/file-processor/internal/db"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
)

const (
	// SessionCookieName is the name of the session cookie
	SessionCookieName = "session"
	// SessionDuration is how long a session lasts
	SessionDuration = 30 * 24 * time.Hour // 30 days
)

// SessionManager handles user sessions.
type SessionManager struct {
	queries *db.Queries
	secure  bool // Whether to use secure cookies (HTTPS only)
}

// NewSessionManager creates a new session manager.
func NewSessionManager(queries *db.Queries, secure bool) *SessionManager {
	return &SessionManager{
		queries: queries,
		secure:  secure,
	}
}

// SessionUser represents user data from a session lookup.
type SessionUser struct {
	ID              uuid.UUID
	Email           string
	Name            string
	AvatarURL       *string
	Role            db.UserRole
	EmailVerifiedAt *time.Time
	SessionID       uuid.UUID
}

// CreateSession creates a new session for a user and sets the cookie.
func (sm *SessionManager) CreateSession(ctx context.Context, w http.ResponseWriter, r *http.Request, userID uuid.UUID) error {
	token, tokenHash, err := GenerateToken()
	if err != nil {
		return apperror.Wrap(err, apperror.ErrInternal)
	}

	userAgent := r.UserAgent()
	ipAddr := getClientIP(r)

	pgUserID := pgtype.UUID{Bytes: userID, Valid: true}

	var ipAddrPtr *netip.Addr
	if ipAddr != nil {
		ipAddrPtr = ipAddr
	}

	_, err = sm.queries.CreateSession(ctx, db.CreateSessionParams{
		UserID:    pgUserID,
		TokenHash: tokenHash,
		UserAgent: &userAgent,
		IpAddress: ipAddrPtr,
		ExpiresAt: pgtype.Timestamptz{Time: time.Now().Add(SessionDuration), Valid: true},
	})
	if err != nil {
		return apperror.Wrap(err, apperror.ErrInternal)
	}

	http.SetCookie(w, &http.Cookie{
		Name:     SessionCookieName,
		Value:    token,
		Path:     "/",
		HttpOnly: true,
		Secure:   sm.secure,
		SameSite: http.SameSiteLaxMode,
		MaxAge:   int(SessionDuration.Seconds()),
	})

	return nil
}

// GetSession retrieves the current session from the request.
// Returns nil if no valid session exists.
func (sm *SessionManager) GetSession(ctx context.Context, r *http.Request) (*SessionUser, error) {
	cookie, err := r.Cookie(SessionCookieName)
	if err != nil {
		return nil, nil // No cookie = no session
	}

	tokenHash := HashToken(cookie.Value)

	row, err := sm.queries.GetSessionByTokenHash(ctx, tokenHash)
	if err != nil {
		return nil, nil // Session not found or expired
	}

	userID, err := uuid.FromBytes(row.UserID.Bytes[:])
	if err != nil {
		return nil, apperror.Wrap(err, apperror.ErrInternal)
	}

	sessionID, err := uuid.FromBytes(row.ID.Bytes[:])
	if err != nil {
		return nil, apperror.Wrap(err, apperror.ErrInternal)
	}

	user := &SessionUser{
		ID:        userID,
		Email:     row.Email,
		Name:      row.Name,
		AvatarURL: row.AvatarUrl,
		Role:      row.Role,
		SessionID: sessionID,
	}

	if row.EmailVerifiedAt.Valid {
		user.EmailVerifiedAt = &row.EmailVerifiedAt.Time
	}

	return user, nil
}

// DeleteSession removes the current session.
func (sm *SessionManager) DeleteSession(ctx context.Context, w http.ResponseWriter, r *http.Request) error {
	cookie, err := r.Cookie(SessionCookieName)
	if err != nil {
		return nil
	}

	tokenHash := HashToken(cookie.Value)

	if err := sm.queries.DeleteSessionByTokenHash(ctx, tokenHash); err != nil {
		return apperror.Wrap(err, apperror.ErrInternal)
	}

	http.SetCookie(w, &http.Cookie{
		Name:     SessionCookieName,
		Value:    "",
		Path:     "/",
		HttpOnly: true,
		Secure:   sm.secure,
		SameSite: http.SameSiteLaxMode,
		MaxAge:   -1,
	})

	return nil
}

// DeleteAllUserSessions removes all sessions for a user.
func (sm *SessionManager) DeleteAllUserSessions(ctx context.Context, userID uuid.UUID) error {
	pgUserID := pgtype.UUID{Bytes: userID, Valid: true}
	return sm.queries.DeleteUserSessions(ctx, pgUserID)
}

// CleanupExpiredSessions removes all expired sessions from the database.
func (sm *SessionManager) CleanupExpiredSessions(ctx context.Context) error {
	return sm.queries.DeleteExpiredSessions(ctx)
}

// getClientIP extracts the client IP from the request.
func getClientIP(r *http.Request) *netip.Addr {
	// Check X-Forwarded-For header first (for proxies)
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		// Take the first IP in the chain
		if ip, err := netip.ParseAddr(xff); err == nil {
			return &ip
		}
	}

	// Check X-Real-IP header
	if xri := r.Header.Get("X-Real-IP"); xri != "" {
		if ip, err := netip.ParseAddr(xri); err == nil {
			return &ip
		}
	}

	// Fall back to RemoteAddr
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return nil
	}

	if ip, err := netip.ParseAddr(host); err == nil {
		return &ip
	}

	return nil
}
