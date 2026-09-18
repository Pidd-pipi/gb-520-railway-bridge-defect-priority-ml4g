#!/usr/bin/env sh
set -eu

project_root=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
cd "$project_root"
set -a
if [ -f .env ]; then . ./.env; else . ./.env.example; fi
set +a

(command -v jq >/dev/null 2>&1) || { echo "jq is required for API validation" >&2; exit 1; }

cleanup() { docker compose down -v --remove-orphans; }
docker compose down -v --remove-orphans
if [ "${KEEP_RUNNING:-0}" = "1" ]; then
	trap cleanup INT TERM
else
	trap cleanup EXIT INT TERM
fi

(cd backend && go test ./... && go vet ./... && go build ./...)
(cd frontend && npm ci --no-audit --no-fund && npm run typecheck && npm run build)
docker compose config --quiet
docker compose up -d --build

i=0
until curl -fsS "http://127.0.0.1:${BACKEND_PORT:-19520}/healthz" >/dev/null; do
	i=$((i+1))
	[ "$i" -lt 60 ] || { docker compose logs; exit 1; }
	sleep 2
done
curl -fsS "http://127.0.0.1:${FRONTEND_PORT:-18520}/" >/dev/null

login_token() {
	curl -fsS -X POST "http://127.0.0.1:${BACKEND_PORT:-19520}/api/auth/login" \
		-H 'Content-Type: application/json' \
		-d "{\"username\":\"$1\",\"password\":\"Admin123!\"}" | jq -er '.data.token'
}

viewer_token=$(login_token viewer)
operator_token=$(login_token operator)
reviewer_token=$(login_token reviewer)
admin_token=$(login_token admin)

curl -fsS "http://127.0.0.1:${BACKEND_PORT}/api/session" -H "Authorization: Bearer $viewer_token" | jq -e '.data.role == "viewer"' >/dev/null
viewer_write_status=$(curl -sS -o /dev/null -w '%{http_code}' -X POST "http://127.0.0.1:${BACKEND_PORT}/api/bridges" -H "Authorization: Bearer $viewer_token" -H 'Content-Type: application/json' -d '{}')
[ "$viewer_write_status" = "403" ]
viewer_audit_status=$(curl -sS -o /dev/null -w '%{http_code}' "http://127.0.0.1:${BACKEND_PORT}/api/audits" -H "Authorization: Bearer $viewer_token")
[ "$viewer_audit_status" = "403" ]
curl -fsS "http://127.0.0.1:${BACKEND_PORT}/api/audits?page=1&pageSize=20" -H "Authorization: Bearer $reviewer_token" | jq -e '.data | type == "array"' >/dev/null
curl -fsS "http://127.0.0.1:${BACKEND_PORT}/api/runtime" -H "Authorization: Bearer $admin_token" | jq -e '.data.appName and .data.databaseDriver and (.data.requestLimit > 0)' >/dev/null

now=$(date -u '+%Y-%m-%dT%H:%M:%SZ')
code="PD-SMOKE-$(date +%s)"
create_payload=$(jq -n --arg code "$code" --arg now "$now" '{code:$code,name:"空卷验收优先级决定",description:"验证不可变版本链",facility:"K42 桥梁作业区",owner:"现场处置组",category:"结构缺陷",riskLevel:"critical",metricValue:88,metricUnit:"score",effectiveAt:$now,evidence:"裂缝照片与量测记录 v1",relatedCode:"DF-001"}')
created=$(curl -fsS -X POST "http://127.0.0.1:${BACKEND_PORT}/api/priorities" -H "Authorization: Bearer $operator_token" -H 'X-Request-ID: smoke-create' -H 'Content-Type: application/json' -d "$create_payload")
priority_id=$(printf '%s' "$created" | jq -er '.data.id')
printf '%s' "$created" | jq -e '.data.status == "draft" and .data.version == 1 and .data.preparedBy == "operator" and (.data.revisions | length == 1)' >/dev/null

