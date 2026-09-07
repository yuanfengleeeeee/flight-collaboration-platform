// Package management contains Core-owned management commands and read models.
// It is deliberately separate from the task lifecycle so that administrative
// writes can validate organization and identity facts before task use cases
// consume them.
package management

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/yuanfengleeeeee/flight-collaboration-platform/internal/platform/clock"
	"github.com/yuanfengleeeeee/flight-collaboration-platform/internal/platform/security"
	"golang.org/x/crypto/bcrypt"
)

const (
	PermissionAreaManage          security.Permission = "area:manage"
	PermissionTeamManage          security.Permission = "team:manage"
	PermissionPersonnelManage     security.Permission = "personnel:manage"
	PermissionTemplateManage      security.Permission = "template:manage"
	PermissionAdminIdentityManage security.Permission = "admin_identity:manage"
	PermissionAreaRead            security.Permission = "area:read"
	PermissionTeamRead            security.Permission = "team:read"
	PermissionTemplateRead        security.Permission = "template:read"
	PermissionFlightRead          security.Permission = "flight:read"
	PermissionPositionRead        security.Permission = "position:read"
	PermissionPositionManage      security.Permission = "position:manage"
	PermissionCapabilityRead      security.Permission = "capability:read"
	PermissionCapabilityManage    security.Permission = "capability:manage"
)

var (
	ErrRepositoryNotConfigured = errors.New("management repository is not configured")
	ErrForbidden               = errors.New("principal is not allowed to manage this resource")
	ErrInvalidInput            = errors.New("invalid management input")
	ErrNotFound                = errors.New("management resource not found")
	ErrConflict                = errors.New("management resource conflicts with an existing fact")
	ErrResourceBusy            = errors.New("management resource is active and cannot be changed")
)

type AuditMeta struct {
	Principal security.Principal
	RequestID string
	TraceID   string
	SourceIP  string
	Now       time.Time
}

type Area struct {
	PublicID string `json:"public_id"`
	Code     string `json:"code"`
	Name     string `json:"name"`
	Enabled  bool   `json:"enabled"`
}

type Team struct {
	PublicID     string `json:"public_id"`
	AreaPublicID string `json:"area_public_id"`
	AreaName     string `json:"area_name"`
	Code         string `json:"code"`
	Name         string `json:"name"`
	Enabled      bool   `json:"enabled"`
}

type Personnel struct {
	PublicID           string   `json:"public_id"`
	UserPublicID       string   `json:"user_public_id"`
	EmployeeNo         string   `json:"employee_no"`
	DisplayName        string   `json:"display_name"`
	AreaPublicID       string   `json:"area_public_id"`
	TeamPublicID       string   `json:"team_public_id"`
	PositionCode       string   `json:"position_code"`
	CapabilityCode     string   `json:"capability_code"`
	Capabilities       []string `json:"capabilities"`
	WorkState          string   `json:"work_state"`
	StatusVersion      uint64   `json:"status_version"`
	Enabled            bool     `json:"enabled"`
	HasCredential      bool     `json:"has_credential"`
	LastStateChangedAt string   `json:"last_state_changed_at"`
}

type TaskTemplate struct {
	PublicID               string   `json:"public_id"`
	Name                   string   `json:"name"`
	TriggerType            string   `json:"trigger_type"`
	Version                uint     `json:"template_version"`
	Enabled                bool     `json:"enabled"`
	AreaPublicID           string   `json:"area_public_id"`
	TeamPublicID           string   `json:"team_public_id"`
	RequiredPositionCode   string   `json:"required_position_code"`
	RequiredCapabilityCode string   `json:"required_capability_code"`
	RequiredCapabilities   []string `json:"required_capabilities"`
	PlannedOffsetSeconds   int      `json:"planned_offset_seconds"`
	DefaultMessage         string   `json:"default_message"`
}

