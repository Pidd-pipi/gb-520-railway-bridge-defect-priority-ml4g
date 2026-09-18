<script setup lang="ts">
import { computed } from 'vue';
import type { HandlingAdviceSnapshot } from '../../types/handling-advice';
import StatusBadge from './StatusBadge.vue';
import { formatDate } from '../../utils/format';

const props = defineProps<{ snapshot: HandlingAdviceSnapshot | null | undefined }>();

interface SnapshotRecord {
  code?: string;
  name?: string;
  status?: string;
  version?: number;
  riskLevel?: string;
  owner?: string;
  facility?: string;
  effectiveAt?: string;
  evidence?: string;
}

function parse(raw: string | undefined): SnapshotRecord | null {
  if (!raw) return null;
  try { return JSON.parse(raw) as SnapshotRecord; } catch { return null; }
}

const bridge = computed(() => parse(props.snapshot?.bridgeSnapshot));
const defect = computed(() => parse(props.snapshot?.defectSnapshot));
const inspection = computed(() => parse(props.snapshot?.inspectionSnapshot));

const sections = computed(() => [
  { key: 'bridge', title: '来源桥梁', record: bridge.value, lockedStatus: props.snapshot?.bridgeStatus },
  { key: 'defect', title: '来源缺陷', record: defect.value, lockedStatus: props.snapshot?.defectStatus },
  { key: 'inspection', title: '最近检查批次', record: inspection.value, lockedStatus: props.snapshot?.inspectionStatus },
]);
</script>

<template>
  <section v-if="snapshot" class="snapshot-panel">
    <header>
      <strong>来源快照</strong>
      <span>生成建议时冻结 · {{ formatDate(snapshot.capturedAt) }}</span>
    </header>
    <div class="snapshot-grid">
      <article v-for="section in sections" :key="section.key">
        <header>
          <strong>{{ section.title }}</strong>
          <StatusBadge :status="section.lockedStatus || ''"/>
        </header>
        <template v-if="section.record">
          <p><code>{{ section.record.code }}</code> · {{ section.record.name }}</p>
          <small>责任方：{{ section.record.owner || '-' }}</small>
          <small>位置：{{ section.record.facility || '-' }}</small>
          <small v-if="section.record.effectiveAt">生效时间：{{ formatDate(section.record.effectiveAt) }}</small>
          <small v-if="section.record.riskLevel">风险等级：{{ section.record.riskLevel }}</small>
          <small class="snapshot-evidence">证据：{{ section.record.evidence || '-' }}</small>
        </template>
        <small v-else class="muted">快照内容无法解析</small>
      </article>
    </div>
  </section>
</template>

<style scoped>
.snapshot-panel { background: white; border: 1px solid #dbe4e8; padding: 14px; margin-top: 14px; }
.snapshot-panel > header { display: flex; justify-content: space-between; align-items: center; margin-bottom: 10px; gap: 12px; flex-wrap: wrap; }
.snapshot-panel > header span { color: #768893; font-size: 12px; }
.snapshot-grid { display: grid; grid-template-columns: repeat(3, minmax(0, 1fr)); gap: 10px; }
.snapshot-grid article { min-width: 0; border: 1px solid #dbe4e8; padding: 12px; display: grid; gap: 5px; align-content: start; }
.snapshot-grid article > header { display: flex; justify-content: space-between; gap: 8px; align-items: center; margin-bottom: 4px; }
.snapshot-grid p { margin: 0; font-weight: 650; overflow-wrap: anywhere; }
.snapshot-grid code { color: #285c77; }
.snapshot-grid small { color: #667886; overflow-wrap: anywhere; }
.snapshot-evidence { border-top: 1px dashed #dbe4e8; padding-top: 5px; }
@media (max-width: 900px) { .snapshot-grid { grid-template-columns: minmax(0, 1fr); } }
</style>
