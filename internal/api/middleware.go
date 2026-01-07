package api

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/abdul-hamid-achik/file-processor/internal/auth"
	"github.com/abdul-hamid-achik/file-processor/internal/billing"
	"github.com/abdul-hamid-achik/file-processor/internal/db"
	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
)

type contextKey string

const (
	UserIDKey  contextKey = "user_id"
	BillingKey contextKey = "billing"
)

type BillingInfo struct {
	Tier                 db.SubscriptionTier
	Status               db.SubscriptionStatus
	FilesLimit           int
	MaxFileSize          int64
	FilesCount           int64
	TransformationsCount int
	TransformationsLimit int
}

type TokenQuerier interface {
	GetAPITokenByHash(ctx context.Context, tokenHash string) (db.GetAPITokenByHashRow, error)
	UpdateAPITokenLastUsed(ctx context.Context, id pgtype.UUID) error
}

func DualAuthMiddleware(jwtSecret string, queries TokenQuerier) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			authHeader := r.Header.Get("Authorization")
			if authHeader == "" {
				http.Error(w, `{"error":{"code":"unauthorized","message":"missing authorization header"}}`, http.StatusUnauthorized)
				return
			}

			tokenString := extractBearerToken(authHeader)
			if tokenString == "" {
				http.Error(w, `{"error":{"code":"unauthorized","message":"invalid authorization format"}}`, http.StatusUnauthorized)
				return
			}

			if strings.HasPrefix(tokenString, auth.APITokenPrefix) && queries != nil {
				handleAPIKeyAuth(w, r, next, tokenString, queries)
				return
			}

			handleJWTAuth(w, r, next, tokenString, jwtSecret)
		})
	}
}

func handleAPIKeyAuth(w http.ResponseWriter, r *http.Request, next http.Handler, token string, queries TokenQuerier) {
	rawToken := strings.TrimPrefix(token, auth.APITokenPrefix)
	tokenHash := auth.HashToken(rawToken)

	row, err := queries.GetAPITokenByHash(r.Context(), tokenHash)
	if err != nil {
		http.Error(w, `{"error":{"code":"unauthorized","message":"invalid API token"}}`, http.StatusUnauthorized)
		return
	}

	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		queries.UpdateAPITokenLastUsed(ctx, row.ID)
	}()

	userID, err := uuid.FromBytes(row.UserID.Bytes[:])
	if err != nil {
		http.Error(w, `{"error":{"code":"unauthorized","message":"invalid user ID"}}`, http.StatusUnauthorized)
		return
	}

	ctx := context.WithValue(r.Context(), UserIDKey, userID)
	next.ServeHTTP(w, r.WithContext(ctx))
}

func handleJWTAuth(w http.ResponseWriter, r *http.Request, next http.Handler, tokenString, jwtSecret string) {
	token, err := parseToken(tokenString, jwtSecret)
	if err != nil || !token.Valid {
		http.Error(w, `{"error":{"code":"unauthorized","message":"invalid JWT token"}}`, http.StatusUnauthorized)
		return
	}

	claims, ok := token.Claims.(jwt.MapClaims)
	if !ok {
		http.Error(w, `{"error":{"code":"unauthorized","message":"invalid token claims"}}`, http.StatusUnauthorized)
		return
	}

	sub, ok := claims["sub"].(string)
	if !ok {
		http.Error(w, `{"error":{"code":"unauthorized","message":"missing subject claim"}}`, http.StatusUnauthorized)
		return
	}

	userID, err := uuid.Parse(sub)
	if err != nil {
		http.Error(w, `{"error":{"code":"unauthorized","message":"invalid user ID in token"}}`, http.StatusUnauthorized)
		return
	}

	ctx := context.WithValue(r.Context(), UserIDKey, userID)
	next.ServeHTTP(w, r.WithContext(ctx))
}

func AuthMiddleware(jwtSecret string) func(http.Handler) http.Handler {
	return DualAuthMiddleware(jwtSecret, nil)
}

func GetUserID(ctx context.Context) (uuid.UUID, bool) {
	id, ok := ctx.Value(UserIDKey).(uuid.UUID)
	return id, ok
}

