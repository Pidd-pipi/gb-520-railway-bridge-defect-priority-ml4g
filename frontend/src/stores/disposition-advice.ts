import { defineStore } from 'pinia';
import {
  generateDispositionAdvice,
  getDispositionAdvice,
  listDispositionAdvices,
} from '../api/disposition-advice';
import type {
  DispositionAdvice,
  DispositionLevel,
} from '../types/disposition-advice';
import type { PageMeta } from '../types/domain';

interface DispositionAdviceState {
  items: DispositionAdvice[];
  meta: PageMeta;
  loading: boolean;
  generating: boolean;
  error: string;
  notice: string;
}

export const useDispositionAdviceStore = defineStore('dispositionAdvice', {
  state: (): DispositionAdviceState => ({
    items: [],
    meta: { page: 1, pageSize: 20, total: 0 },
    loading: false,
    generating: false,
    error: '',
    notice: '',
  }),
  getters: {
    urgentCount: (state) => state.items.filter((item) => item.dispositionLevel === 'urgent').length,
    seriousCount: (state) => state.items.filter((item) => item.defectGrade === 'serious').length,
  },
  actions: {
    async load(search = '', dispositionLevel: DispositionLevel | '' = '') {
      this.loading = true;
      this.error = '';
      try {
        const { items, meta } = await listDispositionAdvices({ search, dispositionLevel });
        this.items = items;
        this.meta = meta ?? { page: 1, pageSize: 20, total: items.length };
      } catch (error) {
        this.error = error instanceof Error ? error.message : String(error);
      } finally {
        this.loading = false;
      }
    },
    async loadOne(id: number) {
      this.error = '';
      try {
        const response = await getDispositionAdvice(id);
        return response.data;
      } catch (error) {
        this.error = error instanceof Error ? error.message : String(error);
        return undefined;
      }
    },
    async generate(defectId: number) {
      this.generating = true;
      this.error = '';
      this.notice = '';
      try {
        const response = await generateDispositionAdvice(defectId);
        const advice = response.data.advice;
        this.notice = response.data.alreadyExisted
          ? `该缺陷已有建议 ${advice.code}，重复核验未重复生成`
          : `已生成建议 ${advice.code}：${advice.reason}`;
        await this.load();
        return advice;
      } catch (error) {
        this.error = error instanceof Error ? error.message : String(error);
        return undefined;
      } finally {
        this.generating = false;
      }
    },
  },
});
