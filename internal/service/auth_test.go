package service

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/ERBM-Capstone/ERBM-backend/dto"
	"github.com/ERBM-Capstone/ERBM-backend/internal/model"
	passwordhash "github.com/ERBM-Capstone/ERBM-backend/internal/pkg/hash"
	jwtmanager "github.com/ERBM-Capstone/ERBM-backend/internal/pkg/jwt"
)

type failedLoginCall struct {
	userID       int64
	maxAttempts  int
	lockDuration time.Duration
}

type authRepositoryStub struct {
	user              *model.User
	role              string
	getByEmailError   error
	recordFailedError error
	updateLoginError  error
	failedLoginCalls  []failedLoginCall
	updateLoginCalls  int
}

func (s *authRepositoryStub) GetUserByEmail(context.Context, string) (*model.User, string, error) {
	return s.user, s.role, s.getByEmailError
}

func (s *authRepositoryStub) GetUserByID(context.Context, int64) (*model.User, string, error) {
	return s.user, s.role, nil
}

func (s *authRepositoryStub) RecordFailedLogin(
	_ context.Context,
	userID int64,
	maxAttempts int,
	lockDuration time.Duration,
) error {
	s.failedLoginCalls = append(s.failedLoginCalls, failedLoginCall{
		userID:       userID,
		maxAttempts:  maxAttempts,
		lockDuration: lockDuration,
	})
	return s.recordFailedError
}

func (s *authRepositoryStub) UpdateLastLogin(context.Context, int64) error {
	s.updateLoginCalls++
	return s.updateLoginError
}

type tokenManagerStub struct {
	accessTokenCalls  int
	refreshTokenCalls int
}

func (s *tokenManagerStub) GenerateAccessToken(int64, string) (string, error) {
	s.accessTokenCalls++
	return "access-token", nil
}

func (s *tokenManagerStub) GenerateRefreshToken(int64) (string, error) {
	s.refreshTokenCalls++
	return "refresh-token", nil
}

func (s *tokenManagerStub) ParseRefreshToken(string) (*jwtmanager.RefreshClaims, error) {
	return &jwtmanager.RefreshClaims{UserID: 1}, nil
}

func (s *tokenManagerStub) AccessTokenTTL() time.Duration {
	return 15 * time.Minute
}

func TestLoginRejectsLockedAccountBeforeCheckingPassword(t *testing.T) {
	now := time.Date(2026, time.September, 13, 12, 0, 0, 0, time.UTC)
	repo := &authRepositoryStub{
		user: activeUserWithPassword(t, "correct-password"),
		role: "PC",
	}
	repo.user.LockedUntil = timePointer(now.Add(time.Minute))
	tokens := &tokenManagerStub{}
	service := NewAuthService(repo, tokens)
	service.now = func() time.Time { return now }

	_, _, err := service.Login(context.Background(), &dto.LoginRequest{
		Email:    "user@example.com",
		Password: "correct-password",
	})

	if !errors.Is(err, dto.ErrAccountLocked) {
		t.Fatalf("Login() error = %v, want %v", err, dto.ErrAccountLocked)
	}
	if err.Error() != "account temporarily locked" {
		t.Fatalf("Login() error message = %q, want %q", err.Error(), "account temporarily locked")
	}
	if len(repo.failedLoginCalls) != 0 || repo.updateLoginCalls != 0 {
		t.Fatalf("repository writes: failed=%d success=%d, want 0, 0", len(repo.failedLoginCalls), repo.updateLoginCalls)
	}
	if tokens.accessTokenCalls != 0 || tokens.refreshTokenCalls != 0 {
		t.Fatal("tokens were generated for a locked account")
	}
}

