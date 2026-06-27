// Package user implements admin user management (CRUD + roles assignment). The
// casbin g-rules (user->role) are owned by internal/rbac; this package owns the
// users table. password_hash never leaves the service layer.
package user

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
)

// BcryptCost mirrors the auth package's work factor (>= 12).
const BcryptCost = 12

// Sentinel errors mapped to envelope codes by the handler.
var (
	ErrUsernameTaken = errors.New("username already taken")
	ErrNotFound      = errors.New("user not found")
)

// Service owns the users table.
type Service struct {
	q gendb.Querier
}

// NewService wires the user service. q is the gendb.Querier interface (satisfied
// by *gendb.Queries) so unit tests can substitute a fake to exercise the
// defensive DB-error branches without a live database.
func NewService(q gendb.Querier) *Service { return &Service{q: q} }

// List returns a page of users plus the total count for the same filter.
func (s *Service) List(ctx context.Context, q *string, isActive *bool, limit, offset int) ([]oapi.User, int64, error) {
	rows, err := s.q.ListUsers(ctx, gendb.ListUsersParams{
		Limit: int32(limit), Offset: int32(offset),
		Q: q, IsActive: isActive,
	})
	if err != nil {
		return nil, 0, fmt.Errorf("user: list: %w", err)
	}
	total, err := s.q.CountUsers(ctx, gendb.CountUsersParams{Q: q, IsActive: isActive})
	if err != nil {
		return nil, 0, fmt.Errorf("user: count: %w", err)
	}
	out := make([]oapi.User, 0, len(rows))
	for _, r := range rows {
		out = append(out, oapi.User{
			Id: r.ID, Username: r.Username, DisplayName: r.DisplayName,
			Phone: r.Phone, Email: r.Email, AvatarMediaId: r.AvatarMediaID,
			IsActive: r.IsActive, LastLoginAt: ptrTime(r.LastLoginAt),
			CreatedAt: r.CreatedAt, UpdatedAt: r.UpdatedAt,
		})
	}
	return out, total, nil
}

// Get returns a single non-deleted user.
func (s *Service) Get(ctx context.Context, id int64) (*oapi.User, error) {
	r, err := s.q.GetUserByID(ctx, id)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("user: get: %w", err)
	}
	u := oapi.User{
		Id: r.ID, Username: r.Username, DisplayName: r.DisplayName,
		Phone: r.Phone, Email: r.Email, AvatarMediaId: r.AvatarMediaID,
		IsActive: r.IsActive, LastLoginAt: ptrTime(r.LastLoginAt),
		CreatedAt: r.CreatedAt, UpdatedAt: r.UpdatedAt,
	}
	return &u, nil
}

// Create hashes the password and inserts a user. A taken username -> ErrUsernameTaken.
func (s *Service) Create(ctx context.Context, in oapi.UserCreate, actorID int64) (*oapi.User, error) {
	exists, err := s.q.UsernameExists(ctx, in.Username)
	if err != nil {
		return nil, fmt.Errorf("user: check username: %w", err)
	}
	if exists {
		return nil, ErrUsernameTaken
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(in.Password), BcryptCost)
	if err != nil {
		return nil, fmt.Errorf("user: hash password: %w", err)
	}
	isActive := true
	if in.IsActive != nil {
		isActive = *in.IsActive
	}
	r, err := s.q.CreateUser(ctx, gendb.CreateUserParams{
		Username: in.Username, PasswordHash: string(hash),
		DisplayName: deref(in.DisplayName), Phone: in.Phone, Email: in.Email,
		IsActive: isActive, CreatedBy: i64ptr(actorID),
	})
	if err != nil {
		if isUniqueViolation(err) {
			return nil, ErrUsernameTaken
		}
		return nil, fmt.Errorf("user: create: %w", err)
	}
	return s.Get(ctx, r.ID)
}

// Update applies a partial update (nil fields are left unchanged).
func (s *Service) Update(ctx context.Context, id int64, in oapi.UserUpdate, actorID int64) (*oapi.User, error) {
	_, err := s.q.UpdateUser(ctx, gendb.UpdateUserParams{
		ID:            id,
		DisplayName:   in.DisplayName,
		Phone:         in.Phone,
		Email:         in.Email,
		AvatarMediaID: in.AvatarMediaId,
		IsActive:      in.IsActive,
		UpdatedBy:     i64ptr(actorID),
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("user: update: %w", err)
	}
	return s.Get(ctx, id)
}

// Delete soft-deletes a user. Missing/already-deleted -> ErrNotFound.
func (s *Service) Delete(ctx context.Context, id, actorID int64) error {
	n, err := s.q.SoftDeleteUser(ctx, gendb.SoftDeleteUserParams{ID: id, UpdatedBy: i64ptr(actorID)})
	if err != nil {
		return fmt.Errorf("user: delete: %w", err)
	}
	if n == 0 {
		return ErrNotFound
	}
	return nil
}

// ResetPassword sets a new bcrypt hash for a user (admin action).
func (s *Service) ResetPassword(ctx context.Context, id int64, newPassword string, actorID int64) error {
	hash, err := bcrypt.GenerateFromPassword([]byte(newPassword), BcryptCost)
	if err != nil {
		return fmt.Errorf("user: hash password: %w", err)
	}
	n, err := s.q.UpdateUserPassword(ctx, gendb.UpdateUserPasswordParams{
		ID: id, PasswordHash: string(hash), UpdatedBy: i64ptr(actorID),
	})
	if err != nil {
		return fmt.Errorf("user: reset password: %w", err)
	}
	if n == 0 {
		return ErrNotFound
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

func deref(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

func i64ptr(v int64) *int64 { return &v }
