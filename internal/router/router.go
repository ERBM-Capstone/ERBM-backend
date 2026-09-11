package router

import (
	"github.com/ERBM-Capstone/ERBM-backend/internal/handler"
	"github.com/ERBM-Capstone/ERBM-backend/internal/middleware"
	jwtmanager "github.com/ERBM-Capstone/ERBM-backend/internal/pkg/jwt"
	"github.com/ERBM-Capstone/ERBM-backend/internal/repository"
	"github.com/ERBM-Capstone/ERBM-backend/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5/pgxpool"
	"gorm.io/gorm"
)

// New builds and returns the root Gin engine with all routes registered.
func New(pool *pgxpool.Pool, authDB *gorm.DB, tokens *jwtmanager.Manager) *gin.Engine {
	r := gin.Default()

	// ── API v1 ────────────────────────────────────────────────────────────────
	v1 := r.Group("/api/v1")

	// ── Authentication ────────────────────────────────────────────────────────
	authRepo := repository.NewAuthRepository(authDB)
	authSvc := service.NewAuthService(authRepo, tokens)
	authHandler := handler.NewAuthHandler(authSvc)
	v1.POST("/auth/login", authHandler.Login)

	protected := v1.Group("")
	protected.Use(middleware.RequireAuth())

	// ── Example ───────────────────────────────────────────────────────────────
	examplesRepo := repository.NewExampleRepository(pool)
	examplesSvc := service.NewExampleService(examplesRepo)
	examplesHandler := handler.NewExampleHandler(examplesSvc)

	examples := protected.Group("/examples")
	{
		examples.GET("", examplesHandler.List)
		examples.GET("/:id", examplesHandler.GetByID)

		examplesWrite := examples.Group("")
		examplesWrite.Use(middleware.RequireRole("admin", "editor"))
		{
			examplesWrite.POST("", examplesHandler.Create)
			examplesWrite.PUT("/:id", examplesHandler.Update)
			examplesWrite.DELETE("/:id", examplesHandler.Delete)
		}
	}

	return r
}
