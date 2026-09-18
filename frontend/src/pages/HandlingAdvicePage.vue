<script setup lang="ts">
import { computed, onMounted, reactive, ref } from 'vue';
import { storeToRefs } from 'pinia';
import { useHandlingAdviceStore } from '../stores/handling-advice';
import { useAuth } from '../hooks/useAuth';
import HandlingLevelBadge from '../components/common/HandlingLevelBadge.vue';
import DefectGradeBadge from '../components/common/DefectGradeBadge.vue';
import StatusBadge from '../components/common/StatusBadge.vue';
import MetricCard from '../components/common/MetricCard.vue';
import SourceSnapshot from '../components/common/SourceSnapshot.vue';
import { formatDate } from '../utils/format';
import type { HandlingAdvice } from '../types/handling-advice';

const store = useHandlingAdviceStore();
const { items, meta, loading, generating, error, selected, query } = storeToRefs(store);
const { canAtLeast, session } = useAuth();
const canVerify = computed(() => canAtLeast('reviewer'));

const filters = reactive({
  search: '',
  defectCode: '',
  handlingLevel: '' as '' | 'observe' | 'restrict' | 'urgent',
  defectGrade: '' as '' | 'general' | 'severe',
});

const verifyForm = reactive({ defectCode: '', reason: '' });
const showVerify = ref(false);
const formError = ref('');

const urgentCount = computed(() => items.value.filter((item) => item.handlingLevel === 'urgent').length);
const severeCount = computed(() => items.value.filter((item) => item.defectGrade === 'severe').length);

onMounted(() => { void store.load(); });

function submitQuery() {
  void store.load({
    search: filters.search.trim(),
    defectCode: filters.defectCode.trim().toUpperCase(),
    handlingLevel: filters.handlingLevel,
    defectGrade: filters.defectGrade,
  });
}

function resetQuery() {
  filters.search = '';
  filters.defectCode = '';
  filters.handlingLevel = '';
  filters.defectGrade = '';
  void store.load({ search: '', defectCode: '', handlingLevel: '', defectGrade: '' });
}

function openVerify(prefill?: HandlingAdvice) {
  verifyForm.defectCode = prefill?.defectCode || '';
  verifyForm.reason = '';
  formError.value = '';
  showVerify.value = true;
}

async function submitVerify() {
  if (verifyForm.defectCode.trim().length < 2) {
    formError.value = '请填写需要核验的缺陷编码';
    return;
  }
  if (verifyForm.reason.trim().length < 3) {
    formError.value = '请填写不少于 3 个字的复核说明';
    return;
  }
  const ok = await store.verifyAndGenerate(verifyForm.defectCode, verifyForm.reason);
  if (ok) {
    showVerify.value = false;
    filters.defectCode = '';
  }
}

function viewDetail(item: HandlingAdvice) {
  void store.select(item.id);
}
</script>

