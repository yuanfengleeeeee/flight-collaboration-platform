package application

import (
	"context"
	"fmt"
	"time"

	platformmysql "github.com/yuanfengleeeeee/flight-collaboration-platform/internal/platform/mysql"
	"go.uber.org/zap"
	"gorm.io/gorm"
)

// Worker is the Core-owned synchronization lifecycle. Message stores and
// transport are connected in the sync foundation step; this skeleton only
// proves the process is independently cancellable and owns Core DB resources.
type Worker struct {
	db       *gorm.DB
	log      *zap.Logger
	interval time.Duration
}

func NewWorker(db *gorm.DB, log *zap.Logger, interval time.Duration) *Worker {
	if log == nil {
		log = zap.NewNop()
	}
	if interval <= 0 {
		interval = time.Second
	}
	return &Worker{db: db, log: log, interval: interval}
}

func (w *Worker) Run(ctx context.Context) error {
	if ctx == nil {
		return fmt.Errorf("worker context is nil")
	}
	ticker := time.NewTicker(w.interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
			if w.db == nil {
				w.log.Warn("sync worker tick: core mysql unavailable")
				continue
			}
			if err := platformmysql.Ping(ctx, w.db); err != nil {
				w.log.Warn("sync worker tick: core mysql unavailable", zap.Error(err))
				continue
			}
			w.log.Debug("sync worker tick")
		}
	}
}

func (w *Worker) Shutdown() error { return platformmysql.Close(w.db) }
