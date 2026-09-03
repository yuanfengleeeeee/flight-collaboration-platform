import { useCallback, useEffect, useState } from "react";
import type { FormEvent } from "react";
import { Link, Navigate, Outlet, Route, Routes, useLocation, useNavigate, useParams } from "react-router-dom";
import { ApiClientError } from "@flight/api-client";
import type { RealtimeState } from "@flight/api-client";
import type { CommandStatus, EdgeTaskProjection, Provider, TaskStatus } from "@flight/contracts";
import { CommandBadge, Notice, StatusBadge } from "@flight/ui";
import { canAccept, canComplete, commandStatusLabel, createClientID, projectionStatus } from "@flight/task-domain";
import { edgeApi, employeeRealtime, restoreEmployeeSession, sessionManager } from "./runtime";
import { taskCommandStore, type TaskCommandReceipt } from "./task-command-store";

type TaskCommandDraft = TaskCommandReceipt;

export function App(): JSX.Element {
  const [ready, setReady] = useState(false);
  useEffect(() => { void restoreEmployeeSession().finally(() => setReady(true)); }, []);
  if (!ready) return <main className="employee-loading"><span className="loading-mark">A</span><p>正在恢复员工会话…</p></main>;
  return <Routes>
    <Route path="/login" element={<EmployeeLoginPage />} />
    <Route path="/bind" element={<BindingPage />} />
    <Route element={<RequireEmployee />}>
      <Route element={<EmployeeLayout />}>
        <Route path="/tasks" element={<EmployeeTaskListPage />} />
        <Route path="/tasks/:taskPublicID" element={<EmployeeTaskDetailPage />} />
        <Route path="/notifications" element={<EmployeeEmptyPage title="通知" description="任务变更和同步提醒将在 Edge 通知 Projection 就绪后显示。" />} />
        <Route path="/exceptions" element={<EmployeeEmptyPage title="异常反馈" description="现场异常反馈入口已保留，待异常接口契约接入。" />} />
        <Route path="/history" element={<EmployeeEmptyPage title="历史任务" description="历史任务查询接口接入后，这里会显示你的完成记录。" />} />
        <Route path="/account" element={<EmployeeAccountPage />} />
        <Route path="*" element={<Navigate to="/tasks" replace />} />
      </Route>
    </Route>
  </Routes>;
}

function RequireEmployee(): JSX.Element { return sessionManager.getSnapshot() ? <Outlet /> : <Navigate to="/login" replace />; }

function EmployeeLoginPage(): JSX.Element {
  const navigate = useNavigate();
  const [employeeNo, setEmployeeNo] = useState("E000123");
  const [password, setPassword] = useState("");
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<string>();

  async function submit(event: FormEvent<HTMLFormElement>): Promise<void> {
    event.preventDefault(); setBusy(true); setError(undefined);
    try {
      const result = await sessionManager.passwordLogin({ employee_no: employeeNo, password, client: "employee-web" });
      if (result.state === "binding_required") navigate("/bind"); else navigate("/tasks");
    } catch (value) { setError(toErrorMessage(value, "登录失败，请检查工号和密码。")); } finally { setBusy(false); }
  }

  return <main className="employee-auth-page"><section className="employee-auth-card"><div className="employee-brand"><span className="brand-mark">A</span><span><strong>航班保障协同</strong><small>EMPLOYEE / EDGE</small></span></div><p className="eyebrow">STAFF ACCESS</p><h1>进入我的任务</h1><p className="auth-lead">使用工号和密码登录。登录后会保持可刷新的长期会话，员工端只读取属于你的 Edge Projection。</p><form className="employee-form" onSubmit={submit}><label htmlFor="employee-no">工号<input id="employee-no" value={employeeNo} onChange={(event) => setEmployeeNo(event.target.value)} autoComplete="username" required /></label><label htmlFor="employee-password">密码<input id="employee-password" value={password} onChange={(event) => setPassword(event.target.value)} type="password" autoComplete="current-password" required /></label><button className="employee-primary" disabled={busy} type="submit">{busy ? "正在验证…" : "登录并查看任务"}</button></form>{error && <Notice tone="error">{error}</Notice>}<div className="auth-entry-note"><strong>个人微信 / 企业微信</strong><span>两类入口最终绑定到同一个员工账号；网页版当前使用工号密码，平台 Provider 适配器接入后提供快捷登录。</span></div><p className="auth-note">管理端请从独立的 admin-web 入口访问。</p></section></main>;
}

