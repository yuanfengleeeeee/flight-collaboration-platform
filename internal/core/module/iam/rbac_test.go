package iam

import (
	"testing"

	"github.com/yuanfengleeeeee/flight-collaboration-platform/internal/platform/security"
)

func TestRBACAndScope(t *testing.T) {
	authorizer := NewAuthorizer()
	manager := security.Principal{Type: security.HumanPrincipal, PublicID: "manager", Roles: []string{security.RoleManager}, Scopes: security.AccessScope{AreaIDs: []uint64{10}, TeamIDs: []uint64{20}}}
	if err := authorizer.Authorize(manager, "task:assign", security.AccessScope{TeamIDs: []uint64{20}}); err != nil {
		t.Fatal(err)
	}
	if err := authorizer.Authorize(manager, "rule:manage", security.AccessScope{AreaIDs: []uint64{10}}); err == nil {
		t.Fatal("expected missing permission")
	}
	if err := authorizer.Authorize(manager, "task:read", security.AccessScope{TeamIDs: []uint64{99}}); err == nil {
		t.Fatal("expected scope denial")
	}
}

func TestMachinePrincipalCannotUseHumanRole(t *testing.T) {
	authorizer := NewAuthorizer()
	device := security.Principal{Type: security.MachinePrincipal, PublicID: "device", Roles: []string{security.RoleStaff}, MachineUse: "rfid-gateway"}
	if err := authorizer.Authorize(device, "task:complete", security.AccessScope{}); err == nil {
		t.Fatal("expected machine/human boundary")
	}
	if err := authorizer.Authorize(device, "device:observe", security.AccessScope{}); err != nil {
		t.Fatal(err)
	}
}
