package repository

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"sync"

	"github.com/blueship581/railway-bridge-defect-priority/backend/internal/dto"
	"github.com/blueship581/railway-bridge-defect-priority/backend/internal/model"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// HandlingAdviceRepository owns all persistence operations for 缺陷处置优先级建议.
type HandlingAdviceRepository interface {
	List(context.Context, dto.HandlingAdviceQuery) (Page[model.DefectHandlingAdvice], error)
	Get(context.Context, uint) (model.DefectHandlingAdvice, error)
	GetByDefectID(context.Context, uint) (model.DefectHandlingAdvice, error)
	CountByLevel(context.Context) (map[string]int64, error)
	// Generate locks the defect, its bridge and the latest inspection round in
	// one serializable-ish transaction and hands the locked source set to
	// derive. The returned advice, source snapshot and audit row are inserted
	// atomically; a duplicate verification short-circuits to the existing row.
	Generate(
		ctx context.Context,
		defectCode string,
		derive func(source AdviceSource) (DerivedAdvice, error),
	) (model.DefectHandlingAdvice, bool, error)
}

// AdviceSource is the locked, internally consistent read model passed to the
// service rule function. Flags distinguish "record missing" from "record
// present but not concluded yet".
type AdviceSource struct {
	Defect          model.DefectFinding
	Bridge          model.BridgeAsset
	Inspection      model.InspectionRound
	BridgeFound     bool
	InspectionFound bool
}

// DerivedAdvice bundles everything the transaction must persist together so a
// write failure can never leave an advice without its snapshot or audit.
type DerivedAdvice struct {
	Advice   model.DefectHandlingAdvice
	Snapshot model.DefectAdviceSnapshot
	Audit    model.AuditLog
}

var ErrAdviceDuplicate = errors.New("handling advice already exists for defect")

type handlingAdviceRepository struct {
	db     *gorm.DB
	sqlite *keyedMutex
}

func NewHandlingAdviceRepository(db *gorm.DB) HandlingAdviceRepository {
	return &handlingAdviceRepository{db: db, sqlite: newKeyedMutex()}
}

func (r *handlingAdviceRepository) List(ctx context.Context, q dto.HandlingAdviceQuery) (Page[model.DefectHandlingAdvice], error) {
	page, pageSize := normalizePage(q.Page, q.PageSize)
	db := r.db.WithContext(ctx).Model(&model.DefectHandlingAdvice{})
	if search := strings.TrimSpace(strings.ToLower(q.Search)); search != "" {
		wildcard := "%" + search + "%"
		db = db.Where("LOWER(code) LIKE ? OR LOWER(defect_code) LIKE ? OR LOWER(bridge_code) LIKE ?", wildcard, wildcard, wildcard)
	}
	if defectCode := strings.TrimSpace(strings.ToUpper(q.DefectCode)); defectCode != "" {
		db = db.Where("defect_code = ?", defectCode)
	}
	if level := strings.TrimSpace(q.HandlingLevel); level != "" {
		db = db.Where("handling_level = ?", level)
	}
	if grade := strings.TrimSpace(q.DefectGrade); grade != "" {
		db = db.Where("defect_grade = ?", grade)
	}
	var total int64
	if err := db.Count(&total).Error; err != nil {
		return Page[model.DefectHandlingAdvice]{}, err
	}
	items := make([]model.DefectHandlingAdvice, 0)
	err := db.Preload("Snapshot").
		Order("updated_at DESC, id DESC").
		Offset((page - 1) * pageSize).Limit(pageSize).Find(&items).Error
	return Page[model.DefectHandlingAdvice]{Items: items, Total: total, Page: page, PageSize: pageSize}, err
}

func (r *handlingAdviceRepository) Get(ctx context.Context, id uint) (model.DefectHandlingAdvice, error) {
	var item model.DefectHandlingAdvice
	err := r.db.WithContext(ctx).Preload("Snapshot").First(&item, id).Error
	return item, err
}

func (r *handlingAdviceRepository) GetByDefectID(ctx context.Context, defectID uint) (model.DefectHandlingAdvice, error) {
	var item model.DefectHandlingAdvice
	err := r.db.WithContext(ctx).Preload("Snapshot").
		Where("defect_id = ?", defectID).First(&item).Error
	return item, err
}

