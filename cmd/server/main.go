package main

import (
	"context"
	"log"
	"time"

	"github.com/ERBM-Capstone/ERBM-backend/config"
	"github.com/ERBM-Capstone/ERBM-backend/internal/mail"
	"github.com/ERBM-Capstone/ERBM-backend/internal/ocr"
	jwtmanager "github.com/ERBM-Capstone/ERBM-backend/internal/pkg/jwt"
	"github.com/ERBM-Capstone/ERBM-backend/internal/repository"
	"github.com/ERBM-Capstone/ERBM-backend/internal/router"
	"github.com/ERBM-Capstone/ERBM-backend/internal/service"
	"github.com/ERBM-Capstone/ERBM-backend/internal/storage"
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

// Omit host and schemes so Swagger UI uses the origin serving the documentation.
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

	db, err := gorm.Open(postgres.Open(cfg.DatabaseURL), &gorm.Config{})
	if err != nil {
		log.Fatalf("unable to initialize GORM: %v", err)
	}
	sqlDB, err := db.DB()
	if err != nil {
		log.Fatalf("unable to access GORM database connection: %v", err)
	}
	defer sqlDB.Close()
	if err := sqlDB.PingContext(context.Background()); err != nil {
		log.Fatalf("GORM database ping failed: %v", err)
	}
	log.Println("connected to database")

	// Build and run the router
	localStore, err := storage.NewLocal(cfg.ReceiptStorageRoot)
	if err != nil {
		log.Fatal("unable to initialize private receipt storage")
	}
	providers := []storage.Provider{localStore}
	if cfg.ReceiptGCSBucket != "" {
		gcsStore, err := storage.NewGCS(context.Background(), cfg.ReceiptGCSBucket)
		if err != nil {
			log.Fatal("unable to initialize GCS receipt storage")
		}
		providers = append(providers, gcsStore)
	}
	stores := storage.NewManager(cfg.ReceiptStorageProvider, providers...)
	defer stores.Close()
	if _, err := stores.Current(); err != nil {
		log.Fatal("invalid receipt storage provider or missing GCS bucket")
	}
	// expenseOptions := service.ExpenseOptions{Storage: stores}
	aiProvider, err := ocr.NewHTTPProvider(ocr.HTTPConfig{
		OCRBaseURL: cfg.OCRBaseURL,
		LLMBaseURL: cfg.LLMBaseURL,
		Timeout:    cfg.AIrequestTimeout,
	})
	if err != nil {
		log.Fatal(err)
	}

	expenseOptions := service.ExpenseOptions{
		Storage:    stores,
		OCR:        aiProvider,
		OCRTimeout: cfg.AIrequestTimeout,
	}
	maintenance := service.NewExpenseService(repository.NewExpenseRepository(db), expenseOptions)
	maintenanceCtx, stopMaintenance := context.WithCancel(context.Background())
	defer stopMaintenance()
	go func() {
		ticker := time.NewTicker(time.Minute)
		defer ticker.Stop()
		for {
			select {
			case <-maintenanceCtx.Done():
				return
			case <-ticker.C:
				ctx, cancel := context.WithTimeout(maintenanceCtx, 45*time.Second)
				if maintenance.RunMaintenance(ctx) != nil {
					log.Print("receipt cleanup will retry")
				}
				cancel()
			}
		}
	}()
	// log.Print("private OCR adapter awaits the server API contract; OCR requests return 503")
	mailSender := mail.NewGmailSMTPSender(cfg.SMTPHost, cfg.SMTPPort, cfg.SMTPUsername, cfg.SMTPGmailAppPassword, cfg.SMTPFromAddress)
	r := router.New(db, tokens, cfg.RefreshCookieSecure, service.LoginPolicy{
		MaxFailedAttempts: cfg.AuthMaxFailedLoginAttempts,
		LockDuration:      cfg.AuthLoginLockDuration,
	}, service.ActivationConfig{FrontendURL: cfg.FrontendActivationURL}, mailSender, expenseOptions)
	log.Printf("server starting on :%s", cfg.Port)
	if err := r.Run(":" + cfg.Port); err != nil {
		log.Fatalf("server error: %v", err)
	}
}
