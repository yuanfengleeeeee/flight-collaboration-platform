package mysql

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	coreidentity "github.com/yuanfengleeeeee/flight-collaboration-platform/internal/core/application/identity"
	"github.com/yuanfengleeeeee/flight-collaboration-platform/internal/shared/id"
	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

var _ coreidentity.Repository = (*IdentityRepository)(nil)

// IdentityRepository is deliberately separate from the flight/task
// repository. It owns only credential and identity-binding persistence in
// Core, while Edge receives the result through the internal Identity API.
type IdentityRepository struct {
	db *gorm.DB
}

func NewIdentityRepository(db *gorm.DB) *IdentityRepository {
	return &IdentityRepository{db: db}
}

func (r *IdentityRepository) FindStaff(ctx context.Context, staffPublicID string) (coreidentity.Staff, error) {
	if r == nil || r.db == nil {
		return coreidentity.Staff{}, fmt.Errorf("identity repository is not configured")
	}
	var row personnelIdentityRow
	if err := r.db.WithContext(ctx).Table("personnel").Where("public_id = ?", strings.TrimSpace(staffPublicID)).First(&row).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return coreidentity.Staff{}, coreidentity.ErrNotFound
		}
		return coreidentity.Staff{}, fmt.Errorf("find staff identity: %w", err)
	}
	if !row.Enabled {
		return coreidentity.Staff{}, coreidentity.ErrStaffInactive
	}
	return coreidentity.Staff{PublicID: row.PublicID, EmployeeNo: row.EmployeeNo, DisplayName: row.DisplayName, Role: "staff"}, nil
}

func (r *IdentityRepository) VerifyPassword(ctx context.Context, employeeNo, password string, now time.Time, maxAttempts int, lockout time.Duration) (coreidentity.Staff, error) {
	if r == nil || r.db == nil {
		return coreidentity.Staff{}, fmt.Errorf("identity repository is not configured")
	}
	if now.IsZero() {
		now = time.Now().UTC()
	}
	if maxAttempts <= 0 {
		maxAttempts = coreidentity.DefaultMaxLoginAttempts
	}
	if lockout <= 0 {
		lockout = coreidentity.DefaultLockoutDuration
	}

	var staff coreidentity.Staff
	var resultErr error
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var row employeeCredentialLoginRow
		err := tx.Table("employee_credential AS ec").
			Select("ec.personnel_id, ec.password_hash, ec.failed_attempts, ec.locked_until, p.public_id AS personnel_public_id, p.employee_no, p.display_name, p.enabled").
			Joins("JOIN personnel AS p ON p.id = ec.personnel_id").
			Where("p.employee_no = ?", strings.TrimSpace(employeeNo)).
			Clauses(clause.Locking{Strength: "UPDATE"}).
			First(&row).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			resultErr = coreidentity.ErrInvalidCredentials
			return nil
		}
		if err != nil {
			return fmt.Errorf("find employee credential: %w", err)
		}
		if !row.Enabled {
			resultErr = coreidentity.ErrStaffInactive
			return nil
		}
		if row.LockedUntil != nil && row.LockedUntil.After(now) {
			resultErr = coreidentity.ErrInvalidCredentials
			return nil
		}
		if bcrypt.CompareHashAndPassword([]byte(row.PasswordHash), []byte(password)) != nil {
			failedAttempts := row.FailedAttempts + 1
			updates := map[string]any{"failed_attempts": failedAttempts, "updated_at": now}
			if failedAttempts >= maxAttempts {
				lockedUntil := now.Add(lockout)
				updates["locked_until"] = lockedUntil
			}
			if err := tx.Table("employee_credential").Where("personnel_id = ?", row.PersonnelID).Updates(updates).Error; err != nil {
				return fmt.Errorf("record failed employee login: %w", err)
			}
			resultErr = coreidentity.ErrInvalidCredentials
			return nil
		}
		if err := tx.Table("employee_credential").Where("personnel_id = ?", row.PersonnelID).Updates(map[string]any{
			"failed_attempts": 0,
			"locked_until":    nil,
			"updated_at":      now,
		}).Error; err != nil {
			return fmt.Errorf("reset employee login failures: %w", err)
		}
		staff = coreidentity.Staff{PublicID: row.PersonnelPublicID, EmployeeNo: row.EmployeeNo, DisplayName: row.DisplayName, Role: "staff"}
		return nil
	})
	if err != nil {
		return coreidentity.Staff{}, err
	}
	if resultErr != nil {
		return coreidentity.Staff{}, resultErr
	}
	return staff, nil
}

