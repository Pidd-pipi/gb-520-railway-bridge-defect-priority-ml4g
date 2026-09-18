export interface DispositionSourceSnapshot {
  id: number;
  dispositionAdviceId: number;
  bridgeState: string;
  bridgeStatus: string;
  bridgeVersion: number;
  defectState: string;
  defectStatus: string;
  defectGrade: DefectGrade;
  defectRiskLevel: string;
  defectVersion: number;
  inspectionStatus: string;
  inspectionVersion: number;
  payload: string;
  createdAt: string;
}

export type DispositionLevel = 'observe' | 'restrict' | 'urgent';
export type DefectGrade = 'general' | 'serious';
export type DispositionAdviceStatus = 'generated';

export interface DispositionAdvice {
  id: number;
  code: string;
  status: DispositionAdviceStatus;
  defectId: number;
  defectCode: string;
  bridgeId: number;
  bridgeCode: string;
  inspectionRoundId: number;
  inspectionCode: string;
  defectGrade: DefectGrade;
  riskLevel: string;
  bridgeState: string;
  dispositionLevel: DispositionLevel;
  reason: string;
  reviewedBy: string;
  requestId: string;
  version: number;
  createdAt: string;
  updatedAt: string;
  snapshot: DispositionSourceSnapshot;
}

export interface GenerateAdviceResponse {
  advice: DispositionAdvice;
  alreadyExisted: boolean;
}
