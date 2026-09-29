package router

import (
	"time"

	_ "github.com/ERBM-Capstone/ERBM-backend/docs"
	"github.com/ERBM-Capstone/ERBM-backend/internal/handler"
	"github.com/ERBM-Capstone/ERBM-backend/internal/mail"
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
func New(pool *pgxpool.Pool, authDB *gorm.DB, tokens *jwtmanager.Manager, refreshCookieSecure bool, loginPolicy service.LoginPolicy, activationConfig service.ActivationConfig, mailSender mail.Sender) *gin.Engine {
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
	authSvc := service.NewAuthService(authRepo, tokens, loginPolicy)
	activationRepo := repository.NewAccountActivationTokenRepository(authDB)
	activationSvc := service.NewActivationService(activationRepo, mailSender, activationConfig)
	authHandler := handler.NewAuthHandler(&authActivationService{AuthService: authSvc, ActivationService: activationSvc}, tokens.RefreshTokenTTL(), refreshCookieSecure)
	auth := v1.Group("/auth")
	{
		auth.POST("/login", authHandler.Login)
		auth.POST("/refresh", authHandler.Refresh)
		auth.POST("/logout", authHandler.Logout)
		auth.POST("/activation/set-password", authHandler.SetInitialPassword)
		auth.POST("/activation/resend", authHandler.ResendActivation)
	}

	protected := v1.Group("")
	protected.Use(middleware.RequireAuth(tokens))
	protected.GET("/me", authHandler.Me)

	rolesRepo := repository.NewRoleRepository(authDB)
	rolesSvc := service.NewRoleService(rolesRepo)
	rolesHandler := handler.NewRoleHandler(rolesSvc)
	roles := protected.Group("/roles")
	roles.Use(middleware.RequireRole("ADMIN"))
	{
		roles.GET("", rolesHandler.List)
	}

	// ── Employees ─────────────────────────────────────────────────────────────
	employeesRepo := repository.NewEmployeeRepository(authDB)
	usersRepo := repository.NewUserRepository(authDB)
	employeesSvc := service.NewEmployeeService(employeesRepo, usersRepo, activationSvc)
	employeesHandler := handler.NewEmployeeHandler(employeesSvc)
	employees := protected.Group("/employees")
	employees.Use(middleware.RequireRole("ADMIN"))
	{
		employees.GET("", employeesHandler.List)
		employees.GET("/:id", employeesHandler.GetByID)
		employees.POST("", employeesHandler.Create)
		employees.PATCH("/:id", employeesHandler.Update)
		employees.PATCH("/:id/deactivate", employeesHandler.Deactivate)
		employees.POST("/:id/resend-activation", employeesHandler.ResendActivation)
	}

	// ── Projects ──────────────────────────────────────────────────────────────
	projectsRepo := repository.NewProjectRepository(authDB)
	projectsSvc := service.NewProjectService(projectsRepo)
	projectsHandler := handler.NewProjectHandler(projectsSvc)
	projects := protected.Group("/projects")
	projects.Use(middleware.RequireRole("ADMIN", "PM", "PC", "ACCOUNTING"))
	{
		projects.GET("", projectsHandler.List)
		projects.GET("/:id", projectsHandler.GetByID)

		projectsWrite := projects.Group("")
		projectsWrite.Use(middleware.RequireRole("ADMIN", "PM", "PC"))
		{
			projectsWrite.POST("", projectsHandler.Create)
			projectsWrite.PATCH("/:id", projectsHandler.Update)
		}
	}

	return r
}

// authActivationService keeps the existing auth service and the invitation
// workflow behind the handler's intentionally narrow interface.
type authActivationService struct {
	*service.AuthService
	*service.ActivationService
}