func (r *IdentityRepository) FindBindingForStaff(ctx context.Context, staffPublicID, provider, providerApp string) (coreidentity.Staff, error) {
	if r == nil || r.db == nil {
		return coreidentity.Staff{}, fmt.Errorf("identity repository is not configured")
	}
	var row externalIdentityRow
	err := r.db.WithContext(ctx).Table("external_identity_binding AS b").
		Select("b.personnel_id, p.public_id AS personnel_public_id, p.employee_no, p.display_name, p.enabled").
		Joins("JOIN personnel AS p ON p.id = b.personnel_id").
		Where("b.personnel_id = (SELECT id FROM personnel WHERE public_id = ?) AND b.provider = ? AND b.provider_app = ?", staffPublicID, provider, providerApp).
		First(&row).Error
	return identityStaffFromRow(row, err)
}

func (r *IdentityRepository) FindBindingByExternal(ctx context.Context, external coreidentity.ExternalIdentity) (coreidentity.Staff, error) {
	if r == nil || r.db == nil {
		return coreidentity.Staff{}, fmt.Errorf("identity repository is not configured")
	}
	var row externalIdentityRow
	err := r.db.WithContext(ctx).Table("external_identity_binding AS b").
		Select("b.personnel_id, p.public_id AS personnel_public_id, p.employee_no, p.display_name, p.enabled").
		Joins("JOIN personnel AS p ON p.id = b.personnel_id").
		Where("b.provider = ? AND b.provider_app = ? AND b.external_subject = ?", external.Provider, external.ProviderApp, external.ExternalSubject).
		First(&row).Error
	staff, err := identityStaffFromRow(row, err)
	if err == nil && !row.Enabled {
		return coreidentity.Staff{}, coreidentity.ErrStaffInactive
	}
	return staff, err
}

func (r *IdentityRepository) IssueBindingTicket(ctx context.Context, staffPublicID, client string, now time.Time, ttl time.Duration) (string, error) {
	if r == nil || r.db == nil {
		return "", fmt.Errorf("identity repository is not configured")
	}
	if now.IsZero() {
		now = time.Now().UTC()
	}
	if ttl <= 0 {
		ttl = coreidentity.DefaultBindingTicketTTL
	}
	var personnelID uint64
	if err := r.db.WithContext(ctx).Table("personnel").Select("id").Where("public_id = ? AND enabled = ?", staffPublicID, true).Scan(&personnelID).Error; err != nil {
		return "", fmt.Errorf("find staff for binding ticket: %w", err)
	}
	if personnelID == 0 {
		return "", coreidentity.ErrStaffInactive
	}
	ticket, err := id.NewPublicID()
	if err != nil {
		return "", err
	}
	publicID, err := id.NewPublicID()
	if err != nil {
		return "", err
	}
	row := identityBindingTicketRow{PublicID: publicID, TicketHash: coreidentity.HashBindingTicket(ticket), PersonnelID: personnelID, Client: client, ExpiresAt: now.Add(ttl), CreatedAt: now}
	if err := r.db.WithContext(ctx).Create(&row).Error; err != nil {
		return "", fmt.Errorf("create identity binding ticket: %w", err)
	}
	return ticket, nil
}

