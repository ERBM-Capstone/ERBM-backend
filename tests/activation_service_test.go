package tests

import (
	"context"
	"errors"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/ERBM-Capstone/ERBM-backend/dto"
	"github.com/ERBM-Capstone/ERBM-backend/internal/mail"
	"github.com/ERBM-Capstone/ERBM-backend/internal/model"
	activationtoken "github.com/ERBM-Capstone/ERBM-backend/internal/pkg/activation"
	passwordhash "github.com/ERBM-Capstone/ERBM-backend/internal/pkg/hash"
	jwtmanager "github.com/ERBM-Capstone/ERBM-backend/internal/pkg/jwt"
	"github.com/ERBM-Capstone/ERBM-backend/internal/service"
	"gorm.io/gorm"
)

type activationRepoFake struct {
	user      *model.User
	tokens    map[string]*model.AccountActivationToken
	nextID    int64
	markErr   error
	activeErr error
}

func newActivationRepoFake() *activationRepoFake {
	return &activationRepoFake{user: &model.User{ID: 1, EmployeeID: 9, Email: "ada@example.com", Status: model.UserStatusInvited}, tokens: map[string]*model.AccountActivationToken{}, nextID: 1}
}

func (r *activationRepoFake) WithTx(_ context.Context, fn func(*gorm.DB) error) error {
	oldUser := *r.user
	oldTokens := make(map[string]*model.AccountActivationToken, len(r.tokens))
	for hash, token := range r.tokens {
		copy := *token
		oldTokens[hash] = &copy
	}
	oldNext := r.nextID
	if err := fn(nil); err != nil {
		r.user, r.tokens, r.nextID = &oldUser, oldTokens, oldNext
		return err
	}
	return nil
}

func (r *activationRepoFake) Create(_ context.Context, _ *gorm.DB, token *model.AccountActivationToken) error {
	token.ID, r.nextID = r.nextID, r.nextID+1
	copy := *token
	r.tokens[token.TokenHash] = &copy
	return nil
}

func (r *activationRepoFake) FindByHashForUpdate(_ context.Context, _ *gorm.DB, hash string) (*model.AccountActivationToken, error) {
	token, ok := r.tokens[hash]
	if !ok {
		return nil, dto.ErrNotFound
	}
	return token, nil
}
func (r *activationRepoFake) FindUserByIDForUpdate(_ context.Context, _ *gorm.DB, userID int64) (*model.User, error) {
	if r.user.ID != userID {
		return nil, dto.ErrNotFound
	}
	return r.user, nil
}
func (r *activationRepoFake) FindUserByEmployeeIDForUpdate(_ context.Context, _ *gorm.DB, employeeID int64) (*model.User, error) {
	if r.user.EmployeeID != employeeID {
		return nil, dto.ErrNotFound
	}
	return r.user, nil
}
func (r *activationRepoFake) FindUserByEmailForUpdate(_ context.Context, _ *gorm.DB, email string) (*model.User, error) {
	if r.user.Email != email {
		return nil, dto.ErrNotFound
	}
	return r.user, nil
}
func (r *activationRepoFake) InvalidateOutstanding(_ context.Context, _ *gorm.DB, userID int64, now time.Time) error {
	for _, token := range r.tokens {
		if token.UserID == userID && token.UsedAt == nil && token.InvalidatedAt == nil {
			value := now
			token.InvalidatedAt = &value
		}
	}
	return nil
}
func (r *activationRepoFake) MarkUsed(_ context.Context, _ *gorm.DB, tokenID int64, now time.Time) error {
	if r.markErr != nil {
		return r.markErr
	}
	for _, token := range r.tokens {
		if token.ID == tokenID && token.UsedAt == nil {
			value := now
			token.UsedAt = &value
			return nil
		}
	}
	return dto.ErrNotFound
}
func (r *activationRepoFake) ActivateUser(_ context.Context, _ *gorm.DB, userID int64, passwordHash string) error {
	if r.activeErr != nil {
		return r.activeErr
	}
	if r.user.ID != userID || r.user.Status != model.UserStatusInvited {
		return dto.ErrNotFound
	}
	r.user.PasswordHash, r.user.Status = &passwordHash, model.UserStatusActive
	return nil
}