function BindingPage(): JSX.Element {
  const navigate = useNavigate();
  const pending = sessionManager.getPendingBinding();
  const [provider, setProvider] = useState<Provider>("personal_wechat");
  const [providerCode, setProviderCode] = useState("mock:wx-user-1");
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<string>();
  if (!pending) return <main className="employee-auth-page"><section className="employee-auth-card"><h1>绑定请求已失效</h1><p className="auth-lead">请返回登录页重新开始一次绑定。</p><button className="employee-primary" onClick={() => navigate("/login")}>返回登录</button></section></main>;
  const bindingTicket = pending.ticket;

  async function submit(event: FormEvent<HTMLFormElement>): Promise<void> {
    event.preventDefault(); setBusy(true); setError(undefined);
    try { await sessionManager.completeBinding({ binding_ticket: bindingTicket, provider, provider_code: providerCode, client: "employee-web" }); navigate("/tasks"); } catch (value) { setError(toErrorMessage(value, "身份绑定失败，请重新发起平台登录。")); } finally { setBusy(false); }
  }
  return <main className="employee-auth-page"><section className="employee-auth-card"><p className="eyebrow">IDENTITY BINDING</p><h1>绑定快捷入口</h1><p className="auth-lead">工号已经验证。选择一个入口完成绑定，之后个人微信和企业微信都可以访问同一个员工账号。</p><form className="employee-form" onSubmit={submit}><div className="provider-switch"><button type="button" className={provider === "personal_wechat" ? "is-active" : ""} onClick={() => setProvider("personal_wechat")}>个人微信</button><button type="button" className={provider === "wecom" ? "is-active" : ""} onClick={() => setProvider("wecom")}>企业微信</button></div><label htmlFor="provider-code">平台授权码<input id="provider-code" value={providerCode} onChange={(event) => setProviderCode(event.target.value)} required /><small>开发环境使用 mock:subject；生产环境由微信/企业微信适配器填充一次性 code。</small></label><button className="employee-primary" disabled={busy} type="submit">{busy ? "正在绑定…" : "完成绑定并进入任务"}</button></form>{error && <Notice tone="error">{error}</Notice>}<button className="employee-secondary" onClick={() => navigate("/login")}>返回登录</button></section></main>;
}

function EmployeeLayout(): JSX.Element {
  const location = useLocation();
  const navigate = useNavigate();
  const principal = sessionManager.getSnapshot()?.principal;
  const [realtimeState, setRealtimeState] = useState<RealtimeState>(employeeRealtime.getState());
  useEffect(() => {
    const handleState = (event: Event) => setRealtimeState((event as CustomEvent<RealtimeState>).detail);
    const handleUnauthorized = () => { sessionManager.clear(); navigate("/login", { replace: true }); };
    window.addEventListener("flight:realtime-state", handleState);
    window.addEventListener("flight:realtime-unauthorized", handleUnauthorized);
    employeeRealtime.start();
    return () => {
      window.removeEventListener("flight:realtime-state", handleState);
      window.removeEventListener("flight:realtime-unauthorized", handleUnauthorized);
      employeeRealtime.stop();
    };
  }, [navigate]);
  async function logout(): Promise<void> { await sessionManager.logout(); navigate("/login"); }
  return <div className="employee-shell"><header className="employee-header"><Link to="/tasks" className="employee-brand"><span className="brand-mark">A</span><span><strong>我的任务</strong><small>EDGE PROJECTION</small></span></Link><div className="employee-header-right"><span className={`sync-state sync-${realtimeState}`} role="status"><i aria-hidden="true" /> {realtimeStateLabel[realtimeState]}</span><span className="role-chip">{principal?.role ?? "staff"}</span><button className="logout-button" onClick={logout}>退出</button></div></header><main className="employee-main"><Outlet /></main><nav className="employee-nav" aria-label="员工端导航">{[["/tasks", "任务", "⌂"], ["/notifications", "通知", "!"], ["/exceptions", "异常", "△"], ["/history", "历史", "↺"], ["/account", "账号", "○"]].map(([path, label, icon]) => <Link key={path} className={location.pathname === path || (path === "/tasks" && location.pathname.startsWith("/tasks/")) ? "is-active" : ""} to={path}><span>{icon}</span>{label}</Link>)}</nav></div>;
}

