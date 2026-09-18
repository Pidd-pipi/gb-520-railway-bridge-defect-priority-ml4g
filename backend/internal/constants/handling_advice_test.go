package constants

import "testing"

func TestResolveHandlingLevel(t *testing.T) {
	cases := []struct {
		name   string
		grade  DefectGrade
		bridge string
		want   HandlingLevel
	}{
		{"一般缺陷-桥梁通行保持观察", DefectGradeGeneral, "active", HandlingLevelObserve},
		{"一般缺陷-桥梁限行仍保持观察", DefectGradeGeneral, "restricted", HandlingLevelObserve},
		{"严重缺陷-桥梁通行建议限行", DefectGradeSevere, "active", HandlingLevelRestrict},
		{"严重缺陷-桥梁限行升级立即处置", DefectGradeSevere, "restricted", HandlingLevelUrgent},
		{"严重缺陷-桥梁封闭升级立即处置", DefectGradeSevere, "closed", HandlingLevelUrgent},
		{"严重缺陷-桥梁退役不再升级", DefectGradeSevere, "retired", HandlingLevelRestrict},
	}
	for _, tc := range cases {
		if got := ResolveHandlingLevel(tc.grade, tc.bridge); got != tc.want {
			t.Fatalf("%s: got %s want %s", tc.name, got, tc.want)
		}
	}
}

func TestRiskLevelToDefectGrade(t *testing.T) {
	if RiskLevelToDefectGrade["critical"] != DefectGradeSevere || RiskLevelToDefectGrade["high"] != DefectGradeSevere {
		t.Fatal("high/critical risk levels must map to severe defect grade")
	}
	if RiskLevelToDefectGrade["medium"] != DefectGradeGeneral || RiskLevelToDefectGrade["low"] != DefectGradeGeneral {
		t.Fatal("medium/low risk levels must map to general defect grade")
	}
}
