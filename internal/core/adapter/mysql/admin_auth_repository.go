package mysql

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	adminauth "github.com/yuanfengleeeeee/flight-collaboration-platform/internal/core/application/adminauth"
	"github.com/yuanfengleeeeee/flight-collaboration-platform/internal/platform/security"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

var _ adminauth.Repository = (*AdminAuthRepository)(nil)

type AdminAuthRepository struct{ db *gorm.DB }

func NewAdminAuthRepository(db *gorm.DB) *AdminAuthRepository { return &AdminAuthRepository{db: db} }

func (r *AdminAuthRepository) CreateSSOState(ctx context.Context, value adminauth.SSOState) error {
	if r == nil || r.db == nil {
		return fmt.Errorf("admin auth repository is not configured")
	}
	if strings.TrimSpace(value.PublicID) == "" || strings.TrimSpace(value.StateHash) == "" || strings.TrimSpace(value.Provider) == "" || strings.TrimSpace(value.RedirectURI) == "" || value.ExpiresAt.IsZero() {
		return fmt.Errorf("admin SSO state fields are required")
	}
	now := time.Now().UTC()
	row := adminSSOStateRow{PublicID: value.PublicID, StateHash: value.StateHash, Provider: value.Provider, RedirectURI: value.RedirectURI, ExpiresAt: value.ExpiresAt.UTC(), CreatedAt: now}
	if err := r.db.WithContext(ctx).Create(&row).Error; err != nil {
		return fmt.Errorf("create admin SSO state: %w", err)
	}
	return nil
}

func (r *AdminAuthRepository) ConsumeSSOState(ctx context.Context, stateHash string, now time.Time) (adminauth.SSOState, error) {
	if r == nil || r.db == nil {
		return adminauth.SSOState{}, fmt.Errorf("admin auth repository is not configured")
	}
	if now.IsZero() {
		now = time.Now().UTC()
	}
	var value adminauth.SSOState
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var row adminSSOStateRow
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("state_hash = ?", strings.TrimSpace(stateHash)).First(&row).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return adminauth.ErrInvalidState
			}
			return fmt.Errorf("find admin SSO state: %w", err)
		}
		if row.ConsumedAt != nil || !row.ExpiresAt.After(now) {
			return adminauth.ErrInvalidState
		}
		if err := tx.Model(&adminSSOStateRow{}).Where("id = ? AND consumed_at IS NULL", row.ID).Updates(map[string]any{"consumed_at": now, "updated_at": now}).Error; err != nil {
			return fmt.Errorf("consume admin SSO state: %w", err)
		}
		value = row.toDomain()
		return nil
	})
	if err != nil {
		return adminauth.SSOState{}, err
	}
	return value, nil
}

func (r *AdminAuthRepository) FindIdentity(ctx context.Context, provider, externalSubject string) (adminauth.AdminIdentity, error) {
	if r == nil || r.db == nil {
		return adminauth.AdminIdentity{}, fmt.Errorf("admin auth repository is not configured")
	}
	var row adminIdentityRow
	err := r.db.WithContext(ctx).Where("provider = ? AND external_subject = ?", strings.TrimSpace(provider), strings.TrimSpace(externalSubject)).First(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return adminauth.AdminIdentity{}, adminauth.ErrIdentityUnmapped
	}
	if err != nil {
		return adminauth.AdminIdentity{}, fmt.Errorf("find admin identity: %w", err)
	}
	return row.toDomain()
}

func (r *AdminAuthRepository) CreateSession(ctx context.Context, value adminauth.Session) error {
	if r == nil || r.db == nil {
		return fmt.Errorf("admin auth repository is not configured")
	}
	if strings.TrimSpace(value.PublicID) == "" || strings.TrimSpace(value.AdminIdentityPublicID) == "" || value.ExpiresAt.IsZero() || value.AbsoluteExpiresAt.IsZero() {
		return fmt.Errorf("admin session fields are required")
	}
	now := time.Now().UTC()
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var identity adminIdentityRow
		if err := tx.Where("public_id = ?", value.AdminIdentityPublicID).First(&identity).Error; err != nil {
			return fmt.Errorf("find admin identity for session: %w", err)
		}
		if !identity.Enabled || !adminauth.IsValidAdminRole(identity.Role) {
			return adminauth.ErrAdminInactive
		}
		row := adminSessionRow{PublicID: value.PublicID, AdminIdentityID: identity.ID, CreatedAt: value.CreatedAt.UTC(), LastSeenAt: value.LastSeenAt.UTC(), ExpiresAt: value.ExpiresAt.UTC(), AbsoluteExpiresAt: value.AbsoluteExpiresAt.UTC()}
		if err := tx.Create(&row).Error; err != nil {
			return fmt.Errorf("create admin session: %w", err)
		}
		if err := tx.Create(&auditRow{ActorType: "human", ActorID: value.AdminIdentityPublicID, Action: "admin.sso.login", ResourceType: "admin_session", ResourceID: value.PublicID, Result: "success", OccurredAt: now, CreatedAt: now}).Error; err != nil {
			return fmt.Errorf("audit admin SSO login: %w", err)
		}
		return nil
	})
}

