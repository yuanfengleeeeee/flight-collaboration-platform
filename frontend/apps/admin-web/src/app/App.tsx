import { useCallback, useEffect, useRef, useState } from "react";
import { Link, Navigate, Outlet, Route, Routes, useLocation, useNavigate, useParams } from "react-router-dom";
import type { FormEvent } from "react";
import { ApiClientError } from "@flight/api-client";
import type { CoreTask, TaskStatus } from "@flight/contracts";
import { Notice, StatusBadge } from "@flight/ui";
import { createClientID } from "@flight/task-domain";
import { adminRuntime, adminSsoEnabled, adminSsoReturnPath, beginAdminSso, clearAdminAccessToken, coreApi, exchangeAdminSsoCode, hasAdminAccess, readAdminAccessToken, saveAdminAccessToken } from "./runtime";

const navGroups = [
  { label: "运行", items: [["overview", "运行总览"], ["flights", "航班运行"]] },
  { label: "任务", items: [["tasks", "任务工作台"], ["templates", "任务模板"], ["assignments", "任务分配"], ["history", "任务历史"]] },
  { label: "人员", items: [["personnel", "人员档案"], ["positions", "岗位与能力"], ["status", "人员状态"]] },
  { label: "治理", items: [["rules", "规则配置"], ["events", "事件与通知"], ["audit", "审计日志"], ["reports", "报表统计"]] },
  { label: "系统", items: [["users-roles", "用户与角色"], ["scopes", "权限范围"], ["diagnostics", "开发诊断"]] },
] as const;

const moduleCopy: Record<string, { title: string; description: string }> = {
  overview: { title: "运行总览", description: "全场航班、任务、人员和同步状态的统一入口。" },
  flights: { title: "航班运行", description: "查看航班运行事实和到达触发记录。" },
  templates: { title: "任务模板", description: "维护任务名称、触发规则和岗位能力要求。" },
  assignments: { title: "任务分配", description: "维护队长管辖、候选人和 Assignment 入口。" },
  history: { title: "任务历史", description: "查看任务状态轨迹和历史处理记录。" },
  personnel: { title: "人员档案", description: "查看员工档案、团队和区域归属。" },
  positions: { title: "岗位与能力", description: "查看岗位定义和能力要求。" },
  status: { title: "人员状态", description: "查看现场可用状态和状态变更。" },
  rules: { title: "规则配置", description: "查看任务生成、候选计算和分配规则。" },
  events: { title: "事件与通知", description: "查看运行事件、通知和同步异常。" },
  audit: { title: "审计日志", description: "查看关键操作和状态变更的审计记录。" },
  reports: { title: "报表统计", description: "汇总任务、航班、人员和同步数据。" },
  "users-roles": { title: "用户与角色", description: "管理管理端用户、角色和入口权限。" },
  scopes: { title: "权限范围", description: "维护团队、区域与角色 Scope。" },
  diagnostics: { title: "开发诊断", description: "查看开发环境 API 请求和同步诊断信息。" },
};

export function App(): JSX.Element {
  return <Routes>
    <Route path="/login" element={<AdminLoginPage />} />
    <Route path="/sso/callback" element={<AdminSsoCallbackPage />} />
    <Route element={<RequireAdminAccess />}>
      <Route element={<AdminLayout />}>
        <Route path="/overview" element={<ModulePage section="overview" />} />
        <Route path="/flights" element={<ModulePage section="flights" />} />
        <Route path="/tasks" element={<TaskListPage />} />
        <Route path="/tasks/:taskPublicID" element={<TaskDetailPage />} />
        {Object.keys(moduleCopy).filter((key) => !["overview", "flights"].includes(key)).map((section) => <Route key={section} path={`/${section}`} element={<ModulePage section={section} />} />)}
        <Route path="*" element={<Navigate to="/tasks" replace />} />
      </Route>
    </Route>
  </Routes>;
}

