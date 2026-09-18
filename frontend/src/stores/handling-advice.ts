import { defineStore } from 'pinia';
import { generateHandlingAdvice, getHandlingAdvice, listHandlingAdvices } from '../api/handling-advice';
import type { HandlingAdvice, HandlingAdviceQuery } from '../types/handling-advice';
import type { PageMeta } from '../types/domain';

interface HandlingAdviceState {
  items: HandlingAdvice[];
  meta: PageMeta;
  query: Required<Pick<HandlingAdviceQuery, 'search' | 'defectCode' | 'handlingLevel' | 'defectGrade'>>;
  selected: HandlingAdvice | null;
  loading: boolean;
  generating: boolean;
  error: string;
}

export const useHandlingAdviceStore = defineStore('handlingAdvice', {
  state: (): HandlingAdviceState => ({
    items: [],
    meta: { page: 1, pageSize: 20, total: 0 },
    query: { search: '', defectCode: '', handlingLevel: '', defectGrade: '' },
    selected: null,
    loading: false,
    generating: false,
    error: '',
  }),
  actions: {
    async load(overrides: HandlingAdviceQuery = {}) {
      Object.assign(this.query, Object.fromEntries(Object.entries(overrides).filter(([, value]) => value !== undefined)));
      this.loading = true;
      this.error = '';
      try {
        const page = await listHandlingAdvices(this.query);
        this.items = page.items;
        this.meta = page.meta;
      } catch (error) {
        this.error = error instanceof Error ? error.message : String(error);
      } finally {
        this.loading = false;
      }
    },
    async select(id: number) {
      this.loading = true;
      this.error = '';
      try {
        // 回读建议与来源快照，确保展示的是服务端最终落库状态。
        const result = await getHandlingAdvice(id);
        this.selected = result.data;
      } catch (error) {
        this.error = error instanceof Error ? error.message : String(error);
      } finally {
        this.loading = false;
      }
    },
    clearSelection() {
      this.selected = null;
    },
    async verifyAndGenerate(defectCode: string, reason: string): Promise<boolean> {
      this.generating = true;
      this.error = '';
      try {
        const result = await generateHandlingAdvice(defectCode.trim().toUpperCase(), reason.trim());
        this.selected = result.data;
        await this.load();
        return true;
      } catch (error) {
        this.error = error instanceof Error ? error.message : String(error);
        return false;
      } finally {
        this.generating = false;
      }
    },
  },
});
