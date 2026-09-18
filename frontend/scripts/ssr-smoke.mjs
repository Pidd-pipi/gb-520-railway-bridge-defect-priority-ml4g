// Component render smoke for the handling advice module.
// Run with: npx vite-node scripts/ssr-smoke.mjs
// (requires `npm i -D vite-node @vue/server-renderer`; kept out of the default
// dependency set so production installs stay minimal).
import { createSSRApp, h } from 'vue';
import { renderToString } from '@vue/server-renderer';
import HandlingLevelBadge from '../src/components/common/HandlingLevelBadge.vue';
import DefectGradeBadge from '../src/components/common/DefectGradeBadge.vue';
import SourceSnapshot from '../src/components/common/SourceSnapshot.vue';

async function render(component, props = {}) {
  const app = createSSRApp({ render: () => h(component, props) });
  return renderToString(app);
}

let failed = 0;
function check(name, ok) {
  console.log(`${ok ? 'PASS' : 'FAIL'} ${name}`);
  if (!ok) failed++;
}

const observe = await render(HandlingLevelBadge, { level: 'observe' });
const restrict = await render(HandlingLevelBadge, { level: 'restrict' });
const urgent = await render(HandlingLevelBadge, { level: 'urgent' });
check('observe 徽标显示保持观察', observe.includes('保持观察') && observe.includes('status--success'));
check('restrict 徽标显示限行处置', restrict.includes('限行处置') && restrict.includes('status--warning'));
check('urgent 徽标显示立即处置', urgent.includes('立即处置') && urgent.includes('status--danger'));

const general = await render(DefectGradeBadge, { grade: 'general' });
const severe = await render(DefectGradeBadge, { grade: 'severe' });
check('一般缺陷徽标', general.includes('一般缺陷'));
check('严重缺陷徽标', severe.includes('严重缺陷') && severe.includes('severity--critical'));

const snapshot = {
  id: 1, adviceId: 9,
  bridgeSnapshot: JSON.stringify({ code: 'BA-002', name: '二号桥', status: 'restricted', owner: '工务段', facility: 'K42', evidence: '巡检照片', effectiveAt: '2026-09-18T10:00:00Z' }),
  defectSnapshot: JSON.stringify({ code: 'DF-002', name: '梁体裂纹', status: 'verified', owner: '检查组', facility: 'K42', riskLevel: 'critical' }),
  inspectionSnapshot: JSON.stringify({ code: 'IR-002', name: '三季度批次', status: 'completed', owner: '复核组', facility: 'K42' }),
  bridgeStatus: 'restricted', defectStatus: 'verified', defectGrade: 'severe',
  inspectionStatus: 'completed', resolvedHandlingLevel: 'urgent', capturedAt: '2026-09-18T11:30:00Z',
};
const panel = await render(SourceSnapshot, { snapshot });
check('快照面板标题', panel.includes('来源快照'));
check('快照含来源桥梁', panel.includes('来源桥梁') && panel.includes('BA-002'));
check('快照含来源缺陷', panel.includes('来源缺陷') && panel.includes('DF-002'));
check('快照含最近检查批次', panel.includes('最近检查批次') && panel.includes('IR-002'));
check('快照冻结三类状态', panel.includes('restricted') && panel.includes('verified') && panel.includes('completed'));
check('快照证据回显', panel.includes('巡检照片'));

const emptyPanel = await render(SourceSnapshot, { snapshot: null });
check('无快照时面板不渲染', !emptyPanel.includes('来源快照') && !emptyPanel.includes('snapshot-panel'));

process.exit(failed ? 1 : 0);
