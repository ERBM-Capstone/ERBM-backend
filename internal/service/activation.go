package service

import (
	"context"
	"errors"
	"net/url"
	"strings"
	"time"

	"github.com/ERBM-Capstone/ERBM-backend/dto"
	"github.com/ERBM-Capstone/ERBM-backend/internal/mail"
	"github.com/ERBM-Capstone/ERBM-backend/internal/model"
	activationtoken "github.com/ERBM-Capstone/ERBM-backend/internal/pkg/activation"
	passwordhash "github.com/ERBM-Capstone/ERBM-backend/internal/pkg/hash"
	"gorm.io/gorm"
)

type activationRepository interface {
	WithTx(ctx context.Context, fn func(*gorm.DB) error) error
	Create(ctx context.Context, tx *gorm.DB, token *model.AccountActivationToken) error
	FindByHashForUpdate(ctx context.Context, tx *gorm.DB, hash string) (*model.AccountActivationToken, error)
	FindUserByIDForUpdate(ctx context.Context, tx *gorm.DB, userID int64) (*model.User, error)
	FindUserByEmployeeIDForUpdate(ctx context.Context, tx *gorm.DB, employeeID int64) (*model.User, error)
	FindUserByEmailForUpdate(ctx context.Context, tx *gorm.DB, email string) (*model.User, error)
	InvalidateOutstanding(ctx context.Context, tx *gorm.DB, userID int64, now time.Time) error
	MarkUsed(ctx context.Context, tx *gorm.DB, tokenID int64, now time.Time) error
	ActivateUser(ctx context.Context, tx *gorm.DB, userID int64, passwordHash string) error
}

type activationMailSender interface {
	SendActivation(ctx context.Context, message mail.ActivationMessage) error
}

type ActivationConfig struct {
	FrontendURL string
}

// PendingActivation retains plaintext only in process memory until the
// transaction commits and the invitation email can be delivered.
type PendingActivation struct {
	UserID    int64
	Email     string
	Plaintext string
}

type ActivationService struct {
	repo        activationRepository
	mailSender  activationMailSender
	frontendURL string
	now         func() time.Time
}

func NewActivationService(repo activationRepository, mailSender activationMailSender, cfg ActivationConfig) *ActivationService {
	return &ActivationService{repo: repo, mailSender: mailSender, frontendURL: cfg.FrontendURL, now: time.Now}
}

// IssueForUsersInTx stores invitation hashes inside an existing provisioning
// transaction. It must be called only after users have database IDs.
func (s *ActivationService) IssueForUsersInTx(ctx context.Context, tx *gorm.DB, users []*model.User) ([]PendingActivation, error) {
	pending := make([]PendingActivation, 0, len(users))
	for _, user := range users {
		plaintext, tokenHash, expiresAt, err := activationtoken.Generate(s.now())
		if err != nil {
			return nil, err
		}
		if err := s.repo.Create(ctx, tx, &model.AccountActivationToken{UserID: user.ID, TokenHash: tokenHash, ExpiresAt: expiresAt}); err != nil {
			return nil, err
		}
		pending = append(pending, PendingActivation{UserID: user.ID, Email: user.Email, Plaintext: plaintext})
	}
	return pending, nil
}

// SendPending sends post-commit invitations. A caller must never roll back a
// completed database transaction because this external operation failed.
func (s *ActivationService) SendPending(ctx context.Context, pending []PendingActivation) error {
	for _, invitation := range pending {
		if err := s.mailSender.SendActivation(ctx, mail.ActivationMessage{To: invitation.Email, ActivationURL: s.activationURL(invitation.Plaintext)}); err != nil {
			return err
		}
	}
	return nil
}

