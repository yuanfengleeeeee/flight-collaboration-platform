package mysql

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	coremanagement "github.com/yuanfengleeeeee/flight-collaboration-platform/internal/core/application/management"
	"github.com/yuanfengleeeeee/flight-collaboration-platform/internal/platform/security"
	"github.com/yuanfengleeeeee/flight-collaboration-platform/internal/shared/id"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// ManagementRepository is the write-side adapter for the Core management
// workbench. Each mutation and its audit record share one transaction so a
// management change can never be visible without its audit trail.
type ManagementRepository struct{ db *gorm.DB }

var _ coremanagement.Repository = (*ManagementRepository)(nil)

func NewManagementRepository(db *gorm.DB) *ManagementRepository {
	return &ManagementRepository{db: db}
}

func (r *ManagementRepository) ListAreas(ctx context.Context, search string, page coremanagement.PageQuery, includeDisabled bool) (coremanagement.AreaListResult, error) {
	db, err := r.database()
	if err != nil {
		return coremanagement.AreaListResult{}, err
	}
	query := db.WithContext(ctx).Model(&managementAreaRow{})
	if !includeDisabled {
		query = query.Where("enabled = ?", true)
	}
	if search = strings.TrimSpace(search); search != "" {
		pattern := managementSearchPattern(search)
		query = query.Where("(code LIKE ? OR name LIKE ?)", pattern, pattern)
	}
	var total int64
	if err := query.Session(&gorm.Session{}).Count(&total).Error; err != nil {
		return coremanagement.AreaListResult{}, fmt.Errorf("count operation areas: %w", err)
	}
	var rows []managementAreaRow
	if err := query.Order("code ASC, public_id ASC").Offset((page.Page - 1) * page.PageSize).Limit(page.PageSize).Find(&rows).Error; err != nil {
		return coremanagement.AreaListResult{}, fmt.Errorf("list operation areas: %w", err)
	}
	items := make([]coremanagement.Area, 0, len(rows))
	for _, row := range rows {
		items = append(items, row.toArea())
	}
	return coremanagement.AreaListResult{Items: items, Page: page.Page, PageSize: page.PageSize, Total: total}, nil
}

func (r *ManagementRepository) CreateArea(ctx context.Context, input coremanagement.AreaInput, meta coremanagement.AuditMeta) (coremanagement.Area, error) {
	db, err := r.database()
	if err != nil {
		return coremanagement.Area{}, err
	}
	publicID, err := id.NewPublicID()
	if err != nil {
		return coremanagement.Area{}, fmt.Errorf("generate area public id: %w", err)
	}
	now := managementNow(meta)
	row := managementAreaRow{PublicID: publicID, Code: input.Code, Name: input.Name, Enabled: input.Enabled, CreatedAt: now, UpdatedAt: now}
	if err := db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(&row).Error; err != nil {
			return mapManagementWriteError(err)
		}
		return appendManagementAudit(tx, meta, "management.area.create", "operation_area", row.PublicID, "success")
	}); err != nil {
		return coremanagement.Area{}, err
	}
	return row.toArea(), nil
}

func (r *ManagementRepository) UpdateArea(ctx context.Context, publicID string, patch coremanagement.AreaPatch, meta coremanagement.AuditMeta) (coremanagement.Area, error) {
	db, err := r.database()
	if err != nil {
		return coremanagement.Area{}, err
	}
	var result coremanagement.Area
	now := managementNow(meta)
	err = db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var row managementAreaRow
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("public_id = ?", publicID).First(&row).Error; err != nil {
			return mapManagementReadError(err)
		}
		if patch.Enabled != nil && !*patch.Enabled {
			var activeTeams int64
			if err := tx.Model(&managementTeamRow{}).Where("area_id = ? AND enabled = ?", row.ID, true).Count(&activeTeams).Error; err != nil {
				return fmt.Errorf("check active area teams: %w", err)
			}
			if activeTeams > 0 {
				return coremanagement.ErrResourceBusy
			}
		}
		updates := map[string]any{"updated_at": now}
		if patch.Code != nil {
			updates["code"] = *patch.Code
		}
		if patch.Name != nil {
			updates["name"] = *patch.Name
		}
		if patch.Enabled != nil {
			updates["enabled"] = *patch.Enabled
		}
		if err := tx.Model(&managementAreaRow{}).Where("id = ?", row.ID).Updates(updates).Error; err != nil {
			return mapManagementWriteError(err)
		}
		if err := appendManagementAudit(tx, meta, "management.area.update", "operation_area", row.PublicID, "success"); err != nil {
			return err
		}
		if err := tx.Where("id = ?", row.ID).First(&row).Error; err != nil {
			return fmt.Errorf("reload operation area: %w", err)
		}
		result = row.toArea()
		return nil
	})
	return result, err
}

func (r *ManagementRepository) ListTeams(ctx context.Context, areaPublicID, search string, page coremanagement.PageQuery, includeDisabled bool) (coremanagement.TeamListResult, error) {
	db, err := r.database()
	if err != nil {
		return coremanagement.TeamListResult{}, err
	}
	query := db.WithContext(ctx).Table("team AS t").Select("t.public_id, t.area_id, oa.public_id AS area_public_id, oa.name AS area_name, t.code, t.name, t.enabled").Joins("JOIN operation_area AS oa ON oa.id = t.area_id")
	if areaPublicID != "" {
		query = query.Where("oa.public_id = ?", areaPublicID)
	}
	if !includeDisabled {
		query = query.Where("t.enabled = ? AND oa.enabled = ?", true, true)
	}
	if search = strings.TrimSpace(search); search != "" {
		pattern := managementSearchPattern(search)
		query = query.Where("(t.code LIKE ? OR t.name LIKE ? OR oa.code LIKE ? OR oa.name LIKE ?)", pattern, pattern, pattern, pattern)
	}
	var total int64
	if err := query.Session(&gorm.Session{}).Count(&total).Error; err != nil {
		return coremanagement.TeamListResult{}, fmt.Errorf("count teams: %w", err)
	}
	var rows []managementTeamQueryRow
	if err := query.Order("oa.code ASC, t.code ASC, t.public_id ASC").Offset((page.Page - 1) * page.PageSize).Limit(page.PageSize).Find(&rows).Error; err != nil {
		return coremanagement.TeamListResult{}, fmt.Errorf("list teams: %w", err)
	}
	items := make([]coremanagement.Team, 0, len(rows))
	for _, row := range rows {
		items = append(items, row.toTeam())
	}
	return coremanagement.TeamListResult{Items: items, Page: page.Page, PageSize: page.PageSize, Total: total}, nil
}