function AdminSsoCallbackPage(): JSX.Element {
  const navigate = useNavigate();
  const [message, setMessage] = useState("正在完成管理端 SSO 登录…");
  const started = useRef(false);
  useEffect(() => {
    if (started.current) return;
    started.current = true;
    const query = new URLSearchParams(window.location.search);
    const code = query.get("code");
    const state = query.get("state");
    if (!code || !state) {
      setMessage(query.get("error_description") || "SSO 回调缺少一次性授权码");
      return;
    }
    const destination = adminSsoReturnPath();
    let active = true;
    exchangeAdminSsoCode(code, state).then(() => { if (active) navigate(destination, { replace: true }); }).catch((error: unknown) => {
      if (active) setMessage(error instanceof Error ? error.message : "SSO 登录失败");
    });
    return () => { active = false; };
  }, [navigate]);
  return <main className="admin-auth-page"><section className="auth-panel"><div className="brand-mark">A</div><p className="eyebrow">CORE / SSO CALLBACK</p><h1>正在验证管理身份</h1><p className="auth-lead">{message}</p>{message !== "正在完成管理端 SSO 登录…" && <button className="secondary-button" onClick={() => navigate("/login", { replace: true })}>返回登录</button>}</section></main>;
}

function RequireAdminAccess(): JSX.Element {
  return hasAdminAccess() ? <Outlet /> : <Navigate to="/login" replace />;
}

function AdminLoginPage(): JSX.Element {
  const navigate = useNavigate();
  const [token, setToken] = useState(readAdminAccessToken());
  const [message, setMessage] = useState<string>();

  function submit(event: FormEvent<HTMLFormElement>): void {
    event.preventDefault();
    if (!token.trim() && !adminRuntime.devActorEnabled) {
      setMessage("请提供 Core Bearer Token。当前后端尚未提供管理用户登录接口。");
      return;
    }
    if (token.trim()) saveAdminAccessToken(token);
    navigate("/tasks");
  }

  return <main className="admin-auth-page">
    <section className="auth-panel">
      <div className="brand-mark">A</div>
      <p className="eyebrow">CORE / MANAGEMENT ACCESS</p>
      <h1>进入运行管理台</h1>
      <p className="auth-lead">管理端只调用 Core API。主任查看全部任务，队长由 Core 按团队/区域 Scope 返回可见任务。</p>
      <form onSubmit={submit} className="auth-form">
        <label htmlFor="core-token">Core Bearer Token <span>开发联调</span></label>
        <textarea id="core-token" value={token} onChange={(event) => setToken(event.target.value)} placeholder="粘贴已签发的 Core JWT；生产环境由管理端 SSO 适配器提供" rows={4} />
        <button className="primary-button" type="submit">进入管理端</button>
      </form>
      {adminSsoEnabled() && <><div className="auth-divider"><span>或</span></div><button className="secondary-button sso-button" type="button" onClick={() => beginAdminSso("/tasks")}>使用企业身份 SSO 登录</button><p className="auth-note">SSO 只交换一次性授权码，回调后由服务端签发 Core 会话；令牌不会放在 URL 中。</p></>}
      {adminRuntime.devActorEnabled && <Notice tone="success">已启用显式开发 Actor：{adminRuntime.devActorRole}。请求会发送到真实 Core API，不使用 Mock 数据。</Notice>}
      {message && <Notice tone="error">{message}</Notice>}
      <p className="auth-note">当前 Core 不提供管理用户密码登录；配置 SSO 后通过企业身份进入，未配置时仅支持开发联调 Token/Actor。管理权限始终由 Core 的管理会话和角色 Scope 决定。</p>
      <p className="auth-note">员工端请从独立的 employee-web 入口访问。</p>
    </section>
  </main>;
}

