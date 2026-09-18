package service

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/blueship581/railway-bridge-defect-priority/backend/internal/dto"
	"github.com/blueship581/railway-bridge-defect-priority/backend/internal/model"
	"github.com/blueship581/railway-bridge-defect-priority/backend/internal/repository"
	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

type adviceFixture struct {
	db      *gorm.DB
	service HandlingAdviceService
}

func newAdviceFixture(t *testing.T) adviceFixture {
	t.Helper()
	dsn := fmt.Sprintf("file:advice-%d?mode=memory&cache=shared&_pragma=busy_timeout(5000)&_pragma=foreign_keys(1)", time.Now().UnixNano())
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := db.AutoMigrate(
		&model.BridgeAsset{}, &model.InspectionRound{}, &model.DefectFinding{},
		&model.DefectHandlingAdvice{}, &model.DefectAdviceSnapshot{}, &model.AuditLog{},
	); err != nil {
		t.Fatalf("migrate sqlite: %v", err)
	}
	return adviceFixture{db: db, service: NewHandlingAdviceService(repository.NewHandlingAdviceRepository(db))}
}

func (f adviceFixture) seedEntity(t *testing.T, bridgeStatus, inspectionStatus, defectStatus, riskLevel, suffix string) {
	t.Helper()
	now := time.Now().UTC()
	bridge := model.BridgeAsset{
		BaseModel: model.BaseModel{Code: "BA-" + suffix, Name: "桥梁" + suffix, Status: bridgeStatus, Version: 1},
		Facility:  "K42", Owner: "工务段", Category: "结构", RiskLevel: riskLevel, EffectiveAt: now,
	}
	if err := f.db.Create(&bridge).Error; err != nil {
		t.Fatalf("seed bridge: %v", err)
	}
	round := model.InspectionRound{
		BaseModel: model.BaseModel{Code: "IR-" + suffix, Name: "批次" + suffix, Status: inspectionStatus, Version: 1},
		Facility:  "K42", Owner: "复核组", Category: "结构", RiskLevel: riskLevel, EffectiveAt: now.Add(time.Hour),
		RelatedCode: bridge.Code,
	}
	if err := f.db.Create(&round).Error; err != nil {
		t.Fatalf("seed inspection: %v", err)
	}
	defect := model.DefectFinding{
		BaseModel: model.BaseModel{Code: "DF-" + suffix, Name: "缺陷" + suffix, Status: defectStatus, Version: 1},
		Facility:  "K42", Owner: "检查组", Category: "结构", RiskLevel: riskLevel, EffectiveAt: now.Add(2 * time.Hour),
		RelatedCode: bridge.Code,
	}
	if err := f.db.Create(&defect).Error; err != nil {
		t.Fatalf("seed defect: %v", err)
	}
}

func input(defectCode string) dto.GenerateHandlingAdvice {
	return dto.GenerateHandlingAdvice{DefectCode: defectCode, Reason: "复核员现场核验并锁定来源状态"}
}

func (f adviceFixture) counts(t *testing.T) (int64, int64, int64) {
	t.Helper()
	var advices, snapshots, audits int64
	f.db.Model(&model.DefectHandlingAdvice{}).Count(&advices)
	f.db.Model(&model.DefectAdviceSnapshot{}).Count(&snapshots)
	f.db.Model(&model.AuditLog{}).Where("entity_type = ?", "DefectHandlingAdvice").Count(&audits)
	return advices, snapshots, audits
}

func TestAdviceGeneralDefectStaysObserve(t *testing.T) {
	fixture := newAdviceFixture(t)
	ctx := context.Background()
	fixture.seedEntity(t, "active", "completed", "verified", "medium", "G1")

	advice, created, err := fixture.service.VerifyAndGenerate(ctx, input("DF-G1"), "reviewer", "req-g1")
	if err != nil || !created {
		t.Fatalf("generate advice: created=%v err=%v", created, err)
	}
	if advice.HandlingLevel != "observe" || advice.DefectGrade != "general" {
		t.Fatalf("general defect must stay observe, got grade=%s level=%s", advice.DefectGrade, advice.HandlingLevel)
	}
	if advice.Snapshot.BridgeStatus != "active" || advice.Snapshot.InspectionStatus != "completed" ||
		advice.Snapshot.DefectStatus != "verified" || advice.Snapshot.BridgeSnapshot == "" ||
		advice.Snapshot.DefectSnapshot == "" || advice.Snapshot.InspectionSnapshot == "" {
		t.Fatalf("source snapshot incomplete: %+v", advice.Snapshot)
	}
	advices, snapshots, audits := fixture.counts(t)
	if advices != 1 || snapshots != 1 || audits != 1 {
		t.Fatalf("expected 1/1/1 advice/snapshot/audit, got %d/%d/%d", advices, snapshots, audits)
	}
}

