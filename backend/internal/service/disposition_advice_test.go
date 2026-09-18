package service

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/blueship581/railway-bridge-defect-priority/backend/internal/config"
	"github.com/blueship581/railway-bridge-defect-priority/backend/internal/dto"
	"github.com/blueship581/railway-bridge-defect-priority/backend/internal/model"
	"github.com/blueship581/railway-bridge-defect-priority/backend/internal/repository"
	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

type dispositionFixture struct {
	db      *gorm.DB
	service DispositionAdviceService
	defect  model.DefectFinding
	bridge  model.BridgeAsset
	round   model.InspectionRound
}

func newDispositionFixture(t *testing.T) dispositionFixture {
	t.Helper()
	temp, err := os.MkdirTemp("", "disposition-test-*")
	if err != nil {
		t.Fatalf("create temp dir: %v", err)
	}
	dsn := filepath.Join(temp, fmt.Sprintf("disp-%d.db", time.Now().UnixNano())) +
		"?_pragma=journal_mode(WAL)&_pragma=busy_timeout(10000)&_pragma=foreign_keys(ON)"
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := db.AutoMigrate(
		&model.BridgeAsset{}, &model.InspectionRound{}, &model.DefectFinding{},
		&model.DispositionAdvice{}, &model.DispositionSourceSnapshot{}, &model.AuditLog{},
	); err != nil {
		t.Fatalf("migrate sqlite: %v", err)
	}
	now := time.Now().UTC()
	bridge := model.BridgeAsset{
		BaseModel: model.BaseModel{Code: "BA-D1", Name: "测试桥梁", Status: "restricted", Version: 1},
		Facility:  "K42 作业区", RiskLevel: "high", EffectiveAt: now, Evidence: "桥梁巡查记录",
	}
	round := model.InspectionRound{
		BaseModel: model.BaseModel{Code: "IR-D1", Name: "测试检查批次", Status: "completed", Version: 1},
		Facility:  "K42 作业区", EffectiveAt: now, Evidence: "批次结论已出具",
	}
	defect := model.DefectFinding{
		BaseModel: model.BaseModel{Code: "DF-D1", Name: "测试严重缺陷", Status: "verified", Version: 1},
		Facility:  "K42 作业区", RiskLevel: "critical", EffectiveAt: now,
		Evidence: "复核确认的严重缺陷", RelatedCode: "BA-D1",
	}
	if err := db.Create(&bridge).Error; err != nil {
		t.Fatalf("seed bridge: %v", err)
	}
	if err := db.Create(&round).Error; err != nil {
		t.Fatalf("seed round: %v", err)
	}
	if err := db.Create(&defect).Error; err != nil {
		t.Fatalf("seed defect: %v", err)
	}
	security := NewSecurityService(repository.NewSecurityRepository(db), config.Config{})
	svc := NewDispositionAdviceService(repository.NewDispositionAdviceRepository(db), security)
	fixture := dispositionFixture{db: db, service: svc, defect: defect, bridge: bridge, round: round}
	t.Cleanup(func() {
		if sqlDB, err := db.DB(); err == nil {
			_ = sqlDB.Close()
		}
		_ = os.RemoveAll(temp)
	})
	return fixture
}

func (f dispositionFixture) countAdvice(t *testing.T) (int64, int64) {
	t.Helper()
	var advices, snapshots int64
	if err := f.db.Model(&model.DispositionAdvice{}).Count(&advices).Error; err != nil {
		t.Fatalf("count advices: %v", err)
	}
	if err := f.db.Model(&model.DispositionSourceSnapshot{}).Count(&snapshots).Error; err != nil {
		t.Fatalf("count snapshots: %v", err)
	}
	return advices, snapshots
}