type Flight struct {
	PublicID            string  `json:"public_id"`
	DisplayNo           string  `json:"flight_display_no"`
	SourceProvider      string  `json:"source_provider"`
	ExternalFlightID    string  `json:"external_flight_id,omitempty"`
	OperatingDate       string  `json:"operating_date"`
	ScheduledAt         string  `json:"scheduled_at"`
	SourceLastSyncedAt  *string `json:"source_last_synced_at,omitempty"`
	SourceState         string  `json:"source_state"`
	SourceLastAttemptAt *string `json:"source_last_attempt_at,omitempty"`
	SourceLastError     string  `json:"source_last_error,omitempty"`
	ActualArrivalAt     *string `json:"actual_arrival_at,omitempty"`
	Status              string  `json:"status"`
	StatusVersion       uint64  `json:"status_version"`
	LastStatusChangedAt string  `json:"last_status_changed_at"`
}

type AdminIdentity struct {
	PublicID      string   `json:"public_id"`
	Provider      string   `json:"provider"`
	ExternalSub   string   `json:"external_subject"`
	DisplayName   string   `json:"display_name"`
	Role          string   `json:"role"`
	Enabled       bool     `json:"enabled"`
	GlobalScope   bool     `json:"global_scope"`
	AreaIDs       []uint64 `json:"area_ids"`
	TeamIDs       []uint64 `json:"team_ids"`
	AreaPublicIDs []string `json:"area_public_ids"`
	TeamPublicIDs []string `json:"team_public_ids"`
	UserID        uint64   `json:"user_id"`
}

type AreaInput struct {
	Code    string
	Name    string
	Enabled bool
}

type AreaPatch struct {
	Code    *string
	Name    *string
	Enabled *bool
}

type TeamInput struct {
	AreaPublicID string
	Code         string
	Name         string
	Enabled      bool
}

type TeamPatch struct {
	AreaPublicID *string
	Code         *string
	Name         *string
	Enabled      *bool
}

type PersonnelWrite struct {
	EmployeeNo     string
	DisplayName    string
	TeamPublicID   string
	PositionCode   string
	CapabilityCode string
	Capabilities   []string
	PasswordHash   string
}

type PersonnelPatch struct {
	EmployeeNo     *string
	DisplayName    *string
	TeamPublicID   *string
	PositionCode   *string
	CapabilityCode *string
	Capabilities   *[]string
	Enabled        *bool
}

type TaskTemplateWrite struct {
	Name                   string
	TriggerType            string
	Version                uint
	Enabled                bool
	AreaPublicID           string
	TeamPublicID           string
	RequiredPositionCode   string
	RequiredCapabilityCode string
	RequiredCapabilities   []string
	PlannedOffsetSeconds   int
	DefaultMessage         string
}

type TaskTemplatePatch struct {
	Name                   *string
	Enabled                *bool
	AreaPublicID           *string
	TeamPublicID           *string
	RequiredPositionCode   *string
	RequiredCapabilityCode *string
	RequiredCapabilities   *[]string
	PlannedOffsetSeconds   *int
	DefaultMessage         *string
}

// PageQuery is shared by every management read endpoint. Keeping pagination
// at the Core application boundary prevents a new UI from accidentally
// turning a reference-data query into an unbounded table scan.
type PageQuery struct {
	Page     int
	PageSize int
}

const (
	defaultPage     = 1
	defaultPageSize = 20
	maxPageSize     = 100
)

type AreaListResult struct {
	Items    []Area `json:"items"`
	Page     int    `json:"page"`
	PageSize int    `json:"page_size"`
	Total    int64  `json:"total"`
}

type TeamListResult struct {
	Items    []Team `json:"items"`
	Page     int    `json:"page"`
	PageSize int    `json:"page_size"`
	Total    int64  `json:"total"`
}

type TemplateListResult struct {
	Items    []TaskTemplate `json:"items"`
	Page     int            `json:"page"`
	PageSize int            `json:"page_size"`
	Total    int64          `json:"total"`
}

type FlightListResult struct {
	Items    []Flight `json:"items"`
	Page     int      `json:"page"`
	PageSize int      `json:"page_size"`
	Total    int64    `json:"total"`
}

