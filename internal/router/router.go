package router

import (
	"time"

	_ "github.com/ERBM-Capstone/ERBM-backend/docs"
	"github.com/ERBM-Capstone/ERBM-backend/internal/handler"
	"github.com/ERBM-Capstone/ERBM-backend/internal/middleware"
	jwtmanager "github.com/ERBM-Capstone/ERBM-backend/internal/pkg/jwt"
	"github.com/ERBM-Capstone/ERBM-backend/internal/repository"
	"github.com/ERBM-Capstone/ERBM-backend/internal/service"
	"github.com/gin-contrib/cors"
	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5/pgxpool"
	swaggerFiles "github.com/swaggo/files"
	ginSwagger "github.com/swaggo/gin-swagger"
	"gorm.io/gorm"
)

// New builds and returns the root Gin engine with all routes registered.
func New(pool *pgxpool.Pool, authDB *gorm.DB, tokens *jwtmanager.Manager, refreshCookieSecure bool) *gin.Engine {
	r := gin.Default()
	r.Use(cors.New(cors.Config{
		AllowOrigins: []string{
			"http://localhost:3000",
		},

		AllowMethods: []string{"GET", "POST", "PUT", "PATCH", "DELETE", "HEAD", "OPTIONS"},

		AllowHeaders: []string{
			"Origin",
			"Content-Type",
			"Authorization",
		},

		AllowCredentials: true,

		MaxAge: 12 * time.Hour,
	}))
	r.GET("/swagger/*any", ginSwagger.WrapHandler(swaggerFiles.Handler))

	// ── API v1 ────────────────────────────────────────────────────────────────
	v1 := r.Group("/api/v1")

	// ── Authentication ────────────────────────────────────────────────────────
	authRepo := repository.NewAuthRepository(authDB)
	authSvc := service.NewAuthService(authRepo, tokens)
	authHandler := handler.NewAuthHandler(authSvc, tokens.RefreshTokenTTL(), refreshCookieSecure)
	auth := v1.Group("/auth")
	{
		auth.POST("/login", authHandler.Login)
		auth.POST("/refresh", authHandler.Refresh)
		auth.POST("/logout", authHandler.Logout)
	}

	protected := v1.Group("")
	// protected.Use(middleware.RequireAuth(tokens))

	// ── Employees ─────────────────────────────────────────────────────────────
	employeesRepo := repository.NewEmployeeRepository(authDB)
	usersRepo := repository.NewUserRepository(authDB)
	employeesSvc := service.NewEmployeeService(employeesRepo, usersRepo)
	employeesHandler := handler.NewEmployeeHandler(employeesSvc)
	employees := protected.Group("/employees")
	// employees.Use(middleware.RequireRole("ADMIN"))
	{
		employees.POST("", employeesHandler.Create)
	}

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
