package auth

import (
	"context"
	"net/http"
)

// Context key type to avoid collisions
type contextKey string

const (
	// UserContextKey is the context key for the authenticated user
	UserContextKey contextKey = "user"
)

// RequireAuth is middleware that requires a valid session.
// If no valid session exists, it redirects to the login page.
func RequireAuth(sm *SessionManager) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			user, err := sm.GetSession(r.Context(), r)
			if err != nil || user == nil {
				// Redirect to login with return URL
				returnURL := r.URL.Path
				if r.URL.RawQuery != "" {
					returnURL += "?" + r.URL.RawQuery
				}
				http.Redirect(w, r, "/login?return="+returnURL, http.StatusFound)
				return
			}

			// Add user to context
			ctx := context.WithValue(r.Context(), UserContextKey, user)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// RequireEmailVerified is middleware that requires the user's email to be verified.
// Must be used after RequireAuth.
func RequireEmailVerified(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		user := GetUserFromContext(r.Context())
		if user == nil {
			http.Redirect(w, r, "/login", http.StatusFound)
			return
		}

		if user.EmailVerifiedAt == nil {
			http.Redirect(w, r, "/verify-email", http.StatusFound)
			return
		}

		next.ServeHTTP(w, r)
	})
}

// RequireAdmin is middleware that requires admin role.
// Must be used after RequireAuth.
func RequireAdmin(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		user := GetUserFromContext(r.Context())
		if user == nil {
			http.Redirect(w, r, "/login", http.StatusFound)
			return
		}

		if user.Role != "admin" {
			http.Error(w, "Forbidden", http.StatusForbidden)
			return
		}

		next.ServeHTTP(w, r)
	})
}

// OptionalAuth is middleware that loads the session if present but doesn't require it.
func OptionalAuth(sm *SessionManager) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			user, _ := sm.GetSession(r.Context(), r)
			if user != nil {
				ctx := context.WithValue(r.Context(), UserContextKey, user)
				r = r.WithContext(ctx)
			}
			next.ServeHTTP(w, r)
		})
	}
}

// GetUserFromContext retrieves the user from the request context.
// Returns nil if no user is set.
func GetUserFromContext(ctx context.Context) *SessionUser {
	user, _ := ctx.Value(UserContextKey).(*SessionUser)
	return user
}

// RedirectIfAuthenticated redirects to dashboard if user is already logged in.
func RedirectIfAuthenticated(sm *SessionManager) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			user, _ := sm.GetSession(r.Context(), r)
			if user != nil {
				http.Redirect(w, r, "/dashboard", http.StatusFound)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}