type fakeActivationMailSender struct {
	messages []mail.ActivationMessage
	err      error
}

func (s *fakeActivationMailSender) SendActivation(_ context.Context, message mail.ActivationMessage) error {
	if s.err != nil {
		return s.err
	}
	s.messages = append(s.messages, message)
	return nil
}

func addActivationToken(t *testing.T, repo *activationRepoFake, plaintext string, expiry time.Time) *model.AccountActivationToken {
	t.Helper()
	token := &model.AccountActivationToken{ID: repo.nextID, UserID: repo.user.ID, TokenHash: activationtoken.Hash(plaintext), ExpiresAt: expiry}
	repo.nextID++
	repo.tokens[token.TokenHash] = token
	return token
}

func TestActivationServiceSetsPasswordAndConsumesToken(t *testing.T) {
	repo, sender := newActivationRepoFake(), &fakeActivationMailSender{}
	plaintext := "activation-token"
	token := addActivationToken(t, repo, plaintext, time.Now().Add(time.Hour))
	svc := service.NewActivationService(repo, sender, service.ActivationConfig{FrontendURL: "https://app.example/activate"})

	if err := svc.SetInitialPassword(context.Background(), &dto.SetInitialPasswordRequest{Token: plaintext, Password: "correct-password"}); err != nil {
		t.Fatalf("SetInitialPassword() error = %v", err)
	}
	if repo.user.Status != model.UserStatusActive || repo.user.PasswordHash == nil {
		t.Fatalf("user after activation = %#v", repo.user)
	}
	if passwordhash.VerifyPassword("correct-password", *repo.user.PasswordHash) != nil {
		t.Fatal("stored password hash does not verify")
	}
	if token.UsedAt == nil {
		t.Fatal("token was not marked used")
	}
	if err := svc.SetInitialPassword(context.Background(), &dto.SetInitialPasswordRequest{Token: plaintext, Password: "another-password"}); !errors.Is(err, dto.ErrInvalidActivationToken) {
		t.Fatalf("reuse error = %v", err)
	}
}

func TestActivationServiceRejectsExpiredAndNonInvitedTokens(t *testing.T) {
	for name, setup := range map[string]func(*activationRepoFake){
		"expired": func(r *activationRepoFake) { addActivationToken(t, r, "expired", time.Now().Add(-time.Minute)) },
		"active user": func(r *activationRepoFake) {
			addActivationToken(t, r, "active", time.Now().Add(time.Hour))
			r.user.Status = model.UserStatusActive
		},
	} {
		t.Run(name, func(t *testing.T) {
			repo := newActivationRepoFake()
			setup(repo)
			token := "expired"
			if name == "active user" {
				token = "active"
			}
			err := service.NewActivationService(repo, &fakeActivationMailSender{}, service.ActivationConfig{}).SetInitialPassword(context.Background(), &dto.SetInitialPasswordRequest{Token: token, Password: "correct-password"})
			if !errors.Is(err, dto.ErrInvalidActivationToken) {
				t.Fatalf("error = %v", err)
			}
		})
	}
}

func TestActivationServiceResendInvalidatesOldTokenAndUsesMailSender(t *testing.T) {
	repo, sender := newActivationRepoFake(), &fakeActivationMailSender{}
	old := addActivationToken(t, repo, "old-token", time.Now().Add(time.Hour))
	svc := service.NewActivationService(repo, sender, service.ActivationConfig{FrontendURL: "https://app.example/activate?source=invite"})
	if err := svc.ResendForEmployee(context.Background(), repo.user.EmployeeID); err != nil {
		t.Fatalf("ResendForEmployee() error = %v", err)
	}
	if old.InvalidatedAt == nil {
		t.Fatal("old token was not invalidated")
	}
	if len(sender.messages) != 1 {
		t.Fatalf("sent messages = %d, want 1", len(sender.messages))
	}
	link, err := url.Parse(sender.messages[0].ActivationURL)
	if err != nil {
		t.Fatal(err)
	}
	plaintext := link.Query().Get("token")
	if plaintext == "" || plaintext == "old-token" {
		t.Fatalf("activation URL token = %q", plaintext)
	}
	if _, ok := repo.tokens[plaintext]; ok {
		t.Fatal("plaintext token was stored")
	}
	if _, ok := repo.tokens[activationtoken.Hash(plaintext)]; !ok {
		t.Fatal("token hash was not stored")
	}
}

