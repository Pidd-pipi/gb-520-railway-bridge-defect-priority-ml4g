package constants

// HandlingAdviceGrade and HandlingLevel are mirrored in
// frontend/src/types/status.ts. The advice module derives both values from a
// single locked snapshot, so the lists stay deliberately small and explicit.

type DefectGrade string

const (
	DefectGradeGeneral DefectGrade = "general"
	DefectGradeSevere  DefectGrade = "severe"
)

var AllDefectGrade = []string{"general", "severe"}

// RiskLevelToDefectGrade locks how a 缺陷风险等级 maps to a 缺陷等级. high/critical
// defects are structural or safety critical; medium/low stay under observation.
var RiskLevelToDefectGrade = map[string]DefectGrade{
	"low":      DefectGradeGeneral,
	"medium":   DefectGradeGeneral,
	"high":     DefectGradeSevere,
	"critical": DefectGradeSevere,
}

type HandlingLevel string

const (
	HandlingLevelObserve  HandlingLevel = "observe"
	HandlingLevelRestrict HandlingLevel = "restrict"
	HandlingLevelUrgent   HandlingLevel = "urgent"
)

var AllHandlingLevel = []string{"observe", "restrict", "urgent"}

// RestrictedBridgeStatuses marks bridge states where traffic is already limited
// (限行). A severe defect on one of these bridges escalates to immediate
// handling instead of a mere restriction recommendation.
var RestrictedBridgeStatuses = map[string]bool{
	"restricted": true,
	"closed":     true,
}

// ResolveHandlingLevel is the single rule table used by the advice service:
//   - 一般缺陷 (general) keeps observation regardless of bridge state.
//   - 严重缺陷 (severe) recommends restriction while the bridge is open, and is
//     escalated to 立即处置 (urgent) once the bridge is under traffic control.
//
// Retired bridges no longer carry live traffic; a severe defect on them stays
// at restriction level pending formal closure review.
func ResolveHandlingLevel(grade DefectGrade, bridgeStatus string) HandlingLevel {
	if grade != DefectGradeSevere {
		return HandlingLevelObserve
	}
	if RestrictedBridgeStatuses[bridgeStatus] {
		return HandlingLevelUrgent
	}
	return HandlingLevelRestrict
}