func (r *AdminAuthRepository) FindSession(ctx context.Context, publicID string) (adminauth.Session, error) {
	if r == nil || r.db == nil {
		return adminauth.Session{}, fmt.Errorf("admin auth repository is not configured")
	}
	var row adminSessionWithIdentityRow
	err := r.db.WithContext(ctx).Table("admin_session AS s").
		Select(`s.public_id AS session_public_id, s.admin_identity_id, s.created_at AS session_created_at,
			s.last_seen_at, s.expires_at, s.absolute_expires_at, s.revoked_at,
			i.public_id, i.provider, i.external_subject, i.display_name, i.role, i.enabled,
			i.global_scope, i.area_ids, i.team_ids, i.user_id`).
		Joins("JOIN admin_identity AS i ON i.id = s.admin_identity_id").
		Where("s.public_id = ?", strings.TrimSpace(publicID)).First(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return adminauth.Session{}, adminauth.ErrSessionNotFound
	}
	if err != nil {
		return adminauth.Session{}, fmt.Errorf("find admin session: %w", err)
	}
	return row.toDomain()
}

func (r *AdminAuthRepository) RevokeSession(ctx context.Context, publicID string, now time.Time) error {
	if r == nil || r.db == nil {
		return fmt.Errorf("admin auth repository is not configured")
	}
	if now.IsZero() {
		now = time.Now().UTC()
	}
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		result := tx.Model(&adminSessionRow{}).Where("public_id = ? AND revoked_at IS NULL", strings.TrimSpace(publicID)).Updates(map[string]any{"revoked_at": now, "last_seen_at": now})
		if result.Error != nil {
			return fmt.Errorf("revoke admin session: %w", result.Error)
		}
		if result.RowsAffected == 0 {
			return adminauth.ErrSessionNotFound
		}
		if err := tx.Create(&auditRow{ActorType: "human", Action: "admin.sso.logout", ResourceType: "admin_session", ResourceID: strings.TrimSpace(publicID), Result: "success", OccurredAt: now, CreatedAt: now}).Error; err != nil {
			return fmt.Errorf("audit admin SSO logout: %w", err)
		}
		return nil
	})
}

type adminIdentityRow struct {
	ID          uint64 `gorm:"column:id;primaryKey"`
	PublicID    string `gorm:"column:public_id"`
	Provider    string `gorm:"column:provider"`
	ExternalSub string `gorm:"column:external_subject"`
	DisplayName string `gorm:"column:display_name"`
	Role        string `gorm:"column:role"`
	Enabled     bool   `gorm:"column:enabled"`
	GlobalScope bool   `gorm:"column:global_scope"`
	AreaIDs     []byte `gorm:"column:area_ids"`
	TeamIDs     []byte `gorm:"column:team_ids"`
	UserID      uint64 `gorm:"column:user_id"`
}

func (adminIdentityRow) TableName() string { return "admin_identity" }

type adminSSOStateRow struct {
	ID          uint64     `gorm:"column:id;primaryKey;autoIncrement"`
	PublicID    string     `gorm:"column:public_id"`
	StateHash   string     `gorm:"column:state_hash"`
	Provider    string     `gorm:"column:provider"`
	RedirectURI string     `gorm:"column:redirect_uri"`
	ExpiresAt   time.Time  `gorm:"column:expires_at"`
	ConsumedAt  *time.Time `gorm:"column:consumed_at"`
	CreatedAt   time.Time  `gorm:"column:created_at"`
	UpdatedAt   time.Time  `gorm:"column:updated_at"`
}

func (adminSSOStateRow) TableName() string { return "admin_sso_state" }

