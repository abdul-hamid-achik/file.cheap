package auth

import (
	"context"
	"net"
	"net/http"
	"net/netip"
	"strings"
	"time"

	"github.com/abdul-hamid-achik/file.cheap/internal/apperror"
	"github.com/abdul-hamid-achik/file.cheap/internal/db"
	"github.com/abdul-hamid-achik/file.cheap/internal/metrics"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
)

const (
	SessionCookieName = "session"
	FlashCookieName   = "flash"
	SessionDuration   = 30 * 24 * time.Hour // 30 days
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
	ID               uuid.UUID
	Email            string
	Name             string
	AvatarURL        *string
	Role             db.UserRole
	SubscriptionTier db.SubscriptionTier
	EmailVerifiedAt  *time.Time
	SessionID        uuid.UUID
	HasPassword      bool
}

// CreateSession creates a new session for a user and sets the cookie.
func (sm *SessionManager) CreateSession(ctx context.Context, w http.ResponseWriter, r *http.Request, userID uuid.UUID) error {
	token, tokenHash, err := GenerateToken()
	if err != nil {
		metrics.RecordAuthOperation("create_session", "error")
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
		metrics.RecordAuthOperation("create_session", "error")
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

	metrics.RecordAuthOperation("create_session", "success")
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
		ID:               userID,
		Email:            row.Email,
		Name:             row.Name,
		AvatarURL:        row.AvatarUrl,
		Role:             row.Role,
		SubscriptionTier: row.SubscriptionTier,
		SessionID:        sessionID,
		HasPassword:      row.HasPassword,
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
		metrics.RecordAuthOperation("delete_session", "error")
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

	metrics.RecordAuthOperation("delete_session", "success")
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

// SetFlash sets a flash message cookie that will be cleared on the next read.
func (sm *SessionManager) SetFlash(w http.ResponseWriter, key, value string) {
	http.SetCookie(w, &http.Cookie{
		Name:     FlashCookieName + "_" + key,
		Value:    value,
		Path:     "/",
		HttpOnly: true,
		Secure:   sm.secure,
		SameSite: http.SameSiteLaxMode,
		MaxAge:   300, // 5 minutes
	})
}

// GetFlash retrieves and clears a flash message.
func (sm *SessionManager) GetFlash(w http.ResponseWriter, r *http.Request, key string) string {
	cookie, err := r.Cookie(FlashCookieName + "_" + key)
	if err != nil {
		return ""
	}

	http.SetCookie(w, &http.Cookie{
		Name:     FlashCookieName + "_" + key,
		Value:    "",
		Path:     "/",
		HttpOnly: true,
		Secure:   sm.secure,
		SameSite: http.SameSiteLaxMode,
		MaxAge:   -1,
	})

	return cookie.Value
}

// trustedProxies defines the IP ranges that are trusted to set X-Forwarded-For.
// In production, this should be configured to match your load balancer/proxy IPs.
// Common ranges include:
// - 10.0.0.0/8 (private class A)
// - 172.16.0.0/12 (private class B)
// - 192.168.0.0/16 (private class C)
// - 127.0.0.0/8 (localhost)
var trustedProxies = []netip.Prefix{
	netip.MustParsePrefix("10.0.0.0/8"),
	netip.MustParsePrefix("172.16.0.0/12"),
	netip.MustParsePrefix("192.168.0.0/16"),
	netip.MustParsePrefix("127.0.0.0/8"),
	netip.MustParsePrefix("::1/128"),
}

// isTrustedProxy checks if an IP address belongs to a trusted proxy network.
func isTrustedProxy(ip netip.Addr) bool {
	for _, prefix := range trustedProxies {
		if prefix.Contains(ip) {
			return true
		}
	}
	return false
}

// getClientIP extracts the client IP from the request.
// X-Forwarded-For and X-Real-IP headers are only trusted when the request
// comes from a known proxy IP to prevent header spoofing.
func getClientIP(r *http.Request) *netip.Addr {
	// First, get the remote address (the direct connection)
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return nil
	}

	remoteIP, err := netip.ParseAddr(host)
	if err != nil {
		return nil
	}

	// Only trust proxy headers if the request comes from a trusted proxy
	if isTrustedProxy(remoteIP) {
		// X-Forwarded-For may contain multiple IPs; take the first (client) IP
		if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
			// Take the first IP in the chain (leftmost)
			parts := strings.Split(xff, ",")
			if len(parts) > 0 {
				clientIP := strings.TrimSpace(parts[0])
				if ip, err := netip.ParseAddr(clientIP); err == nil {
					return &ip
				}
			}
		}

		if xri := r.Header.Get("X-Real-IP"); xri != "" {
			if ip, err := netip.ParseAddr(xri); err == nil {
				return &ip
			}
		}
	}

	return &remoteIP
}