function AdminLayout(): JSX.Element {
  const location = useLocation();
  const navigate = useNavigate();
  function logout(): void { clearAdminAccessToken(); navigate("/login"); }
  return <div className="admin-shell">
    <aside className="admin-sidebar">
      <Link to="/tasks" className="brand-lockup"><span className="brand-mark">A</span><span><strong>航班保障协同</strong><small>OPERATIONS / CONTROL</small></span></Link>
      <nav aria-label="管理端工作区" className="admin-nav">{navGroups.map((group) => <div className="nav-group" key={group.label}><span className="nav-label">{group.label}</span>{group.items.map(([section, label]) => <Link key={section} className={location.pathname.includes(`/${section}`) ? "is-active" : ""} to={`/${section}`}>{label}</Link>)}</div>)}</nav>
      <div className="scope-card"><span>当前授权范围</span><strong>{adminRuntime.devActorEnabled ? (adminRuntime.devActorRole === "leader" ? "当前团队 / 区域" : "全部团队 · 全部区域") : "由 Core JWT 决定"}</strong><small>前端不进行本地越权过滤</small></div>
      <div className="sidebar-footer"><div><span className="avatar">{adminRuntime.devActorEnabled ? adminRuntime.devActorRole.slice(0, 1).toUpperCase() : "C"}</span><span><strong>{adminRuntime.devActorEnabled ? adminRuntime.devActorRole : "Core session"}</strong><small>管理端会话</small></span></div><button className="text-button" onClick={logout}>退出</button></div>
    </aside>
    <section className="admin-content"><header className="content-topline"><span>运行管理台 <i>/</i> {location.pathname.startsWith("/tasks") ? "任务工作台" : "工作区"}</span><span className="sync-pill">Core API · 实时读取</span></header><div className="page-body"><Outlet /></div></section>
  </div>;
}

function ModulePage({ section }: { section: string }): JSX.Element {
  const copy = moduleCopy[section] ?? moduleCopy.overview;
  return <div className="module-page"><p className="eyebrow">MODULE / {section.toUpperCase()}</p><h1>{copy.title}</h1><p className="page-description">{copy.description}</p><section className="empty-panel"><span className="empty-symbol">□</span><p className="eyebrow">FUNCTIONAL SCAFFOLD</p><h2>页面边界已注册</h2><p>此模块已进入首期管理端导航。真实数据与操作将在对应后端接口契约确认后接入，当前不伪造业务内容。</p><div className="empty-facts"><span><small>数据源</small><strong>待接入 Core API</strong></span><span><small>授权范围</small><strong>由服务端 Scope 决定</strong></span><span><small>当前状态</small><strong>空态</strong></span></div></section></div>;
}

