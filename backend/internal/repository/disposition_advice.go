package repository

import (
	"context"
	"strings"
	"sync"
	"time"

	"github.com/blueship581/railway-bridge-defect-priority/backend/internal/dto"
	"github.com/blueship581/railway-bridge-defect-priority/backend/internal/model"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// LockedSources exposes the row-locked defect, bridge and inspection records
// read inside the advice-generation transaction.
type LockedSources struct {
	Defect     model.DefectFinding
	Bridge     *model.BridgeAsset
	Inspection *model.InspectionRound
}

// DispositionAdviceRepository owns all persistence operations for 缺陷处置优先级建议.
type DispositionAdviceRepository interface {
	List(context.Context, dto.PageQuery) (Page[model.DispositionAdvice], error)
	Get(context.Context, uint) (model.DispositionAdvice, error)
	GetByDefectID(context.Context, uint) (model.DispositionAdvice, error)
	Delete(context.Context, uint, func(*gorm.DB, model.DispositionAdvice) error) error
	// GenerateInTx runs resolve against row-locked source records. When an advice
	// already exists for the defect it is returned with existing=true and resolve
	// is never invoked, so repeat verification stays idempotent. audit is appended
	// inside the same transaction on creation, so a failed audit leaves no advice.
	GenerateInTx(context.Context, uint,
		func(LockedSources) (model.DispositionAdvice, model.DispositionSourceSnapshot, error),
		func(*gorm.DB, model.DispositionAdvice) error,
	) (model.DispositionAdvice, bool, error)
	CountByStatus(context.Context) (map[string]int64, error)
	CountByDispositionLevel(context.Context) (map[string]int64, error)
}

type dispositionAdviceRepository struct {
	db         *gorm.DB
	store      *Store[model.DispositionAdvice]
	generateMu sync.Mutex
}

func NewDispositionAdviceRepository(db *gorm.DB) DispositionAdviceRepository {
	return &dispositionAdviceRepository{db: db, store: NewStore[model.DispositionAdvice](db)}
}

func (r *dispositionAdviceRepository) List(ctx context.Context, q dto.PageQuery) (Page[model.DispositionAdvice], error) {
	page, pageSize := normalizePage(q.Page, q.PageSize)
	db := r.db.WithContext(ctx).Model(&model.DispositionAdvice{})
	if search := strings.TrimSpace(strings.ToLower(q.Search)); search != "" {
		wildcard := "%" + search + "%"
		db = db.Where("LOWER(code) LIKE ? OR LOWER(defect_code) LIKE ? OR LOWER(bridge_code) LIKE ?", wildcard, wildcard, wildcard)
	}
	if status := strings.TrimSpace(q.Status); status != "" {
		switch status {
		case "observe", "restrict", "urgent":
			db = db.Where("disposition_level = ?", status)
		default:
			db = db.Where("status = ?", status)
		}
	}
	var total int64
	if err := db.Count(&total).Error; err != nil {
		return Page[model.DispositionAdvice]{}, err
	}
	items := make([]model.DispositionAdvice, 0)
	err := db.Preload("Snapshot").Order("updated_at DESC, id DESC").
		Offset((page - 1) * pageSize).Limit(pageSize).Find(&items).Error
	return Page[model.DispositionAdvice]{Items: items, Total: total, Page: page, PageSize: pageSize}, err
}

func (r *dispositionAdviceRepository) Get(ctx context.Context, id uint) (model.DispositionAdvice, error) {
	var item model.DispositionAdvice
	err := r.db.WithContext(ctx).Preload("Snapshot").First(&item, id).Error
	return item, err
}

func (r *dispositionAdviceRepository) GetByDefectID(ctx context.Context, defectID uint) (model.DispositionAdvice, error) {
	var item model.DispositionAdvice
	err := r.db.WithContext(ctx).Preload("Snapshot").Where("defect_id = ?", defectID).First(&item).Error
	return item, err
}

func (r *dispositionAdviceRepository) GenerateInTx(ctx context.Context, defectID uint,
	resolve func(LockedSources) (model.DispositionAdvice, model.DispositionSourceSnapshot, error),
	appendAudit func(*gorm.DB, model.DispositionAdvice) error,
) (model.DispositionAdvice, bool, error) {
	var result model.DispositionAdvice
	var existed bool
	// Databases without row locking (SQLite) can let two transactions pass the
	// existence check together; the unique index on defect_id then rejects one.
	// Re-reading after that conflict still returns the winner's advice, keeping
	// repeat verification idempotent on every driver.
	for attempt := 0; attempt < 3; attempt++ {
		var err error
		result, existed, err = r.attemptGenerateInTx(ctx, defectID, resolve, appendAudit)
		if err != nil {
			if isRetriableWriteConflict(err) && attempt < 2 {
				time.Sleep(time.Duration(attempt+1) * 5 * time.Millisecond)
				continue
			}
			return model.DispositionAdvice{}, false, err
		}
		if existed {
			return result, true, nil
		}
		advice, getErr := r.Get(ctx, result.ID)
		if getErr != nil {
			return model.DispositionAdvice{}, false, getErr
		}
		return advice, false, nil
	}
	return model.DispositionAdvice{}, false, gorm.ErrInvalidTransaction
}

func isRetriableWriteConflict(err error) bool {
	message := strings.ToLower(err.Error())
	return strings.Contains(message, "unique constraint") ||
		strings.Contains(message, "duplicate key") ||
		strings.Contains(message, "duplicate entry") ||
		strings.Contains(message, "database is locked") ||
		strings.Contains(message, "deadlocked")
}

func (r *dispositionAdviceRepository) attemptGenerateInTx(ctx context.Context, defectID uint,
	resolve func(LockedSources) (model.DispositionAdvice, model.DispositionSourceSnapshot, error),
	appendAudit func(*gorm.DB, model.DispositionAdvice) error,
) (model.DispositionAdvice, bool, error) {
	var result model.DispositionAdvice
	var existed bool
	// SQLite cannot upgrade a read lock into a write lock concurrently, so the
	// development driver serializes the generation transaction in-process.
	// PostgreSQL/MySQL rely on SELECT ... FOR UPDATE inside the transaction.
	if r.db.Dialector.Name() == "sqlite" {
		r.generateMu.Lock()
		defer r.generateMu.Unlock()
	}
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		locked := tx
		if tx.Dialector.Name() == "postgres" || tx.Dialector.Name() == "mysql" {
			locked = tx.Clauses(clause.Locking{Strength: "UPDATE"})
		}

		var defect model.DefectFinding
		if err := locked.First(&defect, defectID).Error; err != nil {
			return err
		}

		var existing model.DispositionAdvice
		switch err := tx.Where("defect_id = ?", defectID).First(&existing).Error; {
		case err == nil:
			if err := tx.Where("disposition_advice_id = ?", existing.ID).First(&existing.Snapshot).Error; err != nil {
				return err
			}
			result = existing
			existed = true
			return nil
		case err != gorm.ErrRecordNotFound:
			return err
		}

		sources := LockedSources{Defect: defect}

		var bridge model.BridgeAsset
		if err := locked.Where("code = ?", defect.RelatedCode).First(&bridge).Error; err == nil {
			sources.Bridge = &bridge
		} else if err != gorm.ErrRecordNotFound {
			return err
		}

		if strings.TrimSpace(defect.Facility) != "" {
			var inspection model.InspectionRound
			if err := locked.Where("facility = ?", defect.Facility).
				Order("effective_at DESC, updated_at DESC, id DESC").First(&inspection).Error; err == nil {
				sources.Inspection = &inspection
			} else if err != gorm.ErrRecordNotFound {
				return err
			}
		}

		advice, snapshot, err := resolve(sources)
		if err != nil {
			return err
		}
		advice.Snapshot = model.DispositionSourceSnapshot{}
		if err := tx.Create(&advice).Error; err != nil {
			return err
		}
		snapshot.DispositionAdviceID = advice.ID
		if err := tx.Create(&snapshot).Error; err != nil {
			return err
		}
		if appendAudit != nil {
			if err := appendAudit(tx, advice); err != nil {
				return err
			}
		}
		result = advice
		return nil
	})
	if err != nil {
		return model.DispositionAdvice{}, false, err
	}
	return result, existed, nil
}