func managementSearchPattern(value string) string {
	value = strings.ReplaceAll(value, `\`, `\\`)
	value = strings.ReplaceAll(value, "%", `\%`)
	value = strings.ReplaceAll(value, "_", `\_`)
	return "%" + value + "%"
}

func (r *ManagementRepository) CreateTeam(ctx context.Context, input coremanagement.TeamInput, meta coremanagement.AuditMeta) (coremanagement.Team, error) {
	db, err := r.database()
	if err != nil {
		return coremanagement.Team{}, err
	}
	publicID, err := id.NewPublicID()
	if err != nil {
		return coremanagement.Team{}, fmt.Errorf("generate team public id: %w", err)
	}
	now := managementNow(meta)
	var result coremanagement.Team
	err = db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		area, err := findManagementArea(tx, input.AreaPublicID, true)
		if err != nil {
			return err
		}
		row := managementTeamRow{PublicID: publicID, AreaID: area.ID, Code: input.Code, Name: input.Name, Enabled: input.Enabled, CreatedAt: now, UpdatedAt: now}
		if err := tx.Create(&row).Error; err != nil {
			return mapManagementWriteError(err)
		}
		if err := appendManagementAudit(tx, meta, "management.team.create", "team", row.PublicID, "success"); err != nil {
			return err
		}
		result = coremanagement.Team{PublicID: row.PublicID, AreaPublicID: area.PublicID, AreaName: area.Name, Code: row.Code, Name: row.Name, Enabled: row.Enabled}
		return nil
	})
	return result, err
}

func (r *ManagementRepository) UpdateTeam(ctx context.Context, publicID string, patch coremanagement.TeamPatch, meta coremanagement.AuditMeta) (coremanagement.Team, error) {
	db, err := r.database()
	if err != nil {
		return coremanagement.Team{}, err
	}
	now := managementNow(meta)
	var result coremanagement.Team
	err = db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var row managementTeamRow
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("public_id = ?", publicID).First(&row).Error; err != nil {
			return mapManagementReadError(err)
		}
		if patch.Enabled != nil && !*patch.Enabled {
			if err := ensureTeamHasNoEnabledPersonnel(tx, row.ID); err != nil {
				return err
			}
		}
		area, err := findManagementAreaByID(tx, row.AreaID)
		if err != nil {
			return err
		}
		if patch.AreaPublicID != nil && *patch.AreaPublicID != area.PublicID {
			newArea, findErr := findManagementArea(tx, *patch.AreaPublicID, true)
			if findErr != nil {
				return findErr
			}
			var personnelCount int64
			if err := tx.Model(&managementPersonnelRow{}).Where("team_id = ? AND enabled = ?", row.ID, true).Count(&personnelCount).Error; err != nil {
				return fmt.Errorf("check personnel before moving team: %w", err)
			}
			if personnelCount > 0 {
				return coremanagement.ErrResourceBusy
			}
			row.AreaID = newArea.ID
			area = newArea
		}
		updates := map[string]any{"updated_at": now}
		if patch.AreaPublicID != nil {
			updates["area_id"] = row.AreaID
		}
		if patch.Code != nil {
			updates["code"] = *patch.Code
		}
		if patch.Name != nil {
			updates["name"] = *patch.Name
		}
		if patch.Enabled != nil {
			updates["enabled"] = *patch.Enabled
		}
		if err := tx.Model(&managementTeamRow{}).Where("id = ?", row.ID).Updates(updates).Error; err != nil {
			return mapManagementWriteError(err)
		}
		if err := appendManagementAudit(tx, meta, "management.team.update", "team", row.PublicID, "success"); err != nil {
			return err
		}
		if err := tx.Where("id = ?", row.ID).First(&row).Error; err != nil {
			return fmt.Errorf("reload team: %w", err)
		}
		result = coremanagement.Team{PublicID: row.PublicID, AreaPublicID: area.PublicID, AreaName: area.Name, Code: row.Code, Name: row.Name, Enabled: row.Enabled}
		return nil
	})
	return result, err
}

func (r *ManagementRepository) CreatePersonnel(ctx context.Context, input coremanagement.PersonnelWrite, meta coremanagement.AuditMeta) (coremanagement.Personnel, error) {
	db, err := r.database()
	if err != nil {
		return coremanagement.Personnel{}, err
	}
	personnelID, err := id.NewPublicID()
	if err != nil {
		return coremanagement.Personnel{}, fmt.Errorf("generate personnel public id: %w", err)
	}
	userID, err := id.NewPublicID()
	if err != nil {
		return coremanagement.Personnel{}, fmt.Errorf("generate personnel user id: %w", err)
	}
	credentialID, err := id.NewPublicID()
	if err != nil {
		return coremanagement.Personnel{}, fmt.Errorf("generate credential public id: %w", err)
	}
	capabilities, err := json.Marshal(input.Capabilities)
	if err != nil {
		return coremanagement.Personnel{}, fmt.Errorf("encode personnel capabilities: %w", err)
	}
	now := managementNow(meta)
	var result coremanagement.Personnel
	err = db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		team, err := findManagementTeam(tx, input.TeamPublicID, true)
		if err != nil {
			return err
		}
		if err := ensureEnabledDictionaryCode(tx, "job_position", input.PositionCode); err != nil {
			return err
		}
		if err := ensureEnabledDictionaryCode(tx, "capability", input.CapabilityCode); err != nil {
			return err
		}
		area, err := findManagementAreaByID(tx, team.AreaID)
		if err != nil {
			return err
		}
		personnel := managementPersonnelRow{PublicID: personnelID, UserPublicID: userID, EmployeeNo: input.EmployeeNo, DisplayName: input.DisplayName, TeamID: team.ID, PositionCode: input.PositionCode, Capabilities: capabilities, WorkState: "idle", StatusVersion: 0, LastStateChangedAt: now, Enabled: true, CreatedAt: now, UpdatedAt: now}
		if err := tx.Create(&personnel).Error; err != nil {
			return mapManagementWriteError(err)
		}
		memberID, err := id.NewPublicID()
		if err != nil {
			return fmt.Errorf("generate team member public id: %w", err)
		}
		member := managementTeamMemberRow{PublicID: memberID, TeamID: team.ID, PersonnelID: personnel.ID, IsPrimary: true, JoinedAt: now, CreatedAt: now}
		if err := tx.Create(&member).Error; err != nil {
			return mapManagementWriteError(err)
		}
		credential := managementEmployeeCredentialRow{PublicID: credentialID, PersonnelID: personnel.ID, PasswordHash: input.PasswordHash, PasswordChangedAt: now, CreatedAt: now, UpdatedAt: now}
		if err := tx.Create(&credential).Error; err != nil {
			return mapManagementWriteError(err)
		}
		if err := appendManagementAudit(tx, meta, "management.personnel.create", "personnel", personnel.PublicID, "success"); err != nil {
			return err
		}
		result = personnel.toPersonnel(team)
		result.AreaPublicID = area.PublicID
		result.HasCredential = true
		return nil
	})
	return result, err
}

func (r *ManagementRepository) UpdatePersonnel(ctx context.Context, publicID string, patch coremanagement.PersonnelPatch, meta coremanagement.AuditMeta) (coremanagement.Personnel, error) {
	db, err := r.database()
	if err != nil {
		return coremanagement.Personnel{}, err
	}
	now := managementNow(meta)
	var result coremanagement.Personnel
	err = db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var row managementPersonnelRow
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("public_id = ?", publicID).First(&row).Error; err != nil {
			return mapManagementReadError(err)
		}
		if (patch.TeamPublicID != nil || patch.Enabled != nil && !*patch.Enabled) && row.WorkState != "idle" {
			return coremanagement.ErrResourceBusy
		}
		team, err := findManagementTeamByID(tx, row.TeamID)
		if err != nil {
			return err
		}
		area, err := findManagementAreaByID(tx, team.AreaID)
		if err != nil {
			return err
		}
		if patch.Enabled != nil && *patch.Enabled && (!team.Enabled || !area.Enabled) {
			return coremanagement.ErrResourceBusy
		}
		if patch.TeamPublicID != nil && *patch.TeamPublicID != team.PublicID {
			newTeam, findErr := findManagementTeam(tx, *patch.TeamPublicID, true)
			if findErr != nil {
				return findErr
			}
			if err := tx.Model(&managementTeamMemberRow{}).Where("personnel_id = ? AND left_at IS NULL", row.ID).Updates(map[string]any{"left_at": now}).Error; err != nil {
				return fmt.Errorf("close personnel team membership: %w", err)
			}
			memberID, generateErr := id.NewPublicID()
			if generateErr != nil {
				return fmt.Errorf("generate replacement team member id: %w", generateErr)
			}
			if err := tx.Create(&managementTeamMemberRow{PublicID: memberID, TeamID: newTeam.ID, PersonnelID: row.ID, IsPrimary: true, JoinedAt: now, CreatedAt: now}).Error; err != nil {
				return mapManagementWriteError(err)
			}
			row.TeamID = newTeam.ID
			team = newTeam
			area, err = findManagementAreaByID(tx, team.AreaID)
			if err != nil {
				return err
			}
		}
		updates := map[string]any{"updated_at": now}
		if patch.EmployeeNo != nil {
			updates["employee_no"] = *patch.EmployeeNo
		}
		if patch.DisplayName != nil {
			updates["display_name"] = *patch.DisplayName
		}
		if patch.TeamPublicID != nil {
			updates["team_id"] = row.TeamID
		}
		if patch.PositionCode != nil {
			if err := ensureEnabledDictionaryCode(tx, "job_position", *patch.PositionCode); err != nil {
				return err
			}
			updates["position_code"] = *patch.PositionCode
		}
		if patch.Capabilities != nil {
			if len(*patch.Capabilities) != 1 {
				return coremanagement.ErrInvalidInput
			}
			if err := ensureEnabledDictionaryCode(tx, "capability", (*patch.Capabilities)[0]); err != nil {
				return err
			}
			capabilities, marshalErr := json.Marshal(*patch.Capabilities)
			if marshalErr != nil {
				return fmt.Errorf("encode personnel capabilities: %w", marshalErr)
			}
			updates["capabilities"] = capabilities
		}
		if patch.Enabled != nil {
			updates["enabled"] = *patch.Enabled
		}
		if err := tx.Model(&managementPersonnelRow{}).Where("id = ?", row.ID).Updates(updates).Error; err != nil {
			return mapManagementWriteError(err)
		}
		if err := appendManagementAudit(tx, meta, "management.personnel.update", "personnel", row.PublicID, "success"); err != nil {
			return err
		}
		loaded, err := loadPersonnel(tx, row.ID)
		if err != nil {
			return err
		}
		result = loaded.toPersonnel(team)
		result.AreaPublicID = area.PublicID
		result.HasCredential = loaded.HasCredential
		return nil
	})
	return result, err
}

func (r *ManagementRepository) ResetPersonnelPassword(ctx context.Context, publicID, passwordHash string, meta coremanagement.AuditMeta) error {
	db, err := r.database()
	if err != nil {
		return err
	}
	now := managementNow(meta)
	return db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var personnel managementPersonnelRow
		if err := tx.Where("public_id = ?", publicID).First(&personnel).Error; err != nil {
			return mapManagementReadError(err)
		}
		var credential managementEmployeeCredentialRow
		err := tx.Where("personnel_id = ?", personnel.ID).First(&credential).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			credentialID, generateErr := id.NewPublicID()
			if generateErr != nil {
				return fmt.Errorf("generate credential public id: %w", generateErr)
			}
			if err := tx.Create(&managementEmployeeCredentialRow{PublicID: credentialID, PersonnelID: personnel.ID, PasswordHash: passwordHash, PasswordChangedAt: now, CreatedAt: now, UpdatedAt: now}).Error; err != nil {
				return mapManagementWriteError(err)
			}
		} else if err != nil {
			return fmt.Errorf("find personnel credential: %w", err)
		} else if err := tx.Model(&managementEmployeeCredentialRow{}).Where("id = ?", credential.ID).Updates(map[string]any{"password_hash": passwordHash, "failed_attempts": 0, "locked_until": nil, "password_changed_at": now, "updated_at": now}).Error; err != nil {
			return fmt.Errorf("update personnel password: %w", err)
		}
		return appendManagementAudit(tx, meta, "management.personnel.password_reset", "personnel", personnel.PublicID, "success")
	})
}

func (r *ManagementRepository) ListTemplates(ctx context.Context, page coremanagement.PageQuery, includeDisabled bool) (coremanagement.TemplateListResult, error) {
	db, err := r.database()
	if err != nil {
		return coremanagement.TemplateListResult{}, err
	}
	query := db.WithContext(ctx).Table("task_template AS tt").Select(`tt.public_id, tt.name, tt.trigger_type, tt.template_version, tt.enabled,
		oa.public_id AS area_public_id, t.public_id AS team_public_id, tt.required_position_code,
		tt.required_capabilities, tt.planned_offset_seconds, tt.default_message`).
		Joins("JOIN operation_area AS oa ON oa.id = tt.target_area_id").Joins("JOIN team AS t ON t.id = tt.target_team_id")
	if !includeDisabled {
		query = query.Where("tt.enabled = ?", true)
	}
	var total int64
	if err := query.Session(&gorm.Session{}).Count(&total).Error; err != nil {
		return coremanagement.TemplateListResult{}, fmt.Errorf("count task templates: %w", err)
	}
	var rows []managementTemplateQueryRow
	if err := query.Order("tt.name ASC, tt.template_version DESC, tt.public_id ASC").Offset((page.Page - 1) * page.PageSize).Limit(page.PageSize).Find(&rows).Error; err != nil {
		return coremanagement.TemplateListResult{}, fmt.Errorf("list task templates: %w", err)
	}
	items := make([]coremanagement.TaskTemplate, 0, len(rows))
	for _, row := range rows {
		item, err := row.toTemplate()
		if err != nil {
			return coremanagement.TemplateListResult{}, err
		}
		items = append(items, item)
	}
	return coremanagement.TemplateListResult{Items: items, Page: page.Page, PageSize: page.PageSize, Total: total}, nil
}

func (r *ManagementRepository) CreateTemplate(ctx context.Context, input coremanagement.TaskTemplateWrite, meta coremanagement.AuditMeta) (coremanagement.TaskTemplate, error) {
	db, err := r.database()
	if err != nil {
		return coremanagement.TaskTemplate{}, err
	}
	publicID, err := id.NewPublicID()
	if err != nil {
		return coremanagement.TaskTemplate{}, fmt.Errorf("generate task template public id: %w", err)
	}
	capabilities, err := json.Marshal(input.RequiredCapabilities)
	if err != nil {
		return coremanagement.TaskTemplate{}, fmt.Errorf("encode template capabilities: %w", err)
	}
	now := managementNow(meta)
	var result coremanagement.TaskTemplate
	err = db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		areaID, teamID, err := resolveTemplateTarget(tx, input.AreaPublicID, input.TeamPublicID)
		if err != nil {
			return err
		}
		if err := ensureEnabledDictionaryCode(tx, "job_position", input.RequiredPositionCode); err != nil {
			return err
		}
		if err := ensureEnabledDictionaryCode(tx, "capability", input.RequiredCapabilityCode); err != nil {
			return err
		}
		row := managementTaskTemplateRow{PublicID: publicID, Name: input.Name, TriggerType: input.TriggerType, Version: input.Version, Enabled: input.Enabled, AreaID: areaID, TeamID: teamID, RequiredPositionCode: input.RequiredPositionCode, RequiredCapabilities: capabilities, PlannedOffsetSeconds: input.PlannedOffsetSeconds, DefaultMessage: input.DefaultMessage, CreatedByPublicID: nullableString(meta.Principal.PublicID), CreatedAt: now, UpdatedAt: now}
		if err := tx.Create(&row).Error; err != nil {
			return mapManagementWriteError(err)
		}
		if err := appendManagementAudit(tx, meta, "management.template.create", "task_template", row.PublicID, "success"); err != nil {
			return err
		}
		result = row.toTemplate(input.AreaPublicID, input.TeamPublicID)
		return nil
	})
	return result, err
}

func (r *ManagementRepository) UpdateTemplate(ctx context.Context, publicID string, patch coremanagement.TaskTemplatePatch, meta coremanagement.AuditMeta) (coremanagement.TaskTemplate, error) {
	db, err := r.database()
	if err != nil {
		return coremanagement.TaskTemplate{}, err
	}
	now := managementNow(meta)
	var result coremanagement.TaskTemplate
	err = db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var row managementTaskTemplateRow
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("public_id = ?", publicID).First(&row).Error; err != nil {
			return mapManagementReadError(err)
		}
		area, err := findManagementAreaByID(tx, row.AreaID)
		if err != nil {
			return err
		}
		team, err := findManagementTeamByID(tx, row.TeamID)
		if err != nil {
			return err
		}
		areaPublicID, teamPublicID := area.PublicID, team.PublicID
		if patch.AreaPublicID != nil {
			areaPublicID = *patch.AreaPublicID
		}
		if patch.TeamPublicID != nil {
			teamPublicID = *patch.TeamPublicID
		}
		if patch.AreaPublicID != nil || patch.TeamPublicID != nil {
			row.AreaID, row.TeamID, err = resolveTemplateTarget(tx, areaPublicID, teamPublicID)
			if err != nil {
				return err
			}
		}
		updates := map[string]any{"updated_at": now}
		if patch.Name != nil {
			updates["name"] = *patch.Name
		}
		if patch.Enabled != nil {
			updates["enabled"] = *patch.Enabled
		}
		if patch.AreaPublicID != nil || patch.TeamPublicID != nil {
			updates["target_area_id"] = row.AreaID
			updates["target_team_id"] = row.TeamID
		}
		if patch.RequiredPositionCode != nil {
			if err := ensureEnabledDictionaryCode(tx, "job_position", *patch.RequiredPositionCode); err != nil {
				return err
			}
			updates["required_position_code"] = *patch.RequiredPositionCode
		}
		if patch.RequiredCapabilities != nil {
			if len(*patch.RequiredCapabilities) != 1 {
				return coremanagement.ErrInvalidInput
			}
			if err := ensureEnabledDictionaryCode(tx, "capability", (*patch.RequiredCapabilities)[0]); err != nil {
				return err
			}
			capabilities, marshalErr := json.Marshal(*patch.RequiredCapabilities)
			if marshalErr != nil {
				return fmt.Errorf("encode template capabilities: %w", marshalErr)
			}
			updates["required_capabilities"] = capabilities
		}
		if patch.PlannedOffsetSeconds != nil {
			updates["planned_offset_seconds"] = *patch.PlannedOffsetSeconds
		}
		if patch.DefaultMessage != nil {
			updates["default_message"] = *patch.DefaultMessage
		}
		if err := tx.Model(&managementTaskTemplateRow{}).Where("id = ?", row.ID).Updates(updates).Error; err != nil {
			return mapManagementWriteError(err)
		}
		if err := appendManagementAudit(tx, meta, "management.template.update", "task_template", row.PublicID, "success"); err != nil {
			return err
		}
		if err := tx.Where("id = ?", row.ID).First(&row).Error; err != nil {
			return fmt.Errorf("reload task template: %w", err)
		}
		area, err = findManagementAreaByID(tx, row.AreaID)
		if err != nil {
			return err
		}
		team, err = findManagementTeamByID(tx, row.TeamID)
		if err != nil {
			return err
		}
		result = row.toTemplate(area.PublicID, team.PublicID)
		return nil
	})
	return result, err
}

func (r *ManagementRepository) ListFlights(ctx context.Context, operatingDate string, page coremanagement.PageQuery, includeTerminal bool) (coremanagement.FlightListResult, error) {
	db, err := r.database()
	if err != nil {
		return coremanagement.FlightListResult{}, err
	}
	query := db.WithContext(ctx).Model(&flightRow{})
	if operatingDate != "" {
		query = query.Where("operating_date = ?", operatingDate)
	}
	if !includeTerminal {
		query = query.Where("status NOT IN ?", []string{"departed", "cancelled"})
	}
	var total int64
	if err := query.Session(&gorm.Session{}).Count(&total).Error; err != nil {
		return coremanagement.FlightListResult{}, fmt.Errorf("count flights: %w", err)
	}
	var rows []flightRow
	if err := query.Order("scheduled_at ASC, public_id ASC").Offset((page.Page - 1) * page.PageSize).Limit(page.PageSize).Find(&rows).Error; err != nil {
		return coremanagement.FlightListResult{}, fmt.Errorf("list flights: %w", err)
	}
	items := make([]coremanagement.Flight, 0, len(rows))
	for _, row := range rows {
		items = append(items, row.toManagementFlight())
	}
	return coremanagement.FlightListResult{Items: items, Page: page.Page, PageSize: page.PageSize, Total: total}, nil
}

func (r *ManagementRepository) ListAdminIdentities(ctx context.Context, page coremanagement.PageQuery, includeDisabled bool) (coremanagement.AdminIdentityListResult, error) {
	db, err := r.database()
	if err != nil {
		return coremanagement.AdminIdentityListResult{}, err
	}
	query := db.WithContext(ctx).Model(&adminIdentityRow{})
	if !includeDisabled {
		query = query.Where("enabled = ?", true)
	}
	var total int64
	if err := query.Session(&gorm.Session{}).Count(&total).Error; err != nil {
		return coremanagement.AdminIdentityListResult{}, fmt.Errorf("count admin identities: %w", err)
	}
	var rows []adminIdentityRow
	if err := query.Order("display_name ASC, public_id ASC").Offset((page.Page - 1) * page.PageSize).Limit(page.PageSize).Find(&rows).Error; err != nil {
		return coremanagement.AdminIdentityListResult{}, fmt.Errorf("list admin identities: %w", err)
	}
	items := make([]coremanagement.AdminIdentity, 0, len(rows))
	for _, row := range rows {
		item, err := row.toManagementAdminIdentity(db)
		if err != nil {
			return coremanagement.AdminIdentityListResult{}, err
		}
		items = append(items, item)
	}
	return coremanagement.AdminIdentityListResult{Items: items, Page: page.Page, PageSize: page.PageSize, Total: total}, nil
}

func (r *ManagementRepository) CreateAdminIdentity(ctx context.Context, input coremanagement.AdminIdentityWrite, meta coremanagement.AuditMeta) (coremanagement.AdminIdentity, error) {
	db, err := r.database()
	if err != nil {
		return coremanagement.AdminIdentity{}, err
	}
	publicID, err := id.NewPublicID()
	if err != nil {
		return coremanagement.AdminIdentity{}, fmt.Errorf("generate admin identity public id: %w", err)
	}
	var result coremanagement.AdminIdentity
	err = db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		areaIDs, teamIDs, err := resolveIdentityScopes(tx, input.AreaPublicIDs, input.TeamPublicIDs)
		if err != nil {
			return err
		}
		areaJSON, _ := json.Marshal(areaIDs)
		teamJSON, _ := json.Marshal(teamIDs)
		row := adminIdentityRow{PublicID: publicID, Provider: input.Provider, ExternalSub: input.ExternalSubject, DisplayName: input.DisplayName, Role: input.Role, Enabled: input.Enabled, GlobalScope: input.GlobalScope, AreaIDs: areaJSON, TeamIDs: teamJSON, UserID: input.UserID}
		if err := tx.Create(&row).Error; err != nil {
			return mapManagementWriteError(err)
		}
		if err := appendManagementAudit(tx, meta, "management.admin_identity.create", "admin_identity", row.PublicID, "success"); err != nil {
			return err
		}
		result = coremanagement.AdminIdentity{PublicID: row.PublicID, Provider: row.Provider, ExternalSub: row.ExternalSub, DisplayName: row.DisplayName, Role: row.Role, Enabled: row.Enabled, GlobalScope: row.GlobalScope, AreaIDs: areaIDs, TeamIDs: teamIDs, AreaPublicIDs: append([]string(nil), input.AreaPublicIDs...), TeamPublicIDs: append([]string(nil), input.TeamPublicIDs...), UserID: row.UserID}
		return nil
	})
	return result, err
}

func (r *ManagementRepository) UpdateAdminIdentity(ctx context.Context, publicID string, patch coremanagement.AdminIdentityPatch, meta coremanagement.AuditMeta) (coremanagement.AdminIdentity, error) {
	db, err := r.database()
	if err != nil {
		return coremanagement.AdminIdentity{}, err
	}
	now := managementNow(meta)
	var result coremanagement.AdminIdentity
	err = db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var row adminIdentityRow
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("public_id = ?", publicID).First(&row).Error; err != nil {
			return mapManagementReadError(err)
		}
		updates := map[string]any{}
		if patch.DisplayName != nil {
			updates["display_name"] = *patch.DisplayName
		}
		if patch.Role != nil {
			updates["role"] = *patch.Role
		}
		if patch.Enabled != nil {
			updates["enabled"] = *patch.Enabled
		}
		if patch.GlobalScope != nil {
			updates["global_scope"] = *patch.GlobalScope
		}
		if patch.UserID != nil {
			updates["user_id"] = *patch.UserID
		}
		if patch.AreaPublicIDs != nil || patch.TeamPublicIDs != nil {
			var currentAreaIDs, currentTeamIDs []uint64
			currentAreaIDs, err = decodeUint64List(row.AreaIDs)
			if err != nil {
				return fmt.Errorf("decode admin area scope: %w", err)
			}
			currentTeamIDs, err = decodeUint64List(row.TeamIDs)
			if err != nil {
				return fmt.Errorf("decode admin team scope: %w", err)
			}
			areaPublicIDs, teamPublicIDs := []string(nil), []string(nil)
			if patch.AreaPublicIDs != nil {
				areaPublicIDs = *patch.AreaPublicIDs
			}
			if patch.TeamPublicIDs != nil {
				teamPublicIDs = *patch.TeamPublicIDs
			}
			areaIDs, teamIDs, resolveErr := resolveIdentityScopes(tx, areaPublicIDs, teamPublicIDs)
			if resolveErr != nil {
				return resolveErr
			}
			if patch.AreaPublicIDs == nil {
				areaIDs = currentAreaIDs
			}
			if patch.TeamPublicIDs == nil {
				teamIDs = currentTeamIDs
			}
			areaJSON, _ := json.Marshal(areaIDs)
			teamJSON, _ := json.Marshal(teamIDs)
			updates["area_ids"] = areaJSON
			updates["team_ids"] = teamJSON
		}
		updates["updated_at"] = now
		if err := tx.Model(&adminIdentityRow{}).Where("id = ?", row.ID).Updates(updates).Error; err != nil {
			return mapManagementWriteError(err)
		}
		if err := appendManagementAudit(tx, meta, "management.admin_identity.update", "admin_identity", row.PublicID, "success"); err != nil {
			return err
		}
		if err := tx.Where("id = ?", row.ID).First(&row).Error; err != nil {
			return fmt.Errorf("reload admin identity: %w", err)
		}
		result, err = row.toManagementAdminIdentity(tx)
		return err
	})
	return result, err
}

func (r *ManagementRepository) database() (*gorm.DB, error) {
	if r == nil || r.db == nil {
		return nil, coremanagement.ErrRepositoryNotConfigured
	}
	return r.db, nil
}

func appendManagementAudit(tx *gorm.DB, meta coremanagement.AuditMeta, action, resourceType, resourceID, result string) error {
	now := managementNow(meta)
	return tx.Create(&auditRow{ActorType: auditActorType(meta), ActorID: meta.Principal.PublicID, Action: action, ResourceType: resourceType, ResourceID: resourceID, Result: result, RequestID: meta.RequestID, TraceID: meta.TraceID, SourceIP: meta.SourceIP, OccurredAt: now, CreatedAt: now}).Error
}

func auditActorType(meta coremanagement.AuditMeta) string {
	if meta.Principal.Type == security.MachinePrincipal {
		return "machine"
	}
	return "human"
}

func managementNow(meta coremanagement.AuditMeta) time.Time {
	if meta.Now.IsZero() {
		return time.Now().UTC()
	}
	return meta.Now.UTC()
}

func nullableString(value string) *string {
	if strings.TrimSpace(value) == "" {
		return nil
	}
	value = strings.TrimSpace(value)
	return &value
}

func mapManagementReadError(err error) error {
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return coremanagement.ErrNotFound
	}
	return err
}

func mapManagementWriteError(err error) error {
	if errors.Is(err, gorm.ErrDuplicatedKey) {
		return coremanagement.ErrConflict
	}
	return err
}

func findManagementArea(tx *gorm.DB, publicID string, enabledOnly bool) (managementAreaRow, error) {
	var row managementAreaRow
	query := tx.Where("public_id = ?", publicID)
	if enabledOnly {
		query = query.Where("enabled = ?", true)
	}
	if err := query.First(&row).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) && enabledOnly {
			return row, coremanagement.ErrResourceBusy
		}
		return row, mapManagementReadError(err)
	}
	return row, nil
}

func findManagementAreaByID(tx *gorm.DB, areaID uint64) (managementAreaRow, error) {
	var row managementAreaRow
	if err := tx.Where("id = ?", areaID).First(&row).Error; err != nil {
		return row, mapManagementReadError(err)
	}
	return row, nil
}

func findManagementTeam(tx *gorm.DB, publicID string, enabledOnly bool) (managementTeamRow, error) {
	var row managementTeamRow
	query := tx.Where("public_id = ?", publicID)
	if enabledOnly {
		query = query.Where("enabled = ?", true)
	}
	if err := query.First(&row).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) && enabledOnly {
			return row, coremanagement.ErrResourceBusy
		}
		return row, mapManagementReadError(err)
	}
	area, err := findManagementAreaByID(tx, row.AreaID)
	if err != nil {
		return row, err
	}
	if enabledOnly && !area.Enabled {
		return row, coremanagement.ErrResourceBusy
	}
	return row, nil
}

func findManagementTeamByID(tx *gorm.DB, teamID uint64) (managementTeamRow, error) {
	var row managementTeamRow
	if err := tx.Where("id = ?", teamID).First(&row).Error; err != nil {
		return row, mapManagementReadError(err)
	}
	return row, nil
}

func ensureTeamHasNoEnabledPersonnel(tx *gorm.DB, teamID uint64) error {
	var count int64
	if err := tx.Model(&managementPersonnelRow{}).Where("team_id = ? AND enabled = ?", teamID, true).Count(&count).Error; err != nil {
		return fmt.Errorf("check team personnel: %w", err)
	}
	if count > 0 {
		return coremanagement.ErrResourceBusy
	}
	return nil
}

func resolveTemplateTarget(tx *gorm.DB, areaPublicID, teamPublicID string) (uint64, uint64, error) {
	area, err := findManagementArea(tx, areaPublicID, true)
	if err != nil {
		return 0, 0, err
	}
	team, err := findManagementTeam(tx, teamPublicID, true)
	if err != nil {
		return 0, 0, err
	}
	if team.AreaID != area.ID {
		return 0, 0, coremanagement.ErrConflict
	}
	return area.ID, team.ID, nil
}

func resolveIdentityScopes(tx *gorm.DB, areaPublicIDs, teamPublicIDs []string) ([]uint64, []uint64, error) {
	areaIDs := make([]uint64, 0, len(areaPublicIDs))
	for _, publicID := range areaPublicIDs {
		area, err := findManagementArea(tx, publicID, false)
		if err != nil {
			return nil, nil, err
		}
		areaIDs = append(areaIDs, area.ID)
	}
	teamIDs := make([]uint64, 0, len(teamPublicIDs))
	for _, publicID := range teamPublicIDs {
		// Scope JSON stores internal IDs while the HTTP contract exposes public IDs.
		var team managementTeamRow
		if err := tx.Where("public_id = ?", publicID).First(&team).Error; err != nil {
			return nil, nil, mapManagementReadError(err)
		}
		teamIDs = append(teamIDs, team.ID)
	}
	return areaIDs, teamIDs, nil
}

// resolveIdentityScopePublicIDs converts persisted internal scope IDs back to
// public references so an administrator can safely edit an existing scope.
// The JSON columns intentionally keep numeric foreign keys for authorization;
// the management response must not expose those keys as writable public IDs.
func resolveIdentityScopePublicIDs(db *gorm.DB, table string, ids []uint64) ([]string, error) {
	if len(ids) == 0 {
		return []string{}, nil
	}
	type scopeReferenceRow struct {
		ID       uint64 `gorm:"column:id"`
		PublicID string `gorm:"column:public_id"`
	}
	rows := make([]scopeReferenceRow, 0, len(ids))
	if err := db.Table(table).Select("id, public_id").Where("id IN ?", ids).Find(&rows).Error; err != nil {
		return nil, err
	}
	publicIDs := make(map[uint64]string, len(rows))
	for _, row := range rows {
		publicIDs[row.ID] = row.PublicID
	}
	result := make([]string, 0, len(ids))
	for _, id := range ids {
		publicID, ok := publicIDs[id]
		if !ok {
			return nil, fmt.Errorf("scope reference %d was not found in %s", id, table)
		}
		result = append(result, publicID)
	}
	return result, nil
}

func loadPersonnel(tx *gorm.DB, personnelID uint64) (managementPersonnelQueryRow, error) {
	var row managementPersonnelQueryRow
	if err := tx.Table("personnel AS p").Select(`p.id, p.public_id, p.user_public_id, p.employee_no, p.display_name,
		p.team_id, p.position_code, p.capabilities, p.work_state, p.status_version,
		p.last_state_changed_at, p.enabled, EXISTS (SELECT 1 FROM employee_credential ec WHERE ec.personnel_id = p.id) AS has_credential`).
		Where("p.id = ?", personnelID).First(&row).Error; err != nil {
		return row, mapManagementReadError(err)
	}
	return row, nil
}

type managementAreaRow struct {
	ID        uint64    `gorm:"column:id;primaryKey"`
	PublicID  string    `gorm:"column:public_id"`
	Code      string    `gorm:"column:code"`
	Name      string    `gorm:"column:name"`
	Enabled   bool      `gorm:"column:enabled"`
	CreatedAt time.Time `gorm:"column:created_at"`
	UpdatedAt time.Time `gorm:"column:updated_at"`
}

func (managementAreaRow) TableName() string { return "operation_area" }
func (row managementAreaRow) toArea() coremanagement.Area {
	return coremanagement.Area{PublicID: row.PublicID, Code: row.Code, Name: row.Name, Enabled: row.Enabled}
}

type managementTeamRow struct {
	ID        uint64    `gorm:"column:id;primaryKey"`
	PublicID  string    `gorm:"column:public_id"`
	AreaID    uint64    `gorm:"column:area_id"`
	Code      string    `gorm:"column:code"`
	Name      string    `gorm:"column:name"`
	Enabled   bool      `gorm:"column:enabled"`
	CreatedAt time.Time `gorm:"column:created_at"`
	UpdatedAt time.Time `gorm:"column:updated_at"`
}

func (managementTeamRow) TableName() string { return "team" }

type managementTeamQueryRow struct {
	PublicID     string `gorm:"column:public_id"`
	AreaPublicID string `gorm:"column:area_public_id"`
	AreaName     string `gorm:"column:area_name"`
	Code         string `gorm:"column:code"`
	Name         string `gorm:"column:name"`
	Enabled      bool   `gorm:"column:enabled"`
}

func (row managementTeamQueryRow) toTeam() coremanagement.Team {
	return coremanagement.Team{PublicID: row.PublicID, AreaPublicID: row.AreaPublicID, AreaName: row.AreaName, Code: row.Code, Name: row.Name, Enabled: row.Enabled}
}

type managementPersonnelRow struct {
	ID                 uint64    `gorm:"column:id;primaryKey"`
	PublicID           string    `gorm:"column:public_id"`
	UserPublicID       string    `gorm:"column:user_public_id"`
	EmployeeNo         string    `gorm:"column:employee_no"`
	DisplayName        string    `gorm:"column:display_name"`
	TeamID             uint64    `gorm:"column:team_id"`
	PositionCode       string    `gorm:"column:position_code"`
	Capabilities       []byte    `gorm:"column:capabilities"`
	WorkState          string    `gorm:"column:work_state"`
	StatusVersion      uint64    `gorm:"column:status_version"`
	LastStateChangedAt time.Time `gorm:"column:last_state_changed_at"`
	Enabled            bool      `gorm:"column:enabled"`
	CreatedAt          time.Time `gorm:"column:created_at"`
	UpdatedAt          time.Time `gorm:"column:updated_at"`
}

func (managementPersonnelRow) TableName() string { return "personnel" }

type managementPersonnelQueryRow struct {
	managementPersonnelRow
	HasCredential bool `gorm:"column:has_credential"`
}

func (row managementPersonnelRow) toPersonnel(team managementTeamRow) coremanagement.Personnel {
	capabilities, _ := decodeCapabilities(row.Capabilities)
	return coremanagement.Personnel{PublicID: row.PublicID, UserPublicID: row.UserPublicID, EmployeeNo: row.EmployeeNo, DisplayName: row.DisplayName, AreaPublicID: "", TeamPublicID: team.PublicID, PositionCode: row.PositionCode, CapabilityCode: firstCapability(capabilities), Capabilities: capabilities, WorkState: row.WorkState, StatusVersion: row.StatusVersion, Enabled: row.Enabled, LastStateChangedAt: formatTime(row.LastStateChangedAt)}
}

type managementTeamMemberRow struct {
	ID          uint64     `gorm:"column:id;primaryKey"`
	PublicID    string     `gorm:"column:public_id"`
	TeamID      uint64     `gorm:"column:team_id"`
	PersonnelID uint64     `gorm:"column:personnel_id"`
	IsPrimary   bool       `gorm:"column:is_primary"`
	JoinedAt    time.Time  `gorm:"column:joined_at"`
	LeftAt      *time.Time `gorm:"column:left_at"`
	CreatedAt   time.Time  `gorm:"column:created_at"`
}

func (managementTeamMemberRow) TableName() string { return "team_member" }

type managementEmployeeCredentialRow struct {
	ID                uint64    `gorm:"column:id;primaryKey"`
	PublicID          string    `gorm:"column:public_id"`
	PersonnelID       uint64    `gorm:"column:personnel_id"`
	PasswordHash      string    `gorm:"column:password_hash"`
	PasswordChangedAt time.Time `gorm:"column:password_changed_at"`
	CreatedAt         time.Time `gorm:"column:created_at"`
	UpdatedAt         time.Time `gorm:"column:updated_at"`
}

func (managementEmployeeCredentialRow) TableName() string { return "employee_credential" }

type managementTaskTemplateRow struct {
	ID                   uint64    `gorm:"column:id;primaryKey"`
	PublicID             string    `gorm:"column:public_id"`
	Name                 string    `gorm:"column:name"`
	TriggerType          string    `gorm:"column:trigger_type"`
	Version              uint      `gorm:"column:template_version"`
	Enabled              bool      `gorm:"column:enabled"`
	AreaID               uint64    `gorm:"column:target_area_id"`
	TeamID               uint64    `gorm:"column:target_team_id"`
	RequiredPositionCode string    `gorm:"column:required_position_code"`
	RequiredCapabilities []byte    `gorm:"column:required_capabilities"`
	PlannedOffsetSeconds int       `gorm:"column:planned_offset_seconds"`
	DefaultMessage       string    `gorm:"column:default_message"`
	CreatedByPublicID    *string   `gorm:"column:created_by_public_id"`
	CreatedAt            time.Time `gorm:"column:created_at"`
	UpdatedAt            time.Time `gorm:"column:updated_at"`
}

func (managementTaskTemplateRow) TableName() string { return "task_template" }

func (row managementTaskTemplateRow) toTemplate(areaPublicID, teamPublicID string) coremanagement.TaskTemplate {
	capabilities, _ := decodeCapabilities(row.RequiredCapabilities)
	return coremanagement.TaskTemplate{PublicID: row.PublicID, Name: row.Name, TriggerType: row.TriggerType, Version: row.Version, Enabled: row.Enabled, AreaPublicID: areaPublicID, TeamPublicID: teamPublicID, RequiredPositionCode: row.RequiredPositionCode, RequiredCapabilityCode: firstCapability(capabilities), RequiredCapabilities: capabilities, PlannedOffsetSeconds: row.PlannedOffsetSeconds, DefaultMessage: row.DefaultMessage}
}

type managementTemplateQueryRow struct {
	PublicID             string `gorm:"column:public_id"`
	Name                 string `gorm:"column:name"`
	TriggerType          string `gorm:"column:trigger_type"`
	Version              uint   `gorm:"column:template_version"`
	Enabled              bool   `gorm:"column:enabled"`
	AreaPublicID         string `gorm:"column:area_public_id"`
	TeamPublicID         string `gorm:"column:team_public_id"`
	RequiredPositionCode string `gorm:"column:required_position_code"`
	RequiredCapabilities []byte `gorm:"column:required_capabilities"`
	PlannedOffsetSeconds int    `gorm:"column:planned_offset_seconds"`
	DefaultMessage       string `gorm:"column:default_message"`
}

func (row managementTemplateQueryRow) toTemplate() (coremanagement.TaskTemplate, error) {
	capabilities, err := decodeCapabilities(row.RequiredCapabilities)
	if err != nil {
		return coremanagement.TaskTemplate{}, fmt.Errorf("decode template %s capabilities: %w", row.PublicID, err)
	}
	return coremanagement.TaskTemplate{PublicID: row.PublicID, Name: row.Name, TriggerType: row.TriggerType, Version: row.Version, Enabled: row.Enabled, AreaPublicID: row.AreaPublicID, TeamPublicID: row.TeamPublicID, RequiredPositionCode: row.RequiredPositionCode, RequiredCapabilityCode: firstCapability(capabilities), RequiredCapabilities: capabilities, PlannedOffsetSeconds: row.PlannedOffsetSeconds, DefaultMessage: row.DefaultMessage}, nil
}

func (row flightRow) toManagementFlight() coremanagement.Flight {
	return coremanagement.Flight{PublicID: row.PublicID, DisplayNo: row.DisplayNo, SourceProvider: row.SourceProvider, ExternalFlightID: row.ExternalFlightID, OperatingDate: row.OperatingDate.Format("2006-01-02"), ScheduledAt: formatTime(row.ScheduledAt), SourceLastSyncedAt: formatOptionalTime(row.SourceLastSyncedAt), SourceState: row.SourceState, SourceLastAttemptAt: formatOptionalTime(row.SourceLastAttemptAt), SourceLastError: row.SourceLastError, ActualArrivalAt: formatOptionalTime(row.ActualArrivalAt), Status: row.Status, StatusVersion: row.StatusVersion, LastStatusChangedAt: formatTime(row.LastStatusChangedAt)}
}

func (row adminIdentityRow) toManagementAdminIdentity(db *gorm.DB) (coremanagement.AdminIdentity, error) {
	areaIDs, err := decodeUint64List(row.AreaIDs)
	if err != nil {
		return coremanagement.AdminIdentity{}, fmt.Errorf("decode admin area scope: %w", err)
	}
	teamIDs, err := decodeUint64List(row.TeamIDs)
	if err != nil {
		return coremanagement.AdminIdentity{}, fmt.Errorf("decode admin team scope: %w", err)
	}
	areaPublicIDs, err := resolveIdentityScopePublicIDs(db, "operation_area", areaIDs)
	if err != nil {
		return coremanagement.AdminIdentity{}, fmt.Errorf("resolve admin area scope public ids: %w", err)
	}
	teamPublicIDs, err := resolveIdentityScopePublicIDs(db, "team", teamIDs)
	if err != nil {
		return coremanagement.AdminIdentity{}, fmt.Errorf("resolve admin team scope public ids: %w", err)
	}
	return coremanagement.AdminIdentity{PublicID: row.PublicID, Provider: row.Provider, ExternalSub: row.ExternalSub, DisplayName: row.DisplayName, Role: row.Role, Enabled: row.Enabled, GlobalScope: row.GlobalScope, AreaIDs: areaIDs, TeamIDs: teamIDs, AreaPublicIDs: areaPublicIDs, TeamPublicIDs: teamPublicIDs, UserID: row.UserID}, nil
}