func TestSeriousDefectOnRestrictedBridgeBecomesUrgent(t *testing.T) {
	fixture := newDispositionFixture(t)
	advice, existed, err := fixture.service.Generate(context.Background(),
		dto.GenerateDispositionAdvice{DefectID: fixture.defect.ID}, "reviewer", model.RoleReviewer, "req-urgent")
	if err != nil {
		t.Fatalf("generate urgent advice: %v", err)
	}
	if existed {
		t.Fatal("first verification must create a new advice")
	}
	if advice.DispositionLevel != "urgent" || advice.DefectGrade != "serious" || advice.BridgeState != "restricted" {
		t.Fatalf("expected urgent advice for serious defect on restricted bridge, got %+v", advice)
	}
	if advice.Code != "DA-D1" || advice.ReviewedBy != "reviewer" || advice.RequestID != "req-urgent" {
		t.Fatalf("advice provenance not preserved: %+v", advice)
	}
	if advice.Snapshot.Payload == "" || advice.Snapshot.BridgeVersion != 1 || advice.Snapshot.InspectionStatus != "completed" {
		t.Fatalf("source snapshot not locked: %+v", advice.Snapshot)
	}
	advices, snapshots := fixture.countAdvice(t)
	if advices != 1 || snapshots != 1 {
		t.Fatalf("expected exactly one advice and one snapshot, got %d/%d", advices, snapshots)
	}
}

func TestGeneralDefectStaysObserveEvenWhenBridgeRestricted(t *testing.T) {
	fixture := newDispositionFixture(t)
	if err := fixture.db.Model(&model.DefectFinding{}).Where("id = ?", fixture.defect.ID).
		Update("risk_level", "medium").Error; err != nil {
		t.Fatalf("downgrade defect risk: %v", err)
	}
	advice, _, err := fixture.service.Generate(context.Background(),
		dto.GenerateDispositionAdvice{DefectID: fixture.defect.ID}, "reviewer", model.RoleReviewer, "req-observe")
	if err != nil {
		t.Fatalf("generate observe advice: %v", err)
	}
	if advice.DispositionLevel != "observe" || advice.DefectGrade != "general" {
		t.Fatalf("general defect must stay observe, got level=%s grade=%s", advice.DispositionLevel, advice.DefectGrade)
	}
}

func TestSeriousDefectOnActiveBridgeIsRestrictBeforeLimit(t *testing.T) {
	fixture := newDispositionFixture(t)
	if err := fixture.db.Model(&model.BridgeAsset{}).Where("id = ?", fixture.bridge.ID).
		Updates(map[string]any{"status": "active", "version": 2}).Error; err != nil {
		t.Fatalf("reopen bridge: %v", err)
	}
	advice, _, err := fixture.service.Generate(context.Background(),
		dto.GenerateDispositionAdvice{DefectID: fixture.defect.ID}, "reviewer", model.RoleReviewer, "req-restrict")
	if err != nil {
		t.Fatalf("generate restrict advice: %v", err)
	}
	if advice.DispositionLevel != "restrict" {
		t.Fatalf("serious defect on active bridge should be restrict, got %s", advice.DispositionLevel)
	}
}

func TestRepeatVerificationKeepsSingleAdviceWithFrozenSnapshot(t *testing.T) {
	fixture := newDispositionFixture(t)
	ctx := context.Background()
	first, existed, err := fixture.service.Generate(ctx, dto.GenerateDispositionAdvice{DefectID: fixture.defect.ID}, "reviewer", model.RoleReviewer, "req-first")
	if err != nil {
		t.Fatalf("first generate: %v", err)
	}
	if existed {
		t.Fatal("first verification must create rather than reuse")
	}

	// Bridge and inspection change concurrently before the second verification;
	// the existing advice must keep the old, frozen source snapshot.
	if err := fixture.db.Model(&model.BridgeAsset{}).Where("id = ?", fixture.bridge.ID).
		Updates(map[string]any{"status": "active", "version": 3}).Error; err != nil {
		t.Fatalf("mutate bridge: %v", err)
	}

	second, existedAgain, err := fixture.service.Generate(ctx, dto.GenerateDispositionAdvice{DefectID: fixture.defect.ID}, "reviewer", model.RoleReviewer, "req-second")
	if err != nil {
		t.Fatalf("repeat generate: %v", err)
	}
	if !existedAgain || second.ID != first.ID || second.RequestID != "req-first" {
		t.Fatalf("repeat verification must return the original advice, got %+v vs %+v", first, second)
	}
	if second.BridgeState != "restricted" || second.Snapshot.BridgeVersion != 1 || second.DispositionLevel != "urgent" {
		t.Fatalf("frozen snapshot was mixed with newer bridge state: %+v", second)
	}
	advices, snapshots := fixture.countAdvice(t)
	if advices != 1 || snapshots != 1 {
		t.Fatalf("repeat verification must not add rows, got advices=%d snapshots=%d", advices, snapshots)
	}
}