func TestAdviceSevereDefectEscalatesAfterRestriction(t *testing.T) {
	fixture := newAdviceFixture(t)
	ctx := context.Background()
	// 严重缺陷但桥梁尚未限行：建议先为 restrict。
	fixture.seedEntity(t, "active", "completed", "verified", "critical", "S1")
	before, _, err := fixture.service.VerifyAndGenerate(ctx, input("DF-S1"), "reviewer", "req-before")
	if err != nil {
		t.Fatalf("generate pre-restriction advice: %v", err)
	}
	if before.HandlingLevel != "restrict" {
		t.Fatalf("severe defect on open bridge must be restrict, got %s", before.HandlingLevel)
	}
	// 快照必须保留限行前的旧桥梁状态，不受后续桥梁状态迁移影响。
	if err := fixture.db.Model(&model.BridgeAsset{}).Where("code = ?", "BA-S1").
		Update("status", "restricted").Error; err != nil {
		t.Fatalf("restrict bridge: %v", err)
	}
	reread, _, err := fixture.service.VerifyAndGenerate(ctx, input("DF-S1"), "reviewer", "req-repeat")
	if err != nil {
		t.Fatalf("repeat verification: %v", err)
	}
	if reread.ID != before.ID || reread.HandlingLevel != "restrict" || reread.Snapshot.BridgeStatus != "active" {
		t.Fatalf("repeat verification must return the single frozen advice, got %+v snapshot bridge=%s",
			reread, reread.Snapshot.BridgeStatus)
	}

	// 桥梁限行后新核验的严重缺陷必须升级为立即处置。
	fixture.seedEntity(t, "restricted", "completed", "verified", "high", "S2")
	urgent, _, err := fixture.service.VerifyAndGenerate(ctx, input("DF-S2"), "reviewer", "req-urgent")
	if err != nil {
		t.Fatalf("generate urgent advice: %v", err)
	}
	if urgent.HandlingLevel != "urgent" || urgent.DefectGrade != "severe" {
		t.Fatalf("severe defect on restricted bridge must escalate to urgent, got %+v", urgent)
	}
}

func TestAdviceRejectsIncompleteSourcesWithoutHalfData(t *testing.T) {
	fixture := newAdviceFixture(t)
	ctx := context.Background()

	// 缺陷尚未核验：不得生成建议。
	fixture.seedEntity(t, "active", "completed", "new", "high", "N1")
	if _, _, err := fixture.service.VerifyAndGenerate(ctx, input("DF-N1"), "reviewer", "req-new"); !errors.Is(err, ErrDefectNotVerified) {
		t.Fatalf("unverified defect must fail, got %v", err)
	}

	// 检查批次未出结论（review）：不得生成建议。
	fixture.seedEntity(t, "restricted", "review", "verified", "critical", "N2")
	if _, _, err := fixture.service.VerifyAndGenerate(ctx, input("DF-N2"), "reviewer", "req-open"); !errors.Is(err, ErrInspectionOpen) {
		t.Fatalf("open inspection must fail, got %v", err)
	}

	// 缺陷无桥梁关联：不得生成建议。
	fixture.seedEntity(t, "active", "completed", "verified", "critical", "N3")
	if err := fixture.db.Model(&model.DefectFinding{}).Where("code = ?", "DF-N3").
		Update("related_code", "").Error; err != nil {
		t.Fatalf("unlink defect: %v", err)
	}
	if _, _, err := fixture.service.VerifyAndGenerate(ctx, input("DF-N3"), "reviewer", "req-nobridge"); !errors.Is(err, ErrMissingBridge) {
		t.Fatalf("defect without bridge must fail, got %v", err)
	}

	// 桥梁存在但没有任何检查批次：不得生成建议。
	fixture.seedEntity(t, "active", "completed", "verified", "critical", "N4")
	if err := fixture.db.Where("related_code = ?", "BA-N4").Delete(&model.InspectionRound{}).Error; err != nil {
		t.Fatalf("remove inspections: %v", err)
	}
	if _, _, err := fixture.service.VerifyAndGenerate(ctx, input("DF-N4"), "reviewer", "req-noinsp"); !errors.Is(err, ErrMissingInspection) {
		t.Fatalf("missing inspection must fail, got %v", err)
	}

	if advices, snapshots, audits := fixture.counts(t); advices != 0 || snapshots != 0 || audits != 0 {
		t.Fatalf("rejected verifications must leave no half data, got %d/%d/%d advice/snapshot/audit", advices, snapshots, audits)
	}
}