func (row adminSSOStateRow) toDomain() adminauth.SSOState {
	return adminauth.SSOState{PublicID: row.PublicID, StateHash: row.StateHash, Provider: row.Provider, RedirectURI: row.RedirectURI, ExpiresAt: row.ExpiresAt.UTC(), ConsumedAt: row.ConsumedAt}
}

type adminSessionRow struct {
	ID                uint64     `gorm:"column:id;primaryKey;autoIncrement"`
	PublicID          string     `gorm:"column:public_id"`
	AdminIdentityID   uint64     `gorm:"column:admin_identity_id"`
	CreatedAt         time.Time  `gorm:"column:created_at"`
	LastSeenAt        time.Time  `gorm:"column:last_seen_at"`
	ExpiresAt         time.Time  `gorm:"column:expires_at"`
	AbsoluteExpiresAt time.Time  `gorm:"column:absolute_expires_at"`
	RevokedAt         *time.Time `gorm:"column:revoked_at"`
}

func (adminSessionRow) TableName() string { return "admin_session" }

type adminSessionWithIdentityRow struct {
	SessionPublicID   string     `gorm:"column:session_public_id"`
	AdminIdentityID   uint64     `gorm:"column:admin_identity_id"`
	SessionCreatedAt  time.Time  `gorm:"column:session_created_at"`
	LastSeenAt        time.Time  `gorm:"column:last_seen_at"`
	ExpiresAt         time.Time  `gorm:"column:expires_at"`
	AbsoluteExpiresAt time.Time  `gorm:"column:absolute_expires_at"`
	RevokedAt         *time.Time `gorm:"column:revoked_at"`
	PublicID          string     `gorm:"column:public_id"`
	Provider          string     `gorm:"column:provider"`
	ExternalSub       string     `gorm:"column:external_subject"`
	DisplayName       string     `gorm:"column:display_name"`
	Role              string     `gorm:"column:role"`
	Enabled           bool       `gorm:"column:enabled"`
	GlobalScope       bool       `gorm:"column:global_scope"`
	AreaIDs           []byte     `gorm:"column:area_ids"`
	TeamIDs           []byte     `gorm:"column:team_ids"`
	UserID            uint64     `gorm:"column:user_id"`
}

func (row adminIdentityRow) toDomain() (adminauth.AdminIdentity, error) {
	areaIDs, err := decodeUint64List(row.AreaIDs)
	if err != nil {
		return adminauth.AdminIdentity{}, fmt.Errorf("decode admin area scope: %w", err)
	}
	teamIDs, err := decodeUint64List(row.TeamIDs)
	if err != nil {
		return adminauth.AdminIdentity{}, fmt.Errorf("decode admin team scope: %w", err)
	}
	return adminauth.AdminIdentity{PublicID: row.PublicID, Provider: row.Provider, ExternalSub: row.ExternalSub, DisplayName: row.DisplayName, Role: row.Role, Enabled: row.Enabled, AccessScope: security.AccessScope{Global: row.GlobalScope, AreaIDs: areaIDs, TeamIDs: teamIDs, UserID: row.UserID}}, nil
}

func (row adminSessionWithIdentityRow) toDomain() (adminauth.Session, error) {
	identity, err := (adminIdentityRow{PublicID: row.PublicID, Provider: row.Provider, ExternalSub: row.ExternalSub, DisplayName: row.DisplayName, Role: row.Role, Enabled: row.Enabled, GlobalScope: row.GlobalScope, AreaIDs: row.AreaIDs, TeamIDs: row.TeamIDs, UserID: row.UserID}).toDomain()
	if err != nil {
		return adminauth.Session{}, err
	}
	return adminauth.Session{PublicID: row.SessionPublicID, AdminIdentityPublicID: identity.PublicID, CreatedAt: row.SessionCreatedAt.UTC(), LastSeenAt: row.LastSeenAt.UTC(), ExpiresAt: row.ExpiresAt.UTC(), AbsoluteExpiresAt: row.AbsoluteExpiresAt.UTC(), RevokedAt: row.RevokedAt, Identity: identity}, nil
}

func encodeUint64List(values []uint64) ([]byte, error) {
	if values == nil {
		values = []uint64{}
	}
	return json.Marshal(values)
}

func decodeUint64List(value []byte) ([]uint64, error) {
	if len(strings.TrimSpace(string(value))) == 0 || string(value) == "null" {
		return []uint64{}, nil
	}
	var result []uint64
	if err := json.Unmarshal(value, &result); err != nil {
		return nil, err
	}
	if result == nil {
		return []uint64{}, nil
	}
	return result, nil
}