update_payload=$(jq -n --arg now "$now" '{expectedVersion:1,name:"空卷验收优先级决定",description:"复核前补充量测证据",facility:"K42 桥梁作业区",owner:"现场处置组",category:"结构缺陷",riskLevel:"critical",metricValue:93,metricUnit:"score",effectiveAt:$now,evidence:"裂缝照片、量测记录与复测记录 v2",relatedCode:"DF-001"}')
updated=$(curl -fsS -X PUT "http://127.0.0.1:${BACKEND_PORT}/api/priorities/$priority_id" -H "Authorization: Bearer $operator_token" -H 'X-Request-ID: smoke-update' -H 'Content-Type: application/json' -d "$update_payload")
printf '%s' "$updated" | jq -e '.data.version == 2 and (.data.revisions | length == 2)' >/dev/null

transition_payload='{"status":"urgent","expectedVersion":2,"reason":"独立复核确认需立即处置"}'
operator_final_status=$(curl -sS -o /dev/null -w '%{http_code}' -X POST "http://127.0.0.1:${BACKEND_PORT}/api/priorities/$priority_id/transition" -H "Authorization: Bearer $operator_token" -H 'Content-Type: application/json' -d "$transition_payload")
[ "$operator_final_status" = "403" ]

self_code="PD-SELF-$(date +%s)"
self_payload=$(printf '%s' "$create_payload" | jq --arg code "$self_code" '.code = $code')
self_created=$(curl -fsS -X POST "http://127.0.0.1:${BACKEND_PORT}/api/priorities" -H "Authorization: Bearer $reviewer_token" -H 'Content-Type: application/json' -d "$self_payload")
self_id=$(printf '%s' "$self_created" | jq -er '.data.id')
self_final_status=$(curl -sS -o /dev/null -w '%{http_code}' -X POST "http://127.0.0.1:${BACKEND_PORT}/api/priorities/$self_id/transition" -H "Authorization: Bearer $reviewer_token" -H 'Content-Type: application/json' -d '{"status":"observe","expectedVersion":1,"reason":"不得自行复核自己的决定"}')
[ "$self_final_status" = "422" ]

curl -fsS -X POST "http://127.0.0.1:${BACKEND_PORT}/api/priorities/$priority_id/transition" -H "Authorization: Bearer $reviewer_token" -H 'X-Request-ID: smoke-review' -H 'Content-Type: application/json' -d "$transition_payload" | jq -e '.data.status == "urgent" and .data.version == 3' >/dev/null
curl -fsS "http://127.0.0.1:${BACKEND_PORT}/api/priorities/$priority_id" -H "Authorization: Bearer $reviewer_token" | jq -e '
	.data.status == "urgent" and
	(.data.revisions | length == 3) and
	([.data.revisions[].evidence] == ["裂缝照片与量测记录 v1","裂缝照片、量测记录与复测记录 v2","裂缝照片、量测记录与复测记录 v2"]) and
	([.data.revisions[].actor] == ["operator","operator","reviewer"]) and
	([.data.revisions[].requestId] == ["smoke-create","smoke-update","smoke-review"])' >/dev/null

locked_status=$(curl -sS -o /dev/null -w '%{http_code}' -X PUT "http://127.0.0.1:${BACKEND_PORT}/api/priorities/$priority_id" -H "Authorization: Bearer $operator_token" -H 'Content-Type: application/json' -d "$(printf '%s' "$update_payload" | jq '.expectedVersion = 3')")
[ "$locked_status" = "422" ]
curl -fsS "http://127.0.0.1:${BACKEND_PORT}/api/audit-summary?windowHours=24" -H "Authorization: Bearer $reviewer_token" | jq -e '.data.total >= 3 and .data.transitions >= 1' >/dev/null