func TestAdviceRepeatedVerificationKeepsSingleRow(t *testing.T) {
	fixture := newAdviceFixture(t)
	ctx := context.Background()
	fixture.seedEntity(t, "restricted", "completed", "verified", "critical", "R1")

	first, created, err := fixture.service.VerifyAndGenerate(ctx, input("DF-R1"), "reviewer", "req-1")
	if err != nil || !created {
		t.Fatalf("first verification: created=%v err=%v", created, err)
	}
	for attempt := 2; attempt <= 5; attempt++ {
		again, createdAgain, err := fixture.service.VerifyAndGenerate(ctx, input("DF-R1"), "reviewer", fmt.Sprintf("req-%d", attempt))
		if err != nil {
			t.Fatalf("repeat verification %d: %v", attempt, err)
		}
		if createdAgain || again.ID != first.ID || again.RequestID != "req-1" || again.VerifiedBy != "reviewer" {
			t.Fatalf("attempt %d must return the unchanged original advice, got created=%v advice=%+v", attempt, createdAgain, again)
		}
	}
	if advices, snapshots, _ := fixture.counts(t); advices != 1 || snapshots != 1 {
		t.Fatalf("expected exactly one advice and snapshot, got %d/%d", advices, snapshots)
	}
}

func TestAdviceConcurrentVerificationCreatesExactlyOne(t *testing.T) {
	fixture := newAdviceFixture(t)
	ctx := context.Background()
	fixture.seedEntity(t, "restricted", "completed", "verified", "critical", "C1")

	const workers = 16
	var wg sync.WaitGroup
	results := make(chan uint, workers)
	errs := make(chan error, workers)
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			advice, _, err := fixture.service.VerifyAndGenerate(ctx, input("DF-C1"), "reviewer", fmt.Sprintf("req-conc-%d", i))
			if err != nil {
				errs <- err
				return
			}
			results <- advice.ID
		}(i)
	}
	wg.Wait()
	close(results)
	close(errs)
	for err := range errs {
		t.Fatalf("concurrent verification failed: %v", err)
	}
	unique := make(map[uint]struct{})
	for id := range results {
		unique[id] = struct{}{}
	}
	if len(unique) != 1 {
		t.Fatalf("concurrent verifications produced %d distinct advices, want exactly 1", len(unique))
	}
	if advices, snapshots, _ := fixture.counts(t); advices != 1 || snapshots != 1 {
		t.Fatalf("expected 1/1 advice/snapshot under concurrency, got %d/%d", advices, snapshots)
	}
}

func TestAdviceConcurrentSourceChangesDoNotMixSnapshots(t *testing.T) {
	fixture := newAdviceFixture(t)
	ctx := context.Background()
	fixture.seedEntity(t, "active", "completed", "verified", "critical", "M1")

	// 核验生成建议的同时，另一个请求尝试推进桥梁状态。无论谁先提交，快照都
	// 必须与同事务内读取到的桥梁状态一致，不允许出现旧缺陷配新桥梁状态。
	var wg sync.WaitGroup
	wg.Add(2)
	var advice model.DefectHandlingAdvice
	var generateErr error
	go func() {
		defer wg.Done()
		advice, _, generateErr = fixture.service.VerifyAndGenerate(ctx, input("DF-M1"), "reviewer", "req-mix-advice")
	}()
	go func() {
		defer wg.Done()
		time.Sleep(5 * time.Millisecond)
		_ = fixture.db.Model(&model.BridgeAsset{}).Where("code = ?", "BA-M1").Update("status", "restricted").Error
	}()
	wg.Wait()
	if generateErr != nil {
		t.Fatalf("verification during concurrent bridge change: %v", generateErr)
	}
	expected := map[string]string{
		"active":     "restrict",
		"restricted": "urgent",
	}
	wantLevel, ok := expected[advice.Snapshot.BridgeStatus]
	if !ok {
		t.Fatalf("snapshot captured unexpected bridge status %q", advice.Snapshot.BridgeStatus)
	}
	if advice.HandlingLevel != wantLevel || advice.Snapshot.ResolvedHandlingLevel != wantLevel {
		t.Fatalf("snapshot mixed states: bridge=%s level=%s resolved=%s, want level %s",
			advice.Snapshot.BridgeStatus, advice.HandlingLevel, advice.Snapshot.ResolvedHandlingLevel, wantLevel)
	}
}

