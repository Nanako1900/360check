package auth

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"golang.org/x/crypto/bcrypt"

	gendb "github.com/nnkglobal/c5-backend/internal/gen/db"
	"github.com/nnkglobal/c5-backend/internal/gen/oapi"
	"github.com/nnkglobal/c5-backend/internal/rbac"
)

// BcryptCost is the work factor for password hashing (>= 12 per the security baseline).
const BcryptCost = 12

// Sentinel errors mapped to envelope codes by the handler.
var (
	ErrInvalidCredentials = errors.New("invalid username or password")
	ErrOldPasswordWrong   = errors.New("old password is incorrect")
)

// Service implements login/refresh/logout/me/change-password.
type Service struct {
	q       gendb.Querier
	jwt     jwtIssuer
	refresh *RefreshStore
	enf     *rbac.Enforcer
}

// jwtIssuer is the slice of jwt.Manager the auth service needs (small interface).
type jwtIssuer interface {
	Issue(userID int64, username string, roles []string, now time.Time) (string, int, error)
}

// NewService wires the auth service. q is the gendb.Querier interface (satisfied
// by *gendb.Queries) so unit tests can substitute a fake to exercise the
// defensive DB-error branches without a live database.
func NewService(q gendb.Querier, jwtMgr jwtIssuer, refresh *RefreshStore, enf *rbac.Enforcer) *Service {
	return &Service{q: q, jwt: jwtMgr, refresh: refresh, enf: enf}
}

// Login verifies credentials (bcrypt), issues an access + refresh token, and
// records last_login_at. Unknown user, wrong password and disabled account all
// return ErrInvalidCredentials (no user enumeration).
func (s *Service) Login(ctx context.Context, username, password string) (*oapi.AuthTokens, error) {
	row, err := s.q.GetUserByUsername(ctx, username)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrInvalidCredentials
	}
	if err != nil {
		return nil, fmt.Errorf("auth: load user: %w", err)
	}
	if !row.IsActive {
		return nil, ErrInvalidCredentials
	}
	if bcrypt.CompareHashAndPassword([]byte(row.PasswordHash), []byte(password)) != nil {
		return nil, ErrInvalidCredentials
	}

	roles, err := s.enf.RolesForUser(row.Username)
	if err != nil {
		return nil, err
	}
	access, expiresIn, err := s.jwt.Issue(row.ID, row.Username, roles, time.Now())
	if err != nil {
		return nil, err
	}
	refreshTok, err := s.refresh.Create(ctx, row.ID)
	if err != nil {
		return nil, err
	}
	if err := s.q.TouchUserLastLogin(ctx, row.ID); err != nil {
		return nil, fmt.Errorf("auth: touch last_login: %w", err)
	}

	user := oapi.User{
		Id: row.ID, Username: row.Username, DisplayName: row.DisplayName,
		Phone: row.Phone, Email: row.Email, IsActive: row.IsActive,
		LastLoginAt: ptrTime(row.LastLoginAt), CreatedAt: row.CreatedAt, UpdatedAt: row.UpdatedAt,
	}
	return &oapi.AuthTokens{
		AccessToken:  access,
		ExpiresIn:    expiresIn,
		RefreshToken: refreshTok,
		User:         user,
	}, nil
}

// Refresh rotates the refresh token (single-use) and mints a new access token.
func (s *Service) Refresh(ctx context.Context, refreshToken string) (*oapi.AuthTokens, error) {
	newRefresh, userID, err := s.refresh.Rotate(ctx, refreshToken)
	if err != nil {
		return nil, err // ErrRefreshNotFound -> 401 at handler
	}
	row, err := s.q.GetUserByID(ctx, userID)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrRefreshNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("auth: load user: %w", err)
	}
	if !row.IsActive {
		_ = s.refresh.Revoke(ctx, newRefresh)
		return nil, ErrRefreshNotFound
	}
	roles, err := s.enf.RolesForUser(row.Username)
	if err != nil {
		return nil, err
	}
	access, expiresIn, err := s.jwt.Issue(row.ID, row.Username, roles, time.Now())
	if err != nil {
		return nil, err
	}
	return &oapi.AuthTokens{
		AccessToken:  access,
		ExpiresIn:    expiresIn,
		RefreshToken: newRefresh,
		User: oapi.User{
			Id: row.ID, Username: row.Username, DisplayName: row.DisplayName,
			Phone: row.Phone, Email: row.Email, IsActive: row.IsActive,
			LastLoginAt: ptrTime(row.LastLoginAt), CreatedAt: row.CreatedAt, UpdatedAt: row.UpdatedAt,
		},
	}, nil
}

