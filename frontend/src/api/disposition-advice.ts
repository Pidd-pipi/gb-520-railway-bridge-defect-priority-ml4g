import { request } from './client';
import type { DispositionAdvice, GenerateAdviceResponse } from '../types/disposition-advice';
import type { PageMeta } from '../types/domain';

export interface DispositionAdviceQuery {
  page?: number;
  pageSize?: number;
  search?: string;
  dispositionLevel?: string;
}

export async function listDispositionAdvices(query: DispositionAdviceQuery = {}) {
  const params = new URLSearchParams();
  params.set('page', String(query.page ?? 1));
  params.set('pageSize', String(query.pageSize ?? 20));
  params.set('search', query.search ?? '');
  if (query.dispositionLevel) params.set('status', query.dispositionLevel);
  const response = await request<DispositionAdvice[]>(`/disposition-advices?${params.toString()}`);
  return { items: response.data, meta: response.meta as PageMeta | undefined };
}

export async function getDispositionAdvice(id: number) {
  return request<DispositionAdvice>(`/disposition-advices/${id}`);
}

export async function generateDispositionAdvice(defectId: number) {
  return request<GenerateAdviceResponse>('/disposition-advices/generate', {
    method: 'POST',
    body: JSON.stringify({ defectId }),
  });
}
