package dto

// GenerateHandlingAdvice is the only write contract for 缺陷处置优先级建议. The
// caller identifies a verified defect by code; bridge state, defect grade and
// inspection round are all locked server-side, never supplied by the client.
type GenerateHandlingAdvice struct {
	DefectCode string `json:"defectCode" binding:"required,min=2,max=64"`
	Reason     string `json:"reason" binding:"required,min=3,max=500"`
}

// HandlingAdviceQuery supports the page query form plus filtering by the
// derived handling level and defect grade.
type HandlingAdviceQuery struct {
	Page          int    `form:"page"`
	PageSize      int    `form:"pageSize"`
	Search        string `form:"search"`
	DefectCode    string `form:"defectCode"`
	HandlingLevel string `form:"handlingLevel"`
	DefectGrade   string `form:"defectGrade"`
}
