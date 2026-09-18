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
	"gorm.io/gorm"
)

type DispositionAdviceService interface {
	List(context.Context, dto.PageQuery) (repository.Page[model.DispositionAdvice], error)
	Get(context.Context, uint) (model.DispositionAdvice, error)
	Generate(context.Context, dto.GenerateDispositionAdvice, string, string, string) (model.DispositionAdvice, bool, error)
	Delete(context.Context, uint, string, string) error
	StatusCounts(context.Context) (map[string]int64, error)
	LevelCounts(context.Context) (map[string]int64, error)
}

type dispositionAdviceService struct {
	repository repository.DispositionAdviceRepository
	security   SecurityService
}

func NewDispositionAdviceService(repo repository.DispositionAdviceRepository, security SecurityService) DispositionAdviceService {
	return &dispositionAdviceService{repository: repo, security: security}
}

func (s *dispositionAdviceService) List(ctx context.Context, query dto.PageQuery) (repository.Page[model.DispositionAdvice], error) {
	return s.repository.List(ctx, query)
}

func (s *dispositionAdviceService) Get(ctx context.Context, id uint) (model.DispositionAdvice, error) {
	return s.repository.Get(ctx, id)
}

// Generate locks the defect, its bridge and the latest inspection round in one
// transaction, validates that the inspection reached a conclusion, resolves the
// disposition level and persists advice plus source snapshot atomically. The
// boolean is true when an advice already existed (repeat verification): the same
// record is returned unchanged.
func (s *dispositionAdviceService) Generate(ctx context.Context, input dto.GenerateDispositionAdvice, actor, role, requestID string) (model.DispositionAdvice, bool, error) {
	if role != model.RoleReviewer && role != model.RoleAdmin {
		return model.DispositionAdvice{}, false, ErrReviewRole
	}

	advice, existed, err := s.repository.GenerateInTx(ctx, input.DefectID,
		func(sources repository.LockedSources) (model.DispositionAdvice, model.DispositionSourceSnapshot, error) {
			defect := sources.Defect
			if defect.Status != string(constants.DefectStateVerified) {
				return model.DispositionAdvice{}, model.DispositionSourceSnapshot{},
					fmt.Errorf("%w: defect %s is %s", ErrDefectNotVerified, defect.Code, defect.Status)
			}
			if sources.Bridge == nil {
				return model.DispositionAdvice{}, model.DispositionSourceSnapshot{},
					fmt.Errorf("%w: defect %s related code %s", ErrBridgeLinkMissing, defect.Code, defect.RelatedCode)
			}
			if sources.Inspection == nil || sources.Inspection.Status != "completed" {
				state := "missing"
				if sources.Inspection != nil {
					state = sources.Inspection.Status
				}
				return model.DispositionAdvice{}, model.DispositionSourceSnapshot{},
					fmt.Errorf("%w: round state %s", ErrInspectionIncomplete, state)
			}

			bridge := sources.Bridge
			inspection := sources.Inspection
			grade := constants.RiskLevelToDefectGrade(defect.RiskLevel)
			level, reason, err := resolveDispositionLevel(grade, bridge.Status)
			if err != nil {
				return model.DispositionAdvice{}, model.DispositionSourceSnapshot{}, err
			}

			snapshot, err := newDispositionSnapshot(defect, bridge, inspection, grade)
			if err != nil {
				return model.DispositionAdvice{}, model.DispositionSourceSnapshot{}, err
			}
			now := time.Now().UTC()
			advice := model.DispositionAdvice{
				Code:              dispositionAdviceCode(defect.Code),
				Status:            model.DispositionAdviceInitialStatus,
				DefectID:          defect.ID,
				DefectCode:        defect.Code,
				BridgeID:          bridge.ID,
				BridgeCode:        bridge.Code,
				InspectionRoundID: inspection.ID,
				InspectionCode:    inspection.Code,
				DefectGrade:       string(grade),
				RiskLevel:         defect.RiskLevel,
				BridgeState:       bridge.Status,
				DispositionLevel:  string(level),
				Reason:            reason,
				ReviewedBy:        actor,
				RequestID:         requestID,
				Version:           1,
				CreatedAt:         now,
				UpdatedAt:         now,
				Snapshot:          snapshot,
			}
			return advice, snapshot, nil
		},
		func(tx *gorm.DB, advice model.DispositionAdvice) error {
			return tx.Create(&model.AuditLog{
				Actor: actor, RequestID: requestID, Action: "generate", EntityType: "DispositionAdvice",
				EntityID: advice.ID, BeforeState: "", AfterState: advice.DispositionLevel,
				Detail: fmt.Sprintf("defect=%s bridge=%s inspection=%s grade=%s",
					advice.DefectCode, advice.BridgeCode, advice.InspectionCode, advice.DefectGrade),
				CreatedAt: time.Now().UTC(),
			}).Error
		},
	)
	if err != nil {
		return model.DispositionAdvice{}, false, err
	}

	if existed {
		// Repeat verification only appends a lightweight audit trail; it never
		// rewrites the frozen advice or snapshot.
		if auditErr := s.security.Audit(ctx, actor, requestID, "regenerate_skipped", "DispositionAdvice", advice.ID, "", advice.DispositionLevel,
			fmt.Sprintf("defect=%s bridge=%s inspection=%s grade=%s", advice.DefectCode, advice.BridgeCode, advice.InspectionCode, advice.DefectGrade)); auditErr != nil {
			return model.DispositionAdvice{}, false, fmt.Errorf("persist disposition audit: %w", auditErr)
		}
	}
	return advice, existed, nil
}

