package rbac

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	gendb "github.com/nnkglobal/c5-backend/internal/gen/db"
	"github.com/nnkglobal/c5-backend/internal/gen/oapi"
)

// Sentinel errors mapped to envelope codes by the handler.
var (
	ErrRoleNotFound  = errors.New("role not found")
	ErrUserNotFound  = errors.New("user not found")
	ErrRoleCodeTaken = errors.New("role code already taken")
	ErrSystemRole    = errors.New("system role cannot be deleted")
)

// Service manages the roles/permissions UI-mirror tables and keeps casbin
// (the enforced source of truth) in sync for role/user grants.
type Service struct {
	q   gendb.Querier
	enf *Enforcer
}

// NewService wires the rbac service. q is the gendb.Querier interface (satisfied
// by *gendb.Queries) so unit tests can substitute a fake to exercise the
// defensive DB-error branches without a live database.
func NewService(q gendb.Querier, enf *Enforcer) *Service { return &Service{q: q, enf: enf} }

func roleModel(id int64, code, name, desc string, isSystem bool, sortOrder int32, createdAt, updatedAt time.Time) oapi.Role {
	d, ca, ua := desc, createdAt, updatedAt
	return oapi.Role{
		Id: id, Code: code, Name: name, Description: &d, IsSystem: isSystem,
		SortOrder: int(sortOrder), CreatedAt: &ca, UpdatedAt: &ua,
	}
}

func permModel(id int64, code, name, object, action, group, desc string, sortOrder int32) oapi.Permission {
	g, d, so := group, desc, int(sortOrder)
	return oapi.Permission{
		Id: id, Code: code, Name: name, Object: object, Action: action,
		GroupName: &g, Description: &d, SortOrder: &so,
	}
}

// ListRoles returns all non-deleted roles.
func (s *Service) ListRoles(ctx context.Context) ([]oapi.Role, error) {
	rows, err := s.q.ListRoles(ctx)
	if err != nil {
		return nil, fmt.Errorf("rbac: list roles: %w", err)
	}
	out := make([]oapi.Role, 0, len(rows))
	for _, r := range rows {
		out = append(out, roleModel(r.ID, r.Code, r.Name, r.Description, r.IsSystem, r.SortOrder, r.CreatedAt, r.UpdatedAt))
	}
	return out, nil
}

// CreateRole inserts a non-system role; a taken code -> ErrRoleCodeTaken.
func (s *Service) CreateRole(ctx context.Context, in oapi.RoleCreate, actorID int64) (*oapi.Role, error) {
	exists, err := s.q.RoleCodeExists(ctx, in.Code)
	if err != nil {
		return nil, fmt.Errorf("rbac: check role code: %w", err)
	}
	if exists {
		return nil, ErrRoleCodeTaken
	}
	var desc *string
	if in.Description != nil {
		desc = in.Description
	}
	var sortOrder *int32
	if in.SortOrder != nil {
		v := int32(*in.SortOrder)
		sortOrder = &v
	}
	r, err := s.q.CreateRole(ctx, gendb.CreateRoleParams{
		Code: in.Code, Name: in.Name, Column3: desc, Column4: sortOrder, CreatedBy: &actorID,
	})
	if err != nil {
		return nil, fmt.Errorf("rbac: create role: %w", err)
	}
	m := roleModel(r.ID, r.Code, r.Name, r.Description, r.IsSystem, r.SortOrder, r.CreatedAt, r.UpdatedAt)
	return &m, nil
}

// UpdateRole applies a partial role update.
func (s *Service) UpdateRole(ctx context.Context, id int64, in oapi.RoleUpdate, actorID int64) (*oapi.Role, error) {
	var sortOrder *int32
	if in.SortOrder != nil {
		v := int32(*in.SortOrder)
		sortOrder = &v
	}
	r, err := s.q.UpdateRole(ctx, gendb.UpdateRoleParams{
		ID: id, Name: in.Name, Description: in.Description, SortOrder: sortOrder, UpdatedBy: &actorID,
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrRoleNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("rbac: update role: %w", err)
	}
	m := roleModel(r.ID, r.Code, r.Name, r.Description, r.IsSystem, r.SortOrder, r.CreatedAt, r.UpdatedAt)
	return &m, nil
}

// DeleteRole soft-deletes a non-system role and removes its casbin rules.
func (s *Service) DeleteRole(ctx context.Context, id int64) error {
	r, err := s.q.GetRole(ctx, id)
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrRoleNotFound
	}
	if err != nil {
		return fmt.Errorf("rbac: load role: %w", err)
	}
	if r.IsSystem {
		return ErrSystemRole
	}
	n, err := s.q.SoftDeleteRole(ctx, id)
	if err != nil {
		return fmt.Errorf("rbac: delete role: %w", err)
	}
	if n == 0 {
		return ErrSystemRole
	}
	return s.enf.RemoveRole(r.Code)
}

