package audit

import (
	"context"
	"encoding/json"
	"net"
	"net/http"
	"net/netip"

	"github.com/abdul-hamid-achik/file.cheap/internal/db"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
)

type Action string

const (
	ActionFileUpload         Action = "file.upload"
	ActionFileDownload       Action = "file.download"
	ActionFileDelete         Action = "file.delete"
	ActionFileShare          Action = "file.share"
	ActionShareAccess        Action = "share.access"
	ActionShareDelete        Action = "share.delete"
	ActionUserLogin          Action = "user.login"
	ActionUserLogout         Action = "user.logout"
	ActionUserPasswordChange Action = "user.password_change"
	ActionSettingsUpdate     Action = "settings.update"
	ActionAPITokenCreate     Action = "api_token.create"
	ActionAPITokenDelete     Action = "api_token.delete"
	ActionWebhookCreate      Action = "webhook.create"
	ActionWebhookDelete      Action = "webhook.delete"
)

type Entry struct {
	UserID       uuid.UUID
	Action       Action
	ResourceType string
	ResourceID   uuid.UUID
	IPAddress    string
	UserAgent    string
	Metadata     map[string]any
}

type AuditQuerier interface {
	CreateAuditLog(ctx context.Context, arg db.CreateAuditLogParams) (db.AuditLog, error)
}

type Logger struct {
	queries AuditQuerier
}

func NewLogger(queries AuditQuerier) *Logger {
	return &Logger{queries: queries}
}

func (l *Logger) Log(ctx context.Context, entry Entry) error {
	if l.queries == nil {
		return nil
	}

	var metadataJSON []byte
	if entry.Metadata != nil {
		var err error
		metadataJSON, err = json.Marshal(entry.Metadata)
		if err != nil {
			metadataJSON = nil
		}
	}

	var ipAddress *netip.Addr
	if entry.IPAddress != "" {
		addr, err := netip.ParseAddr(entry.IPAddress)
		if err == nil {
			ipAddress = &addr
		}
	}

	var userAgent *string
	if entry.UserAgent != "" {
		userAgent = &entry.UserAgent
	}

	_, err := l.queries.CreateAuditLog(ctx, db.CreateAuditLogParams{
		UserID:       pgtype.UUID{Bytes: entry.UserID, Valid: entry.UserID != uuid.Nil},
		Action:       db.AuditAction(entry.Action),
		ResourceType: entry.ResourceType,
		ResourceID:   pgtype.UUID{Bytes: entry.ResourceID, Valid: entry.ResourceID != uuid.Nil},
		IpAddress:    ipAddress,
		UserAgent:    userAgent,
		Metadata:     metadataJSON,
	})
	return err
}

func (l *Logger) LogFromRequest(ctx context.Context, r *http.Request, entry Entry) error {
	if entry.IPAddress == "" {
		entry.IPAddress = getClientIP(r)
	}
	if entry.UserAgent == "" {
		entry.UserAgent = r.UserAgent()
	}
	return l.Log(ctx, entry)
}

func getClientIP(r *http.Request) string {
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		return xff
	}
	if xri := r.Header.Get("X-Real-IP"); xri != "" {
		return xri
	}
	host, _, _ := net.SplitHostPort(r.RemoteAddr)
	return host
}