func TestConcurrentVerificationCreatesExactlyOneAdvice(t *testing.T) {
	fixture := newDispositionFixture(t)
	ctx := context.Background()
	const workers = 8
	var wg sync.WaitGroup
	results := make([]uint, workers)
	existedFlags := make([]bool, workers)
	errs := make([]error, workers)
	start := make(chan struct{})
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func(index int) {
			defer wg.Done()
			<-start
			advice, existed, err := fixture.service.Generate(ctx,
				dto.GenerateDispositionAdvice{DefectID: fixture.defect.ID}, "reviewer", model.RoleReviewer, fmt.Sprintf("req-concurrent-%d", index))
			results[index] = advice.ID
			existedFlags[index] = existed
			errs[index] = err
		}(i)
	}
	close(start)
	wg.Wait()
	var created int
	var winner uint
	for i, err := range errs {
		if err != nil {
			t.Fatalf("worker %d failed: %v", i, err)
		}
		if results[i] == 0 {
			t.Fatalf("worker %d returned empty advice", i)
		}
		if winner == 0 {
			winner = results[i]
		} else if results[i] != winner {
			t.Fatalf("workers resolved different advice ids: %d vs %d", winner, results[i])
		}
		if !existedFlags[i] {
			created++
		}
	}
	if created != 1 {
		t.Fatalf("exactly one worker should create, got %d", created)
	}
	advices, snapshots := fixture.countAdvice(t)
	if advices != 1 || snapshots != 1 {
		t.Fatalf("concurrent verification left advices=%d snapshots=%d", advices, snapshots)
	}
}

func TestInspectionWithoutConclusionBlocksAdvice(t *testing.T) {
	fixture := newDispositionFixture(t)
	if err := fixture.db.Model(&model.InspectionRound{}).Where("id = ?", fixture.round.ID).
		Updates(map[string]any{"status": "review", "version": 2}).Error; err != nil {
		t.Fatalf("reopen inspection: %v", err)
	}
	_, _, err := fixture.service.Generate(context.Background(),
		dto.GenerateDispositionAdvice{DefectID: fixture.defect.ID}, "reviewer", model.RoleReviewer, "req-no-conclusion")
	if !errors.Is(err, ErrInspectionIncomplete) {
		t.Fatalf("expected ErrInspectionIncomplete, got %v", err)
	}
	advices, snapshots := fixture.countAdvice(t)
	if advices != 0 || snapshots != 0 {
		t.Fatalf("blocked generation must leave no rows, got advices=%d snapshots=%d", advices, snapshots)
	}
}

func TestUnverifiedDefectBlocksAdvice(t *testing.T) {
	fixture := newDispositionFixture(t)
	if err := fixture.db.Model(&model.DefectFinding{}).Where("id = ?", fixture.defect.ID).
		Updates(map[string]any{"status": "new", "version": 2}).Error; err != nil {
		t.Fatalf("reset defect state: %v", err)
	}
	_, _, err := fixture.service.Generate(context.Background(),
		dto.GenerateDispositionAdvice{DefectID: fixture.defect.ID}, "reviewer", model.RoleReviewer, "req-unverified")
	if !errors.Is(err, ErrDefectNotVerified) {
		t.Fatalf("expected ErrDefectNotVerified, got %v", err)
	}
	if advices, snapshots := fixture.countAdvice(t); advices != 0 || snapshots != 0 {
		t.Fatalf("unverified defect must leave no rows, got advices=%d snapshots=%d", advices, snapshots)
	}
}

func TestMissingBridgeLinkBlocksAdvice(t *testing.T) {
	fixture := newDispositionFixture(t)
	if err := fixture.db.Model(&model.DefectFinding{}).Where("id = ?", fixture.defect.ID).
		Update("related_code", "BA-GHOST").Error; err != nil {
		t.Fatalf("break bridge link: %v", err)
	}
	_, _, err := fixture.service.Generate(context.Background(),
		dto.GenerateDispositionAdvice{DefectID: fixture.defect.ID}, "reviewer", model.RoleReviewer, "req-no-bridge")
	if !errors.Is(err, ErrBridgeLinkMissing) {
		t.Fatalf("expected ErrBridgeLinkMissing, got %v", err)
	}
	if advices, snapshots := fixture.countAdvice(t); advices != 0 || snapshots != 0 {
		t.Fatalf("missing bridge must leave no rows, got advices=%d snapshots=%d", advices, snapshots)
	}
}

