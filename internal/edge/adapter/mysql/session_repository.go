package mysql

import (
	"context"
	"errors"
	"fmt"
	"time"

	edgeidentity "github.com/yuanfengleeeeee/flight-collaboration-platform/internal/edge/application/identity"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

var _ edgeidentity.SessionStore = (*SessionRepository)(nil)

type SessionRepository struct {
	db *gorm.DB
}

func NewSessionRepository(db *gorm.DB) *SessionRepository {
	return &SessionRepository{db: db}
}

func (r *SessionRepository) CreateSession(ctx context.Context, value edgeidentity.Session) error {
	if r == nil || r.db == nil {
		return fmt.Errorf("session repository is not configured")
	}
	if value.CreatedAt.IsZero() {
		value.CreatedAt = time.Now().UTC()
	}
	if value.LastSeenAt.IsZero() {
		value.LastSeenAt = value.CreatedAt
	}
	row := mobileSessionRow{SessionPublicID: value.SessionPublicID, ActorPublicID: value.ActorPublicID, Client: value.Client, RefreshTokenHash: value.RefreshTokenHash, ExpiresAt: value.ExpiresAt.UTC(), AbsoluteExpiresAt: value.AbsoluteExpiresAt.UTC(), LastSeenAt: value.LastSeenAt.UTC(), CreatedAt: value.CreatedAt.UTC()}
	if err := r.db.WithContext(ctx).Create(&row).Error; err != nil {
		return fmt.Errorf("create mobile session: %w", err)
	}
	return nil
}

func (r *SessionRepository) FindSession(ctx context.Context, sessionID string) (edgeidentity.Session, error) {
	if r == nil || r.db == nil {
		return edgeidentity.Session{}, fmt.Errorf("session repository is not configured")
	}
	var row mobileSessionRow
	if err := r.db.WithContext(ctx).Where("session_public_id = ?", sessionID).First(&row).Error; err != nil {
		return edgeidentity.Session{}, normalizeSessionError(err)
	}
	return row.toSession(), nil
}

func (r *SessionRepository) FindSessionByRefreshHash(ctx context.Context, refreshHash string) (edgeidentity.Session, error) {
	if r == nil || r.db == nil {
		return edgeidentity.Session{}, fmt.Errorf("session repository is not configured")
	}
	var row mobileSessionRow
	if err := r.db.WithContext(ctx).Where("refresh_token_hash = ?", refreshHash).First(&row).Error; err != nil {
		return edgeidentity.Session{}, normalizeSessionError(err)
	}
	return row.toSession(), nil
}

func (r *SessionRepository) RotateSession(ctx context.Context, refreshHash string, replacement edgeidentity.Session, now time.Time) error {
	if r == nil || r.db == nil {
		return fmt.Errorf("session repository is not configured")
	}
	if now.IsZero() {
		now = time.Now().UTC()
	}
	refreshReuse := false
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var old mobileSessionRow
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("refresh_token_hash = ?", refreshHash).First(&old).Error; err != nil {
			return normalizeSessionError(err)
		}
		if old.RevokedAt != nil {
			if old.ReplacedBySessionPublicID != "" {
				if err := tx.Model(&mobileSessionRow{}).Where("session_public_id = ?", old.ReplacedBySessionPublicID).Update("revoked_at", now.UTC()).Error; err != nil {
					return fmt.Errorf("revoke replacement session after refresh replay: %w", err)
				}
			}
			// The replacement revocation must commit even though the caller needs
			// the refresh-reuse sentinel. Returning that sentinel from the
			// transaction callback would roll back the security update.
			refreshReuse = true
			return nil
		}
		if replacement.SessionPublicID == "" {
			return edgeidentity.ErrSessionExpired
		}
		if err := tx.Model(&mobileSessionRow{}).Where("id = ? AND revoked_at IS NULL", old.ID).Updates(map[string]any{
			"revoked_at":                    now.UTC(),
			"replaced_by_session_public_id": replacement.SessionPublicID,
			"last_seen_at":                  now.UTC(),
		}).Error; err != nil {
			return fmt.Errorf("rotate mobile session: %w", err)
		}
		row := mobileSessionRow{SessionPublicID: replacement.SessionPublicID, ActorPublicID: replacement.ActorPublicID, Client: replacement.Client, RefreshTokenHash: replacement.RefreshTokenHash, ExpiresAt: replacement.ExpiresAt.UTC(), AbsoluteExpiresAt: replacement.AbsoluteExpiresAt.UTC(), LastSeenAt: replacement.LastSeenAt.UTC(), CreatedAt: replacement.CreatedAt.UTC()}
		if err := tx.Create(&row).Error; err != nil {
			return fmt.Errorf("create rotated mobile session: %w", err)
		}
		return nil
	})
	if err != nil {
		return err
	}
	if refreshReuse {
		return edgeidentity.ErrRefreshReuse
	}
	return nil
}

func (r *SessionRepository) RevokeSession(ctx context.Context, sessionID string, now time.Time) error {
	if r == nil || r.db == nil {
		return fmt.Errorf("session repository is not configured")
	}
	result := r.db.WithContext(ctx).Model(&mobileSessionRow{}).Where("session_public_id = ? AND revoked_at IS NULL", sessionID).Updates(map[string]any{"revoked_at": now.UTC(), "last_seen_at": now.UTC()})
	if result.Error != nil {
		return fmt.Errorf("revoke mobile session: %w", result.Error)
	}
	if result.RowsAffected == 0 {
		var exists int64
		if err := r.db.WithContext(ctx).Model(&mobileSessionRow{}).Where("session_public_id = ?", sessionID).Count(&exists).Error; err != nil {
			return err
		}
		if exists == 0 {
			return edgeidentity.ErrSessionNotFound
		}
	}
	return nil
}

type mobileSessionRow struct {
	ID                        uint64     `gorm:"column:id;primaryKey;autoIncrement"`
	SessionPublicID           string     `gorm:"column:session_public_id"`
	ActorPublicID             string     `gorm:"column:actor_public_id"`
	Client                    string     `gorm:"column:client"`
	RefreshTokenHash          string     `gorm:"column:refresh_token_hash"`
	ExpiresAt                 time.Time  `gorm:"column:expires_at"`
	AbsoluteExpiresAt         time.Time  `gorm:"column:absolute_expires_at"`
	LastSeenAt                time.Time  `gorm:"column:last_seen_at"`
	RevokedAt                 *time.Time `gorm:"column:revoked_at"`
	ReplacedBySessionPublicID string     `gorm:"column:replaced_by_session_public_id"`
	CreatedAt                 time.Time  `gorm:"column:created_at"`
}

func (mobileSessionRow) TableName() string { return "mobile_session" }

func (row mobileSessionRow) toSession() edgeidentity.Session {
	return edgeidentity.Session{SessionPublicID: row.SessionPublicID, ActorPublicID: row.ActorPublicID, Client: row.Client, RefreshTokenHash: row.RefreshTokenHash, CreatedAt: row.CreatedAt.UTC(), LastSeenAt: row.LastSeenAt.UTC(), ExpiresAt: row.ExpiresAt.UTC(), AbsoluteExpiresAt: row.AbsoluteExpiresAt.UTC(), RevokedAt: row.RevokedAt, ReplacedBySessionPublicID: row.ReplacedBySessionPublicID}
}

func normalizeSessionError(err error) error {
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return edgeidentity.ErrSessionNotFound
	}
	return fmt.Errorf("find mobile session: %w", err)
}