# ---- 缺陷处置优先级建议模块 ----
# 种子数据：DF-001 为一般缺陷（active 桥梁，observe），DF-002 为严重缺陷（restricted 桥梁，urgent）。
curl -fsS "http://127.0.0.1:${BACKEND_PORT}/api/handling-advices" -H "Authorization: Bearer $viewer_token" | jq -e '
	(.data | type == "array") and
	([.data[] | select(.defectCode == "DF-002") | .handlingLevel] == ["urgent"]) and
	([.data[] | select(.defectCode == "DF-001") | .handlingLevel] == ["observe"])' >/dev/null

advice_id=$(curl -fsS "http://127.0.0.1:${BACKEND_PORT}/api/handling-advices?defectCode=DF-002" -H "Authorization: Bearer $reviewer_token" | jq -er '.data[0].id')
curl -fsS "http://127.0.0.1:${BACKEND_PORT}/api/handling-advices/$advice_id" -H "Authorization: Bearer $reviewer_token" | jq -e '
	.data.defectGrade == "severe" and .data.handlingLevel == "urgent" and
	.data.snapshot.bridgeStatus == "restricted" and
	.data.snapshot.defectStatus == "verified" and
	.data.snapshot.inspectionStatus == "completed" and
	(.data.snapshot.bridgeSnapshot | length > 0) and
	(.data.snapshot.defectSnapshot | length > 0) and
	(.data.snapshot.inspectionSnapshot | length > 0)' >/dev/null

# 重复核验只保留一份建议，返回 200 且 requestId/快照保持首版。
repeat_status=$(curl -sS -o /tmp/advice-repeat.json -w '%{http_code}' -X POST "http://127.0.0.1:${BACKEND_PORT}/api/handling-advices/generate" -H "Authorization: Bearer $reviewer_token" -H 'Content-Type: application/json' -d '{"defectCode":"DF-002","reason":"空卷验收重复核验只留一份"}')
[ "$repeat_status" = "200" ]
jq -e --arg id "$advice_id" '.data.id == ($id | tonumber) and .data.requestId == "seed-DF-002-verify" and .data.snapshot.bridgeStatus == "restricted"' /tmp/advice-repeat.json >/dev/null
curl -fsS "http://127.0.0.1:${BACKEND_PORT}/api/handling-advices?defectCode=DF-002" -H "Authorization: Bearer $reviewer_token" | jq -e '.meta.total == 1' >/dev/null

# 检查未出结论/缺陷未核验：DF-003 所属批次 review 且缺陷 new，必须 422。
open_status=$(curl -sS -o /dev/null -w '%{http_code}' -X POST "http://127.0.0.1:${BACKEND_PORT}/api/handling-advices/generate" -H "Authorization: Bearer $reviewer_token" -H 'Content-Type: application/json' -d '{"defectCode":"DF-003","reason":"检查未出结论不应生成建议"}')
[ "$open_status" = "422" ]
# operator/viewer 无权触发核验。
operator_advice_status=$(curl -sS -o /dev/null -w '%{http_code}' -X POST "http://127.0.0.1:${BACKEND_PORT}/api/handling-advices/generate" -H "Authorization: Bearer $operator_token" -H 'Content-Type: application/json' -d '{"defectCode":"DF-001","reason":"越权核验"}')
[ "$operator_advice_status" = "403" ]
viewer_advice_status=$(curl -sS -o /dev/null -w '%{http_code}' -X POST "http://127.0.0.1:${BACKEND_PORT}/api/handling-advices/generate" -H "Authorization: Bearer $viewer_token" -H 'Content-Type: application/json' -d '{"defectCode":"DF-001","reason":"越权核验"}')
[ "$viewer_advice_status" = "403" ]
# overview 汇总包含建议级别分布。
curl -fsS "http://127.0.0.1:${BACKEND_PORT}/api/overview" -H "Authorization: Bearer $reviewer_token" | jq -e '.data.handlingAdvices.urgent >= 1 and .data.handlingAdvices.observe >= 1' >/dev/null

docker compose ps
if [ "${KEEP_RUNNING:-0}" = "1" ]; then
	echo "KEEP_RUNNING=1: containers left running for browser validation"
else
	cleanup
	trap - EXIT INT TERM
fi
