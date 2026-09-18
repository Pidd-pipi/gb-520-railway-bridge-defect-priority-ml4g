export type DefectGrade = 'general' | 'severe';
export type HandlingLevel = 'observe' | 'restrict' | 'urgent';

export interface HandlingAdviceSnapshot {
  id: number;
  adviceId: number;
  bridgeSnapshot: string;
  defectSnapshot: string;
  inspectionSnapshot: string;
  bridgeStatus: string;
  defectStatus: string;
  defectGrade: DefectGrade;
  inspectionStatus: string;
  resolvedHandlingLevel: HandlingLevel;
  capturedAt: string;
}

export interface HandlingAdvice {
  id: number;
  code: string;
  defectId: number;
  defectCode: string;
  bridgeId: number;
  bridgeCode: string;
  inspectionId: number;
  inspectionCode: string;
  defectGrade: DefectGrade;
  handlingLevel: HandlingLevel;
  status: string;
  version: number;
  ruleSummary: string;
  verifiedBy: string;
  requestId: string;
  snapshot: HandlingAdviceSnapshot;
  createdAt: string;
  updatedAt: string;
}

export interface GenerateHandlingAdviceInput {
  defectCode: string;
  reason: string;
}

export interface HandlingAdviceQuery {
  page?: number;
  pageSize?: number;
  search?: string;
  defectCode?: string;
  handlingLevel?: HandlingLevel | '';
  defectGrade?: DefectGrade | '';
}