func TestActivationServiceSMTPFailureDoesNotRollBackResend(t *testing.T) {
	repo := newActivationRepoFake()
	sender := &fakeActivationMailSender{err: errors.New("smtp unavailable")}
	err := service.NewActivationService(repo, sender, service.ActivationConfig{FrontendURL: "https://app.example/activate"}).ResendForEmployee(context.Background(), repo.user.EmployeeID)
	if !errors.Is(err, dto.ErrActivationEmailDelivery) {
		t.Fatalf("error = %v", err)
	}
	if len(repo.tokens) != 1 {
		t.Fatalf("stored tokens = %d, want 1 committed token", len(repo.tokens))
	}
}

func TestActivationServiceRollsBackWhenTokenUpdateFails(t *testing.T) {
	repo := newActivationRepoFake()
	repo.markErr = errors.New("write failed")
	addActivationToken(t, repo, "rollback-token", time.Now().Add(time.Hour))
	err := service.NewActivationService(repo, &fakeActivationMailSender{}, service.ActivationConfig{}).SetInitialPassword(context.Background(), &dto.SetInitialPasswordRequest{Token: "rollback-token", Password: "correct-password"})
	if !errors.Is(err, dto.ErrInternalServer) {
		t.Fatalf("error = %v", err)
	}
	if repo.user.Status != model.UserStatusInvited || repo.user.PasswordHash != nil {
		t.Fatalf("activation changes were not rolled back: %#v", repo.user)
	}
}

func TestActivationTokenHashDoesNotContainPlaintext(t *testing.T) {
	plaintext, hash, _, err := activationtoken.Generate(time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(hash, plaintext) || len(hash) != 64 {
		t.Fatalf("unsafe hash %q", hash)
	}
}

type activatedAuthRepo struct{ user *model.User }

func (r activatedAuthRepo) GetUserByEmail(_ context.Context, _ string) (*model.User, string, error) {
	return r.user, "EMPLOYEE", nil
}
func (r activatedAuthRepo) GetUserByID(_ context.Context, _ int64) (*model.User, string, error) {
	return r.user, "EMPLOYEE", nil
}
func (r activatedAuthRepo) GetProfileByUserID(context.Context, int64) (*model.UserProfile, error) {
	return nil, nil
}
func (r activatedAuthRepo) RecordFailedLogin(context.Context, int64, int, time.Duration) error {
	return nil
}
func (r activatedAuthRepo) UpdateLastLogin(context.Context, int64) error { return nil }

type activatedTokenManager struct{}

func (activatedTokenManager) GenerateAccessToken(int64, string) (string, error) { return "access", nil }
func (activatedTokenManager) GenerateRefreshToken(int64) (string, error)        { return "refresh", nil }
func (activatedTokenManager) ParseRefreshToken(string) (*jwtmanager.RefreshClaims, error) {
	return nil, errors.New("not used")
}
func (activatedTokenManager) AccessTokenTTL() time.Duration { return time.Hour }

func TestLoginSucceedsAfterActivation(t *testing.T) {
	repo := newActivationRepoFake()
	addActivationToken(t, repo, "login-token", time.Now().Add(time.Hour))
	activationSvc := service.NewActivationService(repo, &fakeActivationMailSender{}, service.ActivationConfig{})
	if err := activationSvc.SetInitialPassword(context.Background(), &dto.SetInitialPasswordRequest{Token: "login-token", Password: "correct-password"}); err != nil {
		t.Fatal(err)
	}
	authSvc := service.NewAuthService(activatedAuthRepo{user: repo.user}, activatedTokenManager{}, service.LoginPolicy{MaxFailedAttempts: 3, LockDuration: time.Minute})
	response, refresh, err := authSvc.Login(context.Background(), &dto.LoginRequest{Email: repo.user.Email, Password: "correct-password"})
	if err != nil || response.AccessToken != "access" || refresh != "refresh" {
		t.Fatalf("login response = %#v, %q, %v", response, refresh, err)
	}
}