func (s *ActivationService) SetInitialPassword(ctx context.Context, req *dto.SetInitialPasswordRequest) error {
	if req == nil {
		return dto.ErrBadRequest
	}
	tokenHash := activationtoken.Hash(req.Token)
	now := s.now()

	err := s.repo.WithTx(ctx, func(tx *gorm.DB) error {
		token, err := s.repo.FindByHashForUpdate(ctx, tx, tokenHash)
		if err != nil {
			return dto.ErrInvalidActivationToken
		}
		if token.UsedAt != nil || token.InvalidatedAt != nil || !token.ExpiresAt.After(now) {
			return dto.ErrInvalidActivationToken
		}

		user, err := s.repo.FindUserByIDForUpdate(ctx, tx, token.UserID)
		if err != nil || user.Status != model.UserStatusInvited {
			return dto.ErrInvalidActivationToken
		}
		password, err := passwordhash.HashPassword(req.Password)
		if err != nil {
			return dto.ErrInternalServer
		}
		if err := s.repo.ActivateUser(ctx, tx, user.ID, password); err != nil {
			return err
		}
		if err := s.repo.MarkUsed(ctx, tx, token.ID, now); err != nil {
			return err
		}
		return nil
	})
	if err == nil {
		return nil
	}
	if errors.Is(err, dto.ErrInvalidActivationToken) {
		return err
	}
	return dto.ErrInternalServer
}

// ResendForEmployee is the administrator path. It reports SMTP delivery
// failure after the new invitation has been safely committed.
func (s *ActivationService) ResendForEmployee(ctx context.Context, employeeID int64) error {
	pending, err := s.reissue(ctx, func(tx *gorm.DB) (*model.User, error) {
		return s.repo.FindUserByEmployeeIDForUpdate(ctx, tx, employeeID)
	})
	if err != nil {
		return err
	}
	if err := s.SendPending(ctx, []PendingActivation{pending}); err != nil {
		return dto.ErrActivationEmailDelivery
	}
	return nil
}

// ResendForEmail is deliberately non-enumerating: inactive, absent, and
// deactivated accounts produce the same successful response as an invite.
func (s *ActivationService) ResendForEmail(ctx context.Context, email string) error {
	pending, err := s.reissue(ctx, func(tx *gorm.DB) (*model.User, error) {
		return s.repo.FindUserByEmailForUpdate(ctx, tx, strings.ToLower(strings.TrimSpace(email)))
	})
	if errors.Is(err, dto.ErrActivationUnavailable) || errors.Is(err, dto.ErrNotFound) {
		return nil
	}
	if err != nil {
		return err
	}
	_ = s.SendPending(ctx, []PendingActivation{pending})
	return nil
}

func (s *ActivationService) reissue(ctx context.Context, findUser func(*gorm.DB) (*model.User, error)) (PendingActivation, error) {
	plaintext, tokenHash, expiresAt, err := activationtoken.Generate(s.now())
	if err != nil {
		return PendingActivation{}, dto.ErrInternalServer
	}
	var pending PendingActivation
	err = s.repo.WithTx(ctx, func(tx *gorm.DB) error {
		user, err := findUser(tx)
		if err != nil {
			return err
		}
		if user.Status != model.UserStatusInvited {
			return dto.ErrActivationUnavailable
		}
		now := s.now()
		if err := s.repo.InvalidateOutstanding(ctx, tx, user.ID, now); err != nil {
			return err
		}
		if err := s.repo.Create(ctx, tx, &model.AccountActivationToken{UserID: user.ID, TokenHash: tokenHash, ExpiresAt: expiresAt}); err != nil {
			return err
		}
		pending = PendingActivation{UserID: user.ID, Email: user.Email, Plaintext: plaintext}
		return nil
	})
	if err == nil {
		return pending, nil
	}
	if errors.Is(err, dto.ErrNotFound) || errors.Is(err, dto.ErrActivationUnavailable) {
		return PendingActivation{}, err
	}
	return PendingActivation{}, dto.ErrInternalServer
}

func (s *ActivationService) activationURL(token string) string {
	base, err := url.Parse(s.frontendURL)
	if err != nil {
		return s.frontendURL
	}
	query := base.Query()
	query.Set("token", token)
	base.RawQuery = query.Encode()
	return base.String()
}