func (r *IdentityRepository) CompleteBinding(ctx context.Context, ticket, client string, external coreidentity.ExternalIdentity, now time.Time) (coreidentity.Staff, error) {
	if r == nil || r.db == nil {
		return coreidentity.Staff{}, fmt.Errorf("identity repository is not configured")
	}
	if now.IsZero() {
		now = time.Now().UTC()
	}
	var staff coreidentity.Staff
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var ticketRow identityBindingTicketRow
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("ticket_hash = ?", coreidentity.HashBindingTicket(ticket)).First(&ticketRow).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return coreidentity.ErrBindingTicketInvalid
			}
			return fmt.Errorf("find identity binding ticket: %w", err)
		}
		if ticketRow.UsedAt != nil || !ticketRow.ExpiresAt.After(now) || ticketRow.Client != client {
			return coreidentity.ErrBindingTicketInvalid
		}
		var personnel personnelIdentityRow
		if err := tx.Table("personnel").Where("id = ?", ticketRow.PersonnelID).First(&personnel).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return coreidentity.ErrStaffInactive
			}
			return fmt.Errorf("find staff for identity binding: %w", err)
		}
		if !personnel.Enabled {
			return coreidentity.ErrStaffInactive
		}
		var existing externalIdentityBindingRow
		findErr := tx.Where("provider = ? AND provider_app = ? AND external_subject = ?", external.Provider, external.ProviderApp, external.ExternalSubject).First(&existing).Error
		if findErr == nil && existing.PersonnelID != ticketRow.PersonnelID {
			return coreidentity.ErrIdentityBindingConflict
		}
		if findErr != nil && !errors.Is(findErr, gorm.ErrRecordNotFound) {
			return fmt.Errorf("find existing external identity: %w", findErr)
		}
		var providerBinding externalIdentityBindingRow
		providerErr := tx.Where("personnel_id = ? AND provider = ? AND provider_app = ?", ticketRow.PersonnelID, external.Provider, external.ProviderApp).First(&providerBinding).Error
		if providerErr == nil && providerBinding.ExternalSubject != external.ExternalSubject {
			return coreidentity.ErrIdentityBindingConflict
		}
		if providerErr != nil && !errors.Is(providerErr, gorm.ErrRecordNotFound) {
			return fmt.Errorf("find staff provider binding: %w", providerErr)
		}
		if findErr != nil && errors.Is(findErr, gorm.ErrRecordNotFound) {
			publicID, err := id.NewPublicID()
			if err != nil {
				return err
			}
			binding := externalIdentityBindingRow{PublicID: publicID, PersonnelID: ticketRow.PersonnelID, Provider: external.Provider, ProviderApp: external.ProviderApp, ExternalSubject: external.ExternalSubject, CreatedAt: now, UpdatedAt: now}
			if err := tx.Create(&binding).Error; err != nil {
				return mapWriteError(err)
			}
		}
		if err := tx.Model(&identityBindingTicketRow{}).Where("id = ? AND used_at IS NULL", ticketRow.ID).Update("used_at", now).Error; err != nil {
			return fmt.Errorf("consume identity binding ticket: %w", err)
		}
		if err := tx.Create(&auditRow{ActorType: "human", ActorID: personnel.PublicID, Action: "identity.bind", ResourceType: "staff", ResourceID: personnel.PublicID, Result: "success", OccurredAt: now, CreatedAt: now}).Error; err != nil {
			return fmt.Errorf("audit identity binding: %w", err)
		}
		staff = coreidentity.Staff{PublicID: personnel.PublicID, EmployeeNo: personnel.EmployeeNo, DisplayName: personnel.DisplayName, Role: "staff"}
		return nil
	})
	if err != nil {
		return coreidentity.Staff{}, err
	}
	return staff, nil
}

// UpsertEmployeePassword is intended for controlled provisioning and test
// fixtures. It accepts a plaintext password only long enough to hash it.
func (r *IdentityRepository) UpsertEmployeePassword(ctx context.Context, staffPublicID, password string) error {
	if r == nil || r.db == nil {
		return fmt.Errorf("identity repository is not configured")
	}
	if strings.TrimSpace(staffPublicID) == "" || password == "" {
		return coreidentity.ErrInvalidCredentials
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return fmt.Errorf("hash employee password: %w", err)
	}
	var personnelID uint64
	if err := r.db.WithContext(ctx).Table("personnel").Select("id").Where("public_id = ?", staffPublicID).Scan(&personnelID).Error; err != nil {
		return fmt.Errorf("find staff for password: %w", err)
	}
	if personnelID == 0 {
		return coreidentity.ErrNotFound
	}
	publicID, err := id.NewPublicID()
	if err != nil {
		return fmt.Errorf("generate employee credential id: %w", err)
	}
	now := time.Now().UTC()
	row := employeeCredentialRow{PublicID: publicID, PersonnelID: personnelID, PasswordHash: string(hash), PasswordChangedAt: now, CreatedAt: now, UpdatedAt: now}
	return r.db.WithContext(ctx).Where("personnel_id = ?", personnelID).Assign(row).FirstOrCreate(&row).Error
}