type AdminIdentityListResult struct {
	Items    []AdminIdentity `json:"items"`
	Page     int             `json:"page"`
	PageSize int             `json:"page_size"`
	Total    int64           `json:"total"`
}

type AdminIdentityWrite struct {
	Provider        string
	ExternalSubject string
	DisplayName     string
	Role            string
	Enabled         bool
	GlobalScope     bool
	AreaPublicIDs   []string
	TeamPublicIDs   []string
	UserID          uint64
}

type AdminIdentityPatch struct {
	DisplayName   *string
	Role          *string
	Enabled       *bool
	GlobalScope   *bool
	AreaPublicIDs *[]string
	TeamPublicIDs *[]string
	UserID        *uint64
}

type Repository interface {
	ListAreas(context.Context, string, PageQuery, bool) (AreaListResult, error)
	CreateArea(context.Context, AreaInput, AuditMeta) (Area, error)
	UpdateArea(context.Context, string, AreaPatch, AuditMeta) (Area, error)
	ListTeams(context.Context, string, string, PageQuery, bool) (TeamListResult, error)
	CreateTeam(context.Context, TeamInput, AuditMeta) (Team, error)
	UpdateTeam(context.Context, string, TeamPatch, AuditMeta) (Team, error)
	CreatePersonnel(context.Context, PersonnelWrite, AuditMeta) (Personnel, error)
	UpdatePersonnel(context.Context, string, PersonnelPatch, AuditMeta) (Personnel, error)
	ResetPersonnelPassword(context.Context, string, string, AuditMeta) error
	ListTemplates(context.Context, PageQuery, bool) (TemplateListResult, error)
	CreateTemplate(context.Context, TaskTemplateWrite, AuditMeta) (TaskTemplate, error)
	UpdateTemplate(context.Context, string, TaskTemplatePatch, AuditMeta) (TaskTemplate, error)
	ListFlights(context.Context, string, PageQuery, bool) (FlightListResult, error)
	ListAdminIdentities(context.Context, PageQuery, bool) (AdminIdentityListResult, error)
	CreateAdminIdentity(context.Context, AdminIdentityWrite, AuditMeta) (AdminIdentity, error)
	UpdateAdminIdentity(context.Context, string, AdminIdentityPatch, AuditMeta) (AdminIdentity, error)
}

type Service struct {
	repository Repository
	authorizer security.Authorizer
	clock      clock.Clock
}

func NewService(repository Repository, authorizer security.Authorizer, now clock.Clock) *Service {
	if now == nil {
		now = clock.Real{}
	}
	return &Service{repository: repository, authorizer: authorizer, clock: now}
}

func (s *Service) ListAreas(ctx context.Context, principal security.Principal, query string, page PageQuery, includeDisabled bool) (AreaListResult, error) {
	if err := s.authorize(principal, PermissionAreaRead); err != nil {
		return AreaListResult{}, err
	}
	page, err := normalizePageQuery(page)
	if err != nil {
		return AreaListResult{}, err
	}
	query = strings.TrimSpace(query)
	if len(query) > 128 {
		return AreaListResult{}, ErrInvalidInput
	}
	return s.repository.ListAreas(ctx, query, page, includeDisabled)
}

func (s *Service) CreateArea(ctx context.Context, principal security.Principal, input AreaInput, meta AuditMeta) (Area, error) {
	if err := s.authorize(principal, PermissionAreaManage); err != nil {
		return Area{}, err
	}
	input.Code = strings.TrimSpace(input.Code)
	input.Name = strings.TrimSpace(input.Name)
	if input.Code == "" || input.Name == "" || len(input.Code) > 64 || len(input.Name) > 128 {
		return Area{}, ErrInvalidInput
	}
	meta = s.auditMeta(principal, meta)
	return s.repository.CreateArea(ctx, input, meta)
}

