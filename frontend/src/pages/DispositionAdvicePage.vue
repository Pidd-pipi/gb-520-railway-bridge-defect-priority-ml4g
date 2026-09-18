<script setup lang="ts">
import { computed, onMounted, ref } from 'vue';
import { useDispositionAdviceStore } from '../stores/disposition-advice';
import { request } from '../api/client';
import { useAuth } from '../hooks/useAuth';
import { formatDate } from '../utils/format';
import { DEFECT_GRADE_LABELS, DISPOSITION_LEVEL_LABELS, type DispositionLevel } from '../types/status';
import type { DomainRecord } from '../types/domain';
import type { DispositionAdvice } from '../types/disposition-advice';
import StatusBadge from '../components/common/StatusBadge.vue';
import SeverityBadge from '../components/common/SeverityBadge.vue';
import DispositionLevelBadge from '../components/common/DispositionLevelBadge.vue';
import MetricCard from '../components/common/MetricCard.vue';
import ConfirmDialog from '../components/common/ConfirmDialog.vue';

const store = useDispositionAdviceStore();
const { canAtLeast } = useAuth();

const search = ref('');
const levelFilter = ref<DispositionLevel | ''>('');
const verifiedDefects = ref<DomainRecord[]>([]);
const selectedDefectId = ref<number | undefined>(undefined);
const showGenerate = ref(false);
const detail = ref<DispositionAdvice | null>(null);
const levelOptions = Object.entries(DISPOSITION_LEVEL_LABELS).map(([value, label]) => ({ value: value as DispositionLevel, label }));

const canGenerate = computed(() => canAtLeast('reviewer'));

onMounted(async () => {
  await store.load();
  if (canGenerate.value) {
    const response = await request<DomainRecord[]>('/defects?page=1&pageSize=100&status=verified');
    verifiedDefects.value = response.data;
  }
});

async function submitQuery() {
  detail.value = null;
  await store.load(search.value.trim(), levelFilter.value);
}

function resetQuery() {
  search.value = '';
  levelFilter.value = '';
  detail.value = null;
  void store.load();
}

function openGenerate() {
  selectedDefectId.value = verifiedDefects.value[0]?.id;
  showGenerate.value = true;
}

async function confirmGenerate() {
  if (!selectedDefectId.value) return;
  const advice = await store.generate(selectedDefectId.value);
  if (advice) {
    showGenerate.value = false;
    const response = await request<DomainRecord[]>('/defects?page=1&pageSize=100&status=verified');
    verifiedDefects.value = response.data;
  }
}

async function openDetail(item: DispositionAdvice) {
  const advice = await store.loadOne(item.id);
  detail.value = advice ?? item;
}

function snapshotPayload(item: DispositionAdvice): string {
  try {
    return JSON.stringify(JSON.parse(item.snapshot.payload), null, 2);
  } catch {
    return item.snapshot.payload;
  }
}
</script>