func (r *handlingAdviceRepository) CountByLevel(ctx context.Context) (map[string]int64, error) {
	rows, err := r.db.WithContext(ctx).Model(&model.DefectHandlingAdvice{}).
		Select("handling_level, COUNT(*) AS total").Group("handling_level").Rows()
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

// Generate uses row locks (SELECT ... FOR UPDATE on Postgres/MySQL) so two
// concurrent verifications of the same defect serialize at the database. All
// source rows are read through locking clauses in a fixed order (defect →
// bridge → inspection), which both prevents old/new state mixing and makes
// concurrent updates to those rows block until the advice is committed. SQLite
// rejects FOR UPDATE syntactically; there, database-level write locking plus
// the unique defect_id index provide the same end state.
func (r *handlingAdviceRepository) Generate(
	ctx context.Context,
	defectCode string,
	derive func(source AdviceSource) (DerivedAdvice, error),
) (model.DefectHandlingAdvice, bool, error) {
	options := &sql.TxOptions{}
	if name := r.db.Dialector.Name(); name == "postgres" || name == "mysql" {
		// Repeatable read pins every locking read to one coherent database view.
		options.Isolation = sql.LevelRepeatableRead
	} else {
		// SQLite serializes writers but can return SQLITE_BUSY; the keyed mutex
		// makes same-defect verification deterministic in development/tests.
		unlock := r.sqlite.lock(strings.ToUpper(strings.TrimSpace(defectCode)))
		defer unlock()
	}

	var defectID uint
	var created bool
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var defect model.DefectFinding
		if err := locked(tx).Where("code = ?", strings.ToUpper(strings.TrimSpace(defectCode))).
			First(&defect).Error; err != nil {
			return err
		}
		defectID = defect.ID

		var existing model.DefectHandlingAdvice
		switch err := locked(tx).Where("defect_id = ?", defect.ID).First(&existing).Error; {
		case err == nil:
			// Repeated verification keeps the single existing advice untouched.
			return nil
		case !errors.Is(err, gorm.ErrRecordNotFound):
			return err
		}

		source := AdviceSource{Defect: defect}
		bridgeCode := strings.ToUpper(strings.TrimSpace(defect.RelatedCode))
		if bridgeCode != "" {
			var bridge model.BridgeAsset
			switch err := locked(tx).Where("code = ?", bridgeCode).First(&bridge).Error; {
			case err == nil:
				source.Bridge = bridge
				source.BridgeFound = true
			case errors.Is(err, gorm.ErrRecordNotFound):
			default:
				return err
			}
		}

		if source.BridgeFound {
			var inspection model.InspectionRound
			switch err := locked(tx).
				Where("related_code = ?", bridgeCode).
				Order("effective_at DESC, id DESC").First(&inspection).Error; {
			case err == nil:
				source.Inspection = inspection
				source.InspectionFound = true
			case errors.Is(err, gorm.ErrRecordNotFound):
			default:
				return err
			}
		}

		derived, err := derive(source)
		if err != nil {
			return err
		}
		if err := tx.Create(&derived.Advice).Error; err != nil {
			if isUniqueViolation(err) {
				return ErrAdviceDuplicate
			}
			return err
		}
		derived.Snapshot.AdviceID = derived.Advice.ID
		if err := tx.Create(&derived.Snapshot).Error; err != nil {
			return err
		}
		derived.Audit.EntityID = derived.Advice.ID
		if err := tx.Create(&derived.Audit).Error; err != nil {
			return err
		}
		created = true
		return nil
	}, options)
	if err != nil {
		if errors.Is(err, ErrAdviceDuplicate) {
			// Lost a creation race: load the winner's row on a fresh snapshot.
			advice, loadErr := r.GetByDefectID(ctx, defectID)
			return advice, false, loadErr
		}
		return model.DefectHandlingAdvice{}, false, err
	}
	advice, err := r.GetByDefectID(ctx, defectID)
	return advice, created, err
}

func locked(tx *gorm.DB) *gorm.DB {
	if tx.Dialector.Name() == "sqlite" {
		return tx
	}
	return tx.Clauses(clause.Locking{Strength: "UPDATE"})
}

func isUniqueViolation(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, gorm.ErrDuplicatedKey) {
		return true
	}
	text := strings.ToLower(err.Error())
	return strings.Contains(text, "unique constraint") ||
		strings.Contains(text, "duplicate key") ||
		strings.Contains(text, "duplicated key") ||
		strings.Contains(text, "constraint failed:")
}

// keyedMutex serializes SQLite transactions per defect code while allowing
// different defects to proceed independently.
type keyedMutex struct {
	mu    sync.Mutex
	locks map[string]*keyedLock
}

type keyedLock struct {
	mu   sync.Mutex
	refs int
}

func newKeyedMutex() *keyedMutex {
	return &keyedMutex{locks: make(map[string]*keyedLock)}
}

func (m *keyedMutex) lock(key string) func() {
	m.mu.Lock()
	entry := m.locks[key]
	if entry == nil {
		entry = &keyedLock{}
		m.locks[key] = entry
	}
	entry.refs++
	m.mu.Unlock()

	entry.mu.Lock()
	return func() {
		entry.mu.Unlock()
		m.mu.Lock()
		entry.refs--
		if entry.refs == 0 {
			delete(m.locks, key)
		}
		m.mu.Unlock()
	}
}