func (s *dispositionAdviceService) Delete(ctx context.Context, id uint, actor, requestID string) error {
	current, err := s.repository.Get(ctx, id)
	if err != nil {
		return err
	}
	return s.repository.Delete(ctx, id, func(tx *gorm.DB, advice model.DispositionAdvice) error {
		return tx.Create(&model.AuditLog{
			Actor: actor, RequestID: requestID, Action: "delete", EntityType: "DispositionAdvice",
			EntityID: advice.ID, BeforeState: current.Status, AfterState: "deleted",
			Detail: "deleted 缺陷处置优先级建议", CreatedAt: time.Now().UTC(),
		}).Error
	})
}

func (s *dispositionAdviceService) StatusCounts(ctx context.Context) (map[string]int64, error) {
	return s.repository.CountByStatus(ctx)
}

func (s *dispositionAdviceService) LevelCounts(ctx context.Context) (map[string]int64, error) {
	return s.repository.CountByDispositionLevel(ctx)
}

// resolveDispositionLevel keeps the three domain rules explicit:
//   - general defects always stay on observe;
//   - serious defects become urgent after the bridge is restricted/closed;
//   - a serious defect on an active bridge is restrict (limit first, dispose next).
func resolveDispositionLevel(grade constants.DefectGradeClass, bridgeState string) (constants.PriorityLevel, string, error) {
	if grade == constants.DefectGradeGeneral {
		return constants.PriorityLevelObserve, "一般缺陷保持观察，按检查批次持续跟踪", nil
	}
	switch bridgeState {
	case "active":
		return constants.PriorityLevelRestrict, "严重缺陷且桥梁尚未限行，建议先行限行管控", nil
	case "restricted", "closed":
		return constants.PriorityLevelUrgent, "严重缺陷且桥梁已" + bridgeStateInChinese(bridgeState) + "，升级为立即处置", nil
	default:
		return "", "", fmt.Errorf("%w: bridge state %s", ErrBridgeNotDisposable, bridgeState)
	}
}

func bridgeStateInChinese(state string) string {
	switch state {
	case "restricted":
		return "限行"
	case "closed":
		return "封闭"
	default:
		return state
	}
}

type dispositionSnapshotPayload struct {
	Bridge      model.BridgeAsset     `json:"bridge"`
	Defect      model.DefectFinding   `json:"defect"`
	Inspection  model.InspectionRound `json:"inspection"`
	DefectGrade string                `json:"defectGrade"`
	ResolvedAt  time.Time             `json:"resolvedAt"`
}

func newDispositionSnapshot(defect model.DefectFinding, bridge *model.BridgeAsset, inspection *model.InspectionRound, grade constants.DefectGradeClass) (model.DispositionSourceSnapshot, error) {
	payload, err := json.Marshal(dispositionSnapshotPayload{
		Bridge: *bridge, Defect: defect, Inspection: *inspection,
		DefectGrade: string(grade), ResolvedAt: time.Now().UTC(),
	})
	if err != nil {
		return model.DispositionSourceSnapshot{}, fmt.Errorf("serialize disposition source snapshot: %w", err)
	}
	return model.DispositionSourceSnapshot{
		BridgeState:       bridge.Status,
		BridgeStatus:      bridge.Status,
		BridgeVersion:     bridge.Version,
		DefectState:       string(constants.DefectStateVerified),
		DefectStatus:      defect.Status,
		DefectGrade:       string(grade),
		DefectRiskLevel:   defect.RiskLevel,
		DefectVersion:     defect.Version,
		InspectionStatus:  inspection.Status,
		InspectionVersion: inspection.Version,
		Payload:           string(payload),
		CreatedAt:         time.Now().UTC(),
	}, nil
}

func dispositionAdviceCode(defectCode string) string {
	trimmed := strings.ToUpper(strings.TrimSpace(defectCode))
	return "DA-" + strings.TrimPrefix(trimmed, "DF-")
}
