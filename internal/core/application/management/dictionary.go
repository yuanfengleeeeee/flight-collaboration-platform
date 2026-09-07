package management

import (
	"context"
	"strings"

	"github.com/yuanfengleeeeee/flight-collaboration-platform/internal/platform/security"
)

// Position and Capability are independent reference data. Their codes are
// stable integration keys; changing a display label must not invalidate
// personnel, task templates, or historical matching snapshots.
type Position struct {
	PublicID    string `json:"public_id"`
	Code        string `json:"code"`
	Name        string `json:"name"`
	Description string `json:"description"`
	Enabled     bool   `json:"enabled"`
}

type Capability struct {
	PublicID    string `json:"public_id"`
	Code        string `json:"code"`
	Name        string `json:"name"`
	Description string `json:"description"`
	Enabled     bool   `json:"enabled"`
}

type PositionInput struct {
	Code        string
	Name        string
	Description string
	Enabled     bool
}

type PositionPatch struct {
	Name        *string
	Description *string
	Enabled     *bool
}

type CapabilityInput struct {
	Code        string
	Name        string
	Description string
	Enabled     bool
}

type CapabilityPatch struct {
	Name        *string
	Description *string
	Enabled     *bool
}

type PositionListResult struct {
	Items    []Position `json:"items"`
	Page     int        `json:"page"`
	PageSize int        `json:"page_size"`
	Total    int64      `json:"total"`
}

type CapabilityListResult struct {
	Items    []Capability `json:"items"`
	Page     int          `json:"page"`
	PageSize int          `json:"page_size"`
	Total    int64        `json:"total"`
}

// DictionaryRepository is intentionally an optional extension of the
// original management repository. Keeping it separate preserves the small
// fake repositories used by older application tests while allowing the
// production adapter to expose the new dictionaries.
type DictionaryRepository interface {
	ListPositions(context.Context, PageQuery, bool) (PositionListResult, error)
	CreatePosition(context.Context, PositionInput, AuditMeta) (Position, error)
	UpdatePosition(context.Context, string, PositionPatch, AuditMeta) (Position, error)
	DeletePosition(context.Context, string, AuditMeta) error
	ListCapabilities(context.Context, PageQuery, bool) (CapabilityListResult, error)
	CreateCapability(context.Context, CapabilityInput, AuditMeta) (Capability, error)
	UpdateCapability(context.Context, string, CapabilityPatch, AuditMeta) (Capability, error)
	DeleteCapability(context.Context, string, AuditMeta) error
}

func (s *Service) dictionaryRepository() (DictionaryRepository, error) {
	if s == nil || s.repository == nil {
		return nil, ErrRepositoryNotConfigured
	}
	repository, ok := s.repository.(DictionaryRepository)
	if !ok || repository == nil {
		return nil, ErrRepositoryNotConfigured
	}
	return repository, nil
}

func (s *Service) ListPositions(ctx context.Context, principal security.Principal, page PageQuery, includeDisabled bool) (PositionListResult, error) {
	if err := s.authorize(principal, PermissionPositionRead); err != nil {
		return PositionListResult{}, err
	}
	repository, err := s.dictionaryRepository()
	if err != nil {
		return PositionListResult{}, err
	}
	page, err = normalizePageQuery(page)
	if err != nil {
		return PositionListResult{}, err
	}
	return repository.ListPositions(ctx, page, includeDisabled)
}

func (s *Service) CreatePosition(ctx context.Context, principal security.Principal, input PositionInput, meta AuditMeta) (Position, error) {
	if err := s.authorize(principal, PermissionPositionManage); err != nil {
		return Position{}, err
	}
	repository, err := s.dictionaryRepository()
	if err != nil {
		return Position{}, err
	}
	input.Code = strings.TrimSpace(input.Code)
	input.Name = strings.TrimSpace(input.Name)
	input.Description = strings.TrimSpace(input.Description)
	if !validCode(input.Code) || input.Name == "" || len(input.Name) > 128 || len(input.Description) > 255 {
		return Position{}, ErrInvalidInput
	}
	return repository.CreatePosition(ctx, input, s.auditMeta(principal, meta))
}

func (s *Service) UpdatePosition(ctx context.Context, principal security.Principal, publicID string, input PositionPatch, meta AuditMeta) (Position, error) {
	if err := s.authorize(principal, PermissionPositionManage); err != nil {
		return Position{}, err
	}
	repository, err := s.dictionaryRepository()
	if err != nil {
		return Position{}, err
	}
	publicID = strings.TrimSpace(publicID)
	if publicID == "" {
		return Position{}, ErrInvalidInput
	}
	if input.Name != nil {
		value := strings.TrimSpace(*input.Name)
		input.Name = &value
	}
	if input.Description != nil {
		value := strings.TrimSpace(*input.Description)
		input.Description = &value
	}
	if input.Name == nil && input.Description == nil && input.Enabled == nil {
		return Position{}, ErrInvalidInput
	}
	if input.Name != nil && (*input.Name == "" || len(*input.Name) > 128) || input.Description != nil && len(*input.Description) > 255 {
		return Position{}, ErrInvalidInput
	}
	return repository.UpdatePosition(ctx, publicID, input, s.auditMeta(principal, meta))
}

