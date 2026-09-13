package middleware

import (
	"strings"

	"github.com/ERBM-Capstone/ERBM-backend/dto"
	jwtmanager "github.com/ERBM-Capstone/ERBM-backend/internal/pkg/jwt"
	"github.com/gin-gonic/gin"
)

const (
	// ContextKeyUserID stores the verified user's ID in Gin context.
	ContextKeyUserID = "userID"
	// ContextKeyRole stores the verified user's role in Gin context.
	ContextKeyRole = "role"
)

type accessTokenParser interface {
	ParseAccessToken(encodedToken string) (*jwtmanager.Claims, error)
}

// RequireAuth validates an access JWT Bearer token and injects trusted claims.
func RequireAuth(tokens accessTokenParser) gin.HandlerFunc {
	return func(c *gin.Context) {
		parts := strings.Fields(c.GetHeader("Authorization"))
		if len(parts) != 2 || !strings.EqualFold(parts[0], "Bearer") {
			dto.HandleError(c, dto.ErrUnauthorized)
			c.Abort()
			return
		}

		claims, err := tokens.ParseAccessToken(parts[1])
		if err != nil {
			dto.HandleError(c, dto.ErrUnauthorized)
			c.Abort()
			return
		}

		c.Set(ContextKeyUserID, claims.UserID)
		c.Set(ContextKeyRole, claims.Role)
		c.Next()
	}
}

// RequireRole checks that the authenticated user has at least one allowed role.
func RequireRole(roles ...string) gin.HandlerFunc {
	return func(c *gin.Context) {
		value, exists := c.Get(ContextKeyRole)
		role, ok := value.(string)
		if !exists || !ok || strings.TrimSpace(role) == "" {
			dto.HandleError(c, dto.ErrUnauthorized)
			c.Abort()
			return
		}

		for _, allowedRole := range roles {
			if strings.EqualFold(role, allowedRole) {
				c.Next()
				return
			}
		}

		dto.HandleError(c, dto.ErrForbidden)
		c.Abort()
	}
}