func parseToken(tokenString, secret string) (*jwt.Token, error) {
	return jwt.Parse(tokenString, func(token *jwt.Token) (interface{}, error) {
		return []byte(secret), nil
	})
}

func extractBearerToken(authHeader string) string {
	if strings.HasPrefix(authHeader, "Bearer ") {
		return strings.TrimPrefix(authHeader, "Bearer ")
	}
	return ""
}

func CORS(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Accept, Authorization, Content-Type")
		w.Header().Set("Access-Control-Max-Age", "3600")

		if r.Method == "OPTIONS" {
			w.WriteHeader(http.StatusNoContent)
			return
		}

		next.ServeHTTP(w, r)
	})
}

type RateLimiter struct {
	rate    int
	burst   int
	buckets map[string]*bucket
	mu      sync.Mutex
}

type bucket struct {
	tokens    int
	lastReset time.Time
}

func NewRateLimiter(rate, burst int) *RateLimiter {
	return &RateLimiter{
		rate:    rate,
		burst:   burst,
		buckets: make(map[string]*bucket),
	}
}

func (rl *RateLimiter) Allow(key string) bool {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	if rl.burst <= 0 {
		return false
	}

	now := time.Now()
	b, exists := rl.buckets[key]
	if !exists {
		rl.buckets[key] = &bucket{tokens: rl.burst - 1, lastReset: now}
		return true
	}

	elapsed := now.Sub(b.lastReset)
	tokensToAdd := int(elapsed.Seconds()) * rl.rate
	b.tokens += tokensToAdd
	if b.tokens > rl.burst {
		b.tokens = rl.burst
	}
	b.lastReset = now

	if b.tokens > 0 {
		b.tokens--
		return true
	}
	return false
}

func RateLimit(limiter *RateLimiter) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			key := r.RemoteAddr
			if userID, ok := GetUserID(r.Context()); ok {
				key = userID.String()
			}

			if !limiter.Allow(key) {
				w.Header().Set("Content-Type", "application/json")
				http.Error(w, `{"error":{"code":"rate_limit_exceeded","message":"too many requests"}}`, http.StatusTooManyRequests)
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}

type BillingQuerier interface {
	GetUserBillingInfo(ctx context.Context, id pgtype.UUID) (db.GetUserBillingInfoRow, error)
	GetUserFilesCount(ctx context.Context, userID pgtype.UUID) (int64, error)
}

func BillingMiddleware(queries BillingQuerier) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			userID, ok := GetUserID(r.Context())
			if !ok {
				next.ServeHTTP(w, r)
				return
			}

			if queries == nil {
				next.ServeHTTP(w, r)
				return
			}

			pgUserID := pgtype.UUID{Bytes: userID, Valid: true}

			billingRow, err := queries.GetUserBillingInfo(r.Context(), pgUserID)
			if err != nil {
				writeJSONError(w, "billing_error", "Failed to load billing information", http.StatusInternalServerError)
				return
			}

			filesCount, _ := queries.GetUserFilesCount(r.Context(), pgUserID)

			tierLimits := billing.GetTierLimits(billingRow.SubscriptionTier)

			billingInfo := &BillingInfo{
				Tier:                 billingRow.SubscriptionTier,
				Status:               billingRow.SubscriptionStatus,
				FilesLimit:           int(billingRow.FilesLimit),
				MaxFileSize:          billingRow.MaxFileSize,
				FilesCount:           filesCount,
				TransformationsCount: 0,
				TransformationsLimit: tierLimits.TransformationsLimit,
			}

			if !isReadOnlyMethod(r.Method) {
				if !billing.CanUseAPI(billingInfo.Tier, true) {
					writeJSONError(w, "upgrade_required", "API write access requires Pro subscription", http.StatusForbidden)
					return
				}
			}

			ctx := context.WithValue(r.Context(), BillingKey, billingInfo)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

func GetBilling(ctx context.Context) *BillingInfo {
	b, _ := ctx.Value(BillingKey).(*BillingInfo)
	return b
}

func isReadOnlyMethod(method string) bool {
	return method == http.MethodGet || method == http.MethodHead || method == http.MethodOptions
}

func writeJSONError(w http.ResponseWriter, code, message string, status int) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(map[string]any{
		"error": map[string]string{
			"code":    code,
			"message": message,
		},
	})
}