func TestAdviceWriteFailureRollsBackEverything(t *testing.T) {
	fixture := newAdviceFixture(t)
	db := fixture.db
	ctx := context.Background()
	fixture.seedEntity(t, "restricted", "completed", "verified", "critical", "F1")

	// Occupy the unique advice code so the advice INSERT inside Generate must
	// fail midway, proving advice + snapshot + audit share one transaction.
	blocker := model.DefectHandlingAdvice{
		Code: "DHA-BLOCKED", DefectID: 999999, DefectCode: "DF-BLOCKER",
		BridgeID: 1, BridgeCode: "BA-X", InspectionID: 1, InspectionCode: "IR-X",
		DefectGrade: "severe", HandlingLevel: "urgent", Status: model.DefectHandlingAdviceStatus, Version: 1,
		RuleSummary: "unique code blocker", VerifiedBy: "reviewer", RequestID: "req-blocker",
		CreatedAt: time.Now().UTC(), UpdatedAt: time.Now().UTC(),
	}
	if err := db.Create(&blocker).Error; err != nil {
		t.Fatalf("seed blocker advice: %v", err)
	}

	repo := repository.NewHandlingAdviceRepository(db)
	_, _, err := repo.Generate(ctx, "DF-F1", func(source repository.AdviceSource) (repository.DerivedAdvice, error) {
		return repository.DerivedAdvice{
			Advice: model.DefectHandlingAdvice{
				Code: "DHA-BLOCKED", DefectID: source.Defect.ID, DefectCode: source.Defect.Code,
				BridgeID: source.Bridge.ID, BridgeCode: source.Bridge.Code,
				InspectionID: source.Inspection.ID, InspectionCode: source.Inspection.Code,
				DefectGrade: "severe", HandlingLevel: "urgent",
				Status: model.DefectHandlingAdviceStatus, Version: 1, RuleSummary: "should roll back",
				VerifiedBy: "reviewer", RequestID: "req-broken",
			},
			Snapshot: model.DefectAdviceSnapshot{
				BridgeStatus: "restricted", DefectStatus: "verified", DefectGrade: "severe",
				InspectionStatus: "completed", ResolvedHandlingLevel: "urgent",
				BridgeSnapshot: "{}", DefectSnapshot: "{}", InspectionSnapshot: "{}",
			},
			Audit: model.AuditLog{
				Actor: "reviewer", RequestID: "req-broken", Action: "verify",
				EntityType: "DefectHandlingAdvice", AfterState: "urgent",
			},
		}, nil
	})
	if err == nil {
		t.Fatal("expected unique violation to abort the transaction")
	}

	var forDefect int64
	db.Model(&model.DefectHandlingAdvice{}).Where("defect_id = (SELECT id FROM defect_findings WHERE code = 'DF-F1')").Count(&forDefect)
	if forDefect != 0 {
		t.Fatalf("failed write must roll back the advice row, found %d", forDefect)
	}
	var totalAdvices, totalSnapshots, totalAudits int64
	db.Model(&model.DefectHandlingAdvice{}).Count(&totalAdvices)
	db.Model(&model.DefectAdviceSnapshot{}).Count(&totalSnapshots)
	db.Model(&model.AuditLog{}).Where("entity_type = ?", "DefectHandlingAdvice").Count(&totalAudits)
	if totalAdvices != 1 || totalSnapshots != 0 || totalAudits != 0 {
		t.Fatalf("write failure left half data: advices=%d snapshots=%d audits=%d (want 1/0/0)", totalAdvices, totalSnapshots, totalAudits)
	}
}
