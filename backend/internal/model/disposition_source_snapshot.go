package model

import "time"

// DispositionSourceSnapshot freezes the exact bridge state, defect grade and
// inspection conclusion used to resolve a DispositionAdvice. It is inserted in
// the same transaction as the advice; a failed write leaves neither row behind.
type DispositionSourceSnapshot struct {
	ID                  uint      `json:"id" gorm:"primaryKey"`
	DispositionAdviceID uint      `json:"dispositionAdviceId" gorm:"uniqueIndex;not null"`
	BridgeState         string    `json:"bridgeState" gorm:"size:40;not null"`
	BridgeStatus        string    `json:"bridgeStatus" gorm:"size:40;not null"`
	BridgeVersion       uint      `json:"bridgeVersion" gorm:"not null"`
	DefectState         string    `json:"defectState" gorm:"size:40;not null"`
	DefectStatus        string    `json:"defectStatus" gorm:"size:40;not null"`
	DefectGrade         string    `json:"defectGrade" gorm:"size:24;not null"`
	DefectRiskLevel     string    `json:"defectRiskLevel" gorm:"size:32;not null"`
	DefectVersion       uint      `json:"defectVersion" gorm:"not null"`
	InspectionStatus    string    `json:"inspectionStatus" gorm:"size:40;not null"`
	InspectionVersion   uint      `json:"inspectionVersion" gorm:"not null"`
	Payload             string    `json:"payload" gorm:"type:text;not null"`
	CreatedAt           time.Time `json:"createdAt"`
}

func (item DispositionSourceSnapshot) TableName() string { return "disposition_source_snapshots" }
