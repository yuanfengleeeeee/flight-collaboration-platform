package auth

import "testing"

func TestRoleAllows(t *testing.T) {
	tests := []struct {
		role       string
		permission Permission
		want       bool
	}{
		{RoleAdmin, PermissionTaskComplete, true},
		{RoleManager, PermissionAnalyticsRead, true},
		{RoleManager, PermissionTaskAccept, false},
		{RoleLeader, PermissionTaskConfirm, true},
		{RoleStaff, PermissionTaskComplete, true},
		{"unknown", PermissionTaskRead, false},
	}
	for _, tt := range tests {
		if got := RoleAllows(tt.role, tt.permission); got != tt.want {
			t.Errorf("RoleAllows(%q, %q) = %v, want %v", tt.role, tt.permission, got, tt.want)
		}
	}
}