// DeletePosition is a safe delete: dictionary codes remain addressable in
// historical personnel/template snapshots, so a referenced value is disabled
// instead of physically removed. PATCH can re-enable it after review.
func (s *Service) DeletePosition(ctx context.Context, principal security.Principal, publicID string, meta AuditMeta) error {
	if err := s.authorize(principal, PermissionPositionManage); err != nil {
		return err
	}
	repository, err := s.dictionaryRepository()
	if err != nil {
		return err
	}
	publicID = strings.TrimSpace(publicID)
	if publicID == "" {
		return ErrInvalidInput
	}
	return repository.DeletePosition(ctx, publicID, s.auditMeta(principal, meta))
}

func (s *Service) ListCapabilities(ctx context.Context, principal security.Principal, page PageQuery, includeDisabled bool) (CapabilityListResult, error) {
	if err := s.authorize(principal, PermissionCapabilityRead); err != nil {
		return CapabilityListResult{}, err
	}
	repository, err := s.dictionaryRepository()
	if err != nil {
		return CapabilityListResult{}, err
	}
	page, err = normalizePageQuery(page)
	if err != nil {
		return CapabilityListResult{}, err
	}
	return repository.ListCapabilities(ctx, page, includeDisabled)
}

func (s *Service) CreateCapability(ctx context.Context, principal security.Principal, input CapabilityInput, meta AuditMeta) (Capability, error) {
	if err := s.authorize(principal, PermissionCapabilityManage); err != nil {
		return Capability{}, err
	}
	repository, err := s.dictionaryRepository()
	if err != nil {
		return Capability{}, err
	}
	input.Code = strings.TrimSpace(input.Code)
	input.Name = strings.TrimSpace(input.Name)
	input.Description = strings.TrimSpace(input.Description)
	if !capabilityPattern.MatchString(input.Code) || input.Name == "" || len(input.Name) > 128 || len(input.Description) > 255 {
		return Capability{}, ErrInvalidInput
	}
	return repository.CreateCapability(ctx, input, s.auditMeta(principal, meta))
}

func (s *Service) UpdateCapability(ctx context.Context, principal security.Principal, publicID string, input CapabilityPatch, meta AuditMeta) (Capability, error) {
	if err := s.authorize(principal, PermissionCapabilityManage); err != nil {
		return Capability{}, err
	}
	repository, err := s.dictionaryRepository()
	if err != nil {
		return Capability{}, err
	}
	publicID = strings.TrimSpace(publicID)
	if publicID == "" {
		return Capability{}, ErrInvalidInput
	}
	if input.Name != nil {
		value := strings.TrimSpace(*input.Name)
		input.Name = &value
	}
	if input.Description != nil {
		value := strings.TrimSpace(*input.Description)
		input.Description = &value
	}
	if input.Name == nil && input.Description == nil && input.Enabled == nil {
		return Capability{}, ErrInvalidInput
	}
	if input.Name != nil && (*input.Name == "" || len(*input.Name) > 128) || input.Description != nil && len(*input.Description) > 255 {
		return Capability{}, ErrInvalidInput
	}
	return repository.UpdateCapability(ctx, publicID, input, s.auditMeta(principal, meta))
}

// DeleteCapability follows the same soft-delete rule as positions. This
// avoids breaking existing employee and task-template references.
func (s *Service) DeleteCapability(ctx context.Context, principal security.Principal, publicID string, meta AuditMeta) error {
	if err := s.authorize(principal, PermissionCapabilityManage); err != nil {
		return err
	}
	repository, err := s.dictionaryRepository()
	if err != nil {
		return err
	}
	publicID = strings.TrimSpace(publicID)
	if publicID == "" {
		return ErrInvalidInput
	}
	return repository.DeleteCapability(ctx, publicID, s.auditMeta(principal, meta))
}

// normalizeSingleCapability accepts the old array field only as a wire
// compatibility bridge. New callers should use capability_code. More than
// one capability is rejected because staffing is decided at onboarding.
func normalizeSingleCapability(code string, values []string) (string, []string, bool) {
	code = strings.TrimSpace(code)
	values = normalizeList(values)
	if code != "" {
		if len(values) > 0 && (len(values) != 1 || values[0] != code) {
			return "", nil, false
		}
		values = []string{code}
	}
	if len(values) != 1 || !validCapabilityList(values) {
		return "", nil, false
	}
	return values[0], values, true
}
