package middleware

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
)

// ContextKeyUserID is the key used to store the authenticated user's ID in the Gin context.
const ContextKeyUserID = "userID"

// RequireAuth validates the Supabase JWT Bearer token and injects the user ID into the context.
// NOTE: Replace the stub below with real JWT verification (e.g. github.com/golang-jwt/jwt/v5).
func RequireAuth() gin.HandlerFunc {
	return func(c *gin.Context) {
		authHeader := c.GetHeader("Authorization")
		if authHeader == "" || !strings.HasPrefix(authHeader, "Bearer ") {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "missing bearer token"})
			return
		}

		token := strings.TrimPrefix(authHeader, "Bearer ")
		// TODO: verify token with config.Config.SupabaseJWTSecret and extract claims.
		// For now we use the raw token as a stub user ID so the project compiles.
		userID := token

		c.Set(ContextKeyUserID, userID)
		c.Next()
	}
}

// RequireRole returns a middleware that checks the user has at least one of the given roles.
// NOTE: Extend this once real JWT claims are parsed.
func RequireRole(roles ...string) gin.HandlerFunc {
	return func(c *gin.Context) {
		// TODO: read role from JWT claims stored in context and compare against roles.
		_ = roles
		c.Next()
	}
}