func (s *Service) UpdateArea(ctx context.Context, principal security.Principal, publicID string, input AreaPatch, meta AuditMeta) (Area, error) {
	if err := s.authorize(principal, PermissionAreaManage); err != nil {
		return Area{}, err
	}
	publicID = strings.TrimSpace(publicID)
	if publicID == "" {
		return Area{}, ErrInvalidInput
	}
	if input.Code != nil {
		value := strings.TrimSpace(*input.Code)
		input.Code = &value
	}
	if input.Name != nil {
		value := strings.TrimSpace(*input.Name)
		input.Name = &value
	}
	if (input.Code != nil && (*input.Code == "" || len(*input.Code) > 64)) || (input.Name != nil && (*input.Name == "" || len(*input.Name) > 128)) {
		return Area{}, ErrInvalidInput
	}
	if input.Code == nil && input.Name == nil && input.Enabled == nil {
		return Area{}, ErrInvalidInput
	}
	return s.repository.UpdateArea(ctx, publicID, input, s.auditMeta(principal, meta))
}

func (s *Service) ListTeams(ctx context.Context, principal security.Principal, areaPublicID, query string, page PageQuery, includeDisabled bool) (TeamListResult, error) {
	if err := s.authorize(principal, PermissionTeamRead); err != nil {
		return TeamListResult{}, err
	}
	page, err := normalizePageQuery(page)
	if err != nil {
		return TeamListResult{}, err
	}
	query = strings.TrimSpace(query)
	if len(query) > 128 {
		return TeamListResult{}, ErrInvalidInput
	}
	return s.repository.ListTeams(ctx, strings.TrimSpace(areaPublicID), query, page, includeDisabled)
}

func (s *Service) CreateTeam(ctx context.Context, principal security.Principal, input TeamInput, meta AuditMeta) (Team, error) {
	if err := s.authorize(principal, PermissionTeamManage); err != nil {
		return Team{}, err
	}
	input.AreaPublicID, input.Code, input.Name = strings.TrimSpace(input.AreaPublicID), strings.TrimSpace(input.Code), strings.TrimSpace(input.Name)
	if input.AreaPublicID == "" || input.Code == "" || input.Name == "" || len(input.Code) > 64 || len(input.Name) > 128 {
		return Team{}, ErrInvalidInput
	}
	return s.repository.CreateTeam(ctx, input, s.auditMeta(principal, meta))
}

func (s *Service) UpdateTeam(ctx context.Context, principal security.Principal, publicID string, input TeamPatch, meta AuditMeta) (Team, error) {
	if err := s.authorize(principal, PermissionTeamManage); err != nil {
		return Team{}, err
	}
	publicID = strings.TrimSpace(publicID)
	if publicID == "" {
		return Team{}, ErrInvalidInput
	}
	if input.AreaPublicID != nil {
		value := strings.TrimSpace(*input.AreaPublicID)
		input.AreaPublicID = &value
	}
	if input.Code != nil {
		value := strings.TrimSpace(*input.Code)
		input.Code = &value
	}
	if input.Name != nil {
		value := strings.TrimSpace(*input.Name)
		input.Name = &value
	}
	if input.AreaPublicID == nil && input.Code == nil && input.Name == nil && input.Enabled == nil {
		return Team{}, ErrInvalidInput
	}
	if (input.AreaPublicID != nil && *input.AreaPublicID == "") || (input.Code != nil && (*input.Code == "" || len(*input.Code) > 64)) || (input.Name != nil && (*input.Name == "" || len(*input.Name) > 128)) {
		return Team{}, ErrInvalidInput
	}
	return s.repository.UpdateTeam(ctx, publicID, input, s.auditMeta(principal, meta))
}