func (r *dispositionAdviceRepository) Delete(ctx context.Context, id uint, appendAudit func(*gorm.DB, model.DispositionAdvice) error) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var advice model.DispositionAdvice
		if err := tx.First(&advice, id).Error; err != nil {
			return err
		}
		result := tx.Where("disposition_advice_id = ?", id).Delete(&model.DispositionSourceSnapshot{})
		if result.Error != nil {
			return result.Error
		}
		main := tx.Delete(&model.DispositionAdvice{}, id)
		if main.Error != nil {
			return main.Error
		}
		if main.RowsAffected == 0 {
			return gorm.ErrRecordNotFound
		}
		if appendAudit != nil {
			if err := appendAudit(tx, advice); err != nil {
				return err
			}
		}
		return nil
	})
}

func (r *dispositionAdviceRepository) CountByStatus(ctx context.Context) (map[string]int64, error) {
	return r.store.CountByStatus(ctx)
}

func (r *dispositionAdviceRepository) CountByDispositionLevel(ctx context.Context) (map[string]int64, error) {
	rows, err := r.db.WithContext(ctx).Model(&model.DispositionAdvice{}).
		Select("disposition_level, COUNT(*) AS total").Group("disposition_level").Rows()
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	counts := make(map[string]int64)
	for rows.Next() {
		var level string
		var total int64
		if err := rows.Scan(&level, &total); err != nil {
			return nil, err
		}
		counts[level] = total
	}
	return counts, rows.Err()
}
