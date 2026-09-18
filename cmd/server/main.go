package main

import (
	"context"
	"log"

	"github.com/ERBM-Capstone/ERBM-backend/config"
	jwtmanager "github.com/ERBM-Capstone/ERBM-backend/internal/pkg/jwt"
	"github.com/ERBM-Capstone/ERBM-backend/internal/router"
	"github.com/ERBM-Capstone/ERBM-backend/internal/service"
	"github.com/jackc/pgx/v5/pgxpool"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

// @title           ERBM API
// @version         1.0
// @description     Backend service for the Expense Reimbursement and Bill Management System (ERBM).
// @termsOfService  http://swagger.io/terms/

// @contact.name   API Support
// @contact.email  support@your-org.com

// @license.name  MIT

// @host      localhost:8080
// @BasePath  /

// @securityDefinitions.apikey BearerAuth
// @in header
// @name Authorization
func main() {
	cfg := config.Load()
	tokens, err := jwtmanager.NewManager(
		cfg.JWTSecret,
		cfg.JWTRefreshSecret,
		cfg.JWTIssuer,
		cfg.JWTAccessTokenTTL,
		cfg.JWTRefreshTokenTTL,
	)
	if err != nil {
		log.Fatalf("invalid JWT configuration: %v", err)
	}

	// Connect to PostgreSQL
	pool, err := pgxpool.New(context.Background(), cfg.DatabaseURL)
	if err != nil {
		log.Fatalf("unable to connect to database: %v", err)
	}
	defer pool.Close()

	if err := pool.Ping(context.Background()); err != nil {
		log.Fatalf("database ping failed: %v", err)
	}

	authDB, err := gorm.Open(postgres.Open(cfg.DatabaseURL), &gorm.Config{})
	if err != nil {
		log.Fatalf("unable to initialize GORM: %v", err)
	}
	sqlDB, err := authDB.DB()
	if err != nil {
		log.Fatalf("unable to access GORM database connection: %v", err)
	}
	defer sqlDB.Close()
	if err := sqlDB.PingContext(context.Background()); err != nil {
		log.Fatalf("GORM database ping failed: %v", err)
	}
	log.Println("connected to database")

	// Build and run the router
	r := router.New(pool, authDB, tokens, cfg.RefreshCookieSecure, service.LoginPolicy{
		MaxFailedAttempts: cfg.AuthMaxFailedLoginAttempts,
		LockDuration:      cfg.AuthLoginLockDuration,
	})
	log.Printf("server starting on :%s", cfg.Port)
	if err := r.Run(":" + cfg.Port); err != nil {
		log.Fatalf("server error: %v", err)
	}
}