function TaskListPage(): JSX.Element {
  const [status, setStatus] = useState<TaskStatus | "">("");
  const [items, setItems] = useState<CoreTask[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<ApiClientError>();
  const [requestID, setRequestID] = useState("");
  const [reloadKey, setReloadKey] = useState(0);

  useEffect(() => {
    const controller = new AbortController();
    setLoading(true); setError(undefined);
    coreApi.listTasks({ status: status || undefined, page: 1, page_size: 50 }, controller.signal).then((response) => {
      setItems(response.data.items); setRequestID(response.request_id ?? "");
    }).catch((value: unknown) => { if (!controller.signal.aborted) setError(value instanceof ApiClientError ? value : new ApiClientError(0, { code: "network_error", message: "无法连接 Core API" })); }).finally(() => { if (!controller.signal.aborted) setLoading(false); });
    return () => controller.abort();
  }, [status, reloadKey]);

  return <div className="module-page task-page"><div className="page-heading"><div><p className="eyebrow">TASK / CORE FACTS</p><h1>任务工作台</h1><p className="page-description">任务可见范围由 Core 根据主任、队长角色和团队/区域 Scope 决定。</p></div><button className="secondary-button" onClick={() => setReloadKey((value) => value + 1)}>刷新列表</button></div>
    <div className="metric-row"><Metric label="当前可见" value={String(items.length)} /><Metric label="待确认" value={String(items.filter((task) => task.status === "awaiting_confirmation").length)} /><Metric label="执行中" value={String(items.filter((task) => task.status === "in_progress").length)} /><Metric label="已完成" value={String(items.filter((task) => task.status === "completed").length)} /></div>
    <section className="task-panel"><div className="toolbar"><label>状态<select value={status} onChange={(event) => setStatus(event.target.value as TaskStatus | "")}><option value="">全部状态</option><option value="awaiting_confirmation">待确认</option><option value="assigned">已分配</option><option value="in_progress">执行中</option><option value="completed">已完成</option><option value="cancelled">已取消</option></select></label><span className="scope-inline">{adminRuntime.devActorEnabled ? `${adminRuntime.devActorRole} · 开发 Scope` : "Bearer JWT · Core Scope"}</span></div>
      {loading && <div className="panel-state">正在从 Core 读取任务…</div>}
      {error && <div className="panel-state"><Notice tone="error"><strong>{error.body.code}</strong> · {error.message}{error.body.request_id && <small>request_id: {error.body.request_id}</small>}<button className="secondary-button" onClick={() => setReloadKey((value) => value + 1)}>重新读取</button></Notice></div>}
      {!loading && !error && items.length === 0 && <div className="panel-state"><span className="empty-symbol">∅</span><h2>当前范围没有任务</h2><p>如果你是队长，这表示当前团队/区域没有返回可见任务；如果你是主任，请检查 Core 数据或筛选条件。</p></div>}
      {!loading && !error && items.length > 0 && <div className="table-scroll"><table><thead><tr><th>计划时间</th><th>航班 / 任务</th><th>区域 / 团队</th><th>状态</th><th>版本</th><th /></tr></thead><tbody>{items.map((task) => <tr key={task.public_id}><td className="data-number">{formatDate(task.planned_at)}</td><td><Link className="task-link" to={`/tasks/${task.public_id}`}><strong>{task.flight_display_no || task.flight_public_id}</strong><span>{task.name}</span></Link></td><td><span>{task.area_public_id}</span><small>{task.team_public_id}</small></td><td><StatusBadge status={task.status} /></td><td className="data-number">v{task.status_version} / s{task.sync_version}</td><td><Link className="row-link" to={`/tasks/${task.public_id}`}>详情 →</Link></td></tr>)}</tbody></table></div>}
      <footer className="list-footer"><span>Core 返回 {items.length} 条，未在前端二次扩大 Scope</span>{requestID && <span>request_id: {requestID}</span>}</footer>
    </section>
  </div>;
}

function TaskDetailPage(): JSX.Element {
  const { taskPublicID } = useParams();
  const navigate = useNavigate();
  const [task, setTask] = useState<CoreTask>();
  const [candidateID, setCandidateID] = useState("");
  const [reason, setReason] = useState("");
  const [confirmationID, setConfirmationID] = useState<string>();
  const [cancellationID, setCancellationID] = useState<string>();
  const [busy, setBusy] = useState<"confirm" | "cancel" | "load" | "">("load");
  const [notice, setNotice] = useState<{ tone: "success" | "error"; text: string }>();

  const load = useCallback((): void => {
    if (!taskPublicID) return;
    setBusy("load"); setNotice(undefined);
    coreApi.getTask(taskPublicID).then((response) => { setTask(response.data); setCandidateID(response.data.candidates.find((candidate) => candidate.status === "proposed")?.public_id ?? ""); setConfirmationID(undefined); setCancellationID(undefined); }).catch((value: unknown) => setNotice({ tone: "error", text: value instanceof ApiClientError ? `${value.body.code} · ${value.message}` : "无法读取任务详情" })).finally(() => setBusy(""));
  }, [taskPublicID]);
  useEffect(() => load(), [load]);

  async function confirm(): Promise<void> {
    if (!task || !candidateID) return;
    setBusy("confirm"); setNotice(undefined);
    const requestID = confirmationID ?? createClientID();
    setConfirmationID(requestID);
    try { const response = await coreApi.confirmTask(task.public_id, { candidate_public_id: candidateID, confirmation_id: requestID, expected_task_version: task.status_version }); setConfirmationID(undefined); setNotice({ tone: "success", text: `任务已提交确认：${response.data.result_code}` }); load(); } catch (value) { if (value instanceof ApiClientError && value.status === 409) setConfirmationID(undefined); setNotice({ tone: "error", text: value instanceof ApiClientError ? `${value.body.code} · ${value.message}` : "确认请求失败" }); setBusy(""); }
  }
  async function cancel(): Promise<void> {
    if (!task || !reason.trim() || !window.confirm("确认取消这个任务？Core 会记录取消原因并释放相关分配。")) return;
    setBusy("cancel"); setNotice(undefined);
    const requestID = cancellationID ?? createClientID();
    setCancellationID(requestID);
    try { const response = await coreApi.cancelTask(task.public_id, { cancellation_id: requestID, expected_task_version: task.status_version, reason: reason.trim() }); setCancellationID(undefined); setNotice({ tone: "success", text: `任务已提交取消：${response.data.result_code}` }); load(); } catch (value) { if (value instanceof ApiClientError && value.status === 409) setCancellationID(undefined); setNotice({ tone: "error", text: value instanceof ApiClientError ? `${value.body.code} · ${value.message}` : "取消请求失败" }); setBusy(""); }
  }

  if (busy === "load" && !task) return <div className="panel-state">正在从 Core 读取任务详情…</div>;
  if (!task) return <div className="panel-state"><Notice tone="error">任务详情不可用。<button className="secondary-button" onClick={() => navigate("/tasks")}>返回任务列表</button></Notice></div>;
  const canManage = task.status === "awaiting_confirmation";
  return <div className="module-page task-detail-page"><button className="back-link button-reset" onClick={() => navigate("/tasks")}>← 返回任务工作台</button><div className="detail-heading"><div><p className="eyebrow">FLIGHT / {task.flight_display_no || task.flight_public_id}</p><h1>{task.name}</h1><p className="page-description">{task.area_public_id} · {task.team_public_id} · {task.message || "暂无任务说明"}</p></div><StatusBadge status={task.status} /></div>{notice && <Notice tone={notice.tone}>{notice.text}</Notice>}<div className="detail-grid"><section className="detail-card"><h2>任务事实</h2><div className="fact-grid"><Fact label="计划时间" value={formatDate(task.planned_at)} /><Fact label="触发类型" value={task.trigger_type || "—"} /><Fact label="Task 版本" value={`v${task.status_version}`} /><Fact label="同步版本" value={`s${task.sync_version}`} /><Fact label="岗位要求" value={task.required_position_code || "—"} /><Fact label="能力要求" value={task.required_capabilities.join("、") || "—"} /></div></section><section className="detail-card"><h2>候选人员</h2>{task.candidates.length === 0 ? <p className="muted">Core 没有返回候选人员。</p> : <div className="candidate-list">{task.candidates.map((candidate) => <label className={`candidate ${candidate.public_id === candidateID ? "is-selected" : ""}`} key={candidate.public_id}><input type="radio" name="candidate" value={candidate.public_id} checked={candidate.public_id === candidateID} onChange={() => { setCandidateID(candidate.public_id); setConfirmationID(undefined); }} disabled={!canManage || candidate.status !== "proposed"} /><span><strong>#{candidate.rank} · {candidate.personnel_public_id}</strong><small>{candidate.matched_position_code} · {candidate.matched_capabilities.join("、") || "能力快照未提供"}</small></span><em>{candidate.status}</em></label>)}</div>}</section></div><aside className="action-card"><div><p className="eyebrow">CORE COMMAND</p><h2>{canManage ? "处理这个任务" : "当前为只读状态"}</h2><p>{canManage ? "确认会创建 Assignment；取消会记录原因。最终状态以 Core 响应为准。" : "任务当前状态不允许在此处继续确认。"}</p></div>{canManage && <div className="action-controls"><button className="primary-button" disabled={!candidateID || busy !== ""} onClick={confirm}>{busy === "confirm" ? "正在提交…" : confirmationID ? "重试确认" : "确认分配"}</button><label>取消原因<input value={reason} onChange={(event) => { setReason(event.target.value); setCancellationID(undefined); }} placeholder="请输入取消原因" maxLength={255} /></label><button className="danger-button" disabled={!reason.trim() || busy !== ""} onClick={cancel}>{busy === "cancel" ? "正在提交…" : cancellationID ? "重试取消" : "取消任务"}</button></div>}</aside></div>;
}

function Metric({ label, value }: { label: string; value: string }): JSX.Element { return <div className="metric"><small>{label}</small><strong>{value}</strong></div>; }
function Fact({ label, value }: { label: string; value: string }): JSX.Element { return <span><small>{label}</small><strong>{value}</strong></span>; }
function formatDate(value: string): string { if (!value) return "—"; const date = new Date(value); return Number.isNaN(date.valueOf()) ? value : new Intl.DateTimeFormat("zh-CN", { month: "2-digit", day: "2-digit", hour: "2-digit", minute: "2-digit" }).format(date); }