// GetRolePermissions returns the permission catalog rows granted to a role.
func (s *Service) GetRolePermissions(ctx context.Context, id int64) ([]oapi.Permission, error) {
	r, err := s.q.GetRole(ctx, id)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrRoleNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("rbac: load role: %w", err)
	}
	rules, err := s.enf.PermissionsForRole(r.Code)
	if err != nil {
		return nil, err
	}
	granted := make(map[string]bool, len(rules))
	for _, rule := range rules {
		if len(rule) >= 3 {
			granted[rule[1]+"\x00"+rule[2]] = true // obj \0 act
		}
	}
	all, err := s.q.ListPermissions(ctx)
	if err != nil {
		return nil, fmt.Errorf("rbac: list permissions: %w", err)
	}
	out := make([]oapi.Permission, 0)
	for _, p := range all {
		if granted[p.Object+"\x00"+p.Action] {
			out = append(out, permModel(p.ID, p.Code, p.Name, p.Object, p.Action, p.GroupName, p.Description, p.SortOrder))
		}
	}
	return out, nil
}

// SetRolePermissions replaces a role's p-rules from a set of permission ids.
func (s *Service) SetRolePermissions(ctx context.Context, id int64, permissionIDs []int64) ([]oapi.Permission, error) {
	r, err := s.q.GetRole(ctx, id)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrRoleNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("rbac: load role: %w", err)
	}
	perms, err := s.q.PermissionsByIDs(ctx, permissionIDs)
	if err != nil {
		return nil, fmt.Errorf("rbac: load permissions: %w", err)
	}
	objActs := make([][2]string, 0, len(perms))
	for _, p := range perms {
		objActs = append(objActs, [2]string{p.Object, p.Action})
	}
	if err := s.enf.SetPermissionsForRole(r.Code, objActs); err != nil {
		return nil, err
	}
	return s.GetRolePermissions(ctx, id)
}

// ListPermissions returns the full permission catalog (UI tree).
func (s *Service) ListPermissions(ctx context.Context) ([]oapi.Permission, error) {
	all, err := s.q.ListPermissions(ctx)
	if err != nil {
		return nil, fmt.Errorf("rbac: list permissions: %w", err)
	}
	out := make([]oapi.Permission, 0, len(all))
	for _, p := range all {
		out = append(out, permModel(p.ID, p.Code, p.Name, p.Object, p.Action, p.GroupName, p.Description, p.SortOrder))
	}
	return out, nil
}

// GetUserRoles returns the roles granted to a user (casbin g-rules -> role rows).
func (s *Service) GetUserRoles(ctx context.Context, userID int64) ([]oapi.Role, error) {
	u, err := s.q.GetUserByID(ctx, userID)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrUserNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("rbac: load user: %w", err)
	}
	codes, err := s.enf.RolesForUser(u.Username)
	if err != nil {
		return nil, err
	}
	out := make([]oapi.Role, 0, len(codes))
	for _, code := range codes {
		r, err := s.q.GetRoleByCode(ctx, code)
		if errors.Is(err, pgx.ErrNoRows) {
			continue
		}
		if err != nil {
			return nil, fmt.Errorf("rbac: load role: %w", err)
		}
		out = append(out, roleModel(r.ID, r.Code, r.Name, r.Description, r.IsSystem, r.SortOrder, r.CreatedAt, r.UpdatedAt))
	}
	return out, nil
}

// SetUserRoles replaces a user's roles (casbin g-rules) from a set of role ids.
func (s *Service) SetUserRoles(ctx context.Context, userID int64, roleIDs []int64) ([]oapi.Role, error) {
	u, err := s.q.GetUserByID(ctx, userID)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrUserNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("rbac: load user: %w", err)
	}
	roles, err := s.q.RolesByIDs(ctx, roleIDs)
	if err != nil {
		return nil, fmt.Errorf("rbac: load roles: %w", err)
	}
	codes := make([]string, 0, len(roles))
	for _, r := range roles {
		codes = append(codes, r.Code)
	}
	if err := s.enf.SetRolesForUser(u.Username, codes); err != nil {
		return nil, err
	}
	return s.GetUserRoles(ctx, userID)
}