// Logout revokes the supplied refresh token (idempotent).
func (s *Service) Logout(ctx context.Context, refreshToken string) error {
	return s.refresh.Revoke(ctx, refreshToken)
}

// Me returns the current user, their roles, and effective permission codes
// (derived by enforcing each catalog permission, so role wildcards expand correctly).
func (s *Service) Me(ctx context.Context, userID int64) (*oapi.AuthMe, error) {
	row, err := s.q.GetUserByID(ctx, userID)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrInvalidCredentials
	}
	if err != nil {
		return nil, fmt.Errorf("auth: load user: %w", err)
	}

	roleCodes, err := s.enf.RolesForUser(row.Username)
	if err != nil {
		return nil, err
	}
	roles := make([]oapi.Role, 0, len(roleCodes))
	for _, rc := range roleCodes {
		r, err := s.q.GetRoleByCode(ctx, rc)
		if errors.Is(err, pgx.ErrNoRows) {
			continue
		}
		if err != nil {
			return nil, fmt.Errorf("auth: load role: %w", err)
		}
		roles = append(roles, oapi.Role{
			Id: r.ID, Code: r.Code, Name: r.Name, Description: &r.Description,
			IsSystem: r.IsSystem, SortOrder: int(r.SortOrder),
			CreatedAt: &r.CreatedAt, UpdatedAt: &r.UpdatedAt,
		})
	}

	perms, err := s.q.ListPermissions(ctx)
	if err != nil {
		return nil, fmt.Errorf("auth: load permissions: %w", err)
	}
	codes := make([]string, 0)
	for _, p := range perms {
		ok, err := s.enf.Enforce(row.Username, p.Object, p.Action)
		if err != nil {
			return nil, err
		}
		if ok {
			codes = append(codes, p.Code)
		}
	}

	return &oapi.AuthMe{
		User: oapi.User{
			Id: row.ID, Username: row.Username, DisplayName: row.DisplayName,
			Phone: row.Phone, Email: row.Email, IsActive: row.IsActive,
			LastLoginAt: ptrTime(row.LastLoginAt), CreatedAt: row.CreatedAt, UpdatedAt: row.UpdatedAt,
		},
		Roles:       roles,
		Permissions: codes,
	}, nil
}

// ChangePassword verifies the old password and stores a new bcrypt hash.
func (s *Service) ChangePassword(ctx context.Context, userID int64, oldPassword, newPassword string) error {
	hash, err := s.q.GetUserPasswordHash(ctx, userID)
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrInvalidCredentials
	}
	if err != nil {
		return fmt.Errorf("auth: load password hash: %w", err)
	}
	if bcrypt.CompareHashAndPassword([]byte(hash), []byte(oldPassword)) != nil {
		return ErrOldPasswordWrong
	}
	newHash, err := bcrypt.GenerateFromPassword([]byte(newPassword), BcryptCost)
	if err != nil {
		return fmt.Errorf("auth: hash password: %w", err)
	}
	if _, err := s.q.UpdateUserPassword(ctx, gendb.UpdateUserPasswordParams{
		ID: userID, PasswordHash: string(newHash), UpdatedBy: i64ptr(userID),
	}); err != nil {
		return fmt.Errorf("auth: update password: %w", err)
	}
	return nil
}

func ptrTime(t pgtype.Timestamptz) *time.Time {
	if t.Valid {
		tt := t.Time
		return &tt
	}
	return nil
}

func i64ptr(v int64) *int64 { return &v }
