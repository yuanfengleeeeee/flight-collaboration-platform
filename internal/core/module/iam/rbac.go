// Package iam contains the minimum Human/Machine identity and authorization
// foundation. It does not implement a user-management UI.
package iam

import (
	"fmt"
	"strings"

	"github.com/yuanfengleeeeee/flight-collaboration-platform/internal/platform/security"
)

var defaultRolePermissions = map[string]map[security.Permission]bool{
	security.RoleAdmin: {
		"flight:read": true, "task:read": true, "task:assign": true, "task:cancel": true, "task:complete": true,
		"event:handle": true, "personnel:read": true, "rule:manage": true, "analytics:read": true,
	},
	security.RoleManager: {
		"flight:read": true, "task:read": true, "task:assign": true, "task:cancel": true, "task:complete": true,
		"event:handle": true, "personnel:read": true, "analytics:read": true,
	},
	security.RoleLeader: {
		"flight:read": true, "task:read": true, "task:assign": true, "task:cancel": true, "event:handle": true, "personnel:read": true,
	},
	security.RoleStaff: {
		"task:read": true, "task:accept": true, "task:complete": true, "event:handle": true,
	},
}

type Authorizer struct {
	rolePermissions map[string]map[security.Permission]bool
}

func NewAuthorizer() *Authorizer {
	copyMap := make(map[string]map[security.Permission]bool, len(defaultRolePermissions))
	for role, permissions := range defaultRolePermissions {
		copyMap[role] = make(map[security.Permission]bool, len(permissions))
		for permission, allowed := range permissions {
			copyMap[role][permission] = allowed
		}
	}
	return &Authorizer{rolePermissions: copyMap}
}

func (a *Authorizer) Authorize(principal security.Principal, permission security.Permission, requested security.AccessScope) error {
	if principal.PublicID == "" || principal.Type == "" {
		return fmt.Errorf("principal is incomplete: %w", ErrForbidden)
	}
	if principal.Type == security.MachinePrincipal {
		if !machinePermissionAllowed(principal, permission) {
			return fmt.Errorf("machine principal is not allowed %s: %w", permission, ErrForbidden)
		}
		return nil
	}
	if principal.Type != security.HumanPrincipal {
		return fmt.Errorf("unknown principal type %q: %w", principal.Type, ErrForbidden)
	}
	if !a.hasPermission(principal.Roles, permission) {
		return fmt.Errorf("principal lacks %s: %w", permission, ErrForbidden)
	}
	if !security.ScopeAllows(principal.Scopes, requested) {
		return fmt.Errorf("principal scope does not allow request: %w", ErrForbidden)
	}
	return nil
}

func (a *Authorizer) hasPermission(roles []string, permission security.Permission) bool {
	for _, role := range roles {
		if a.rolePermissions[role][permission] {
			return true
		}
	}
	return false
}

func machinePermissionAllowed(principal security.Principal, permission security.Permission) bool {
	if principal.MachineUse == "" {
		return false
	}
	return strings.HasPrefix(string(permission), "sync:") || strings.HasPrefix(string(permission), "device:")
}
