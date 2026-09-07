package mysql

import (
	"context"
	"fmt"
	"time"

	coremanagement "github.com/yuanfengleeeeee/flight-collaboration-platform/internal/core/application/management"
	"github.com/yuanfengleeeeee/flight-collaboration-platform/internal/shared/id"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type managementDictionaryRow struct {
	ID          uint64    `gorm:"column:id;primaryKey"`
	PublicID    string    `gorm:"column:public_id"`
	Code        string    `gorm:"column:code"`
	Name        string    `gorm:"column:name"`
	Description string    `gorm:"column:description"`
	Enabled     bool      `gorm:"column:enabled"`
	CreatedAt   time.Time `gorm:"column:created_at"`
	UpdatedAt   time.Time `gorm:"column:updated_at"`
}

func firstCapability(values []string) string {
	if len(values) == 0 {
		return ""
	}
	return values[0]
}

func (r *ManagementRepository) ListPositions(ctx context.Context, page coremanagement.PageQuery, includeDisabled bool) (coremanagement.PositionListResult, error) {
	items, total, err := listDictionary(ctx, r, "job_position", page, includeDisabled, func(row managementDictionaryRow) coremanagement.Position {
		return coremanagement.Position{PublicID: row.PublicID, Code: row.Code, Name: row.Name, Description: row.Description, Enabled: row.Enabled}
	})
	return coremanagement.PositionListResult{Items: items, Page: page.Page, PageSize: page.PageSize, Total: total}, err
}

func (r *ManagementRepository) CreatePosition(ctx context.Context, input coremanagement.PositionInput, meta coremanagement.AuditMeta) (coremanagement.Position, error) {
	value, err := createDictionary(ctx, r, "job_position", input.Code, input.Name, input.Description, input.Enabled, meta)
	if err != nil {
		return coremanagement.Position{}, err
	}
	return coremanagement.Position{PublicID: value.PublicID, Code: value.Code, Name: value.Name, Description: value.Description, Enabled: value.Enabled}, nil
}

func (r *ManagementRepository) UpdatePosition(ctx context.Context, publicID string, patch coremanagement.PositionPatch, meta coremanagement.AuditMeta) (coremanagement.Position, error) {
	value, err := updateDictionary(ctx, r, "job_position", publicID, patch.Name, patch.Description, patch.Enabled, meta)
	if err != nil {
		return coremanagement.Position{}, err
	}
	return coremanagement.Position{PublicID: value.PublicID, Code: value.Code, Name: value.Name, Description: value.Description, Enabled: value.Enabled}, nil
}

func (r *ManagementRepository) DeletePosition(ctx context.Context, publicID string, meta coremanagement.AuditMeta) error {
	return deleteDictionary(ctx, r, "job_position", publicID, meta)
}

func (r *ManagementRepository) ListCapabilities(ctx context.Context, page coremanagement.PageQuery, includeDisabled bool) (coremanagement.CapabilityListResult, error) {
	items, total, err := listDictionary(ctx, r, "capability", page, includeDisabled, func(row managementDictionaryRow) coremanagement.Capability {
		return coremanagement.Capability{PublicID: row.PublicID, Code: row.Code, Name: row.Name, Description: row.Description, Enabled: row.Enabled}
	})
	return coremanagement.CapabilityListResult{Items: items, Page: page.Page, PageSize: page.PageSize, Total: total}, err
}

func (r *ManagementRepository) CreateCapability(ctx context.Context, input coremanagement.CapabilityInput, meta coremanagement.AuditMeta) (coremanagement.Capability, error) {
	value, err := createDictionary(ctx, r, "capability", input.Code, input.Name, input.Description, input.Enabled, meta)
	if err != nil {
		return coremanagement.Capability{}, err
	}
	return coremanagement.Capability{PublicID: value.PublicID, Code: value.Code, Name: value.Name, Description: value.Description, Enabled: value.Enabled}, nil
}

func (r *ManagementRepository) UpdateCapability(ctx context.Context, publicID string, patch coremanagement.CapabilityPatch, meta coremanagement.AuditMeta) (coremanagement.Capability, error) {
	value, err := updateDictionary(ctx, r, "capability", publicID, patch.Name, patch.Description, patch.Enabled, meta)
	if err != nil {
		return coremanagement.Capability{}, err
	}
	return coremanagement.Capability{PublicID: value.PublicID, Code: value.Code, Name: value.Name, Description: value.Description, Enabled: value.Enabled}, nil
}

func (r *ManagementRepository) DeleteCapability(ctx context.Context, publicID string, meta coremanagement.AuditMeta) error {
	return deleteDictionary(ctx, r, "capability", publicID, meta)
}

func listDictionary[T any](ctx context.Context, repository *ManagementRepository, table string, page coremanagement.PageQuery, includeDisabled bool, convert func(managementDictionaryRow) T) ([]T, int64, error) {
	var result struct {
		Items    []T   `json:"items"`
		Page     int   `json:"page"`
		PageSize int   `json:"page_size"`
		Total    int64 `json:"total"`
	}
	db, err := repository.database()
	if err != nil {
		return nil, 0, err
	}
	query := db.WithContext(ctx).Table(table)
	if !includeDisabled {
		query = query.Where("enabled = ?", true)
	}
	if err := query.Session(&gorm.Session{}).Count(&result.Total).Error; err != nil {
		return nil, 0, fmt.Errorf("count %s dictionary: %w", table, err)
	}
	var rows []managementDictionaryRow
	if err := query.Order("code ASC, public_id ASC").Offset((page.Page - 1) * page.PageSize).Limit(page.PageSize).Find(&rows).Error; err != nil {
		return nil, 0, fmt.Errorf("list %s dictionary: %w", table, err)
	}
	result.Items = make([]T, 0, len(rows))
	for _, row := range rows {
		result.Items = append(result.Items, convert(row))
	}
	result.Page, result.PageSize = page.Page, page.PageSize
	return result.Items, result.Total, nil
}

func createDictionary(ctx context.Context, repository *ManagementRepository, table, code, name, description string, enabled bool, meta coremanagement.AuditMeta) (managementDictionaryRow, error) {
	db, err := repository.database()
	if err != nil {
		return managementDictionaryRow{}, err
	}
	publicID, err := id.NewPublicID()
	if err != nil {
		return managementDictionaryRow{}, fmt.Errorf("generate %s public id: %w", table, err)
	}
	now := managementNow(meta)
	row := managementDictionaryRow{PublicID: publicID, Code: code, Name: name, Description: description, Enabled: enabled, CreatedAt: now, UpdatedAt: now}
	err = db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Table(table).Create(&row).Error; err != nil {
			return mapManagementWriteError(err)
		}
		return appendManagementAudit(tx, meta, "management."+table+".create", table, row.PublicID, "success")
	})
	return row, err
}