type employeeCredentialLoginRow struct {
	PersonnelID       uint64     `gorm:"column:personnel_id"`
	PasswordHash      string     `gorm:"column:password_hash"`
	FailedAttempts    int        `gorm:"column:failed_attempts"`
	LockedUntil       *time.Time `gorm:"column:locked_until"`
	PersonnelPublicID string     `gorm:"column:personnel_public_id"`
	EmployeeNo        string     `gorm:"column:employee_no"`
	DisplayName       string     `gorm:"column:display_name"`
	Enabled           bool       `gorm:"column:enabled"`
}

type employeeCredentialRow struct {
	ID                uint64    `gorm:"column:id;primaryKey;autoIncrement"`
	PublicID          string    `gorm:"column:public_id"`
	PersonnelID       uint64    `gorm:"column:personnel_id"`
	PasswordHash      string    `gorm:"column:password_hash"`
	PasswordChangedAt time.Time `gorm:"column:password_changed_at"`
	CreatedAt         time.Time `gorm:"column:created_at"`
	UpdatedAt         time.Time `gorm:"column:updated_at"`
}

func (employeeCredentialRow) TableName() string { return "employee_credential" }

type externalIdentityRow struct {
	PersonnelID       uint64 `gorm:"column:personnel_id"`
	PersonnelPublicID string `gorm:"column:personnel_public_id"`
	EmployeeNo        string `gorm:"column:employee_no"`
	DisplayName       string `gorm:"column:display_name"`
	Enabled           bool   `gorm:"column:enabled"`
}

type personnelIdentityRow struct {
	ID          uint64 `gorm:"column:id;primaryKey"`
	PublicID    string `gorm:"column:public_id"`
	EmployeeNo  string `gorm:"column:employee_no"`
	DisplayName string `gorm:"column:display_name"`
	Enabled     bool   `gorm:"column:enabled"`
}

type externalIdentityBindingRow struct {
	ID              uint64    `gorm:"column:id;primaryKey;autoIncrement"`
	PublicID        string    `gorm:"column:public_id"`
	PersonnelID     uint64    `gorm:"column:personnel_id"`
	Provider        string    `gorm:"column:provider"`
	ProviderApp     string    `gorm:"column:provider_app"`
	ExternalSubject string    `gorm:"column:external_subject"`
	CreatedAt       time.Time `gorm:"column:created_at"`
	UpdatedAt       time.Time `gorm:"column:updated_at"`
}

func (externalIdentityBindingRow) TableName() string { return "external_identity_binding" }

type identityBindingTicketRow struct {
	ID          uint64     `gorm:"column:id;primaryKey;autoIncrement"`
	PublicID    string     `gorm:"column:public_id"`
	TicketHash  string     `gorm:"column:ticket_hash"`
	PersonnelID uint64     `gorm:"column:personnel_id"`
	Client      string     `gorm:"column:client"`
	ExpiresAt   time.Time  `gorm:"column:expires_at"`
	UsedAt      *time.Time `gorm:"column:used_at"`
	CreatedAt   time.Time  `gorm:"column:created_at"`
}

func (identityBindingTicketRow) TableName() string { return "identity_binding_ticket" }

func identityStaffFromRow(row externalIdentityRow, err error) (coreidentity.Staff, error) {
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return coreidentity.Staff{}, coreidentity.ErrNotFound
	}
	if err != nil {
		return coreidentity.Staff{}, fmt.Errorf("find identity binding: %w", err)
	}
	if !row.Enabled {
		return coreidentity.Staff{}, coreidentity.ErrStaffInactive
	}
	return coreidentity.Staff{PublicID: row.PersonnelPublicID, EmployeeNo: row.EmployeeNo, DisplayName: row.DisplayName, Role: "staff"}, nil
}
