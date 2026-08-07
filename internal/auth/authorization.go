package auth

// Permission 是基础权限骨架中的稳定权限名；具体资源授权由业务模块接入。
type Permission string

const (
	PermissionRuntimeRead   Permission = "runtime:read"
	PermissionTaskRead      Permission = "task:read"
	PermissionTaskAccept    Permission = "task:accept"
	PermissionTaskComplete  Permission = "task:complete"
	PermissionTaskConfirm   Permission = "task:confirm"
	PermissionEventHandle   Permission = "event:handle"
	PermissionAnalyticsRead Permission = "analytics:read"
)

const (
	RoleAdmin   = "admin"
	RoleManager = "manager"
	RoleLeader  = "leader"
	RoleStaff   = "staff"
)

// RoleAllows 返回角色是否拥有指定权限。
func RoleAllows(role string, permission Permission) bool {
	if role == RoleAdmin {
		return true
	}
	switch role {
	case RoleManager:
		return permission == PermissionRuntimeRead || permission == PermissionEventHandle || permission == PermissionAnalyticsRead
	case RoleLeader:
		return permission == PermissionRuntimeRead || permission == PermissionTaskRead || permission == PermissionTaskConfirm || permission == PermissionEventHandle
	case RoleStaff:
		return permission == PermissionTaskRead || permission == PermissionTaskAccept || permission == PermissionTaskComplete
	default:
		return false
	}
}