<template>
  <main class="workspace">
    <header class="page-header">
      <div>
        <p class="eyebrow">缺陷处置优先级建议</p>
        <h1>处置建议工作台</h1>
        <p>复核员核验缺陷后，按桥梁状态、缺陷等级与最近检查批次锁定处置级别并冻结来源快照。</p>
      </div>
      <el-button v-if="canVerify" type="primary" @click="openVerify()">核验缺陷并生成建议</el-button>
    </header>

    <section class="metrics">
      <MetricCard label="建议总数" :value="meta.total" detail="同一缺陷重复核验只保留一份"/>
      <MetricCard label="立即处置" :value="urgentCount" detail="严重缺陷且桥梁已限行"/>
      <MetricCard label="严重缺陷" :value="severeCount" detail="需要优先安排资源"/>
    </section>

    <section class="toolbar">
      <el-input v-model="filters.search" placeholder="搜索建议、缺陷或桥梁编码" clearable/>
      <el-input v-model="filters.defectCode" placeholder="缺陷编码，如 DF-002" style="max-width: 220px"/>
      <el-select v-model="filters.handlingLevel" placeholder="处置级别" clearable style="width: 140px">
        <el-option label="保持观察" value="observe"/>
        <el-option label="限行处置" value="restrict"/>
        <el-option label="立即处置" value="urgent"/>
      </el-select>
      <el-select v-model="filters.defectGrade" placeholder="缺陷等级" clearable style="width: 140px">
        <el-option label="一般缺陷" value="general"/>
        <el-option label="严重缺陷" value="severe"/>
      </el-select>
      <el-button type="primary" @click="submitQuery">查询</el-button>
      <el-button @click="resetQuery">重置</el-button>
    </section>

    <el-alert v-if="error" :title="error" type="error" show-icon/>

    <section class="table-shell">
      <el-table v-loading="loading" :data="items">
        <el-table-column prop="code" label="建议编码" width="130"/>
        <el-table-column label="缺陷 / 桥梁" min-width="200">
          <template #default="{ row }">
            <strong>{{ row.defectCode }}</strong>
            <small>桥梁 {{ row.bridgeCode }} · 批次 {{ row.inspectionCode }}</small>
          </template>
        </el-table-column>
        <el-table-column label="缺陷等级" width="110">
          <template #default="{ row }"><DefectGradeBadge :grade="row.defectGrade"/></template>
        </el-table-column>
        <el-table-column label="处置级别" width="120">
          <template #default="{ row }"><HandlingLevelBadge :level="row.handlingLevel"/></template>
        </el-table-column>
        <el-table-column label="状态" width="100">
          <template #default="{ row }"><StatusBadge :status="row.status"/></template>
        </el-table-column>
        <el-table-column prop="verifiedBy" label="复核员" width="110"/>
        <el-table-column label="判定依据" min-width="260">
          <template #default="{ row }"><small>{{ row.ruleSummary }}</small></template>
        </el-table-column>
        <el-table-column label="生成时间" width="180">
          <template #default="{ row }">{{ formatDate(row.createdAt) }}</template>
        </el-table-column>
        <el-table-column label="操作" width="180" fixed="right">
          <template #default="{ row }">
            <div class="row-actions">
              <el-button link type="primary" @click="viewDetail(row)">回读快照</el-button>
              <el-button v-if="canVerify" link type="primary" @click="openVerify(row)">重复核验</el-button>
            </div>
          </template>
        </el-table-column>
        <template #empty><div class="empty">暂无符合条件的处置建议</div></template>
      </el-table>
    </section>

    <SourceSnapshot v-if="selected" :snapshot="selected.snapshot"/>
    <section v-if="selected" class="detail-panel">
      <header>
        <strong>建议回读</strong>
        <el-button link @click="store.clearSelection()">关闭详情</el-button>
      </header>
      <div class="detail-grid">
        <div><span>建议编码</span><strong>{{ selected.code }}</strong></div>
        <div><span>处置级别</span><HandlingLevelBadge :level="selected.handlingLevel"/></div>
        <div><span>缺陷等级</span><DefectGradeBadge :grade="selected.defectGrade"/></div>
        <div><span>建议状态</span><StatusBadge :status="selected.status"/></div>
        <div><span>复核员</span><strong>{{ selected.verifiedBy }}</strong></div>
        <div><span>请求号</span><code>{{ selected.requestId }}</code></div>
        <div class="detail-rule"><span>判定依据</span><p>{{ selected.ruleSummary }}</p></div>
      </div>
    </section>

    <el-dialog v-model="showVerify" title="复核员核验缺陷" width="min(480px, calc(100vw - 32px))">
      <el-form label-position="top">
        <el-form-item label="缺陷编码" required>
          <el-input v-model="verifyForm.defectCode" placeholder="例如 DF-002（须为已核验缺陷）"/>
        </el-form-item>
        <el-form-item label="复核说明" required>
          <el-input v-model="verifyForm.reason" type="textarea" :rows="3" placeholder="说明现场核验结论与依据"/>
        </el-form-item>
      </el-form>
      <el-alert v-if="formError || error" :title="formError || error" type="error" show-icon style="margin-bottom: 10px"/>
      <p class="muted">规则：一般缺陷保持观察；严重缺陷在桥梁限行后升级为立即处置；检查批次未出结论或缺关联记录时不会生成建议。</p>
      <template #footer>
        <el-button @click="showVerify = false">取消</el-button>
        <el-button type="primary" :loading="generating" @click="submitVerify">提交核验</el-button>
      </template>
    </el-dialog>
  </main>
</template>

<style scoped>
.detail-panel { background: white; border: 1px solid #dbe4e8; padding: 14px; margin-top: 14px; }
.detail-panel > header { display: flex; justify-content: space-between; align-items: center; margin-bottom: 12px; }
.detail-grid { display: grid; grid-template-columns: repeat(3, minmax(0, 1fr)); gap: 12px; }
.detail-grid > div { display: grid; gap: 4px; min-width: 0; }
.detail-grid span { color: #768893; font-size: 12px; }
.detail-grid code { color: #285c77; overflow-wrap: anywhere; }
.detail-rule { grid-column: 1 / -1; border-top: 1px dashed #dbe4e8; padding-top: 10px; }
.detail-rule p { margin: 0; color: #40535f; }
@media (max-width: 900px) { .detail-grid { grid-template-columns: repeat(2, minmax(0, 1fr)); } }
</style>
