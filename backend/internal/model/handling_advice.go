package model

import "time"

// DefectHandlingAdvice models 缺陷处置优先级建议. A piece of advice is derived
// exactly once per defect (unique DefectID) from a locked source snapshot:
// bridge state, defect grade and the latest completed inspection round. The
// aggregate is append-only in spirit — repeated verification of the same
// defect returns the existing advice instead of creating a second row.
type DefectHandlingAdvice struct {
	ID             uint                 `json:"id" gorm:"primaryKey"`
	Code           string               `json:"code" gorm:"size:64;uniqueIndex;not null"`
	DefectID       uint                 `json:"defectId" gorm:"uniqueIndex;not null"`
	DefectCode     string               `json:"defectCode" gorm:"size:64;index;not null"`
	BridgeID       uint                 `json:"bridgeId" gorm:"index;not null"`
	BridgeCode     string               `json:"bridgeCode" gorm:"size:64;index;not null"`
	InspectionID   uint                 `json:"inspectionId" gorm:"index;not null"`
	InspectionCode string               `json:"inspectionCode" gorm:"size:64;index;not null"`
	DefectGrade    string               `json:"defectGrade" gorm:"size:32;index;not null"`
	HandlingLevel  string               `json:"handlingLevel" gorm:"size:32;index;not null"`
	Status         string               `json:"status" gorm:"size:40;index;not null"`
	Version        uint                 `json:"version" gorm:"not null;default:1"`
	RuleSummary    string               `json:"ruleSummary" gorm:"size:500;not null"`
	VerifiedBy     string               `json:"verifiedBy" gorm:"size:80;index;not null"`
	RequestID      string               `json:"requestId" gorm:"size:64;index;not null"`
	Snapshot       DefectAdviceSnapshot `json:"snapshot" gorm:"foreignKey:AdviceID;constraint:OnDelete:CASCADE"`
	CreatedAt      time.Time            `json:"createdAt"`
	UpdatedAt      time.Time            `json:"updatedAt"`
}

func (item *DefectHandlingAdvice) TableName() string { return "defect_handling_advices" }

var DefectHandlingAdviceStatus = "issued"

// DefectAdviceSnapshot freezes the exact source records the handling level was
// derived from. It is written in the same transaction as the advice and never
// mutated afterwards, which prevents mixing old and new bridge/inspection
// state when those records change concurrently.
type DefectAdviceSnapshot struct {
	ID                    uint      `json:"id" gorm:"primaryKey"`
	AdviceID              uint      `json:"adviceId" gorm:"uniqueIndex;not null"`
	BridgeSnapshot        string    `json:"bridgeSnapshot" gorm:"type:text;not null"`
	DefectSnapshot        string    `json:"defectSnapshot" gorm:"type:text;not null"`
	InspectionSnapshot    string    `json:"inspectionSnapshot" gorm:"type:text;not null"`
	BridgeStatus          string    `json:"bridgeStatus" gorm:"size:40;index;not null"`
	DefectStatus          string    `json:"defectStatus" gorm:"size:40;index;not null"`
	DefectGrade           string    `json:"defectGrade" gorm:"size:32;index;not null"`
	InspectionStatus      string    `json:"inspectionStatus" gorm:"size:40;index;not null"`
	ResolvedHandlingLevel string    `json:"resolvedHandlingLevel" gorm:"size:32;not null"`
	CapturedAt            time.Time `json:"capturedAt"`
}

func (item *DefectAdviceSnapshot) TableName() string { return "defect_advice_snapshots" }