func updateDictionary(ctx context.Context, repository *ManagementRepository, table, publicID string, name, description *string, enabled *bool, meta coremanagement.AuditMeta) (managementDictionaryRow, error) {
	db, err := repository.database()
	if err != nil {
		return managementDictionaryRow{}, err
	}
	now := managementNow(meta)
	var result managementDictionaryRow
	err = db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var row managementDictionaryRow
		if err := tx.Table(table).Clauses(clause.Locking{Strength: "UPDATE"}).Where("public_id = ?", publicID).First(&row).Error; err != nil {
			return mapManagementReadError(err)
		}
		updates := map[string]any{"updated_at": now}
		if name != nil {
			updates["name"] = *name
		}
		if description != nil {
			updates["description"] = *description
		}
		if enabled != nil {
			updates["enabled"] = *enabled
		}
		if err := tx.Table(table).Where("id = ?", row.ID).Updates(updates).Error; err != nil {
			return mapManagementWriteError(err)
		}
		if err := appendManagementAudit(tx, meta, "management."+table+".update", table, row.PublicID, "success"); err != nil {
			return err
		}
		if err := tx.Table(table).Where("id = ?", row.ID).First(&row).Error; err != nil {
			return fmt.Errorf("reload %s dictionary: %w", table, err)
		}
		result = row
		return nil
	})
	return result, err
}

func deleteDictionary(ctx context.Context, repository *ManagementRepository, table, publicID string, meta coremanagement.AuditMeta) error {
	db, err := repository.database()
	if err != nil {
		return err
	}
	now := managementNow(meta)
	return db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var row managementDictionaryRow
		if err := tx.Table(table).Clauses(clause.Locking{Strength: "UPDATE"}).Where("public_id = ?", publicID).First(&row).Error; err != nil {
			return mapManagementReadError(err)
		}
		if row.Enabled {
			if err := tx.Table(table).Where("id = ?", row.ID).Updates(map[string]any{"enabled": false, "updated_at": now}).Error; err != nil {
				return mapManagementWriteError(err)
			}
		}
		return appendManagementAudit(tx, meta, "management."+table+".delete", table, row.PublicID, "success")
	})
}

func ensureEnabledDictionaryCode(tx *gorm.DB, table, code string) error {
	var row managementDictionaryRow
	if err := tx.Table(table).Where("code = ?", code).First(&row).Error; err != nil {
		return mapManagementReadError(err)
	}
	if !row.Enabled {
		return coremanagement.ErrResourceBusy
	}
	return nil
}