func TestOperatorCannotGenerateAdvice(t *testing.T) {
	fixture := newDispositionFixture(t)
	_, _, err := fixture.service.Generate(context.Background(),
		dto.GenerateDispositionAdvice{DefectID: fixture.defect.ID}, "operator", model.RoleOperator, "req-operator")
	if !errors.Is(err, ErrReviewRole) {
		t.Fatalf("operator generation must fail with ErrReviewRole, got %v", err)
	}
	if advices, snapshots := fixture.countAdvice(t); advices != 0 || snapshots != 0 {
		t.Fatalf("unauthorized generation must leave no rows, got advices=%d snapshots=%d", advices, snapshots)
	}
}

func TestUnknownDefectReturnsNotFound(t *testing.T) {
	fixture := newDispositionFixture(t)
	_, _, err := fixture.service.Generate(context.Background(),
		dto.GenerateDispositionAdvice{DefectID: 99999}, "reviewer", model.RoleReviewer, "req-missing")
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		t.Fatalf("expected gorm.ErrRecordNotFound, got %v", err)
	}
}

func TestAuditWriteFailureRollsBackAdviceAndSnapshot(t *testing.T) {
	fixture := newDispositionFixture(t)
	repo := repository.NewDispositionAdviceRepository(fixture.db)
	forced := errors.New("forced audit disk failure")
	_, _, err := repo.GenerateInTx(
		context.Background(), fixture.defect.ID,
		func(sources repository.LockedSources) (model.DispositionAdvice, model.DispositionSourceSnapshot, error) {
			return model.DispositionAdvice{
					Code: "DA-FORCE", Status: "generated", DefectID: fixture.defect.ID, DefectCode: fixture.defect.Code,
					BridgeID: fixture.bridge.ID, BridgeCode: fixture.bridge.Code, InspectionRoundID: fixture.round.ID,
					InspectionCode: fixture.round.Code, DefectGrade: "serious", RiskLevel: "critical",
					BridgeState: "restricted", DispositionLevel: "urgent", Reason: "rollback test",
					ReviewedBy: "reviewer", RequestID: "req-force", Version: 1,
				},
				model.DispositionSourceSnapshot{Payload: "{}"}, nil
		},
		func(tx *gorm.DB, advice model.DispositionAdvice) error {
			return forced
		},
	)
	if !errors.Is(err, forced) {
		t.Fatalf("expected forced audit error, got %v", err)
	}
	if advices, snapshots := fixture.countAdvice(t); advices != 0 || snapshots != 0 {
		t.Fatalf("audit failure must roll back everything, got advices=%d snapshots=%d", advices, snapshots)
	}
}

func TestDeleteRemovesAdviceAndSnapshotTogether(t *testing.T) {
	fixture := newDispositionFixture(t)
	advice, _, err := fixture.service.Generate(context.Background(),
		dto.GenerateDispositionAdvice{DefectID: fixture.defect.ID}, "reviewer", model.RoleReviewer, "req-delete")
	if err != nil {
		t.Fatalf("generate advice: %v", err)
	}
	if err := fixture.service.Delete(context.Background(), advice.ID, "admin", "req-delete-go"); err != nil {
		t.Fatalf("delete advice: %v", err)
	}
	var adviceRows, snapshotRows int64
	if err := fixture.db.Model(&model.DispositionAdvice{}).Where("id = ?", advice.ID).Count(&adviceRows).Error; err != nil {
		t.Fatalf("count advice: %v", err)
	}
	if err := fixture.db.Model(&model.DispositionSourceSnapshot{}).Where("disposition_advice_id = ?", advice.ID).Count(&snapshotRows).Error; err != nil {
		t.Fatalf("count snapshot: %v", err)
	}
	if adviceRows != 0 || snapshotRows != 0 {
		t.Fatalf("delete left advice=%d snapshot=%d", adviceRows, snapshotRows)
	}
}