<template>
	<main class="workspace">
		<header class="page-header">
			<div>
				<p class="eyebrow">业务工作台</p>
				<h1>缺陷处置优先级建议</h1>
				<p>复核员核验缺陷后，按桥梁状态、缺陷等级与最近检查批次结论锁定处置级别并保存来源快照。</p>
			</div>
			<el-button v-if="canGenerate" type="primary" :disabled="verifiedDefects.length === 0" @click="openGenerate">
				核验生成建议
			</el-button>
		</header>

		<section class="metrics">
			<MetricCard label="建议总数" :value="store.meta.total" detail="一缺陷仅一份建议"/>
			<MetricCard label="立即处置" :value="store.urgentCount" detail="严重缺陷且桥梁限行"/>
			<MetricCard label="严重缺陷" :value="store.seriousCount" detail="需要持续跟踪"/>
		</section>

		<section class="evidence-panel">
			<header><strong>核验入口</strong><span>仅列出已核验（verified）缺陷；检查批次未出结论时会被拒绝</span></header>
			<p v-if="!canGenerate" class="muted">viewer 仅可查询；复核员及以上角色可在核验缺陷后生成建议。</p>
			<p v-else-if="verifiedDefects.length === 0" class="muted">当前没有已核验且待生成建议的缺陷。</p>
			<div v-else class="verified-strip">
				<article v-for="defect in verifiedDefects" :key="defect.id">
					<header><strong>{{ defect.code }}</strong><SeverityBadge :severity="defect.riskLevel"/></header>
					<small>{{ defect.name }} · 桥梁 {{ defect.relatedCode }}</small>
					<el-button link type="primary" @click="selectedDefectId = defect.id; showGenerate = true">生成建议</el-button>
				</article>
			</div>
		</section>

		<section class="toolbar">
			<el-input v-model="search" placeholder="搜索建议编号 / 缺陷 / 桥梁编码" clearable @keyup.enter="submitQuery"/>
			<el-select v-model="levelFilter" placeholder="处置级别" clearable style="width: 150px">
				<el-option v-for="option in levelOptions" :key="option.value" :label="option.label" :value="option.value"/>
			</el-select>
			<el-button type="primary" @click="submitQuery">查询</el-button>
			<el-button @click="resetQuery">重置</el-button>
		</section>

		<el-alert v-if="store.error" :title="store.error" type="error" show-icon/>
		<el-alert v-if="store.notice" :title="store.notice" type="success" show-icon closable @close="store.notice = ''"/>

		<section class="table-shell">
			<el-table v-loading="store.loading" :data="store.items">
				<el-table-column prop="code" label="建议编号" width="120"/>
				<el-table-column label="缺陷 / 桥梁" min-width="200">
					<template #default="{ row }">
						<strong>{{ row.defectCode }}</strong>
						<small>{{ row.bridgeCode }} · {{ row.inspectionCode }}</small>
					</template>
				</el-table-column>
				<el-table-column label="缺陷等级" width="100">
					<template #default="{ row }">
						<span>{{ DEFECT_GRADE_LABELS[row.defectGrade as keyof typeof DEFECT_GRADE_LABELS] }}</span>
						<small><SeverityBadge :severity="row.riskLevel"/></small>
					</template>
				</el-table-column>
				<el-table-column label="桥梁状态" width="120">
					<template #default="{ row }"><StatusBadge :status="row.bridgeState"/></template>
				</el-table-column>
				<el-table-column label="处置级别" width="120">
					<template #default="{ row }"><DispositionLevelBadge :level="row.dispositionLevel"/></template>
				</el-table-column>
				<el-table-column prop="reason" label="判定理由" min-width="220"/>
				<el-table-column label="复核 / 请求号" width="190">
					<template #default="{ row }">
						<strong>{{ row.reviewedBy }}</strong>
						<small>{{ row.requestId }}</small>
					</template>
				</el-table-column>
				<el-table-column label="生成时间" width="180">
					<template #default="{ row }">{{ formatDate(row.createdAt) }}</template>
				</el-table-column>
				<el-table-column label="操作" width="120">
					<template #default="{ row }">
						<el-button link type="primary" @click="openDetail(row)">回读建议与快照</el-button>
					</template>
				</el-table-column>
			</el-table>
			<p v-if="!store.loading && store.items.length === 0" class="empty">暂无处置建议，可在上方核验入口为已核验缺陷生成。</p>
		</section>

		<ConfirmDialog v-model="showGenerate" title="核验后生成处置建议" @confirm="confirmGenerate">
			<p>系统将在单个事务中读取缺陷、桥梁与最近检查批次，并锁定来源快照：</p>
			<ul class="rule-list">
				<li>严重缺陷在桥梁限行后升级为<strong>立即处置</strong>；</li>
				<li>一般缺陷始终保持<strong>观察</strong>；</li>
				<li>检查批次未出结论、缺桥梁关联或写入失败均不留数据；</li>
				<li>同一缺陷重复核验只保留第一份建议。</li>
			</ul>
			<el-select v-model="selectedDefectId" placeholder="选择已核验缺陷" style="width: 100%">
				<el-option v-for="defect in verifiedDefects" :key="defect.id"
					:label="`${defect.code} · ${defect.name}（桥梁 ${defect.relatedCode}）`" :value="defect.id"/>
			</el-select>
		</ConfirmDialog>

		<el-drawer v-model="detail" :size="460" title="建议与来源快照回读">
			<template v-if="detail">
				<section class="detail-block">
					<h3>{{ detail.code }}</h3>
					<p><DispositionLevelBadge :level="detail.dispositionLevel"/> <StatusBadge :status="detail.status"/></p>
					<dl class="detail-grid">
						<div><dt>缺陷</dt><dd>{{ detail.defectCode }}（{{ DEFECT_GRADE_LABELS[detail.defectGrade] }}）</dd></div>
						<div><dt>桥梁</dt><dd>{{ detail.bridgeCode }} · {{ detail.bridgeState }} v{{ detail.snapshot.bridgeVersion }}</dd></div>
						<div><dt>检查批次</dt><dd>{{ detail.inspectionCode }} · {{ detail.snapshot.inspectionStatus }} v{{ detail.snapshot.inspectionVersion }}</dd></div>
						<div><dt>判定理由</dt><dd>{{ detail.reason }}</dd></div>
						<div><dt>复核员</dt><dd>{{ detail.reviewedBy }}</dd></div>
						<div><dt>请求号</dt><dd>{{ detail.requestId }}</dd></div>
					</dl>
				</section>
				<section class="detail-block">
					<h3>来源快照（生成时锁定）</h3>
					<pre class="snapshot-payload">{{ snapshotPayload(detail) }}</pre>
				</section>
			</template>
		</el-drawer>
	</main>
</template>