func (s *Service) CreatePersonnel(ctx context.Context, principal security.Principal, input PersonnelWrite, password string, meta AuditMeta) (Personnel, error) {
	if err := s.authorize(principal, PermissionPersonnelManage); err != nil {
		return Personnel{}, err
	}
	input.EmployeeNo, input.DisplayName, input.TeamPublicID, input.PositionCode = strings.TrimSpace(input.EmployeeNo), strings.TrimSpace(input.DisplayName), strings.TrimSpace(input.TeamPublicID), strings.TrimSpace(input.PositionCode)
	var capabilityOK bool
	input.CapabilityCode, input.Capabilities, capabilityOK = normalizeSingleCapability(input.CapabilityCode, input.Capabilities)
	if !validEmployeeNo(input.EmployeeNo) || input.DisplayName == "" || input.TeamPublicID == "" || !validCode(input.PositionCode) || !capabilityOK || len(password) < 8 || len(password) > 128 || len(input.DisplayName) > 128 {
		return Personnel{}, ErrInvalidInput
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return Personnel{}, fmt.Errorf("hash personnel password: %w", err)
	}
	input.PasswordHash = string(hash)
	return s.repository.CreatePersonnel(ctx, input, s.auditMeta(principal, meta))
}

func (s *Service) UpdatePersonnel(ctx context.Context, principal security.Principal, publicID string, input PersonnelPatch, meta AuditMeta) (Personnel, error) {
	if err := s.authorize(principal, PermissionPersonnelManage); err != nil {
		return Personnel{}, err
	}
	publicID = strings.TrimSpace(publicID)
	if publicID == "" {
		return Personnel{}, ErrInvalidInput
	}
	if input.EmployeeNo != nil {
		value := strings.TrimSpace(*input.EmployeeNo)
		input.EmployeeNo = &value
	}
	if input.DisplayName != nil {
		value := strings.TrimSpace(*input.DisplayName)
		input.DisplayName = &value
	}
	if input.TeamPublicID != nil {
		value := strings.TrimSpace(*input.TeamPublicID)
		input.TeamPublicID = &value
	}
	if input.PositionCode != nil {
		value := strings.TrimSpace(*input.PositionCode)
		input.PositionCode = &value
	}
	if input.CapabilityCode != nil {
		value := strings.TrimSpace(*input.CapabilityCode)
		input.CapabilityCode = &value
		if input.Capabilities != nil {
			legacy := normalizeList(*input.Capabilities)
			if len(legacy) != 1 || legacy[0] != value {
				return Personnel{}, ErrInvalidInput
			}
		}
		values := []string{value}
		input.Capabilities = &values
	}
	if input.Capabilities != nil {
		values := normalizeList(*input.Capabilities)
		input.Capabilities = &values
	}
	if input.EmployeeNo == nil && input.DisplayName == nil && input.TeamPublicID == nil && input.PositionCode == nil && input.CapabilityCode == nil && input.Capabilities == nil && input.Enabled == nil {
		return Personnel{}, ErrInvalidInput
	}
	if (input.EmployeeNo != nil && !validEmployeeNo(*input.EmployeeNo)) || (input.DisplayName != nil && (*input.DisplayName == "" || len(*input.DisplayName) > 128)) || (input.TeamPublicID != nil && *input.TeamPublicID == "") || (input.PositionCode != nil && !validCode(*input.PositionCode)) || (input.CapabilityCode != nil && !validCapabilityList([]string{*input.CapabilityCode})) || (input.Capabilities != nil && !validCapabilityList(*input.Capabilities)) {
		return Personnel{}, ErrInvalidInput
	}
	return s.repository.UpdatePersonnel(ctx, publicID, input, s.auditMeta(principal, meta))
}

func (s *Service) ResetPersonnelPassword(ctx context.Context, principal security.Principal, publicID, password string, meta AuditMeta) error {
	if err := s.authorize(principal, PermissionPersonnelManage); err != nil {
		return err
	}
	if strings.TrimSpace(publicID) == "" || len(password) < 8 || len(password) > 128 {
		return ErrInvalidInput
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return fmt.Errorf("hash personnel password: %w", err)
	}
	return s.repository.ResetPersonnelPassword(ctx, strings.TrimSpace(publicID), string(hash), s.auditMeta(principal, meta))
}

func (s *Service) ListTemplates(ctx context.Context, principal security.Principal, page PageQuery, includeDisabled bool) (TemplateListResult, error) {
	if err := s.authorize(principal, PermissionTemplateRead); err != nil {
		return TemplateListResult{}, err
	}
	page, err := normalizePageQuery(page)
	if err != nil {
		return TemplateListResult{}, err
	}
	return s.repository.ListTemplates(ctx, page, includeDisabled)
}