function EmployeeTaskListPage(): JSX.Element {
  const [items, setItems] = useState<EdgeTaskProjection[]>([]);
  const [filter, setFilter] = useState<"all" | TaskStatus>("all");
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string>();
  const [reloadKey, setReloadKey] = useState(0);
  useEffect(() => {
    const reload = () => setReloadKey((value) => value + 1);
    window.addEventListener("flight:task-changed", reload);
    window.addEventListener("flight:realtime-connected", reload);
    return () => { window.removeEventListener("flight:task-changed", reload); window.removeEventListener("flight:realtime-connected", reload); };
  }, []);
  useEffect(() => { const controller = new AbortController(); setLoading(true); setError(undefined); edgeApi.listTasks(controller.signal).then((response) => setItems(response.items)).catch((value: unknown) => { if (!controller.signal.aborted) setError(toErrorMessage(value, "无法读取你的任务，请稍后重试。")); }).finally(() => { if (!controller.signal.aborted) setLoading(false); }); return () => controller.abort(); }, [reloadKey]);
  const visible = filter === "all" ? items : items.filter((item) => projectionStatus(item) === filter);
  return <div className="employee-page"><div className="employee-page-heading"><div><p className="eyebrow">TODAY / MY WORK</p><h1>我的任务</h1><p>只展示 Edge 返回给当前员工的任务，不在客户端猜测其他人的数据。</p></div><button className="refresh-button" onClick={() => setReloadKey((value) => value + 1)}>刷新</button></div><div className="employee-counts"><button className={filter === "all" ? "is-active" : ""} onClick={() => setFilter("all")}>全部 <strong>{items.length}</strong></button><button className={filter === "assigned" ? "is-active" : ""} onClick={() => setFilter("assigned")}>待接受 <strong>{items.filter((item) => projectionStatus(item) === "assigned").length}</strong></button><button className={filter === "in_progress" ? "is-active" : ""} onClick={() => setFilter("in_progress")}>进行中 <strong>{items.filter((item) => projectionStatus(item) === "in_progress").length}</strong></button><button className={filter === "completed" ? "is-active" : ""} onClick={() => setFilter("completed")}>已完成 <strong>{items.filter((item) => projectionStatus(item) === "completed").length}</strong></button></div>{loading && <div className="employee-state">正在读取 Edge Projection…</div>}{error && <div className="employee-state"><Notice tone="error">{error}<button className="employee-secondary" onClick={() => setReloadKey((value) => value + 1)}>重新加载</button></Notice></div>}{!loading && !error && visible.length === 0 && <div className="employee-state"><span className="empty-symbol">∅</span><h2>当前没有分配给你的任务</h2><p>任务由管理端确认分配后，通过 Edge Projection 出现在这里。</p></div>}{!loading && !error && visible.length > 0 && <div className="employee-task-list">{visible.map((task) => <EmployeeTaskCard key={task.public_id} task={task} />)}</div>}</div>;
}

function EmployeeTaskCard({ task }: { task: EdgeTaskProjection }): JSX.Element {
  const status = projectionStatus(task);
  return <Link className="employee-task-card" to={`/tasks/${task.public_id}`}><div className="task-card-top"><span className="flight-number">{task.flight_display_no || "—"}</span><StatusBadge status={status} /></div><h2>{task.task_name}</h2><p>{task.area_name || "未提供区域"} · 计划 {formatDate(task.planned_at)}</p><div className="task-card-bottom"><span>{task.message || "暂无任务说明"}</span><strong>查看详情 →</strong></div></Link>;
}

