# 验收记录

## 处置优先级建议模块（2026-09-18）

从零新增「缺陷处置优先级建议」模块并完成验证：

- `go test ./...`、`go test -race -count=1 ./...`、`go vet ./...`、`go build ./...`：通过。
- `npm run typecheck`、`npm run build`：通过。
- 新增服务层测试 7 组、常量测试 2 组，覆盖：一般缺陷 observe、严重缺陷 restrict、限行后 urgent 升级、检查未出结论/缺陷未核验/缺关联记录拒绝、来源快照三要素、重复核验幂等、16 并发只生成一份、桥梁并发变更快照不串状态、写入失败整体回滚。
- SQLite 真实服务端到端（reviewer/operator/viewer）：种子建议规则正确（DF-001 observe、DF-002 urgent）；重复核验返回 200 且建议、request ID、冻结快照不变；最近批次 planned/review 时核验返回 422，批次完成后 severe+active → restrict，桥梁随后限行再次核验仍返回冻结的 restrict；operator/viewer POST 403、viewer GET 200；筛选与详情回读、`overview.handlingAdvices`、verify 审计均正确。
- 前端组件渲染冒烟（`npx vite-node frontend/scripts/ssr-smoke.mjs`）12 项通过；store↔真实后端集成冒烟（`npx vite-node frontend/scripts/store-smoke.mjs`）14 项通过，覆盖 403/201/200、幂等、筛选、快照回读。
- 运行环境无 root 权限安装 Chromium 系统库，未做浏览器点击验证；页面已由 Vue 组件级渲染、Vite 模块编译与 store 集成覆盖。

## 静态质量（2026-08-22）

- `go test ./...`：通过。
- `go test -race ./...`：通过。
- `go vet ./...`：通过。
- `go build ./...`：通过。
- `npm run typecheck`：通过。
- `npm run build`：通过。Vite 仅报告主包大于 500 kB 的性能提示，不影响构建与运行。
- 非测试 Go 代码：3147 行、38 个 `.go` 文件，符合提示词 3000–4200 行、30–42 文件要求。

## 空卷 Compose 与 API

执行 `KEEP_RUNNING=1 ./scripts/validate.sh` 前，脚本已运行 `docker compose down -v --remove-orphans`。PostgreSQL、Redis、MinIO、后端和前端均从空卷启动并通过健康检查。

- viewer 可以读取业务数据；写接口与审计接口均返回 403。
- operator 创建优先级 v1 并补充证据形成 v2；直接定稿返回 403。
- reviewer 自己拟制后自行复核返回 422。
- 独立 reviewer 将 operator 草稿定为 urgent v3。
- v1/v2/v3 的证据、actor 和 request ID 均按原值保留。
- 终态后再次编辑返回 422。
- 审计汇总包含 create、update 和 transition。

## 内置 Browser

仅使用 Codex 内置 Browser 验证，没有调用外部 Chrome 或独立 Playwright。

- viewer 登录后看不到新增、推进和审计导航；直接访问 `/audit` 被守卫重定向到 `/bridges`。
- operator 在 `/defects` 创建缺陷并从 new 推进到 verified；`SeverityBadge` 和证据摘要正常显示。
- `/inspections` 与 `/defects` 均渲染共享 `EvidenceGallery`。
- operator 在 `/priorities` 看不到定稿按钮；reviewer 可对他人拟制的 PD-001 定稿，自己拟制的草稿无操作按钮。
- `/audit` 正确显示操作者、状态迁移和 request ID。
- `/bridges`、`/inspections`、`/defects`、`/priorities`、`/audit` 在 390×844 下的 `innerWidth`、`body.scrollWidth`、`documentElement.scrollWidth` 均为 390。
- 浏览器控制台 error/warning：0。

默认 `./scripts/validate.sh` 会在结束时执行 `docker compose down -v --remove-orphans`，不会保留本项目容器、网络或命名卷。
