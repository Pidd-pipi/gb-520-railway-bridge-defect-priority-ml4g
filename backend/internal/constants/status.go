package constants

// Shared status values are mirrored in frontend/src/types/status.ts. Keeping
// the lists explicit makes state-machine drift visible during code review.

type DefectState string

const (
	DefectStateNew        DefectState = "new"
	DefectStateVerified   DefectState = "verified"
	DefectStateMonitoring DefectState = "monitoring"
	DefectStateMitigated  DefectState = "mitigated"
	DefectStateClosed     DefectState = "closed"
)

var AllDefectState = []string{"new", "verified", "monitoring", "mitigated", "closed"}

type PriorityLevel string

const (
	PriorityLevelObserve  PriorityLevel = "observe"
	PriorityLevelRestrict PriorityLevel = "restrict"
	PriorityLevelUrgent   PriorityLevel = "urgent"
)

var AllPriorityLevel = []string{"observe", "restrict", "urgent"}

// DispositionAdviceLifecycleState 缺陷处置优先级建议只有一个生命周期状态：
// 生成即定稿（与快照同一事务写入），重复核验不产生第二份建议。
type DispositionAdviceLifecycleState string

const (
	DispositionAdviceStateGenerated DispositionAdviceLifecycleState = "generated"
)

var AllDispositionAdviceState = []string{"generated"}

// DefectGradeClass 按缺陷等级划分的处置档位来源：严重缺陷可被桥梁限行升级，
// 一般缺陷始终保持观察。
type DefectGradeClass string

const (
	DefectGradeGeneral DefectGradeClass = "general"
	DefectGradeSerious DefectGradeClass = "serious"
)

// RiskLevelToDefectGrade 将平台统一的风险等级映射为处置判定使用的缺陷等级。
func RiskLevelToDefectGrade(riskLevel string) DefectGradeClass {
	switch riskLevel {
	case "high", "critical":
		return DefectGradeSerious
	default:
		return DefectGradeGeneral
	}
}

var BridgeAssetTransitions = map[string]map[string]bool{
	"active":     {"restricted": true, "closed": true},
	"restricted": {"closed": true, "retired": true, "active": true},
	"closed":     {"retired": true, "restricted": true},
	"retired":    {"closed": true},
}

var InspectionRoundTransitions = map[string]map[string]bool{
	"planned":   {"running": true, "review": true},
	"running":   {"review": true, "completed": true, "planned": true},
	"review":    {"completed": true, "running": true},
	"completed": {"review": true},
}

var DefectFindingTransitions = map[string]map[string]bool{
	"new":        {"verified": true, "monitoring": true},
	"verified":   {"monitoring": true, "mitigated": true, "new": true},
	"monitoring": {"mitigated": true, "closed": true, "verified": true},
	"mitigated":  {"closed": true, "monitoring": true},
	"closed":     {"mitigated": true},
}

var PriorityDecisionTransitions = map[string]map[string]bool{
	"draft":    {"observe": true, "restrict": true, "urgent": true},
	"observe":  {},
	"restrict": {},
	"urgent":   {},
}

func CanTransition(graph map[string]map[string]bool, from, to string) bool {
	targets, exists := graph[from]
	return exists && targets[to]
}