function EmployeeTaskDetailPage(): JSX.Element {
  const { taskPublicID } = useParams();
  const [task, setTask] = useState<EdgeTaskProjection>();
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string>();
  const [note, setNote] = useState("");
  const [command, setCommand] = useState<{ id: string; status: CommandStatus }>();
  const [draftCommand, setDraftCommand] = useState<TaskCommandDraft>();
  const [commandError, setCommandError] = useState<string>();
  const [busy, setBusy] = useState(false);
  useEffect(() => {
    const stored = taskCommandStore.read(taskPublicID);
    setDraftCommand(stored);
    setCommand(stored?.status ? { id: stored.id, status: stored.status } : undefined);
    setCommandError(undefined);
  }, [taskPublicID]);
  const load = useCallback((): void => { if (!taskPublicID) return; const controller = new AbortController(); setLoading(true); setError(undefined); edgeApi.listTasks(controller.signal).then((response) => { const value = response.items.find((item) => item.public_id === taskPublicID); if (!value) throw new Error("任务不在当前员工 Projection 中，可能已被更新或撤回。"); setTask(value); }).catch((value: unknown) => { if (!controller.signal.aborted) setError(toErrorMessage(value, "无法读取任务详情。")); }).finally(() => { if (!controller.signal.aborted) setLoading(false); }); }, [taskPublicID]);
  useEffect(() => load(), [load]);
  useEffect(() => { const reload = () => load(); window.addEventListener("flight:task-changed", reload); window.addEventListener("flight:realtime-connected", reload); return () => { window.removeEventListener("flight:task-changed", reload); window.removeEventListener("flight:realtime-connected", reload); }; }, [load]);
  useEffect(() => { if (!command?.id) return; let stopped = false; let timer: number | undefined; let attempts = 0; const poll = async (): Promise<void> => { try { const response = await edgeApi.commandStatus(command.id); if (stopped) return; const nextStatus = response.data.status; setCommand((current) => current ? { ...current, status: nextStatus } : current); const stored = taskCommandStore.read(taskPublicID); if (stored?.id === command.id) taskCommandStore.write({ ...stored, status: nextStatus }); if (nextStatus === "confirmed" || nextStatus === "failed" || attempts >= 15) { taskCommandStore.clear(command.id); setDraftCommand(undefined); if (nextStatus === "failed") setCommandError(response.data.error_code ?? "command_failed"); if (nextStatus === "confirmed") load(); return; } attempts += 1; timer = window.setTimeout(() => void poll(), 2000); } catch (value) { if (!stopped) { setCommandError(toErrorMessage(value, "暂时无法查询操作同步状态，系统会自动重试。")); if (attempts < 15) { attempts += 1; timer = window.setTimeout(() => void poll(), 2000); } } } }; void poll(); return () => { stopped = true; if (timer) window.clearTimeout(timer); }; }, [command?.id, load, taskPublicID]);

  async function submit(action: "accept" | "complete"): Promise<void> {
    if (!task?.assignment_public_id) return; setBusy(true); setCommandError(undefined);
    const reusableDraft = draftCommand?.action === action && draftCommand.taskPublicID === task.public_id && draftCommand.assignmentPublicID === task.assignment_public_id && draftCommand.expectedSyncVersion === task.sync_version;
    const draft: TaskCommandDraft = reusableDraft && draftCommand ? draftCommand : { id: createClientID(), action, taskPublicID: task.public_id, assignmentPublicID: task.assignment_public_id, expectedSyncVersion: task.sync_version, note: note.trim() || undefined };
    setDraftCommand(draft); taskCommandStore.write(draft);
    const payload = { command_id: draft.id, assignment_public_id: draft.assignmentPublicID, expected_sync_version: draft.expectedSyncVersion, client_occurred_at: new Date().toISOString(), note: draft.note };
    try { const response = action === "accept" ? await edgeApi.acceptTask(task.public_id, payload) : await edgeApi.completeTask(task.public_id, payload); setDraftCommand(undefined); setCommand({ id: response.command_id, status: response.status }); taskCommandStore.write({ ...draft, status: response.status }); setNote(""); } catch (value) { if (value instanceof ApiClientError && value.status === 409) { taskCommandStore.clear(draft.id); setDraftCommand(undefined); load(); } setCommandError(toErrorMessage(value, "操作已提交失败，请检查网络后重试。")); } finally { setBusy(false); }
  }
  if (loading) return <div className="employee-state">正在读取任务详情…</div>;
  if (error || !task) return <div className="employee-state"><Notice tone="error">{error ?? "任务不可用。"}<Link className="employee-secondary" to="/tasks">返回任务列表</Link></Notice></div>;
  const status = projectionStatus(task); const activeCommand = command && command.status !== "confirmed" && command.status !== "failed";
  return <div className="employee-page employee-detail"><Link className="employee-back" to="/tasks">← 返回我的任务</Link><div className="detail-flight-strip"><div><span className="eyebrow">FLIGHT / {task.flight_display_no || "—"}</span><h1>{task.task_name}</h1><p>{task.area_name || "未提供区域"} · 计划 {formatDate(task.planned_at)}</p></div><StatusBadge status={status} /></div><section className="employee-detail-card"><p className="eyebrow">TASK BRIEF</p><h2>任务说明</h2><p className="task-message">{task.message || "暂无任务说明，请按现场要求执行。"}</p><div className="employee-facts"><span><small>Projection 版本</small><strong>s{task.sync_version}</strong></span><span><small>Assignment</small><strong>{task.assignment_public_id || "未分配"}</strong></span><span><small>最后更新</small><strong>{formatDate(task.updated_at)}</strong></span></div></section>{command && <section className="command-panel" role="status"><div><p className="eyebrow">COMMAND STATUS</p><h2>{commandStatusLabel[command.status]}</h2><p>{command.status === "pending" || command.status === "syncing" ? "操作已持久化到 Edge，刷新页面后仍会继续查询；不要重复点击。" : command.status === "confirmed" ? "服务端已确认，任务 Projection 正在刷新。" : "服务端报告操作失败，请查看原因后再决定是否重试。"}</p></div><CommandBadge status={command.status} /></section>}{commandError && <Notice tone="error">{commandError}</Notice>}{task.assignment_public_id && !activeCommand && <section className="employee-action-card"><label>现场备注（可选）<textarea value={note} onChange={(event) => { setNote(event.target.value); setDraftCommand(undefined); taskCommandStore.clearForTask(taskPublicID ?? ""); }} maxLength={255} placeholder="记录需要随本次操作提交的现场说明" /></label><div className="employee-actions">{canAccept(status) && <button className="employee-primary" disabled={busy} onClick={() => submit("accept")}>{busy ? "正在提交…" : draftCommand?.action === "accept" ? "重试提交" : "接受任务"}</button>}{canComplete(status) && <button className="employee-primary" disabled={busy} onClick={() => submit("complete")}>{busy ? "正在提交…" : draftCommand?.action === "complete" ? "重试提交" : "标记完成"}</button>}{!canAccept(status) && !canComplete(status) && <p className="muted">当前状态只读，最终状态由 Core Projection 决定。</p>}</div></section>}</div>;
}

