package sync

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"fmt"
	"strings"
	"time"

	edgerealtime "github.com/yuanfengleeeeee/flight-collaboration-platform/internal/edge/application/realtime"
	platformsecurity "github.com/yuanfengleeeeee/flight-collaboration-platform/internal/platform/security"
	"gorm.io/gorm"
)

// SQLRealtimeTicketStore stores only a hash of the short-lived browser ticket.
// This makes a database read or backup insufficient to impersonate an employee
// and lets any Edge replica consume a ticket issued by another replica.
type SQLRealtimeTicketStore struct {
	db  *gorm.DB
	ttl time.Duration
}

func NewSQLRealtimeTicketStore(db *gorm.DB, ttl time.Duration) *SQLRealtimeTicketStore {
	if ttl <= 0 {
		ttl = edgerealtime.DefaultTicketTTL
	}
	return &SQLRealtimeTicketStore{db: db, ttl: ttl}
}

func (store *SQLRealtimeTicketStore) Issue(principal platformsecurity.Principal) (edgerealtime.TicketResponse, error) {
	if store == nil || store.db == nil {
		return edgerealtime.TicketResponse{}, edgerealtime.ErrRealtimeUnavailable
	}
	if principal.Type != platformsecurity.HumanPrincipal || strings.TrimSpace(principal.PublicID) == "" {
		return edgerealtime.TicketResponse{}, edgerealtime.ErrInvalidPrincipal
	}
	ticket, err := newRealtimeTicket()
	if err != nil {
		return edgerealtime.TicketResponse{}, fmt.Errorf("generate realtime ticket: %w", err)
	}
	now := time.Now().UTC()
	expiresAt := now.Add(store.ttl)
	row := realtimeTicketRow{TicketHash: hashRealtimeTicket(ticket), EmployeePublicID: strings.TrimSpace(principal.PublicID), ExpiresAt: expiresAt, CreatedAt: now}
	if err := store.db.WithContext(context.Background()).Create(&row).Error; err != nil {
		return edgerealtime.TicketResponse{}, fmt.Errorf("store realtime ticket: %w", err)
	}
	return edgerealtime.TicketResponse{Ticket: ticket, Protocol: edgerealtime.Protocol, TicketProtocol: edgerealtime.TicketProtocol(ticket), ExpiresAt: expiresAt}, nil
}

func (store *SQLRealtimeTicketStore) Consume(ticketID string) (platformsecurity.Principal, error) {
	if store == nil || store.db == nil || strings.TrimSpace(ticketID) == "" {
		return platformsecurity.Principal{}, edgerealtime.ErrInvalidTicket
	}
	now := time.Now().UTC()
	hash := hashRealtimeTicket(ticketID)
	var employeePublicID string
	err := store.db.WithContext(context.Background()).Transaction(func(tx *gorm.DB) error {
		var row realtimeTicketRow
		if err := tx.Where("ticket_hash = ?", hash).First(&row).Error; err != nil {
			return err
		}
		if row.ConsumedAt != nil || !row.ExpiresAt.After(now) {
			return edgerealtime.ErrInvalidTicket
		}
		result := tx.Model(&realtimeTicketRow{}).
			Where("ticket_hash = ? AND consumed_at IS NULL AND expires_at > ?", hash, now).
			Updates(map[string]any{"consumed_at": now})
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected != 1 {
			return edgerealtime.ErrInvalidTicket
		}
		employeePublicID = row.EmployeePublicID
		return nil
	})
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) || errors.Is(err, edgerealtime.ErrInvalidTicket) {
			return platformsecurity.Principal{}, edgerealtime.ErrInvalidTicket
		}
		return platformsecurity.Principal{}, fmt.Errorf("consume realtime ticket: %w", err)
	}
	return platformsecurity.Principal{Type: platformsecurity.HumanPrincipal, PublicID: employeePublicID}, nil
}

func newRealtimeTicket() (string, error) {
	var value [24]byte
	if _, err := rand.Read(value[:]); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(value[:]), nil
}

func hashRealtimeTicket(ticket string) []byte {
	hash := sha256.Sum256([]byte(ticket))
	return hash[:]
}

type realtimeTicketRow struct {
	TicketHash       []byte     `gorm:"column:ticket_hash;primaryKey"`
	EmployeePublicID string     `gorm:"column:employee_public_id"`
	ExpiresAt        time.Time  `gorm:"column:expires_at"`
	ConsumedAt       *time.Time `gorm:"column:consumed_at"`
	CreatedAt        time.Time  `gorm:"column:created_at"`
}

func (realtimeTicketRow) TableName() string { return "realtime_connection_ticket" }
