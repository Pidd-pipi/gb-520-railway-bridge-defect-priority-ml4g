package dto

// GenerateDispositionAdvice is the write contract for 缺陷处置优先级建议. The
// reviewer only identifies the verified defect; the level is always resolved by
// the service from the current bridge state, defect grade and latest inspection
// conclusion, so callers can never fabricate a priority.
type GenerateDispositionAdvice struct {
	DefectID uint `json:"defectId" binding:"required"`
}