func (s *Service) CreateTemplate(ctx context.Context, principal security.Principal, input TaskTemplateWrite, meta AuditMeta) (TaskTemplate, error) {
	if err := s.authorize(principal, PermissionTemplateManage); err != nil {
		return TaskTemplate{}, err
	}
	input = normalizeTemplateWrite(input)
	if err := validateTemplateWrite(input); err != nil {
		return TaskTemplate{}, err
	}
	return s.repository.CreateTemplate(ctx, input, s.auditMeta(principal, meta))
}

func (s *Service) UpdateTemplate(ctx context.Context, principal security.Principal, publicID string, input TaskTemplatePatch, meta AuditMeta) (TaskTemplate, error) {
	if err := s.authorize(principal, PermissionTemplateManage); err != nil {
		return TaskTemplate{}, err
	}
	publicID = strings.TrimSpace(publicID)
	if publicID == "" {
		return TaskTemplate{}, ErrInvalidInput
	}
	if input.Name != nil {
		value := strings.TrimSpace(*input.Name)
		input.Name = &value
	}
	if input.AreaPublicID != nil {
		value := strings.TrimSpace(*input.AreaPublicID)
		input.AreaPublicID = &value
	}
	if input.TeamPublicID != nil {
		value := strings.TrimSpace(*input.TeamPublicID)
		input.TeamPublicID = &value
	}
	if input.RequiredPositionCode != nil {
		value := strings.TrimSpace(*input.RequiredPositionCode)
		input.RequiredPositionCode = &value
	}
	if input.RequiredCapabilityCode != nil {
		value := strings.TrimSpace(*input.RequiredCapabilityCode)
		input.RequiredCapabilityCode = &value
		if input.RequiredCapabilities != nil {
			legacy := normalizeList(*input.RequiredCapabilities)
			if len(legacy) != 1 || legacy[0] != value {
				return TaskTemplate{}, ErrInvalidInput
			}
		}
		values := []string{value}
		input.RequiredCapabilities = &values
	}
	if input.RequiredCapabilities != nil {
		values := normalizeList(*input.RequiredCapabilities)
		input.RequiredCapabilities = &values
	}
	if input.DefaultMessage != nil {
		value := strings.TrimSpace(*input.DefaultMessage)
		input.DefaultMessage = &value
	}
	if input.Name == nil && input.Enabled == nil && input.AreaPublicID == nil && input.TeamPublicID == nil && input.RequiredPositionCode == nil && input.RequiredCapabilityCode == nil && input.RequiredCapabilities == nil && input.PlannedOffsetSeconds == nil && input.DefaultMessage == nil {
		return TaskTemplate{}, ErrInvalidInput
	}
	if input.Name != nil && (*input.Name == "" || len(*input.Name) > 255) || input.AreaPublicID != nil && *input.AreaPublicID == "" || input.TeamPublicID != nil && *input.TeamPublicID == "" || input.RequiredPositionCode != nil && !validCode(*input.RequiredPositionCode) || input.RequiredCapabilityCode != nil && !validCapabilityList([]string{*input.RequiredCapabilityCode}) || input.RequiredCapabilities != nil && !validCapabilityList(*input.RequiredCapabilities) || input.PlannedOffsetSeconds != nil && *input.PlannedOffsetSeconds < 0 || input.DefaultMessage != nil && *input.DefaultMessage == "" {
		return TaskTemplate{}, ErrInvalidInput
	}
	return s.repository.UpdateTemplate(ctx, publicID, input, s.auditMeta(principal, meta))
}

