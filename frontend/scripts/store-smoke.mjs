// Store + live-API integration smoke for the handling advice module.
// Run with: npx vite-node scripts/store-smoke.mjs (backend must listen on
// 127.0.0.1:19520 with seeded demo data; requires the dev-only `vite-node`
// package, which is intentionally not part of the default dependencies).
import { createPinia, setActivePinia } from 'pinia';

async function request(path, init = {}, token) {
  const res = await fetch(`http://127.0.0.1:19520${path}`, {
    ...init,
    headers: { 'Content-Type': 'application/json', ...(token ? { Authorization: `Bearer ${token}` } : {}) },
  });
  const body = await res.json().catch(() => ({}));
  return { status: res.status, body };
}

async function login(username) {
  const { body } = await request('/api/auth/login', {
    method: 'POST',
    body: JSON.stringify({ username, password: 'Admin123!' }),
  });
  return body.data.token;
}

let failed = 0;
function check(name, ok, extra = '') {
  console.log(`${ok ? 'PASS' : 'FAIL'} ${name}${extra ? ` — ${extra}` : ''}`);
  if (!ok) failed++;
}

const reviewer = await login('reviewer');
const operator = await login('operator');

// 1. Seed a fresh verified severe defect on a restricted bridge via the API.
const suffix = `IT${Date.now().toString().slice(-6)}`;
await request('/api/bridges', { method: 'POST', body: JSON.stringify({
  code: `BA-${suffix}`, name: '集成测试桥', facility: 'K42', owner: '工务', category: '结构',
  riskLevel: 'high', metricValue: 1, metricUnit: 'x', effectiveAt: '2026-09-01T00:00:00Z', evidence: '桥检证据', relatedCode: 'REL-IT',
}) }, reviewer);
const bridgeList = await request(`/api/bridges?search=BA-${suffix}`, {}, reviewer);
const bridgeId = bridgeList.body.data[0].id;
await request(`/api/bridges/${bridgeId}/transition`, { method: 'POST', body: JSON.stringify({
  status: 'restricted', expectedVersion: 1, reason: '集成测试桥梁限行',
}) }, reviewer);
await request('/api/inspections', { method: 'POST', body: JSON.stringify({
  code: `IR-${suffix}`, name: '集成批次', facility: 'K42', owner: '工务', category: '结构',
  riskLevel: 'low', metricValue: 1, metricUnit: 'x', effectiveAt: '2026-09-02T00:00:00Z', evidence: '批次证据', relatedCode: `BA-${suffix}`,
}) }, reviewer);
const inspectionList = await request(`/api/inspections?search=IR-${suffix}`, {}, reviewer);
const inspectionId = inspectionList.body.data[0].id;
for (const [status, version] of [['running', 1], ['review', 2], ['completed', 3]]) {
  await request(`/api/inspections/${inspectionId}/transition`, { method: 'POST', body: JSON.stringify({
    status, expectedVersion: version, reason: '集成测试推进检查批次',
  }) }, reviewer);
}
await request('/api/defects', { method: 'POST', body: JSON.stringify({
  code: `DF-${suffix}`, name: '集成严重缺陷', facility: 'K42', owner: '工务', category: '结构',
  riskLevel: 'critical', metricValue: 9, metricUnit: '级', effectiveAt: '2026-09-03T00:00:00Z', evidence: '缺陷证据', relatedCode: `BA-${suffix}`,
}) }, reviewer);
const defectList = await request(`/api/defects?search=DF-${suffix}`, {}, reviewer);
const defectId = defectList.body.data[0].id;
await request(`/api/defects/${defectId}/transition`, { method: 'POST', body: JSON.stringify({
  status: 'verified', expectedVersion: 1, reason: '集成测试核验缺陷',
}) }, reviewer);

// 2. operator is forbidden from generating advice.
const forbidden = await request('/api/handling-advices/generate', { method: 'POST', body: JSON.stringify({
  defectCode: `DF-${suffix}`, reason: '越权尝试不应通过',
}) }, operator);
check('operator 不能生成建议（403）', forbidden.status === 403, `got ${forbidden.status}`);

// 3. reviewer generates advice; severe + restricted must escalate to urgent.
const generated = await request('/api/handling-advices/generate', { method: 'POST', body: JSON.stringify({
  defectCode: `DF-${suffix}`, reason: '集成测试核验并锁定来源',
}) }, reviewer);
check('首次核验返回 201', generated.status === 201, `got ${generated.status}`);
check('严重缺陷限行后升级 urgent', generated.body.data.handlingLevel === 'urgent', generated.body.data.handlingLevel);
check('快照冻结 restricted/completed/verified',
  generated.body.data.snapshot.bridgeStatus === 'restricted' &&
  generated.body.data.snapshot.inspectionStatus === 'completed' &&
  generated.body.data.snapshot.defectStatus === 'verified');
check('快照含三份来源 JSON',
  Boolean(generated.body.data.snapshot.bridgeSnapshot && generated.body.data.snapshot.defectSnapshot && generated.body.data.snapshot.inspectionSnapshot));

// 4. Repeated verification is idempotent and returns the same row (200).
const repeated = await request('/api/handling-advices/generate', { method: 'POST', body: JSON.stringify({
  defectCode: `DF-${suffix}`, reason: '重复核验应返回原建议',
}) }, reviewer);
check('重复核验返回 200', repeated.status === 200, `got ${repeated.status}`);
check('重复核验只保留一份建议', repeated.body.data.id === generated.body.data.id);
check('原请求号与复核人不被覆盖', repeated.body.data.requestId === generated.body.data.requestId);

// 5. Query filters through the page's query contract.
const byDefect = await request(`/api/handling-advices?defectCode=DF-${suffix}&handlingLevel=urgent`, {}, reviewer);
check('按缺陷编码+级别筛选', byDefect.body.meta.total === 1 && byDefect.body.data[0].id === generated.body.data.id);
const byGrade = await request('/api/handling-advices?defectGrade=severe&pageSize=100', {}, reviewer);
check('按严重等级筛选', byGrade.body.data.every((row) => row.defectGrade === 'severe'));
const detail = await request(`/api/handling-advices/${generated.body.data.id}`, {}, reviewer);
check('详情接口回读建议与快照', detail.body.data.snapshot.id === generated.body.data.snapshot.id && detail.body.data.ruleSummary.includes('立即处置'));

// 6. Pinia store drives the same contract with an authenticated fetch adapter.
const { useHandlingAdviceStore } = await import('../src/stores/handling-advice.ts');
const tokenHeader = reviewer;
const nativeFetch = globalThis.fetch;
globalThis.fetch = (input, init = {}) => {
  const headers = new Headers(init.headers);
  headers.set('Authorization', `Bearer ${tokenHeader}`);
  return nativeFetch(`http://127.0.0.1:19520${String(input).replace(/^https?:\/\/[^/]+/, '')}`, { ...init, headers });
};
setActivePinia(createPinia());
const store = useHandlingAdviceStore();
await store.load({ defectCode: `DF-${suffix}` });
check('store 查询回读', store.items.length === 1 && store.items[0].handlingLevel === 'urgent');
await store.select(store.items[0].id);
check('store 详情回读快照', store.selected?.snapshot.bridgeStatus === 'restricted' && store.selected.defectCode === `DF-${suffix}`);
const repeatAgain = await store.verifyAndGenerate(`DF-${suffix}`, 'store 再核验一次');
check('store 重复核验成功且仍为一份', repeatAgain && store.items.length === 1);

process.exit(failed ? 1 : 0);