func TestLoginRecordsFailedPasswordWithLockoutPolicy(t *testing.T) {
	repo := &authRepositoryStub{
		user: activeUserWithPassword(t, "correct-password"),
		role: "PC",
	}
	service := NewAuthService(repo, &tokenManagerStub{})

	_, _, err := service.Login(context.Background(), &dto.LoginRequest{
		Email:    "user@example.com",
		Password: "wrong-password",
	})

	if !errors.Is(err, dto.ErrInvalidCredentials) {
		t.Fatalf("Login() error = %v, want %v", err, dto.ErrInvalidCredentials)
	}
	if len(repo.failedLoginCalls) != 1 {
		t.Fatalf("RecordFailedLogin() calls = %d, want 1", len(repo.failedLoginCalls))
	}
	call := repo.failedLoginCalls[0]
	if call.userID != repo.user.ID || call.maxAttempts != maxFailedLoginAttempts || call.lockDuration != loginLockDuration {
		t.Fatalf(
			"RecordFailedLogin() call = %+v, want user=%d attempts=%d duration=%s",
			call,
			repo.user.ID,
			maxFailedLoginAttempts,
			loginLockDuration,
		)
	}
}

func TestLoginRecordsFirstFailureAfterLockExpires(t *testing.T) {
	now := time.Date(2026, time.September, 13, 12, 0, 0, 0, time.UTC)
	repo := &authRepositoryStub{
		user: activeUserWithPassword(t, "correct-password"),
		role: "PC",
	}
	repo.user.FailedLoginAttempts = 5
	repo.user.LockedUntil = timePointer(now.Add(-time.Second))
	service := NewAuthService(repo, &tokenManagerStub{})
	service.now = func() time.Time { return now }

	_, _, err := service.Login(context.Background(), &dto.LoginRequest{
		Email:    "user@example.com",
		Password: "wrong-password",
	})

	if !errors.Is(err, dto.ErrInvalidCredentials) {
		t.Fatalf("Login() error = %v, want %v", err, dto.ErrInvalidCredentials)
	}
	if len(repo.failedLoginCalls) != 1 {
		t.Fatalf("RecordFailedLogin() calls = %d, want 1", len(repo.failedLoginCalls))
	}
}

func TestLoginSuccessfulResetsLockoutState(t *testing.T) {
	repo := &authRepositoryStub{
		user: activeUserWithPassword(t, "correct-password"),
		role: "PC",
	}
	repo.user.FailedLoginAttempts = 4
	tokens := &tokenManagerStub{}
	service := NewAuthService(repo, tokens)

	response, refreshToken, err := service.Login(context.Background(), &dto.LoginRequest{
		Email:    "user@example.com",
		Password: "correct-password",
	})

	if err != nil {
		t.Fatalf("Login() error = %v", err)
	}
	if repo.updateLoginCalls != 1 {
		t.Fatalf("UpdateLastLogin() calls = %d, want 1", repo.updateLoginCalls)
	}
	if len(repo.failedLoginCalls) != 0 {
		t.Fatalf("RecordFailedLogin() calls = %d, want 0", len(repo.failedLoginCalls))
	}
	if response.AccessToken != "access-token" || refreshToken != "refresh-token" {
		t.Fatalf("tokens = %q, %q", response.AccessToken, refreshToken)
	}
}

func TestLoginReturnsInternalErrorWhenFailedAttemptCannotBeRecorded(t *testing.T) {
	repo := &authRepositoryStub{
		user:              activeUserWithPassword(t, "correct-password"),
		role:              "PC",
		recordFailedError: errors.New("database unavailable"),
	}
	service := NewAuthService(repo, &tokenManagerStub{})

	_, _, err := service.Login(context.Background(), &dto.LoginRequest{
		Email:    "user@example.com",
		Password: "wrong-password",
	})

	if !errors.Is(err, dto.ErrInternalServer) {
		t.Fatalf("Login() error = %v, want %v", err, dto.ErrInternalServer)
	}
}

func activeUserWithPassword(t *testing.T, password string) *model.User {
	t.Helper()
	hash, err := passwordhash.HashPassword(password)
	if err != nil {
		t.Fatalf("HashPassword() error = %v", err)
	}
	return &model.User{
		ID:           42,
		Email:        "user@example.com",
		PasswordHash: &hash,
		Status:       model.UserStatusActive,
	}
}

func timePointer(value time.Time) *time.Time {
	return &value
}