func (s *Service) ListFlights(ctx context.Context, principal security.Principal, operatingDate string, page PageQuery, includeTerminal bool) (FlightListResult, error) {
	if err := s.authorize(principal, PermissionFlightRead); err != nil {
		return FlightListResult{}, err
	}
	operatingDate = strings.TrimSpace(operatingDate)
	if operatingDate != "" {
		if _, err := time.Parse("2006-01-02", operatingDate); err != nil {
			return FlightListResult{}, ErrInvalidInput
		}
	}
	page, err := normalizePageQuery(page)
	if err != nil {
		return FlightListResult{}, err
	}
	return s.repository.ListFlights(ctx, operatingDate, page, includeTerminal)
}

func (s *Service) ListAdminIdentities(ctx context.Context, principal security.Principal, page PageQuery, includeDisabled bool) (AdminIdentityListResult, error) {
	if err := s.authorize(principal, PermissionAdminIdentityManage); err != nil {
		return AdminIdentityListResult{}, err
	}
	page, err := normalizePageQuery(page)
	if err != nil {
		return AdminIdentityListResult{}, err
	}
	return s.repository.ListAdminIdentities(ctx, page, includeDisabled)
}

func (s *Service) CreateAdminIdentity(ctx context.Context, principal security.Principal, input AdminIdentityWrite, meta AuditMeta) (AdminIdentity, error) {
	if err := s.authorize(principal, PermissionAdminIdentityManage); err != nil {
		return AdminIdentity{}, err
	}
	input = normalizeAdminIdentityWrite(input)
	if err := validateAdminIdentityWrite(input); err != nil {
		return AdminIdentity{}, err
	}
	return s.repository.CreateAdminIdentity(ctx, input, s.auditMeta(principal, meta))
}

func (s *Service) UpdateAdminIdentity(ctx context.Context, principal security.Principal, publicID string, input AdminIdentityPatch, meta AuditMeta) (AdminIdentity, error) {
	if err := s.authorize(principal, PermissionAdminIdentityManage); err != nil {
		return AdminIdentity{}, err
	}
	publicID = strings.TrimSpace(publicID)
	if publicID == "" {
		return AdminIdentity{}, ErrInvalidInput
	}
	if input.DisplayName != nil {
		value := strings.TrimSpace(*input.DisplayName)
		input.DisplayName = &value
	}
	if input.Role != nil {
		value := strings.TrimSpace(*input.Role)
		input.Role = &value
	}
	if input.AreaPublicIDs != nil {
		values := normalizeList(*input.AreaPublicIDs)
		input.AreaPublicIDs = &values
	}
	if input.TeamPublicIDs != nil {
		values := normalizeList(*input.TeamPublicIDs)
		input.TeamPublicIDs = &values
	}
	if input.DisplayName == nil && input.Role == nil && input.Enabled == nil && input.GlobalScope == nil && input.AreaPublicIDs == nil && input.TeamPublicIDs == nil && input.UserID == nil {
		return AdminIdentity{}, ErrInvalidInput
	}
	if input.DisplayName != nil && (*input.DisplayName == "" || len(*input.DisplayName) > 128) || input.Role != nil && !validAdminRole(*input.Role) {
		return AdminIdentity{}, ErrInvalidInput
	}
	return s.repository.UpdateAdminIdentity(ctx, publicID, input, s.auditMeta(principal, meta))
}

func (s *Service) authorize(principal security.Principal, permission security.Permission) error {
	if s == nil || s.repository == nil || s.authorizer == nil {
		return ErrRepositoryNotConfigured
	}
	if principal.Type != security.HumanPrincipal || strings.TrimSpace(principal.PublicID) == "" {
		return ErrForbidden
	}
	if err := s.authorizer.Authorize(principal, permission, security.AccessScope{}); err != nil {
		return fmt.Errorf("%w: %v", ErrForbidden, err)
	}
	return nil
}

func (s *Service) auditMeta(principal security.Principal, meta AuditMeta) AuditMeta {
	meta.Principal = principal
	if meta.Now.IsZero() {
		meta.Now = s.clock.Now().UTC()
	}
	return meta
}

func normalizeList(values []string) []string {
	result := make([]string, 0, len(values))
	seen := make(map[string]struct{}, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		result = append(result, value)
	}
	return result
}

