import { request } from './client';
import type { HandlingAdvice, HandlingAdviceQuery } from '../types/handling-advice';
import type { PageMeta } from '../types/domain';

interface AdvicePage { items: HandlingAdvice[]; meta: PageMeta }

function buildQuery(query: HandlingAdviceQuery): string {
  const params = new URLSearchParams();
  params.set('page', String(query.page ?? 1));
  params.set('pageSize', String(query.pageSize ?? 20));
  if (query.search) params.set('search', query.search);
  if (query.defectCode) params.set('defectCode', query.defectCode);
  if (query.handlingLevel) params.set('handlingLevel', query.handlingLevel);
  if (query.defectGrade) params.set('defectGrade', query.defectGrade);
  return params.toString();
}

export async function listHandlingAdvices(query: HandlingAdviceQuery = {}) {
  const result = await request<HandlingAdvice[]>(`/handling-advices?${buildQuery(query)}`);
  return { items: result.data, meta: result.meta || { page: 1, pageSize: 20, total: result.data.length } } as AdvicePage;
}

export async function getHandlingAdvice(id: number) {
  return request<HandlingAdvice>(`/handling-advices/${id}`);
}

// 复核员核验缺陷并生成处置优先级建议；重复核验返回同一份建议（HTTP 200）。
export async function generateHandlingAdvice(defectCode: string, reason: string) {
  return request<HandlingAdvice>('/handling-advices/generate', {
    method: 'POST',
    body: JSON.stringify({ defectCode, reason }),
  });
}