function EmployeeEmptyPage({ title, description }: { title: string; description: string }): JSX.Element { return <div className="employee-page"><p className="eyebrow">EMPLOYEE / {title.toUpperCase()}</p><h1>{title}</h1><div className="employee-state empty-page"><span className="empty-symbol">□</span><h2>功能入口已保留</h2><p>{description}</p></div></div>; }

function EmployeeAccountPage(): JSX.Element { const snapshot = sessionManager.getSnapshot(); const navigate = useNavigate(); async function logout(): Promise<void> { await sessionManager.logout(); navigate("/login"); } return <div className="employee-page"><p className="eyebrow">ACCOUNT / SECURITY</p><h1>账号</h1><section className="account-card"><div className="account-avatar">{snapshot?.principal?.role?.slice(0, 1).toUpperCase() ?? "S"}</div><div><h2>员工会话</h2><p>当前角色：{snapshot?.principal?.role ?? "staff"}</p><small>Session：{snapshot?.sessionPublicID ?? "—"}</small></div></section><section className="account-card account-session"><div><strong>长期登录</strong><p>Access Token 短期有效，Refresh Token 由 Edge 轮换并可撤销。</p></div><span className="session-live">已启用</span></section><button className="employee-danger" onClick={logout}>退出当前会话</button></div>; }

function toErrorMessage(value: unknown, fallback: string): string { if (value instanceof ApiClientError) return `${value.body.code} · ${value.message}`; if (value instanceof Error) return value.message; return fallback; }
const realtimeStateLabel: Record<RealtimeState, string> = { idle: "等待连接", connecting: "正在连接", open: "已连接 Edge", retrying: "连接重试中", closed: "实时连接已关闭", unauthorized: "会话已失效" };
function formatDate(value: string): string { if (!value) return "—"; const date = new Date(value); return Number.isNaN(date.valueOf()) ? value : new Intl.DateTimeFormat("zh-CN", { month: "2-digit", day: "2-digit", hour: "2-digit", minute: "2-digit" }).format(date); }
