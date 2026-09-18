package model

import "time"

// DispositionAdvice models 缺陷处置优先级建议 as a one-to-one aggregate keyed by
// DefectID. The level is resolved from the verified defect grade, current bridge
// state and the latest inspection round; a recommendation exists at most once per
// defect, so repeat verification of the same defect never creates duplicates.
type DispositionAdvice struct {
	ID                uint                      `json:"id" gorm:"primaryKey"`
	Code              string                    `json:"code" gorm:"size:64;uniqueIndex;not null"`
	Status            string                    `json:"status" gorm:"size:40;index;not null"`
	DefectID          uint                      `json:"defectId" gorm:"uniqueIndex;not null"`
	DefectCode        string                    `json:"defectCode" gorm:"size:64;index;not null"`
	BridgeID          uint                      `json:"bridgeId" gorm:"index;not null"`
	BridgeCode        string                    `json:"bridgeCode" gorm:"size:64;index;not null"`
	InspectionRoundID uint                      `json:"inspectionRoundId" gorm:"index;not null"`
	InspectionCode    string                    `json:"inspectionCode" gorm:"size:64;index;not null"`
	DefectGrade       string                    `json:"defectGrade" gorm:"size:24;index;not null"`
	RiskLevel         string                    `json:"riskLevel" gorm:"size:32;index;not null"`
	BridgeState       string                    `json:"bridgeState" gorm:"size:40;index;not null"`
	DispositionLevel  string                    `json:"dispositionLevel" gorm:"size:32;index;not null"`
	Reason            string                    `json:"reason" gorm:"size:500;not null"`
	ReviewedBy        string                    `json:"reviewedBy" gorm:"size:80;index;not null"`
	RequestID         string                    `json:"requestId" gorm:"size:64;index;not null"`
	Version           uint                      `json:"version" gorm:"not null;default:1"`
	CreatedAt         time.Time                 `json:"createdAt" gorm:"index"`
	UpdatedAt         time.Time                 `json:"updatedAt"`
	Snapshot          DispositionSourceSnapshot `json:"snapshot" gorm:"foreignKey:DispositionAdviceID;constraint:OnDelete:CASCADE"`
}

func (item *DispositionAdvice) GetBase() *BaseModel {
	return &BaseModel{
		ID: item.ID, Code: item.Code, Name: item.DefectCode + " 处置建议",
		Status: item.Status, Version: item.Version, CreatedAt: item.CreatedAt, UpdatedAt: item.UpdatedAt,
	}
}

func (item DispositionAdvice) TableName() string { return "disposition_advices" }

// DispositionAdviceInitialStatus is the only lifecycle state: an advice is born
// finalized together with its source snapshot.
var DispositionAdviceInitialStatus = "generated"
