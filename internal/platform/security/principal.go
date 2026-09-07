// Package security defines identity and authorization ports without binding
// Core Domain code to JWT or a particular IAM provider.
package security

import "slices"

type PrincipalType string

const (
	HumanPrincipal   PrincipalType = "human"
	MachinePrincipal PrincipalType = "machine"
)

type Principal struct {
	Type       PrincipalType
	PublicID   string
	Subject    string
	SessionID  string
	Roles      []string
	Scopes     AccessScope
	MachineUse string
}

type AccessScope struct {
	Global  bool
	AreaIDs []uint64
	TeamIDs []uint64
	UserID  uint64
}

type Permission string

const (
	RoleAdmin   = "admin"
	RoleManager = "manager"
	RoleLeader  = "leader"
	// RoleSupervisor is the read-only management role for a responsible
	// leader. Its area/team scope controls which operational events it sees.
	RoleSupervisor = "supervisor"
	RoleStaff      = "staff"
)

type IdentityProvider interface {
	Authenticate(subject, credential string) (Principal, error)
}
type Authenticator interface {
	AuthenticateToken(token string) (Principal, error)
}
type Authorizer interface {
	Authorize(principal Principal, permission Permission, scope AccessScope) error
}

func ScopeAllows(actor AccessScope, requested AccessScope) bool {
	if actor.Global {
		return true
	}
	if requested.UserID != 0 {
		return actor.UserID != 0 && actor.UserID == requested.UserID
	}
	if len(requested.TeamIDs) > 0 {
		return slices.ContainsFunc(requested.TeamIDs, func(id uint64) bool { return slices.Contains(actor.TeamIDs, id) })
	}
	if len(requested.AreaIDs) > 0 {
		return slices.ContainsFunc(requested.AreaIDs, func(id uint64) bool { return slices.Contains(actor.AreaIDs, id) })
	}
	return true
}
