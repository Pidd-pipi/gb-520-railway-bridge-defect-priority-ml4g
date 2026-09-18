package service

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/blueship581/railway-bridge-defect-priority/backend/internal/constants"
	"github.com/blueship581/railway-bridge-defect-priority/backend/internal/dto"
	"github.com/blueship581/railway-bridge-defect-priority/backend/internal/model"
	"github.com/blueship581/railway-bridge-defect-priority/backend/internal/repository"
)

type HandlingAdviceService interface {
	List(context.Context, dto.HandlingAdviceQuery) (repository.Page[model.DefectHandlingAdvice], error)
	Get(context.Context, uint) (model.DefectHandlingAdvice, error)
	// VerifyAndGenerate implements 复核员核验缺陷: it locks bridge state, defect
	// grade and the latest inspection round, derives the handling level and
	// persists advice + source snapshot as one unit.
	VerifyAndGenerate(ctx context.Context, input dto.GenerateHandlingAdvice, actor, requestID string) (model.DefectHandlingAdvice, bool, error)
	LevelCounts(context.Context) (map[string]int64, error)
}

type handlingAdviceService struct {
	repository repository.HandlingAdviceRepository
}

func NewHandlingAdviceService(repo repository.HandlingAdviceRepository) HandlingAdviceService {
	return &handlingAdviceService{repository: repo}
}

func (s *handlingAdviceService) List(ctx context.Context, query dto.HandlingAdviceQuery) (repository.Page[model.DefectHandlingAdvice], error) {
	return s.repository.List(ctx, query)
}

func (s *handlingAdviceService) Get(ctx context.Context, id uint) (model.DefectHandlingAdvice, error) {
	return s.repository.Get(ctx, id)
}

func (s *handlingAdviceService) LevelCounts(ctx context.Context) (map[string]int64, error) {
	return s.repository.CountByLevel(ctx)
}

func (s *handlingAdviceService) VerifyAndGenerate(ctx context.Context, input dto.GenerateHandlingAdvice, actor, requestID string) (model.DefectHandlingAdvice, bool, error) {
	defectCode := strings.ToUpper(strings.TrimSpace(input.DefectCode))
	reason := strings.TrimSpace(input.Reason)
	if defectCode == "" || reason == "" {
		return model.DefectHandlingAdvice{}, false, ErrInvalidInput
	}
	return s.repository.Generate(ctx, defectCode, func(source repository.AdviceSource) (repository.DerivedAdvice, error) {
		defect := source.Defect
		// 检查未核验：核验流程的前提是缺陷已经过复核员核验（verified）。
		if defect.Status != string(constants.DefectStateVerified) {
			return repository.DerivedAdvice{}, ErrDefectNotVerified
		}
		// 缺关联记录：缺陷必须关联到一座在册桥梁。
		if !source.BridgeFound {
			return repository.DerivedAdvice{}, ErrMissingBridge
		}
		bridge := source.Bridge
		// 检查未出结论：只接受已完成（completed）的最近检查批次。
		if !source.InspectionFound {
			return repository.DerivedAdvice{}, ErrMissingInspection
		}
		inspection := source.Inspection
		if inspection.Status != "completed" {
			return repository.DerivedAdvice{}, ErrInspectionOpen
		}

		grade := constants.RiskLevelToDefectGrade[strings.ToLower(strings.TrimSpace(defect.RiskLevel))]
		level := constants.ResolveHandlingLevel(grade, bridge.Status)
		ruleSummary := buildRuleSummary(grade, level, bridge.Status, inspection.Code, reason)

		bridgeJSON, err := snapshotJSON(bridge)
		if err != nil {
			return repository.DerivedAdvice{}, err
		}
		defectJSON, err := snapshotJSON(defect)
		if err != nil {
			return repository.DerivedAdvice{}, err
		}
		inspectionJSON, err := snapshotJSON(inspection)
		if err != nil {
			return repository.DerivedAdvice{}, err
		}

		now := time.Now().UTC()
		advice := model.DefectHandlingAdvice{
			Code:           adviceCode(defect.Code),
			DefectID:       defect.ID,
			DefectCode:     defect.Code,
			BridgeID:       bridge.ID,
			BridgeCode:     bridge.Code,
			InspectionID:   inspection.ID,
			InspectionCode: inspection.Code,
			DefectGrade:    string(grade),
			HandlingLevel:  string(level),
			Status:         model.DefectHandlingAdviceStatus,
			Version:        1,
			RuleSummary:    ruleSummary,
			VerifiedBy:     actor,
			RequestID:      requestID,
			CreatedAt:      now,
			UpdatedAt:      now,
		}
		snapshot := model.DefectAdviceSnapshot{
			BridgeSnapshot:        bridgeJSON,
			DefectSnapshot:        defectJSON,
			InspectionSnapshot:    inspectionJSON,
			BridgeStatus:          bridge.Status,
			DefectStatus:          defect.Status,
			DefectGrade:           string(grade),
			InspectionStatus:      inspection.Status,
			ResolvedHandlingLevel: string(level),
			CapturedAt:            now,
		}
		audit := model.AuditLog{
			RequestID:   requestID,
			Actor:       actor,
			Action:      "verify",
			EntityType:  "DefectHandlingAdvice",
			BeforeState: defect.Status,
			AfterState:  string(level),
			Detail:      fmt.Sprintf("核验缺陷 %s 后生成处置建议（桥梁 %s / 批次 %s）：%s", defect.Code, bridge.Code, inspection.Code, ruleSummary),
			CreatedAt:   now,
		}
		return repository.DerivedAdvice{Advice: advice, Snapshot: snapshot, Audit: audit}, nil
	})
}

func adviceCode(defectCode string) string {
	return "DHA-" + strings.TrimPrefix(defectCode, "DF-")
}

func buildRuleSummary(grade constants.DefectGrade, level constants.HandlingLevel, bridgeStatus, inspectionCode, reason string) string {
	var summary string
	switch {
	case grade == constants.DefectGradeSevere && level == constants.HandlingLevelUrgent:
		summary = fmt.Sprintf("严重缺陷且桥梁已限行（%s），升级为立即处置", bridgeStatus)
	case grade == constants.DefectGradeSevere:
		summary = fmt.Sprintf("严重缺陷，桥梁状态 %s，建议限行处置", bridgeStatus)
	default:
		summary = "一般缺陷，保持观察并跟踪检查批次"
	}
	return fmt.Sprintf("%s；依据最近检查批次 %s；复核说明：%s", summary, inspectionCode, reason)
}

func snapshotJSON(value any) (string, error) {
	raw, err := json.Marshal(value)
	if err != nil {
		return "", fmt.Errorf("serialize advice source snapshot: %w", err)
	}
	return string(raw), nil
}
