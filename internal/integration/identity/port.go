// Package identity defines the narrow service-to-service contract used by
// Edge to ask Core about employee credentials and external bindings.
package identity

import "context"

type Staff struct {
	PublicID    string `json:"public_id"`
	EmployeeNo  string `json:"employee_no"`
	DisplayName string `json:"display_name"`
	Role        string `json:"role"`
}

type PasswordVerifyRequest struct {
	EmployeeNo  string `json:"employee_no"`
	Password    string `json:"password"`
	Client      string `json:"client"`
	Provider    string `json:"provider,omitempty"`
	ProviderApp string `json:"provider_app,omitempty"`
}

type PasswordVerifyResult struct {
	State         string `json:"state"`
	Staff         Staff  `json:"staff"`
	BindingTicket string `json:"binding_ticket,omitempty"`
}

type ExchangeRequest struct {
	Provider     string `json:"provider"`
	ProviderCode string `json:"provider_code"`
	Client       string `json:"client"`
	RedirectURI  string `json:"redirect_uri,omitempty"`
}

type BindingRequest struct {
	BindingTicket string `json:"binding_ticket"`
	Provider      string `json:"provider"`
	ProviderCode  string `json:"provider_code"`
	Client        string `json:"client"`
	RedirectURI   string `json:"redirect_uri,omitempty"`
}

type StaffStatusRequest struct {
	PublicID string `json:"public_id"`
}

type Client interface {
	FindStaff(context.Context, string) (Staff, error)
	VerifyPassword(context.Context, PasswordVerifyRequest) (PasswordVerifyResult, error)
	Exchange(context.Context, ExchangeRequest) (Staff, error)
	CompleteBinding(context.Context, BindingRequest) (Staff, error)
}