var (
	employeeNoPattern = regexp.MustCompile(`^[0-9]{4,12}$`)
	codePattern       = regexp.MustCompile(`^[a-z][a-z0-9_-]{1,63}$`)
	capabilityPattern = regexp.MustCompile(`^[a-z][a-z0-9_.-]{1,63}$`)
)

func validEmployeeNo(value string) bool {
	return employeeNoPattern.MatchString(strings.TrimSpace(value))
}

func validCode(value string) bool { return codePattern.MatchString(strings.TrimSpace(value)) }

func validCapabilityList(values []string) bool {
	if len(values) != 1 {
		return false
	}
	for _, value := range values {
		if !capabilityPattern.MatchString(strings.TrimSpace(value)) {
			return false
		}
	}
	return true
}

func normalizePageQuery(page PageQuery) (PageQuery, error) {
	if page.Page == 0 {
		page.Page = defaultPage
	}
	if page.PageSize == 0 {
		page.PageSize = defaultPageSize
	}
	if page.Page < 1 || page.PageSize < 1 || page.PageSize > maxPageSize {
		return PageQuery{}, ErrInvalidInput
	}
	return page, nil
}

func normalizeTemplateWrite(input TaskTemplateWrite) TaskTemplateWrite {
	input.Name = strings.TrimSpace(input.Name)
	input.TriggerType = strings.TrimSpace(input.TriggerType)
	if input.TriggerType == "" {
		input.TriggerType = "flight_arrived"
	}
	input.AreaPublicID = strings.TrimSpace(input.AreaPublicID)
	input.TeamPublicID = strings.TrimSpace(input.TeamPublicID)
	input.RequiredPositionCode = strings.TrimSpace(input.RequiredPositionCode)
	input.RequiredCapabilityCode, input.RequiredCapabilities, _ = normalizeSingleCapability(input.RequiredCapabilityCode, input.RequiredCapabilities)
	input.DefaultMessage = strings.TrimSpace(input.DefaultMessage)
	return input
}

func validateTemplateWrite(input TaskTemplateWrite) error {
	if input.Name == "" || len(input.Name) > 255 || input.TriggerType != "flight_arrived" || input.Version == 0 || input.AreaPublicID == "" || input.TeamPublicID == "" || !validCode(input.RequiredPositionCode) || !validCapabilityList(input.RequiredCapabilities) || input.PlannedOffsetSeconds < 0 || input.DefaultMessage == "" {
		return ErrInvalidInput
	}
	return nil
}

func normalizeAdminIdentityWrite(input AdminIdentityWrite) AdminIdentityWrite {
	input.Provider = strings.TrimSpace(input.Provider)
	input.ExternalSubject = strings.TrimSpace(input.ExternalSubject)
	input.DisplayName = strings.TrimSpace(input.DisplayName)
	input.Role = strings.TrimSpace(input.Role)
	input.AreaPublicIDs = normalizeList(input.AreaPublicIDs)
	input.TeamPublicIDs = normalizeList(input.TeamPublicIDs)
	return input
}

func validateAdminIdentityWrite(input AdminIdentityWrite) error {
	if input.Provider == "" || input.ExternalSubject == "" || input.DisplayName == "" || len(input.DisplayName) > 128 || !validAdminRole(input.Role) || len(input.AreaPublicIDs) > 100 || len(input.TeamPublicIDs) > 100 {
		return ErrInvalidInput
	}
	if input.Provider != "wecom" && input.Provider != "oidc" && input.Provider != "development" {
		return ErrInvalidInput
	}
	return nil
}

func validAdminRole(role string) bool {
	return role == security.RoleAdmin || role == security.RoleManager || role == security.RoleLeader || role == security.RoleSupervisor
}

func formatTime(value time.Time) string {
	if value.IsZero() {
		return ""
	}
	return value.UTC().Format(time.RFC3339Nano)
}

func formatOptionalTime(value *time.Time) *string {
	if value == nil || value.IsZero() {
		return nil
	}
	result := formatTime(*value)
	return &result
}
