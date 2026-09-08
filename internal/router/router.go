package router

import (
	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/your-org/project-hub-backend/internal/handler"
	"github.com/your-org/project-hub-backend/internal/middleware"
	"github.com/your-org/project-hub-backend/internal/repository"
	"github.com/your-org/project-hub-backend/internal/service"
)

// New builds and returns the root Gin engine with all routes registered.
func New(pool *pgxpool.Pool) *gin.Engine {
	r := gin.Default()

	// ── API v1 ────────────────────────────────────────────────────────────────
	v1 := r.Group("/api/v1")
	v1.Use(middleware.RequireAuth())

	// ── Example ───────────────────────────────────────────────────────────────
	examplesRepo    := repository.NewExampleRepository(pool)
	examplesSvc     := service.NewExampleService(examplesRepo)
	examplesHandler := handler.NewExampleHandler(examplesSvc)

	examples := v1.Group("/examples")
	{
		examples.GET("",     examplesHandler.List)
		examples.GET("/:id", examplesHandler.GetByID)

		examplesWrite := examples.Group("")
		examplesWrite.Use(middleware.RequireRole("admin", "editor"))
		{
			examplesWrite.POST("",      examplesHandler.Create)
			examplesWrite.PUT("/:id",   examplesHandler.Update)
			examplesWrite.DELETE("/:id", examplesHandler.Delete)
		}
	}

	return r
}
