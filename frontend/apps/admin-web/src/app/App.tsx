import { createContext, useCallback, useContext, useEffect, useMemo, useRef, useState } from "react";
import { Link, Navigate, Outlet, Route, Routes, useLocation, useNavigate, useParams } from "react-router-dom";
import type { FormEvent, ReactNode } from "react";
import { ApiClientError } from "@flight/api-client";
import type { CoreAdminIdentity, CoreArea, CoreAssignment, CoreAuditEntry, CoreCapability, CoreDiagnostics, CoreEvent, CoreException, CoreFlight, CorePersonnel, CorePersonnelStatus, CorePersonnelStatusHistory, CorePosition, CoreReportOverview, CoreScopeView, CoreTask, CoreTaskChangeRequest, CoreTaskHistory, CoreTemplate, CoreTeam, TaskStatus } from "@flight/contracts";
import { ListPager, Notice, StatusBadge } from "@flight/ui";
import { createClientID } from "@flight/task-domain";
import { adminRuntime, adminSsoEnabled, adminSsoReturnPath, beginAdminSso, clearAdminAccessToken, coreApi, exchangeAdminSsoCode, hasAdminAccess, readAdminAccessToken, saveAdminAccessToken } from "./runtime";

const navGroups = [
  { label: "运行", items: [["overview", "运行总览"], ["flights", "航班运行"]] },
  { label: "任务", items: [["tasks", "任务工作台"], ["templates", "任务模板"], ["assignments", "任务分配"], ["history", "任务历史"]] },
  { label: "人员", items: [["organization", "组织与班组"], ["personnel", "人员档案"], ["positions", "岗位与能力"], ["status", "人员状态"]] },
  { label: "治理", items: [["rules", "规则配置"], ["events", "事件与通知"], ["exceptions", "异常处理"], ["changes", "任务变更"], ["audit", "审计日志"], ["reports", "报表统计"]] },
  { label: "系统", items: [["users-roles", "用户与角色"], ["scopes", "权限范围"], ["diagnostics", "开发诊断"]] },
] as const;

const moduleCopy: Record<string, { title: string; description: string }> = {
  overview: { title: "运行总览", description: "全场航班、任务、人员和同步状态的统一入口。" },
  organization: { title: "组织与班组", description: "维护运行区域和班组基础事实，人员、模板和权限范围都引用这里。" },
  flights: { title: "航班运行", description: "查看航班运行事实和到达触发记录。" },
  templates: { title: "任务模板", description: "维护任务名称、触发规则和岗位能力要求。" },
  assignments: { title: "任务分配", description: "维护队长管辖、候选人和 Assignment 入口。" },
  history: { title: "任务历史", description: "查看任务状态轨迹和历史处理记录。" },
  personnel: { title: "人员档案", description: "查看员工档案、团队和区域归属。" },
  positions: { title: "岗位与能力", description: "查看岗位定义和能力要求。" },
  status: { title: "人员状态", description: "查看现场可用状态和状态变更。" },
  rules: { title: "规则配置", description: "查看任务生成、候选计算和分配规则。" },
  events: { title: "事件与通知", description: "查看运行事件、通知和同步异常。" },
  exceptions: { title: "异常处理", description: "查看员工上报的现场异常，按严重程度和状态跟进。" },
  changes: { title: "任务变更", description: "查看员工和现场提交的变更申请，由值班经理或管理员审批后执行。" },
  audit: { title: "审计日志", description: "查看关键操作和状态变更的审计记录。" },
  reports: { title: "报表统计", description: "汇总任务、航班、人员和同步数据。" },
  "users-roles": { title: "用户与角色", description: "管理管理端用户、角色和入口权限。" },
  scopes: { title: "权限范围", description: "维护团队、区域与角色 Scope。" },
  diagnostics: { title: "开发诊断", description: "查看开发环境 API 请求和同步诊断信息。" },
};

// This matrix only controls navigation and affordances. Core remains the
// authority for every read and write request, including scope checks.
const moduleReadPermissions: Record<string, string> = {
  overview: "analytics:read",
  flights: "flight:read",
  tasks: "task:read",
  templates: "template:read",
  assignments: "assignment:read",
  history: "task:read",
  organization: "area:read",
  personnel: "personnel:read",
  positions: "position:read",
  status: "status:read",
  rules: "template:read",
  events: "event:read",
  exceptions: "exception:read",
  changes: "exception:manage",
  audit: "audit:read",
  reports: "analytics:read",
  "users-roles": "admin_identity:manage",
  scopes: "scope:read",
  diagnostics: "diagnostics:read",
};

const rolePermissions: Record<string, readonly string[]> = {
  admin: [
    "flight:read", "task:read", "task:assign", "task:cancel", "task:complete", "event:handle", "exception:read", "exception:manage", "personnel:read", "assignment:read", "rule:manage", "analytics:read", "area:read", "area:manage", "team:read", "team:manage", "template:read", "template:manage", "flight:manage", "personnel:manage", "admin_identity:manage", "position:read", "position:manage", "capability:read", "capability:manage", "status:read", "event:read", "audit:read", "scope:read", "diagnostics:read",
  ],
  manager: [
    "flight:read", "task:read", "task:assign", "task:cancel", "task:complete", "event:handle", "exception:read", "exception:manage", "personnel:read", "assignment:read", "analytics:read", "area:read", "team:read", "template:read", "position:read", "capability:read", "status:read", "event:read", "audit:read", "scope:read", "diagnostics:read",
  ],
  leader: [
    "flight:read", "task:read", "task:assign", "task:cancel", "event:handle", "exception:read", "exception:manage", "personnel:read", "assignment:read", "analytics:read", "area:read", "team:read", "template:read", "position:read", "capability:read", "status:read", "event:read", "scope:read",
  ],
  supervisor: [
    "flight:read", "task:read", "exception:read", "personnel:read", "assignment:read", "analytics:read", "area:read", "team:read", "template:read", "position:read", "capability:read", "status:read", "event:read", "scope:read",
  ],
};

const moduleBlueprints: Record<string, { dataBoundary: string; controls: string[]; blocks: string[]; nextStep: string }> = {
  positions: { dataBoundary: "岗位编码、能力标签与任务要求", controls: ["岗位筛选", "能力标签", "启用状态"], blocks: ["岗位定义表", "能力字典", "任务要求引用"], nextStep: "等待岗位与能力公开查询契约冻结" },
  status: { dataBoundary: "员工可用状态与状态变更时间线", controls: ["区域 / 班组", "工作状态", "更新时间"], blocks: ["人员状态看板", "状态变更记录", "不可用原因"], nextStep: "等待人员状态查询与审计字段冻结" },
  rules: { dataBoundary: "任务生成、候选计算和提醒规则", controls: ["规则类型", "业务场景", "启用状态"], blocks: ["规则列表", "版本对比", "生效范围"], nextStep: "等待规则配置读写契约和版本策略冻结" },
  events: { dataBoundary: "航班事件、运行通知和同步提示", controls: ["事件类型", "航班号", "处理状态"], blocks: ["事件时间线", "通知投递状态", "同步重试记录"], nextStep: "等待事件与通知公开查询契约冻结" },
  audit: { dataBoundary: "关键操作、状态变化与请求上下文", controls: ["操作者", "资源类型", "时间范围"], blocks: ["审计日志表", "请求详情", "变更前后摘要"], nextStep: "等待审计查询分页和脱敏字段冻结" },
  reports: { dataBoundary: "航班、任务、人员和异常运行指标", controls: ["日期范围", "区域 / 班组", "服务类型"], blocks: ["运行摘要", "任务完成率", "异常趋势"], nextStep: "等待报表统计口径和聚合接口冻结" },
  scopes: { dataBoundary: "角色、团队/区域与可见任务 Scope", controls: ["角色", "区域", "班组"], blocks: ["角色矩阵", "Scope 关系", "越权拒绝记录"], nextStep: "Scope 只由 Core 计算，页面不提供本地授权" },
  diagnostics: { dataBoundary: "API 请求、Projection 版本和同步延迟", controls: ["请求 ID", "客户端", "同步状态"], blocks: ["请求诊断", "Projection 检查", "Command 回执"], nextStep: "诊断信息只面向开发环境，不承载业务事实" },
};

const LIST_PAGE_SIZE = 20;
const REMOTE_OPTION_PAGE_SIZE = 20;

type RemoteOptions<T> = {
  items: T[];
  total: number;
  query: string;
  setQuery: (value: string) => void;
  loading: boolean;
  error?: string;
};

function useDebouncedValue(value: string, delayMs: number): string {
  const [debounced, setDebounced] = useState(value);
  useEffect(() => {
    const timer = globalThis.setTimeout(() => setDebounced(value), delayMs);
    return () => globalThis.clearTimeout(timer);
  }, [delayMs, value]);
  return debounced;
}

function useRemoteAreas(includeDisabled = false): RemoteOptions<CoreArea> {
  const [query, setQuery] = useState("");
  const [items, setItems] = useState<CoreArea[]>([]);
  const [total, setTotal] = useState(0);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string>();
  const debouncedQuery = useDebouncedValue(query.trim(), 250);

  useEffect(() => {
    const controller = new AbortController();
    setLoading(true);
    setError(undefined);
    coreApi.listAreas({ q: debouncedQuery || undefined, include_disabled: includeDisabled, page: 1, page_size: REMOTE_OPTION_PAGE_SIZE }, controller.signal).then((response) => {
      setItems(response.data.items);
      setTotal(response.data.total);
    }).catch((reason: unknown) => {
      if (!controller.signal.aborted) setError(readClientError(reason));
    }).finally(() => {
      if (!controller.signal.aborted) setLoading(false);
    });
    return () => controller.abort();
  }, [debouncedQuery, includeDisabled]);

  return { items, total, query, setQuery, loading, error };
}

function useRemoteTeams(areaPublicID?: string, includeDisabled = false): RemoteOptions<CoreTeam> {
  const [query, setQuery] = useState("");
  const [items, setItems] = useState<CoreTeam[]>([]);
  const [total, setTotal] = useState(0);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string>();
  const debouncedQuery = useDebouncedValue(query.trim(), 250);

  useEffect(() => {
    const controller = new AbortController();
    setLoading(true);
    setError(undefined);
    coreApi.listTeams(areaPublicID, includeDisabled, controller.signal, 1, REMOTE_OPTION_PAGE_SIZE, debouncedQuery).then((response) => {
      setItems(response.data.items);
      setTotal(response.data.total);
    }).catch((reason: unknown) => {
      if (!controller.signal.aborted) setError(readClientError(reason));
    }).finally(() => {
      if (!controller.signal.aborted) setLoading(false);
    });
    return () => controller.abort();
  }, [areaPublicID, debouncedQuery, includeDisabled]);

  return { items, total, query, setQuery, loading, error };
}

function RemoteOptionSelect<T>({ label, value, options, total, query, setQuery, loading, error, getValue, getLabel, onChange, required = false, disabled = false }: { label: string; value: string; options: T[]; total: number; query: string; setQuery: (value: string) => void; loading: boolean; error?: string; getValue: (item: T) => string; getLabel: (item: T) => string; onChange: (value: string) => void; required?: boolean; disabled?: boolean }): JSX.Element {
  return <div className="remote-option-select"><span className="remote-option-select__label">{label}</span><input type="search" value={query} onChange={(event) => setQuery(event.target.value)} placeholder={`搜索${label}`} aria-label={`搜索${label}`} disabled={disabled} /><select value={value} onChange={(event) => { onChange(event.target.value); setQuery(""); }} aria-label={`选择${label}`} required={required} disabled={disabled || loading}><option value="">{loading ? "正在加载…" : `选择${label}`}</option>{options.map((item) => <option key={getValue(item)} value={getValue(item)}>{getLabel(item)}</option>)}</select><small>{error ?? (loading ? "正在查询…" : `匹配 ${total} 条，展示前 ${options.length} 条`)}</small></div>;
}

type AdminAccessContextValue = {
  scope?: CoreScopeView;
  roles: string[];
  loading: boolean;
  error?: string;
  can: (permission: string) => boolean;
  refresh: () => void;
};

const AdminAccessContext = createContext<AdminAccessContextValue>({
  roles: [],
  loading: true,
  can: () => true,
  refresh: () => undefined,
});

function AdminAccessProvider({ children }: { children: ReactNode }): JSX.Element {
  const [scope, setScope] = useState<CoreScopeView>();
  const [roles, setRoles] = useState<string[]>(adminRuntime.devActorEnabled ? [adminRuntime.devActorRole] : []);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string>();
  const [reloadKey, setReloadKey] = useState(0);

  useEffect(() => {
    const controller = new AbortController();
    setLoading(true);
    setError(undefined);
    coreApi.getScopes(controller.signal).then((response) => {
      setScope(response.data);
      setRoles(response.data.roles);
    }).catch((reason: unknown) => {
      if (!controller.signal.aborted) setError(readClientError(reason));
    }).finally(() => {
      if (!controller.signal.aborted) setLoading(false);
    });
    return () => controller.abort();
  }, [reloadKey]);

  const can = useCallback((permission: string): boolean => {
    if (loading) return true;
    return roles.some((role) => rolePermissions[role]?.includes(permission) ?? false);
  }, [loading, roles]);
  const value = useMemo(() => ({ scope, roles, loading, error, can, refresh: () => setReloadKey((value) => value + 1) }), [can, error, loading, roles, scope]);
  return <AdminAccessContext.Provider value={value}>{children}</AdminAccessContext.Provider>;
}

function useAdminAccess(): AdminAccessContextValue {
  return useContext(AdminAccessContext);
}

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
  return <AdminAccessProvider><AdminShell /></AdminAccessProvider>;
}

function AdminShell(): JSX.Element {
  const location = useLocation();
  const navigate = useNavigate();
  const access = useAdminAccess();
  function logout(): void { clearAdminAccessToken(); navigate("/login"); }
  return <div className="admin-shell">
    <aside className="admin-sidebar">
      <Link to="/tasks" className="brand-lockup"><span className="brand-mark">A</span><span><strong>航空客运地面代理协同</strong><small>OPERATIONS / CONTROL</small></span></Link>
      <nav aria-label="管理端工作区" className="admin-nav">{navGroups.map((group) => { const items = group.items.filter(([section]) => access.loading || access.can(moduleReadPermissions[section])); if (items.length === 0) return null; return <div className="nav-group" key={group.label}><span className="nav-label">{group.label}</span>{items.map(([section, label]) => { const active = location.pathname.includes(`/${section}`); return <Link key={section} className={active ? "is-active" : ""} aria-current={active ? "page" : undefined} to={`/${section}`}>{label}</Link>; })}</div>; })}</nav>
      <div className="scope-card"><span>当前授权范围</span><strong>{access.loading ? "正在读取 Core Scope…" : access.scope?.global ? "全部团队 · 全部区域" : access.roles.includes("leader") ? "当前团队 / 区域" : "由 Core Scope 决定"}</strong><small>{access.error ? "权限状态读取失败，操作仍由 Core 校验" : "前端只做入口提示，不替代服务端授权"}</small></div>
      <div className="sidebar-footer"><div><span className="avatar">{adminRuntime.devActorEnabled ? adminRuntime.devActorRole.slice(0, 1).toUpperCase() : "C"}</span><span><strong>{adminRuntime.devActorEnabled ? adminRuntime.devActorRole : "Core session"}</strong><small>管理端会话</small></span></div><button className="text-button" onClick={logout}>退出</button></div>
    </aside>
    <section className="admin-content"><header className="content-topline"><span>运行管理台 <i>/</i> {location.pathname.startsWith("/tasks") ? "任务工作台" : "工作区"}</span><span className="sync-pill">Core API · 实时读取</span></header><div className="page-body"><Outlet /></div></section>
  </div>;
}

function ModulePage({ section }: { section: string }): JSX.Element {
  const access = useAdminAccess();
  const requiredPermission = moduleReadPermissions[section];
  if (!access.loading && requiredPermission && !access.can(requiredPermission)) return <AccessDeniedPage section={section} />;
  if (section === "overview") return <OverviewPage />;
  if (section === "organization") return <OrganizationPage />;
  if (section === "flights") return <FlightManagementPage />;
  if (section === "personnel") return <PersonnelManagementPageV2 />;
  if (section === "templates") return <TemplateManagementPageV2 />;
  if (section === "assignments") return <AssignmentManagementPage />;
  if (section === "users-roles") return <AdminIdentityManagementPage />;
  if (section === "history") return <TaskHistoryPage />;
  if (section === "exceptions") return <ExceptionManagementPage />;
  if (section === "changes") return <TaskChangeRequestsPage />;
  if (section === "rules") return <RulesPage />;
  if (section === "reports") return <ReportsPage />;
  if (section === "positions") return <PositionCapabilityPage />;
  if (section === "status") return <PersonnelStatusPage />;
  if (section === "events") return <EventsPage />;
  if (section === "audit") return <AuditPage />;
  if (section === "scopes") return <ScopesPage />;
  if (section === "diagnostics") return <DiagnosticsPage />;
  const copy = moduleCopy[section] ?? moduleCopy.overview;
  const blueprint = moduleBlueprints[section];
  return <div className="module-page"><div className="page-heading"><div><p className="eyebrow">MODULE / {section.toUpperCase()}</p><h1>{copy.title}</h1><p className="page-description">{copy.description}</p></div><span className="module-stage">首期页面设计壳</span></div><div className="module-blueprint"><section className="blueprint-card blueprint-card-primary"><p className="eyebrow">DATA BOUNDARY</p><h2>{blueprint?.dataBoundary ?? "业务模块数据"}</h2><p>数据由 Core API 提供，页面只负责展示和提交已冻结的业务命令。</p></section><section className="blueprint-card"><p className="eyebrow">PLANNED CONTROLS</p><ul>{(blueprint?.controls ?? ["筛选条件", "状态", "更新时间"]).map((item) => <li key={item}>{item}</li>)}</ul></section><section className="blueprint-card"><p className="eyebrow">PAGE BLOCKS</p><ul>{(blueprint?.blocks ?? ["主数据区", "状态区", "审计区"]).map((item) => <li key={item}>{item}</li>)}</ul></section></div><section className="empty-panel"><span className="empty-symbol">□</span><p className="eyebrow">DATA CONTRACT PENDING</p><h2>页面边界已注册</h2><p>当前模块保留完整页面结构、筛选入口和状态位置，待后端公开接口契约确认后接入真实数据；不会用浏览器内的临时数据冒充业务事实。</p><div className="empty-facts"><span><small>数据源</small><strong>待接入 Core API</strong></span><span><small>授权范围</small><strong>由服务端 Scope 决定</strong></span><span><small>下一步</small><strong>{blueprint?.nextStep ?? "等待接口契约冻结"}</strong></span></div></section></div>;
}

function AccessDeniedPage({ section }: { section: string }): JSX.Element {
  const copy = moduleCopy[section] ?? { title: "当前模块", description: "" };
  return <div className="module-page"><div className="page-heading"><div><p className="eyebrow">CORE / AUTHORIZATION</p><h1>{copy.title}</h1><p className="page-description">{copy.description}</p></div></div><section className="empty-panel"><span className="empty-symbol">⊘</span><p className="eyebrow">ACCESS DENIED</p><h2>当前角色没有访问权限</h2><p>该页面入口已按当前 Core 角色隐藏或限制。若权限配置有误，请联系系统管理员调整管理身份 Scope。</p><div className="empty-facts"><span><small>数据源</small><strong>Core 权限判定</strong></span><span><small>前端行为</small><strong>不提交越权请求</strong></span><span><small>模块</small><strong>{section}</strong></span></div></section></div>;
}

function TaskListPage(): JSX.Element {
  const [status, setStatus] = useState<TaskStatus | "">("");
  const [items, setItems] = useState<CoreTask[]>([]);
  const [page, setPage] = useState(1);
  const [total, setTotal] = useState(0);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<ApiClientError>();
  const [requestID, setRequestID] = useState("");
  const [reloadKey, setReloadKey] = useState(0);

  useEffect(() => {
    const controller = new AbortController();
    setLoading(true); setError(undefined);
    coreApi.listTasks({ status: status || undefined, page, page_size: LIST_PAGE_SIZE }, controller.signal).then((response) => {
      setItems(response.data.items); setPage(response.data.page); setTotal(response.data.total); setRequestID(response.request_id ?? "");
    }).catch((value: unknown) => { if (!controller.signal.aborted) setError(value instanceof ApiClientError ? value : new ApiClientError(0, { code: "network_error", message: "无法连接 Core API" })); }).finally(() => { if (!controller.signal.aborted) setLoading(false); });
    return () => controller.abort();
  }, [page, status, reloadKey]);

  return <div className="module-page task-page"><div className="page-heading"><div><p className="eyebrow">TASK / CORE FACTS</p><h1>任务工作台</h1><p className="page-description">任务可见范围由 Core 根据主任、队长角色和团队/区域 Scope 决定。</p></div><button className="secondary-button" onClick={() => setReloadKey((value) => value + 1)}>刷新列表</button></div>
    <div className="metric-row"><Metric label="当前可见" value={String(items.length)} /><Metric label="待派发/收件" value={String(items.filter((task) => task.status === "pending_dispatch" || task.status === "awaiting_confirmation" || task.status === "assigned").length)} /><Metric label="执行中" value={String(items.filter((task) => task.status === "in_progress").length)} /><Metric label="已完成" value={String(items.filter((task) => task.status === "completed").length)} /></div>
    <section className="task-panel"><div className="toolbar"><label>状态<select value={status} onChange={(event) => { setStatus(event.target.value as TaskStatus | ""); setPage(1); }}><option value="">全部状态</option><option value="pending_dispatch">待自动派发</option><option value="awaiting_confirmation">待确认（兼容）</option><option value="assigned">已派发/待收件</option><option value="in_progress">执行中</option><option value="completed">已完成</option><option value="cancelled">已取消</option></select></label><span className="scope-inline">{adminRuntime.devActorEnabled ? `${adminRuntime.devActorRole} · 开发 Scope` : "Bearer JWT · Core Scope"}</span></div>
      {loading && <div className="panel-state">正在从 Core 读取任务…</div>}
      {error && <div className="panel-state"><Notice tone="error"><strong>{error.body.code}</strong> · {error.message}{error.body.request_id && <small>request_id: {error.body.request_id}</small>}<button className="secondary-button" onClick={() => setReloadKey((value) => value + 1)}>重新读取</button></Notice></div>}
      {!loading && !error && items.length === 0 && <div className="panel-state"><span className="empty-symbol">∅</span><h2>当前范围没有任务</h2><p>如果你是队长，这表示当前团队/区域没有返回可见任务；如果你是主任，请检查 Core 数据或筛选条件。</p></div>}
      {!loading && !error && items.length > 0 && <div className="table-scroll"><table><thead><tr><th>计划时间</th><th>航班 / 任务</th><th>区域 / 团队</th><th>状态</th><th>版本</th><th /></tr></thead><tbody>{items.map((task) => <tr key={task.public_id}><td className="data-number">{formatDate(task.planned_at)}</td><td><Link className="task-link" to={`/tasks/${task.public_id}`}><strong>{task.flight_display_no || task.flight_public_id}</strong><span>{task.name}</span></Link></td><td><span>{task.area_public_id}</span><small>{task.team_public_id}</small></td><td><StatusBadge status={task.status} /></td><td className="data-number">v{task.status_version} / s{task.sync_version}</td><td><Link className="row-link" to={`/tasks/${task.public_id}`}>详情 →</Link></td></tr>)}</tbody></table></div>}
      <ListPager page={page} pageSize={LIST_PAGE_SIZE} total={total} onPageChange={setPage} disabled={loading} />
      <footer className="list-footer"><span>Core 返回当前页 {items.length} 条，列表查询已分页</span>{requestID && <span>request_id: {requestID}</span>}</footer>
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
  const canManage = task.status === "pending_dispatch" || task.status === "awaiting_confirmation";
  const canCancel = task.status !== "completed" && task.status !== "cancelled";
  const actionTitle = canManage ? "处理这个任务" : canCancel ? "受控处置任务" : "当前为只读状态";
  const actionDescription = canManage
    ? "系统会自动从候选人中派发；此处仅用于自动派发失败时的受控补派，不是员工审批。取消会记录原因。"
    : canCancel
      ? "任务已经派发或正在执行。航班延误、取消或现场事件需要调整时，由值班经理按受控指令取消并记录原因。"
      : "任务当前状态不允许在此处继续处理。";
  return <div className="module-page task-detail-page"><button className="back-link button-reset" onClick={() => navigate("/tasks")}>← 返回任务工作台</button><div className="detail-heading"><div><p className="eyebrow">FLIGHT / {task.flight_display_no || task.flight_public_id}</p><h1>{task.name}</h1><p className="page-description">{task.area_public_id} · {task.team_public_id} · {task.message || "暂无任务说明"}</p></div><StatusBadge status={task.status} /></div>{notice && <Notice tone={notice.tone}>{notice.text}</Notice>}<div className="detail-grid"><section className="detail-card"><h2>任务事实</h2><div className="fact-grid"><Fact label="计划时间" value={formatDate(task.planned_at)} /><Fact label="触发类型" value={task.trigger_type || "—"} /><Fact label="Task 版本" value={`v${task.status_version}`} /><Fact label="同步版本" value={`s${task.sync_version}`} /><Fact label="岗位要求" value={task.required_position_code || "—"} /><Fact label="能力要求" value={task.required_capabilities.join("、") || "—"} /></div></section><section className="detail-card"><h2>候选人员</h2>{task.candidates.length === 0 ? <p className="muted">Core 没有返回候选人员。</p> : <div className="candidate-list">{task.candidates.map((candidate) => <label className={`candidate ${candidate.public_id === candidateID ? "is-selected" : ""}`} key={candidate.public_id}><input type="radio" name="candidate" value={candidate.public_id} checked={candidate.public_id === candidateID} onChange={() => { setCandidateID(candidate.public_id); setConfirmationID(undefined); }} disabled={!canManage || candidate.status !== "proposed"} /><span><strong>#{candidate.rank} · {candidate.personnel_public_id}</strong><small>{candidate.matched_position_code} · {candidate.matched_capabilities.join("、") || "能力快照未提供"}</small></span><em>{candidate.status}</em></label>)}</div>}</section></div><aside className="action-card"><div><p className="eyebrow">CORE COMMAND</p><h2>{actionTitle}</h2><p>{actionDescription}</p></div>{(canManage || canCancel) && <div className="action-controls">{canManage && <button className="primary-button" disabled={!candidateID || busy !== ""} onClick={confirm}>{busy === "confirm" ? "正在提交…" : confirmationID ? "重试提交" : "受控补派"}</button>}<label>取消原因<input value={reason} onChange={(event) => { setReason(event.target.value); setCancellationID(undefined); }} placeholder="请输入取消原因" maxLength={255} /></label>{canCancel && <button className="danger-button" disabled={!reason.trim() || busy !== ""} onClick={cancel}>{busy === "cancel" ? "正在提交…" : cancellationID ? "重试取消" : "取消任务"}</button>}</div>}</aside></div>;
}

function TaskHistoryPage(): JSX.Element {
  const [tasks, setTasks] = useState<CoreTask[]>([]);
  const [historyFilter, setHistoryFilter] = useState<"all" | "completed" | "cancelled">("all");
  const [page, setPage] = useState(1);
  const [total, setTotal] = useState(0);
  const [selectedID, setSelectedID] = useState("");
  const [history, setHistory] = useState<CoreTaskHistory>();
  const [loading, setLoading] = useState(true);
  const [historyLoading, setHistoryLoading] = useState(false);
  const [message, setMessage] = useState<string>();
  const [reloadKey, setReloadKey] = useState(0);

  useEffect(() => {
    const controller = new AbortController();
    setLoading(true); setMessage(undefined);
    const requestedPageSize = historyFilter === "all" ? Math.ceil(LIST_PAGE_SIZE / 2) : LIST_PAGE_SIZE;
    const requests = historyFilter === "all"
      ? Promise.all([
        coreApi.listTasks({ status: "completed", page, page_size: requestedPageSize }, controller.signal),
        coreApi.listTasks({ status: "cancelled", page, page_size: requestedPageSize }, controller.signal),
      ]).then(([completed, cancelled]) => ({ items: [...completed.data.items, ...cancelled.data.items], total: completed.data.total + cancelled.data.total }))
      : coreApi.listTasks({ status: historyFilter, page, page_size: requestedPageSize }, controller.signal).then((response) => ({ items: response.data.items, total: response.data.total }));
    requests.then(({ items: historical, total: historicalTotal }) => {
      setTasks(historical); setTotal(historicalTotal);
      setSelectedID((current) => current && historical.some((task) => task.public_id === current) ? current : historical[0]?.public_id ?? "");
    }).catch((error: unknown) => { if (!controller.signal.aborted) setMessage(readClientError(error)); }).finally(() => { if (!controller.signal.aborted) setLoading(false); });
    return () => controller.abort();
  }, [historyFilter, page, reloadKey]);

  useEffect(() => {
    if (!selectedID) { setHistory(undefined); return; }
    const controller = new AbortController();
    setHistoryLoading(true); setMessage(undefined);
    coreApi.getTaskHistory(selectedID, controller.signal).then((response) => setHistory(response.data)).catch((error: unknown) => { if (!controller.signal.aborted) setMessage(readClientError(error)); }).finally(() => { if (!controller.signal.aborted) setHistoryLoading(false); });
    return () => controller.abort();
  }, [selectedID]);

  return <div className="module-page"><div className="page-heading"><div><p className="eyebrow">TASK / IMMUTABLE HISTORY</p><h1>任务历史</h1><p className="page-description">历史轨迹直接来自 Core 状态历史，包含任务、分配和人员状态变化。</p></div><button className="secondary-button" onClick={() => setReloadKey((value) => value + 1)}>刷新历史</button></div>{message && <Notice tone="error">{message}</Notice>}<div className="history-layout"><section className="detail-card history-task-picker"><div className="history-picker-heading"><h2>已结束任务</h2><select value={historyFilter} onChange={(event) => { setHistoryFilter(event.target.value as "all" | "completed" | "cancelled"); setPage(1); }}><option value="all">全部</option><option value="completed">已完成</option><option value="cancelled">已取消</option></select></div>{loading && <div className="panel-state">正在读取 Core 历史任务…</div>}{!loading && tasks.length === 0 && <div className="panel-state">当前 Scope 没有已完成或已取消任务。</div>}{!loading && tasks.map((task) => <button className={`history-task-option ${selectedID === task.public_id ? "is-selected" : ""}`} key={task.public_id} onClick={() => setSelectedID(task.public_id)}><span><strong>{task.flight_display_no || task.flight_public_id}</strong><small>{task.name}</small></span><StatusBadge status={task.status} /></button>)}{!loading && <ListPager page={page} pageSize={LIST_PAGE_SIZE} total={total} onPageChange={setPage} disabled={loading} />}</section><section className="detail-card history-timeline"><h2>{history ? `状态轨迹 · ${history.task_public_id}` : "状态轨迹"}</h2>{historyLoading && <div className="panel-state">正在读取不可变状态历史…</div>}{!historyLoading && history && history.events.length === 0 && <div className="panel-state">该任务还没有可展示的状态历史。</div>}{!historyLoading && history && history.events.length > 0 && <div className="history-events">{history.events.map((event) => <article className="history-event" key={event.public_id}><span className={`history-event-dot history-${event.kind}`} /><div><div className="history-event-top"><strong>{event.kind} · {event.to_status}</strong><small>{formatDate(event.occurred_at)}</small></div><p>{event.from_status ? `${event.from_status} → ` : ""}{event.to_status}{event.reason ? ` · ${event.reason}` : ""}</p><small>actor: {event.actor_public_id || event.actor_type}{event.personnel_public_id ? ` · personnel: ${event.personnel_public_id}` : ""}</small></div></article>)}</div>}{!historyLoading && !history && !loading && <div className="panel-state">选择左侧任务查看完整轨迹。</div>}</section></div></div>;
}

export function LegacyExceptionManagementPage(): JSX.Element {
  const [items, setItems] = useState<CoreException[]>([]);
  const [status, setStatus] = useState("");
  const [severity, setSeverity] = useState("");
  const [loading, setLoading] = useState(true);
  const [message, setMessage] = useState<string>();
  const [reloadKey, setReloadKey] = useState(0);
  useEffect(() => {
    const controller = new AbortController();
    setLoading(true); setMessage(undefined);
    coreApi.listExceptions({ status: status || undefined, severity: severity || undefined, page: 1, page_size: LIST_PAGE_SIZE }, controller.signal).then((response) => setItems(response.data.items)).catch((error: unknown) => { if (!controller.signal.aborted) setMessage(readClientError(error)); }).finally(() => { if (!controller.signal.aborted) setLoading(false); });
    return () => controller.abort();
  }, [reloadKey, status, severity]);
  return <div className="module-page"><div className="page-heading"><div><p className="eyebrow">GOVERNANCE / EXCEPTIONS</p><h1>异常处理</h1><p className="page-description">Core 事实、员工归属和审计记录在同一条 Command 事务中完成。</p></div><button className="secondary-button" onClick={() => setReloadKey((value) => value + 1)}>刷新异常</button></div><section className="task-panel"><div className="toolbar"><label>状态<select value={status} onChange={(event) => setStatus(event.target.value)}><option value="">全部状态</option><option value="open">待处理</option><option value="acknowledged">已确认</option><option value="resolved">已解决</option><option value="rejected">已驳回</option></select></label><label>严重程度<select value={severity} onChange={(event) => setSeverity(event.target.value)}><option value="">全部等级</option><option value="low">低</option><option value="medium">中</option><option value="high">高</option><option value="critical">紧急</option></select></label><span className="scope-inline">Core Scope / {items.length} 条</span></div>{loading && <div className="panel-state">正在读取 Core 异常…</div>}{message && <div className="panel-state"><Notice tone="error">{message}</Notice></div>}{!loading && !message && items.length === 0 && <div className="panel-state">当前 Scope 没有异常记录。</div>}{!loading && !message && items.length > 0 && <div className="table-scroll"><table><thead><tr><th>时间</th><th>航班 / 任务</th><th>员工</th><th>等级</th><th>类别 / 描述</th><th>状态</th></tr></thead><tbody>{items.map((item) => <tr key={item.public_id}><td className="data-number">{formatDate(item.reported_at)}</td><td><strong>{item.flight_display_no || item.flight_public_id}</strong><small>{item.task_public_id}</small></td><td><strong>{item.personnel_name}</strong><small>{item.personnel_public_id}</small></td><td><span className={`exception-severity severity-${item.severity}`}>{item.severity}</span></td><td><strong>{item.category}</strong><small className="exception-description">{item.description}</small></td><td><span className={`exception-status exception-${item.status}`}>{item.status}</span></td></tr>)}</tbody></table></div>}</section></div>;
}

function ExceptionManagementPage(): JSX.Element {
  const [items, setItems] = useState<CoreException[]>([]);
  const [status, setStatus] = useState("");
  const [severity, setSeverity] = useState("");
  const [page, setPage] = useState(1);
  const [total, setTotal] = useState(0);
  const [loading, setLoading] = useState(true);
  const [busyID, setBusyID] = useState<string>();
  const [message, setMessage] = useState<string>();
  const [reloadKey, setReloadKey] = useState(0);
  useEffect(() => {
    const controller = new AbortController();
    setLoading(true); setMessage(undefined);
    coreApi.listExceptions({ status: status || undefined, severity: severity || undefined, page, page_size: LIST_PAGE_SIZE }, controller.signal).then((response) => { setItems(response.data.items); setTotal(response.data.total); }).catch((error: unknown) => { if (!controller.signal.aborted) setMessage(readClientError(error)); }).finally(() => { if (!controller.signal.aborted) setLoading(false); });
    return () => controller.abort();
  }, [page, reloadKey, status, severity]);
  async function update(item: CoreException, nextStatus: "acknowledged" | "resolved" | "rejected"): Promise<void> {
    const promptedNote = nextStatus === "acknowledged" ? "" : window.prompt("请输入处置备注（可留空）", "");
    if (promptedNote === null) return;
    const resolutionNote = promptedNote;
    setBusyID(item.public_id); setMessage(undefined);
    try { await coreApi.updateException(item.public_id, { status: nextStatus, resolution_note: resolutionNote }); setReloadKey((value) => value + 1); } catch (error) { setMessage(readClientError(error)); } finally { setBusyID(undefined); }
  }
  return <div className="module-page"><div className="page-heading"><div><p className="eyebrow">GOVERNANCE / EXCEPTIONS</p><h1>异常处置</h1><p className="page-description">员工上报先写入 Core 异常事实；管理端在当前 Scope 内确认、解决或驳回，并保留处置审计。</p></div><button className="secondary-button" onClick={() => setReloadKey((value) => value + 1)}>刷新异常</button></div><section className="task-panel"><div className="toolbar"><label>状态<select value={status} onChange={(event) => { setStatus(event.target.value); setPage(1); }}><option value="">全部状态</option><option value="open">待处理</option><option value="acknowledged">已确认</option><option value="resolved">已解决</option><option value="rejected">已驳回</option></select></label><label>严重程度<select value={severity} onChange={(event) => { setSeverity(event.target.value); setPage(1); }}><option value="">全部等级</option><option value="low">低</option><option value="medium">中</option><option value="high">高</option><option value="critical">紧急</option></select></label><span className="scope-inline">Core Scope / {total} 条</span></div>{loading && <div className="panel-state">正在读取 Core 异常…</div>}{message && <div className="panel-state"><Notice tone="error">{message}</Notice></div>}{!loading && !message && items.length === 0 && <div className="panel-state">当前 Scope 没有异常记录。</div>}{!loading && !message && items.length > 0 && <div className="table-scroll"><table><thead><tr><th>时间</th><th>航班 / 任务</th><th>员工</th><th>等级</th><th>类别 / 描述</th><th>状态 / 操作</th></tr></thead><tbody>{items.map((item) => <tr key={item.public_id}><td className="data-number">{formatDate(item.reported_at)}</td><td><strong>{item.flight_display_no || item.flight_public_id}</strong><small>{item.task_public_id}</small></td><td><strong>{item.personnel_name}</strong><small>{item.personnel_public_id}</small></td><td><span className={`exception-severity severity-${item.severity}`}>{item.severity}</span></td><td><strong>{item.category}</strong><small className="exception-description">{item.description}</small></td><td><span className={`exception-status exception-${item.status}`}>{item.status}</span>{item.status === "open" && <button className="text-button light-text-button" disabled={!!busyID} onClick={() => update(item, "acknowledged")}>确认</button>}{(item.status === "open" || item.status === "acknowledged") && <><button className="text-button light-text-button" disabled={!!busyID} onClick={() => update(item, "resolved")}>解决</button><button className="text-button light-text-button" disabled={!!busyID} onClick={() => update(item, "rejected")}>驳回</button></>}</td></tr>)}</tbody></table></div>}{!loading && !message && <ListPager page={page} pageSize={LIST_PAGE_SIZE} total={total} onPageChange={setPage} disabled={loading} />}</section></div>;
}

function TaskChangeRequestsPage(): JSX.Element {
  const access = useAdminAccess();
  const canReview = access.roles.includes("admin") || access.roles.includes("manager");
  const [items, setItems] = useState<CoreTaskChangeRequest[]>([]);
  const [page, setPage] = useState(1);
  const [total, setTotal] = useState(0);
  const [loading, setLoading] = useState(true);
  const [busyID, setBusyID] = useState<string>();
  const [message, setMessage] = useState<string>();
  const [reloadKey, setReloadKey] = useState(0);

  useEffect(() => {
    if (!canReview) return;
    const controller = new AbortController();
    setLoading(true); setMessage(undefined);
    coreApi.listTaskChangeRequests({ status: "pending", page, page_size: LIST_PAGE_SIZE }, controller.signal).then((response) => { setItems(response.data.items); setTotal(response.data.total); }).catch((error: unknown) => { if (!controller.signal.aborted) setMessage(readClientError(error)); }).finally(() => { if (!controller.signal.aborted) setLoading(false); });
    return () => controller.abort();
  }, [canReview, page, reloadKey]);

  async function review(item: CoreTaskChangeRequest, decision: "approve" | "reject"): Promise<void> {
    const note = window.prompt(decision === "approve" ? "审批备注（可留空）" : "驳回原因", "");
    if (note === null || (decision === "reject" && !note.trim())) return;
    setBusyID(item.public_id); setMessage(undefined);
    try { await coreApi.reviewTaskChangeRequest(item.public_id, { decision, review_note: note.trim() }); setReloadKey((value) => value + 1); } catch (error) { setMessage(readClientError(error)); } finally { setBusyID(undefined); }
  }

  if (!access.loading && !canReview) return <AccessDeniedPage section="changes" />;
  return <ManagementFrame eyebrow="GOVERNANCE / TASK CHANGES" title="任务变更审批" description="员工或现场只提交申请；审批通过后 Core 在同一事务中变更任务、释放或预留人员，并写入 History、Audit 和 Outbox。员工没有拒绝任务的动作。" message={message}><section className="task-panel"><div className="toolbar"><strong>待审批申请</strong><span className="scope-inline">{total} requests</span><button className="secondary-button" onClick={() => setReloadKey((value) => value + 1)}>刷新</button></div>{loading && <div className="panel-state">正在读取 Core 变更申请…</div>}{!loading && items.length === 0 && <div className="panel-state">当前 Scope 没有待审批变更</div>}{!loading && items.length > 0 && <div className="table-scroll"><table><thead><tr><th>申请时间</th><th>任务</th><th>动作</th><th>原因</th><th>申请人</th><th>操作</th></tr></thead><tbody>{items.map((item) => <tr key={item.public_id}><td className="data-number">{formatDate(item.requested_at)}</td><td><strong>{item.task_public_id}</strong><small>{item.exception_public_id || "现场申请"}</small></td><td>{item.action}{item.target_planned_at && <small>{formatDate(item.target_planned_at)}</small>}</td><td>{item.reason}</td><td>{item.requested_by_public_id}</td><td><button className="text-button light-text-button" disabled={!!busyID} onClick={() => review(item, "approve")}>批准</button><button className="text-button light-text-button" disabled={!!busyID} onClick={() => review(item, "reject")}>驳回</button></td></tr>)}</tbody></table></div>}<ListPager page={page} pageSize={LIST_PAGE_SIZE} total={total} onPageChange={setPage} disabled={loading || !!busyID} /></section></ManagementFrame>;
}

type DictionaryForm = { code: string; name: string; description: string };

function PersonnelManagementPageV2(): JSX.Element {
  const canManage = useAdminAccess().can("personnel:manage");
  const [personnel, setPersonnel] = useState<CorePersonnel[]>([]);
  const [positions, setPositions] = useState<CorePosition[]>([]);
  const [capabilities, setCapabilities] = useState<CoreCapability[]>([]);
  const [page, setPage] = useState(1);
  const [total, setTotal] = useState(0);
  const [form, setForm] = useState({ employee_no: "", display_name: "", team_public_id: "", position_code: "", capability_code: "", password: "" });
  const [busy, setBusy] = useState(false);
  const [message, setMessage] = useState<string>();
  const [reloadKey, setReloadKey] = useState(0);
  const remoteTeams = useRemoteTeams();

  useEffect(() => {
    const controller = new AbortController();
    Promise.all([
      coreApi.listPersonnel({ page, page_size: LIST_PAGE_SIZE }, controller.signal),
      coreApi.listPositions({}, controller.signal),
      coreApi.listCapabilities({}, controller.signal),
    ]).then(([personnelResult, positionResult, capabilityResult]) => {
      setPersonnel(personnelResult.data.items); setTotal(personnelResult.data.total); setPositions(positionResult.data.items); setCapabilities(capabilityResult.data.items);
      setForm((current) => ({ ...current, position_code: current.position_code || positionResult.data.items.find((item) => item.enabled)?.code || "", capability_code: current.capability_code || capabilityResult.data.items.find((item) => item.enabled)?.code || "" }));
    }).catch((error: unknown) => { if (!controller.signal.aborted) setMessage(readClientError(error)); });
    return () => controller.abort();
  }, [page, reloadKey]);

  useEffect(() => {
    if (remoteTeams.items.length === 0) return;
    setForm((current) => current.team_public_id ? current : { ...current, team_public_id: remoteTeams.items[0].public_id });
  }, [remoteTeams.items]);

  async function createPersonnel(event: FormEvent<HTMLFormElement>): Promise<void> {
    event.preventDefault(); setBusy(true); setMessage(undefined);
    try { await coreApi.createPersonnel(form); setForm((current) => ({ ...current, employee_no: "", display_name: "", position_code: "", capability_code: "", password: "" })); setPage(1); setReloadKey((value) => value + 1); } catch (error) { setMessage(readClientError(error)); } finally { setBusy(false); }
  }

  async function toggle(person: CorePersonnel): Promise<void> {
    setBusy(true); setMessage(undefined);
    try { await coreApi.updatePersonnel(person.public_id, { enabled: !person.enabled }); setReloadKey((value) => value + 1); } catch (error) { setMessage(readClientError(error)); } finally { setBusy(false); }
  }

  async function editPersonnel(person: CorePersonnel): Promise<void> {
    const employeeNo = window.prompt("工号（4–12 位数字）", person.employee_no);
    if (employeeNo === null) return;
    const displayName = window.prompt("姓名", person.display_name);
    if (displayName === null) return;
    const teamPublicID = window.prompt("班组 public_id", person.team_public_id);
    if (teamPublicID === null) return;
    const positionCode = window.prompt("岗位编码", person.position_code);
    if (positionCode === null) return;
    const capabilityCode = window.prompt("能力编码", person.capability_code || person.capabilities[0] || "");
    if (capabilityCode === null) return;
    if (!/^[0-9]{4,12}$/.test(employeeNo.trim()) || !displayName.trim() || !teamPublicID.trim() || !positionCode.trim() || !capabilityCode.trim()) {
      setMessage("人员信息不完整：工号必须为 4–12 位数字，其他字段不能为空");
      return;
    }
    setBusy(true); setMessage(undefined);
    try {
      await coreApi.updatePersonnel(person.public_id, { employee_no: employeeNo.trim(), display_name: displayName.trim(), team_public_id: teamPublicID.trim(), position_code: positionCode.trim(), capability_code: capabilityCode.trim(), capabilities: [capabilityCode.trim()] });
      setReloadKey((value) => value + 1);
      setMessage(`已更新 ${displayName.trim()} 的人员档案`);
    } catch (error) { setMessage(readClientError(error)); } finally { setBusy(false); }
  }

  async function resetPassword(person: CorePersonnel): Promise<void> {
    const password = window.prompt("输入新密码（至少 8 位）");
    if (!password) return;
    setBusy(true); setMessage(undefined);
    try { await coreApi.resetPersonnelPassword(person.public_id, password); setMessage(`已重置 ${person.display_name} 的登录密码`); } catch (error) { setMessage(readClientError(error)); } finally { setBusy(false); }
  }

  return <ManagementFrame eyebrow="PEOPLE / PERSONNEL" title="人员档案" description="员工和保障人员由本系统维护。每个人绑定一个团队、一个岗位和一个能力；能力来自启用字典，工号必须为 4–12 位纯数字。" message={message || remoteTeams.error}>{canManage && <section className="detail-card"><h2>新增员工 / 保障人员</h2><form className="management-form form-grid" onSubmit={createPersonnel}><input required pattern="[0-9]{4,12}" title="工号必须为 4–12 位数字" placeholder="工号（4–12 位数字）" value={form.employee_no} onChange={(event) => setForm({ ...form, employee_no: event.target.value })} /><input required placeholder="姓名" value={form.display_name} onChange={(event) => setForm({ ...form, display_name: event.target.value })} /><RemoteOptionSelect label="班组" value={form.team_public_id} options={remoteTeams.items.filter((team) => team.enabled)} total={remoteTeams.total} query={remoteTeams.query} setQuery={remoteTeams.setQuery} loading={remoteTeams.loading} error={remoteTeams.error} getValue={(team) => team.public_id} getLabel={(team) => `${team.area_name} / ${team.name}`} onChange={(teamPublicID) => setForm({ ...form, team_public_id: teamPublicID })} required disabled={busy} /><select required value={form.position_code} onChange={(event) => setForm({ ...form, position_code: event.target.value })}><option value="">选择岗位</option>{positions.filter((item) => item.enabled).map((item) => <option key={item.code} value={item.code}>{item.code} / {item.name}</option>)}</select><select required value={form.capability_code} onChange={(event) => setForm({ ...form, capability_code: event.target.value })}><option value="">选择能力</option>{capabilities.filter((item) => item.enabled).map((item) => <option key={item.code} value={item.code}>{item.code} / {item.name}</option>)}</select><input required type="password" minLength={8} placeholder="初始密码" value={form.password} onChange={(event) => setForm({ ...form, password: event.target.value })} /><button className="primary-button" disabled={busy || !form.team_public_id || !form.position_code || !form.capability_code}>创建人员</button></form></section>}<section className="task-panel"><div className="toolbar"><strong>人员列表</strong><span className="scope-inline">{total} visible personnel</span></div><div className="table-scroll"><table><thead><tr><th>人员</th><th>班组 / 岗位</th><th>能力</th><th>工作状态</th><th>状态</th><th /></tr></thead><tbody>{personnel.map((person) => <tr key={person.public_id}><td><strong>{person.display_name}</strong><small>{person.employee_no}</small></td><td><span>{person.team_name || person.team_public_id}</span><small>{person.position_code}</small></td><td>{person.capability_code || person.capabilities.join(", ")}</td><td>{person.work_state}</td><td>{person.enabled ? "启用" : "停用"}</td><td className="row-actions">{canManage && <><button className="text-button light-text-button" disabled={busy} onClick={() => editPersonnel(person)}>编辑</button><button className="text-button light-text-button" disabled={busy} onClick={() => toggle(person)}>{person.enabled ? "停用" : "启用"}</button><button className="text-button light-text-button" disabled={busy} onClick={() => resetPassword(person)}>重置密码</button></>}</td></tr>)}</tbody></table></div><ListPager page={page} pageSize={LIST_PAGE_SIZE} total={total} onPageChange={setPage} disabled={busy} /></section></ManagementFrame>;
}

function TemplateManagementPageV2(): JSX.Element {
  const canManage = useAdminAccess().can("template:manage");
  const [templates, setTemplates] = useState<CoreTemplate[]>([]);
  const [positions, setPositions] = useState<CorePosition[]>([]);
  const [capabilities, setCapabilities] = useState<CoreCapability[]>([]);
  const [page, setPage] = useState(1);
  const [total, setTotal] = useState(0);
  const [form, setForm] = useState({ name: "", area_public_id: "", team_public_id: "", required_position_code: "", required_capability_code: "", planned_offset_seconds: "0", default_message: "" });
  const [busy, setBusy] = useState(false);
  const [message, setMessage] = useState<string>();
  const [reloadKey, setReloadKey] = useState(0);
  const remoteAreas = useRemoteAreas();
  const remoteTeams = useRemoteTeams(form.area_public_id || undefined);

  useEffect(() => {
    const controller = new AbortController();
    Promise.all([coreApi.listTemplates(true, controller.signal, page, LIST_PAGE_SIZE), coreApi.listPositions({}, controller.signal), coreApi.listCapabilities({}, controller.signal)]).then(([templateResult, positionResult, capabilityResult]) => {
      setTemplates(templateResult.data.items); setTotal(templateResult.data.total); setPositions(positionResult.data.items); setCapabilities(capabilityResult.data.items);
      setForm((current) => ({ ...current, required_position_code: current.required_position_code || positionResult.data.items.find((item) => item.enabled)?.code || "", required_capability_code: current.required_capability_code || capabilityResult.data.items.find((item) => item.enabled)?.code || "" }));
    }).catch((error: unknown) => { if (!controller.signal.aborted) setMessage(readClientError(error)); });
    return () => controller.abort();
  }, [page, reloadKey]);

  useEffect(() => {
    setForm((current) => {
      const areaPublicID = current.area_public_id || remoteAreas.items[0]?.public_id || "";
      const teamPublicID = current.team_public_id || remoteTeams.items[0]?.public_id || "";
      if (areaPublicID === current.area_public_id && teamPublicID === current.team_public_id) return current;
      return { ...current, area_public_id: areaPublicID, team_public_id: teamPublicID };
    });
  }, [remoteAreas.items, remoteTeams.items]);

  async function createTemplate(event: FormEvent<HTMLFormElement>): Promise<void> {
    event.preventDefault(); setBusy(true); setMessage(undefined);
    try { await coreApi.createTemplate({ name: form.name, template_version: 1, area_public_id: form.area_public_id, team_public_id: form.team_public_id, required_position_code: form.required_position_code, required_capability_code: form.required_capability_code, planned_offset_seconds: Number(form.planned_offset_seconds) || 0, default_message: form.default_message }); setForm((current) => ({ ...current, name: "", required_position_code: "", required_capability_code: "", default_message: "" })); setPage(1); setReloadKey((value) => value + 1); } catch (error) { setMessage(readClientError(error)); } finally { setBusy(false); }
  }

  async function toggle(template: CoreTemplate): Promise<void> {
    setBusy(true); setMessage(undefined);
    try { await coreApi.updateTemplate(template.public_id, { enabled: !template.enabled }); setReloadKey((value) => value + 1); } catch (error) { setMessage(readClientError(error)); } finally { setBusy(false); }
  }

  async function editTemplate(template: CoreTemplate): Promise<void> {
    const name = window.prompt("模板名称", template.name);
    if (name === null) return;
    const areaPublicID = window.prompt("目标区域 public_id", template.area_public_id);
    if (areaPublicID === null) return;
    const teamPublicID = window.prompt("目标班组 public_id", template.team_public_id);
    if (teamPublicID === null) return;
    const positionCode = window.prompt("岗位编码", template.required_position_code);
    if (positionCode === null) return;
    const capabilityCode = window.prompt("能力编码", template.required_capability_code || template.required_capabilities[0] || "");
    if (capabilityCode === null) return;
    const offset = window.prompt("计划偏移秒数", String(template.planned_offset_seconds));
    if (offset === null) return;
    const defaultMessage = window.prompt("默认任务说明", template.default_message);
    if (defaultMessage === null) return;
    const plannedOffsetSeconds = Number(offset);
    if (!name.trim() || !areaPublicID.trim() || !teamPublicID.trim() || !positionCode.trim() || !capabilityCode.trim() || !defaultMessage.trim() || !Number.isFinite(plannedOffsetSeconds) || plannedOffsetSeconds < 0) {
      setMessage("模板信息不完整：目标、岗位、能力、说明不能为空，偏移秒数必须为非负数字");
      return;
    }
    setBusy(true); setMessage(undefined);
    try {
      await coreApi.updateTemplate(template.public_id, { name: name.trim(), area_public_id: areaPublicID.trim(), team_public_id: teamPublicID.trim(), required_position_code: positionCode.trim(), required_capability_code: capabilityCode.trim(), required_capabilities: [capabilityCode.trim()], planned_offset_seconds: plannedOffsetSeconds, default_message: defaultMessage.trim() });
      setReloadKey((value) => value + 1);
      setMessage(`已更新模板“${name.trim()}”`);
    } catch (error) { setMessage(readClientError(error)); } finally { setBusy(false); }
  }

  return <ManagementFrame eyebrow="TASKS / TEMPLATES" title="任务模板" description="模板绑定一个目标班组、一个岗位和一个能力；航班到达后的任务由 Core 根据启用模板生成。" message={message || remoteAreas.error || remoteTeams.error}>{canManage && <section className="detail-card"><h2>新增模板</h2><form className="management-form form-grid" onSubmit={createTemplate}><input required placeholder="模板名称" value={form.name} onChange={(event) => setForm({ ...form, name: event.target.value })} /><RemoteOptionSelect label="目标区域" value={form.area_public_id} options={remoteAreas.items.filter((area) => area.enabled)} total={remoteAreas.total} query={remoteAreas.query} setQuery={remoteAreas.setQuery} loading={remoteAreas.loading} error={remoteAreas.error} getValue={(area) => area.public_id} getLabel={(area) => `${area.code} / ${area.name}`} onChange={(areaPublicID) => setForm({ ...form, area_public_id: areaPublicID, team_public_id: "" })} required disabled={busy} /><RemoteOptionSelect label="目标班组" value={form.team_public_id} options={remoteTeams.items.filter((team) => team.enabled)} total={remoteTeams.total} query={remoteTeams.query} setQuery={remoteTeams.setQuery} loading={remoteTeams.loading} error={remoteTeams.error} getValue={(team) => team.public_id} getLabel={(team) => team.name} onChange={(teamPublicID) => setForm({ ...form, team_public_id: teamPublicID })} required disabled={busy || !form.area_public_id} /><select required value={form.required_position_code} onChange={(event) => setForm({ ...form, required_position_code: event.target.value })}><option value="">选择岗位</option>{positions.filter((item) => item.enabled).map((item) => <option key={item.code} value={item.code}>{item.code} / {item.name}</option>)}</select><select required value={form.required_capability_code} onChange={(event) => setForm({ ...form, required_capability_code: event.target.value })}><option value="">选择能力</option>{capabilities.filter((item) => item.enabled).map((item) => <option key={item.code} value={item.code}>{item.code} / {item.name}</option>)}</select><input type="number" min={0} placeholder="计划偏移秒数" value={form.planned_offset_seconds} onChange={(event) => setForm({ ...form, planned_offset_seconds: event.target.value })} /><textarea required placeholder="默认任务说明" value={form.default_message} onChange={(event) => setForm({ ...form, default_message: event.target.value })} /><button className="primary-button" disabled={busy || !form.team_public_id || !form.required_position_code || !form.required_capability_code}>创建模板</button></form></section>}<section className="task-panel"><div className="toolbar"><strong>模板列表</strong><span className="scope-inline">{total} templates / page {page}</span></div><div className="table-scroll"><table><thead><tr><th>模板</th><th>目标</th><th>岗位 / 能力</th><th>状态</th><th /></tr></thead><tbody>{templates.map((template) => <tr key={template.public_id}><td><strong>{template.name}</strong><small>v{template.template_version}</small></td><td>{template.area_public_id}<small>{template.team_public_id}</small></td><td>{template.required_position_code}<small>{template.required_capability_code || template.required_capabilities.join(", ")}</small></td><td>{template.enabled ? "启用" : "停用"}</td><td>{canManage && <><button className="text-button light-text-button" disabled={busy} onClick={() => editTemplate(template)}>编辑</button><button className="text-button light-text-button" disabled={busy} onClick={() => toggle(template)}>{template.enabled ? "停用" : "启用"}</button></>}</td></tr>)}</tbody></table></div><ListPager page={page} pageSize={LIST_PAGE_SIZE} total={total} onPageChange={setPage} disabled={busy} /></section></ManagementFrame>;
}

function PositionCapabilityPage(): JSX.Element {
  const access = useAdminAccess();
  const canManage = access.can("position:manage") && access.can("capability:manage");
  const [positions, setPositions] = useState<CorePosition[]>([]);
  const [capabilities, setCapabilities] = useState<CoreCapability[]>([]);
  const [positionPage, setPositionPage] = useState(1);
  const [capabilityPage, setCapabilityPage] = useState(1);
  const [positionTotal, setPositionTotal] = useState(0);
  const [capabilityTotal, setCapabilityTotal] = useState(0);
  const [positionForm, setPositionForm] = useState<DictionaryForm>({ code: "", name: "", description: "" });
  const [capabilityForm, setCapabilityForm] = useState<DictionaryForm>({ code: "", name: "", description: "" });
  const [loading, setLoading] = useState(true);
  const [busy, setBusy] = useState(false);
  const [message, setMessage] = useState<string>();
  const [reloadKey, setReloadKey] = useState(0);

  useEffect(() => {
    const controller = new AbortController();
    setLoading(true);
    setMessage(undefined);
    Promise.all([
      coreApi.listPositions({ include_disabled: true, page: positionPage, page_size: LIST_PAGE_SIZE }, controller.signal),
      coreApi.listCapabilities({ include_disabled: true, page: capabilityPage, page_size: LIST_PAGE_SIZE }, controller.signal),
    ]).then(([positionResult, capabilityResult]) => {
      setPositions(positionResult.data.items);
      setPositionTotal(positionResult.data.total);
      setCapabilities(capabilityResult.data.items);
      setCapabilityTotal(capabilityResult.data.total);
    }).catch((error: unknown) => { if (!controller.signal.aborted) setMessage(readClientError(error)); }).finally(() => { if (!controller.signal.aborted) setLoading(false); });
    return () => controller.abort();
  }, [capabilityPage, positionPage, reloadKey]);

  async function createPosition(event: FormEvent<HTMLFormElement>): Promise<void> {
    event.preventDefault();
    setBusy(true); setMessage(undefined);
    try { await coreApi.createPosition(positionForm); setPositionForm({ code: "", name: "", description: "" }); setPositionPage(1); setReloadKey((value) => value + 1); } catch (error) { setMessage(readClientError(error)); } finally { setBusy(false); }
  }

  async function createCapability(event: FormEvent<HTMLFormElement>): Promise<void> {
    event.preventDefault();
    setBusy(true); setMessage(undefined);
    try { await coreApi.createCapability(capabilityForm); setCapabilityForm({ code: "", name: "", description: "" }); setCapabilityPage(1); setReloadKey((value) => value + 1); } catch (error) { setMessage(readClientError(error)); } finally { setBusy(false); }
  }

  async function togglePosition(item: CorePosition): Promise<void> {
    setBusy(true); setMessage(undefined);
    try { await coreApi.updatePosition(item.public_id, { enabled: !item.enabled }); setReloadKey((value) => value + 1); } catch (error) { setMessage(readClientError(error)); } finally { setBusy(false); }
  }

  async function toggleCapability(item: CoreCapability): Promise<void> {
    setBusy(true); setMessage(undefined);
    try { await coreApi.updateCapability(item.public_id, { enabled: !item.enabled }); setReloadKey((value) => value + 1); } catch (error) { setMessage(readClientError(error)); } finally { setBusy(false); }
  }

  async function editPosition(item: CorePosition): Promise<void> {
    const name = window.prompt("岗位名称", item.name);
    if (name === null) return;
    const description = window.prompt("岗位说明", item.description);
    if (description === null || !name.trim()) return;
    setBusy(true); setMessage(undefined);
    try { await coreApi.updatePosition(item.public_id, { name: name.trim(), description: description.trim() }); setReloadKey((value) => value + 1); } catch (error) { setMessage(readClientError(error)); } finally { setBusy(false); }
  }

  async function editCapability(item: CoreCapability): Promise<void> {
    const name = window.prompt("能力名称", item.name);
    if (name === null) return;
    const description = window.prompt("能力说明", item.description);
    if (description === null || !name.trim()) return;
    setBusy(true); setMessage(undefined);
    try { await coreApi.updateCapability(item.public_id, { name: name.trim(), description: description.trim() }); setReloadKey((value) => value + 1); } catch (error) { setMessage(readClientError(error)); } finally { setBusy(false); }
  }

  return <ManagementFrame eyebrow="PEOPLE / POSITION DICTIONARIES" title="岗位与能力" description="岗位和能力是 Core 的独立字典。编码创建后保持稳定，只允许调整名称、说明和启用状态；停用不会删除历史人员快照。" message={message}>
    {canManage && <div className="management-grid">
      <section className="detail-card"><h2>新增岗位</h2><form className="management-form" onSubmit={createPosition}><input required pattern="[a-z][a-z0-9_-]{1,63}" title="岗位编码必须以小写字母开头" placeholder="岗位编码，例如 gate_agent" value={positionForm.code} onChange={(event) => setPositionForm({ ...positionForm, code: event.target.value })} /><input required placeholder="岗位名称" value={positionForm.name} onChange={(event) => setPositionForm({ ...positionForm, name: event.target.value })} /><textarea placeholder="岗位说明" value={positionForm.description} onChange={(event) => setPositionForm({ ...positionForm, description: event.target.value })} /><button className="primary-button" disabled={busy}>创建岗位</button></form></section>
      <section className="detail-card"><h2>新增能力</h2><form className="management-form" onSubmit={createCapability}><input required pattern="[a-z][a-z0-9_.-]{1,63}" title="能力编码必须以小写字母开头" placeholder="能力编码，例如 baggage_scan" value={capabilityForm.code} onChange={(event) => setCapabilityForm({ ...capabilityForm, code: event.target.value })} /><input required placeholder="能力名称" value={capabilityForm.name} onChange={(event) => setCapabilityForm({ ...capabilityForm, name: event.target.value })} /><textarea placeholder="能力说明" value={capabilityForm.description} onChange={(event) => setCapabilityForm({ ...capabilityForm, description: event.target.value })} /><button className="primary-button" disabled={busy}>创建能力</button></form></section>
    </div>}
    <div className="management-grid">
      <DictionaryTable title="岗位字典" items={positions} total={positionTotal} page={positionPage} onPageChange={setPositionPage} onToggle={togglePosition} onEdit={editPosition} canManage={canManage} disabled={loading || busy} />
      <DictionaryTable title="能力字典" items={capabilities} total={capabilityTotal} page={capabilityPage} onPageChange={setCapabilityPage} onToggle={toggleCapability} onEdit={editCapability} canManage={canManage} disabled={loading || busy} />
    </div>
  </ManagementFrame>;
}

function DictionaryTable<T extends CorePosition | CoreCapability>({ title, items, total, page, onPageChange, onToggle, onEdit, canManage, disabled }: { title: string; items: T[]; total: number; page: number; onPageChange: (page: number) => void; onToggle: (item: T) => Promise<void>; onEdit: (item: T) => Promise<void>; canManage: boolean; disabled: boolean }): JSX.Element {
  return <section className="task-panel"><div className="toolbar"><strong>{title}</strong><span className="scope-inline">{total} records / page {page}</span></div><div className="table-scroll"><table><thead><tr><th>编码</th><th>名称</th><th>说明</th><th>状态</th><th /></tr></thead><tbody>{items.map((item) => <tr key={item.public_id}><td className="data-number">{item.code}</td><td>{item.name}</td><td>{item.description || "—"}</td><td>{item.enabled ? "启用" : "停用"}</td><td className="row-actions">{canManage && <><button className="text-button light-text-button" disabled={disabled} onClick={() => onEdit(item)}>编辑</button><button className="text-button light-text-button" disabled={disabled} onClick={() => onToggle(item)}>{item.enabled ? "停用" : "启用"}</button></>}</td></tr>)}</tbody></table></div>{!disabled && items.length === 0 && <div className="panel-state">暂无字典记录。</div>}<ListPager page={page} pageSize={LIST_PAGE_SIZE} total={total} onPageChange={onPageChange} disabled={disabled} /></section>;
}

function PersonnelStatusPage(): JSX.Element {
  const [items, setItems] = useState<CorePersonnelStatus[]>([]);
  const [history, setHistory] = useState<CorePersonnelStatusHistory[]>([]);
  const [historyPage, setHistoryPage] = useState(1);
  const [historyTotal, setHistoryTotal] = useState(0);
  const [page, setPage] = useState(1);
  const [total, setTotal] = useState(0);
  const [selectedID, setSelectedID] = useState("");
  const [workState, setWorkState] = useState("");
  const [areaPublicID, setAreaPublicID] = useState("");
  const [teamPublicID, setTeamPublicID] = useState("");
  const [loading, setLoading] = useState(true);
  const [historyLoading, setHistoryLoading] = useState(false);
  const [message, setMessage] = useState<string>();
  const [reloadKey, setReloadKey] = useState(0);
  const remoteAreas = useRemoteAreas();
  const remoteTeams = useRemoteTeams(areaPublicID || undefined);

  useEffect(() => {
    const controller = new AbortController();
    setLoading(true); setMessage(undefined);
    coreApi.listPersonnelStatus({ work_state: workState || undefined, area_public_id: areaPublicID || undefined, team_public_id: teamPublicID || undefined, page, page_size: LIST_PAGE_SIZE }, controller.signal).then((response) => {
      setItems(response.data.items); setTotal(response.data.total); setSelectedID((current) => response.data.items.some((item) => item.public_id === current) ? current : response.data.items[0]?.public_id ?? "");
    }).catch((error: unknown) => { if (!controller.signal.aborted) setMessage(readClientError(error)); }).finally(() => { if (!controller.signal.aborted) setLoading(false); });
    return () => controller.abort();
  }, [areaPublicID, page, reloadKey, teamPublicID, workState]);

  useEffect(() => {
    if (!selectedID) { setHistory([]); setHistoryTotal(0); return; }
    const controller = new AbortController();
    setHistoryLoading(true);
    coreApi.listPersonnelStatusHistory(selectedID, { page: historyPage, page_size: LIST_PAGE_SIZE }, controller.signal).then((response) => { setHistory(response.data.items); setHistoryTotal(response.data.total); }).catch((error: unknown) => { if (!controller.signal.aborted) setMessage(readClientError(error)); }).finally(() => { if (!controller.signal.aborted) setHistoryLoading(false); });
    return () => controller.abort();
  }, [historyPage, selectedID, reloadKey]);

  return <ManagementFrame eyebrow="PEOPLE / LIVE STATUS" title="人员状态" description="当前状态和状态历史均从 Core 事实表分页读取；页面不直接修改状态，状态由任务生命周期和员工命令推进。" message={message || remoteAreas.error || remoteTeams.error}>
    <section className="task-panel"><div className="toolbar"><RemoteOptionSelect label="区域" value={areaPublicID} options={remoteAreas.items.filter((area) => area.enabled)} total={remoteAreas.total} query={remoteAreas.query} setQuery={remoteAreas.setQuery} loading={remoteAreas.loading} error={remoteAreas.error} getValue={(area) => area.public_id} getLabel={(area) => `${area.code} / ${area.name}`} onChange={(value) => { setAreaPublicID(value); setTeamPublicID(""); setPage(1); setHistoryPage(1); }} /><RemoteOptionSelect label="班组" value={teamPublicID} options={remoteTeams.items.filter((team) => team.enabled)} total={remoteTeams.total} query={remoteTeams.query} setQuery={remoteTeams.setQuery} loading={remoteTeams.loading} error={remoteTeams.error} getValue={(team) => team.public_id} getLabel={(team) => team.name} onChange={(value) => { setTeamPublicID(value); setPage(1); setHistoryPage(1); }} disabled={!areaPublicID} /><label>工作状态<select value={workState} onChange={(event) => { setWorkState(event.target.value); setPage(1); setHistoryPage(1); }}><option value="">全部状态</option><option value="idle">idle</option><option value="reserved">reserved</option><option value="busy">busy</option><option value="unavailable">unavailable</option></select></label><span className="scope-inline">{total} personnel in Core</span><button className="secondary-button" onClick={() => setReloadKey((value) => value + 1)}>刷新</button></div>{loading && <div className="panel-state">正在读取人员状态…</div>}{!loading && items.length === 0 && <div className="panel-state">当前 Scope 没有人员状态。</div>}{!loading && items.length > 0 && <div className="table-scroll"><table><thead><tr><th>人员</th><th>组织</th><th>岗位 / 能力</th><th>状态</th><th>更新时间</th></tr></thead><tbody>{items.map((item) => <tr key={item.public_id} className={selectedID === item.public_id ? "is-selected" : ""} onClick={() => { setSelectedID(item.public_id); setHistoryPage(1); }}><td><strong>{item.display_name}</strong><small>{item.employee_no}</small></td><td>{item.area_name}<small>{item.team_name}</small></td><td>{item.position_code}<small>{item.capability_code}</small></td><td><span className={item.enabled ? "rule-enabled" : "rule-disabled"}>{item.work_state}</span><small>{item.unavailable_reason || ""}</small></td><td>{formatDate(item.last_state_changed_at)}</td></tr>)}</tbody></table></div>}<ListPager page={page} pageSize={LIST_PAGE_SIZE} total={total} onPageChange={setPage} disabled={loading} /></section>
    <section className="detail-card"><h2>状态历史 {selectedID ? `· ${selectedID}` : ""}</h2>{historyLoading && <div className="panel-state">正在读取状态历史…</div>}{!historyLoading && history.length === 0 && <div className="panel-state">选择人员查看状态变化。</div>}{!historyLoading && history.length > 0 && <div className="history-events">{history.map((item) => <article className="history-event" key={item.public_id}><span className="history-event-dot history-personnel" /><div><div className="history-event-top"><strong>{item.from_state || "—"} → {item.to_state}</strong><small>{formatDate(item.occurred_at)}</small></div><p>{item.reason || "无原因说明"}</p><small>actor: {item.actor_public_id || item.actor_type}</small></div></article>)}</div>}{!historyLoading && selectedID && <ListPager page={historyPage} pageSize={LIST_PAGE_SIZE} total={historyTotal} onPageChange={setHistoryPage} disabled={historyLoading} />}</section>
  </ManagementFrame>;
}

function EventsPage(): JSX.Element {
  const [items, setItems] = useState<CoreEvent[]>([]);
  const [page, setPage] = useState(1);
  const [total, setTotal] = useState(0);
  const [filters, setFilters] = useState({ status: "", event_type: "", flight_public_id: "", from: "", to: "" });
  const [applied, setApplied] = useState(filters);
  const [loading, setLoading] = useState(true);
  const [message, setMessage] = useState<string>();
  const [reloadKey, setReloadKey] = useState(0);

  useEffect(() => {
    const controller = new AbortController();
    setLoading(true); setMessage(undefined);
    coreApi.listEvents({ status: applied.status || undefined, event_type: applied.event_type || undefined, flight_public_id: applied.flight_public_id || undefined, from: toRFC3339(applied.from) || undefined, to: toRFC3339(applied.to) || undefined, page, page_size: LIST_PAGE_SIZE }, controller.signal).then((response) => { setItems(response.data.items); setTotal(response.data.total); }).catch((error: unknown) => { if (!controller.signal.aborted) setMessage(readClientError(error)); }).finally(() => { if (!controller.signal.aborted) setLoading(false); });
    return () => controller.abort();
  }, [applied, page, reloadKey]);

  function applyFilters(event: FormEvent<HTMLFormElement>): void { event.preventDefault(); setPage(1); setApplied(filters); }
  function clearFilters(): void { const cleared = { status: "", event_type: "", flight_public_id: "", from: "", to: "" }; setFilters(cleared); setApplied(cleared); setPage(1); }

  return <ManagementFrame eyebrow="GOVERNANCE / EVENTS" title="事件与通知" description="查看 Core Outbox 事件及投递状态。事件是只读观测，投递失败不会回滚已经提交的业务事实。" message={message}>
    <form className="report-filter" onSubmit={applyFilters}><label>投递状态<select value={filters.status} onChange={(event) => setFilters({ ...filters, status: event.target.value })}><option value="">全部状态</option><option value="pending">pending</option><option value="processing">processing</option><option value="delivered">delivered</option><option value="failed">failed</option></select></label><label>事件类型<input value={filters.event_type} placeholder="例如 task.confirmed" onChange={(event) => setFilters({ ...filters, event_type: event.target.value })} /></label><label>航班 public_id<input value={filters.flight_public_id} placeholder="可选" onChange={(event) => setFilters({ ...filters, flight_public_id: event.target.value })} /></label><label>开始时间<input type="datetime-local" value={filters.from} onChange={(event) => setFilters({ ...filters, from: event.target.value })} /></label><label>结束时间<input type="datetime-local" value={filters.to} onChange={(event) => setFilters({ ...filters, to: event.target.value })} /></label><button className="primary-button" type="submit">应用筛选</button><button className="secondary-button" type="button" onClick={clearFilters}>清除</button><button className="secondary-button" type="button" onClick={() => setReloadKey((value) => value + 1)}>刷新</button></form>
    <section className="task-panel"><div className="toolbar"><strong>事件列表</strong><span className="scope-inline">{total} events</span></div>{loading && <div className="panel-state">正在读取事件…</div>}{!loading && items.length === 0 && <div className="panel-state">当前筛选范围没有事件。</div>}{!loading && items.length > 0 && <div className="table-scroll"><table><thead><tr><th>事件</th><th>聚合</th><th>航班</th><th>状态</th><th>时间</th><th>错误</th></tr></thead><tbody>{items.map((item) => <tr key={item.public_id}><td><strong>{item.event_type}</strong><small>{item.source}</small></td><td>{item.aggregate_type}<small>{item.aggregate_public_id}</small></td><td>{item.flight_display_no || item.flight_public_id || "—"}</td><td>{item.status}</td><td>{formatDate(item.occurred_at)}</td><td>{item.last_error || "—"}</td></tr>)}</tbody></table></div>}<ListPager page={page} pageSize={LIST_PAGE_SIZE} total={total} onPageChange={setPage} disabled={loading} /></section>
  </ManagementFrame>;
}

function AuditPage(): JSX.Element {
  const [items, setItems] = useState<CoreAuditEntry[]>([]);
  const [page, setPage] = useState(1);
  const [total, setTotal] = useState(0);
  const [filters, setFilters] = useState({ actor_id: "", action: "", resource_type: "", from: "", to: "" });
  const [applied, setApplied] = useState(filters);
  const [loading, setLoading] = useState(true);
  const [message, setMessage] = useState<string>();
  const [reloadKey, setReloadKey] = useState(0);
  useEffect(() => { const controller = new AbortController(); setLoading(true); setMessage(undefined); coreApi.listAudit({ actor_id: applied.actor_id || undefined, action: applied.action || undefined, resource_type: applied.resource_type || undefined, from: toRFC3339(applied.from) || undefined, to: toRFC3339(applied.to) || undefined, page, page_size: LIST_PAGE_SIZE }, controller.signal).then((response) => { setItems(response.data.items); setTotal(response.data.total); }).catch((error: unknown) => { if (!controller.signal.aborted) setMessage(readClientError(error)); }).finally(() => { if (!controller.signal.aborted) setLoading(false); }); return () => controller.abort(); }, [applied, page, reloadKey]);
  function applyFilters(event: FormEvent<HTMLFormElement>): void { event.preventDefault(); setPage(1); setApplied(filters); }
  function clearFilters(): void { const cleared = { actor_id: "", action: "", resource_type: "", from: "", to: "" }; setFilters(cleared); setApplied(cleared); setPage(1); }
  return <ManagementFrame eyebrow="GOVERNANCE / AUDIT" title="审计日志" description="审计列表只展示脱敏后的操作元数据；原始请求体、密码、Token 和内部错误详情不会下发到管理端。" message={message}>
    <form className="report-filter" onSubmit={applyFilters}><label>操作者 ID<input value={filters.actor_id} placeholder="可选" onChange={(event) => setFilters({ ...filters, actor_id: event.target.value })} /></label><label>操作筛选<input value={filters.action} placeholder="例如 task.confirmed" onChange={(event) => setFilters({ ...filters, action: event.target.value })} /></label><label>资源类型<input value={filters.resource_type} placeholder="例如 task" onChange={(event) => setFilters({ ...filters, resource_type: event.target.value })} /></label><label>开始时间<input type="datetime-local" value={filters.from} onChange={(event) => setFilters({ ...filters, from: event.target.value })} /></label><label>结束时间<input type="datetime-local" value={filters.to} onChange={(event) => setFilters({ ...filters, to: event.target.value })} /></label><button className="primary-button" type="submit">应用筛选</button><button className="secondary-button" type="button" onClick={clearFilters}>清除</button><button className="secondary-button" type="button" onClick={() => setReloadKey((value) => value + 1)}>刷新</button></form>
    <section className="task-panel"><div className="toolbar"><strong>审计列表</strong><span className="scope-inline">{total} audit records</span></div>{loading && <div className="panel-state">正在读取审计日志…</div>}{!loading && items.length === 0 && <div className="panel-state">当前筛选范围没有审计记录。</div>}{!loading && items.length > 0 && <div className="table-scroll"><table><thead><tr><th>时间</th><th>操作者</th><th>动作</th><th>资源</th><th>结果</th><th>请求</th></tr></thead><tbody>{items.map((item) => <tr key={`${item.id}`}><td>{formatDate(item.occurred_at)}</td><td>{item.actor_type}<small>{item.actor_id}</small></td><td>{item.action}</td><td>{item.resource_type}<small>{item.resource_id}</small></td><td>{item.result}</td><td><small>{item.request_id}</small><small>{item.source_ip}</small></td></tr>)}</tbody></table></div>}<ListPager page={page} pageSize={LIST_PAGE_SIZE} total={total} onPageChange={setPage} disabled={loading} /></section>
  </ManagementFrame>;
}

function ScopesPage(): JSX.Element {
  const access = useAdminAccess();
  return <ManagementFrame eyebrow="SYSTEM / SCOPES" title="权限范围" description="Scope 由 Core 会话和 RBAC 计算，管理端只读展示，不提供浏览器侧的本地授权开关。" message={access.error}><section className="detail-card"><div className="toolbar"><strong>当前会话 Scope</strong><button className="secondary-button" onClick={access.refresh}>刷新</button></div>{access.loading && <div className="panel-state">正在读取 Scope…</div>}{!access.loading && access.scope && <div className="fact-grid"><Fact label="Principal" value={access.scope.principal_public_id} /><Fact label="角色" value={access.scope.roles.join(", ") || "—"} /><Fact label="全局" value={access.scope.global ? "是" : "否"} /><Fact label="Area IDs" value={access.scope.area_ids.join(", ") || "—"} /><Fact label="Team IDs" value={access.scope.team_ids.join(", ") || "—"} /></div>}</section></ManagementFrame>;
}

function DiagnosticsPage(): JSX.Element {
  const [diagnostics, setDiagnostics] = useState<CoreDiagnostics>();
  const [loading, setLoading] = useState(true);
  const [message, setMessage] = useState<string>();
  const [reloadKey, setReloadKey] = useState(0);
  useEffect(() => { const controller = new AbortController(); setLoading(true); setMessage(undefined); coreApi.getDiagnostics(controller.signal).then((response) => setDiagnostics(response.data)).catch((error: unknown) => { if (!controller.signal.aborted) setMessage(readClientError(error)); }).finally(() => { if (!controller.signal.aborted) setLoading(false); }); return () => controller.abort(); }, [reloadKey]);
  return <ManagementFrame eyebrow="SYSTEM / DIAGNOSTICS" title="开发诊断" description="诊断页面只展示运行环境、数据库可达性和队列计数，不暴露凭据、原始 payload 或敏感配置。" message={message}><div className="page-heading"><div /><button className="secondary-button" onClick={() => setReloadKey((value) => value + 1)}>刷新诊断</button></div>{loading && <div className="panel-state">正在读取诊断…</div>}{!loading && diagnostics && <><div className="metric-row"><Metric label="Source pending" value={String(diagnostics.sync.flight_source_pending)} /><Metric label="Source retry" value={String(diagnostics.sync.flight_source_retry)} /><Metric label="Outbox pending" value={String(diagnostics.sync.outbox_pending)} /><Metric label="Inbox failed" value={String(diagnostics.sync.core_inbox_failed)} /></div><section className="detail-card"><div className="fact-grid"><Fact label="Component" value={diagnostics.component} /><Fact label="Environment" value={diagnostics.runtime.environment} /><Fact label="Database" value={diagnostics.database_reachable ? "reachable" : "unreachable"} /><Fact label="Flight source" value={diagnostics.runtime.flight_source_configured ? "configured" : "not configured"} /><Fact label="Redis" value={diagnostics.runtime.redis_enabled ? "enabled" : "disabled"} /><Fact label="Generated" value={formatDate(diagnostics.generated_at)} /></div></section></>}</ManagementFrame>;
}

function Metric({ label, value }: { label: string; value: string }): JSX.Element { return <div className="metric"><small>{label}</small><strong>{value}</strong></div>; }
function Fact({ label, value }: { label: string; value: string }): JSX.Element { return <span><small>{label}</small><strong>{value}</strong></span>; }
function formatDate(value: string): string { if (!value) return "—"; const date = new Date(value); return Number.isNaN(date.valueOf()) ? value : new Intl.DateTimeFormat("zh-CN", { month: "2-digit", day: "2-digit", hour: "2-digit", minute: "2-digit" }).format(date); }
function toRFC3339(value: string): string { if (!value) return ""; const date = new Date(value); return Number.isNaN(date.valueOf()) ? "" : date.toISOString(); }

function OverviewPage(): JSX.Element {
  const [metrics, setMetrics] = useState({ tasks: 0, flights: 0, personnel: 0, assignments: 0 });
  const [loading, setLoading] = useState(true);
  const [message, setMessage] = useState<string>();
  const [reloadKey, setReloadKey] = useState(0);

  useEffect(() => {
    const controller = new AbortController();
    setLoading(true); setMessage(undefined);
    coreApi.getReportOverview({}, controller.signal).then((response) => {
      const counts = response.data;
      setMetrics({
        tasks: sumCounts(counts.task_counts),
        flights: sumCounts(counts.flight_counts),
        personnel: sumCounts(counts.personnel_counts),
        assignments: sumCounts(counts.assignment_counts),
      });
    }).catch((error: unknown) => { if (!controller.signal.aborted) setMessage(readClientError(error)); }).finally(() => { if (!controller.signal.aborted) setLoading(false); });
    return () => controller.abort();
  }, [reloadKey]);

  return <div className="module-page"><div className="page-heading"><div><p className="eyebrow">CONTROL / CORE FACTS</p><h1>运行总览</h1><p className="page-description">所有数字均由 Core 当前事实源计算，管理端不在浏览器内缓存业务状态。</p></div><button className="secondary-button" onClick={() => setReloadKey((value) => value + 1)}>刷新</button></div>
    {message && <Notice tone="error">{message}</Notice>}
    <div className="metric-row"><Metric label="可见任务" value={loading ? "…" : String(metrics.tasks)} /><Metric label="运行航班" value={loading ? "…" : String(metrics.flights)} /><Metric label="员工档案" value={loading ? "…" : String(metrics.personnel)} /><Metric label="任务分配" value={loading ? "…" : String(metrics.assignments)} /></div>
    <section className="detail-grid"><section className="detail-card"><p className="eyebrow">OPERATING MODEL</p><h2>单机场 Core / Edge</h2><p className="page-description">Core 保存航班、人员、任务和权限事实；Edge 保存员工端 Projection、Command 和 Session。管理端所有写操作都经过 Core 权限和事务审计。</p></section><section className="detail-card"><p className="eyebrow">DELIVERY MODEL</p><h2>实时通知 + HTTP 恢复</h2><p className="page-description">员工端收到 WebSocket 变化提示后重新获取完整快照。断线、重复通知或 Projection 延迟不会改变事实源。</p></section></section>
  </div>;
}

function ManagementFrame({ eyebrow, title, description, message, children }: { eyebrow: string; title: string; description: string; message?: string; children: ReactNode }): JSX.Element {
  const isSuccessMessage = message?.startsWith("已") ?? false;
  return <div className="module-page management-page"><div className="page-heading"><div><p className="eyebrow">{eyebrow}</p><h1>{title}</h1><p className="page-description">{description}</p></div></div>{message && <Notice tone={isSuccessMessage ? "success" : "error"}>{message}</Notice>}{children}</div>;
}

function readClientError(error: unknown): string {
  if (error instanceof ApiClientError) return `${error.body.code}: ${error.message}`;
  return error instanceof Error ? error.message : "Core API 请求失败";
}

function OrganizationPage(): JSX.Element {
  const access = useAdminAccess();
  const canManage = access.can("area:manage") && access.can("team:manage");
  const [areas, setAreas] = useState<CoreArea[]>([]);
  const [teams, setTeams] = useState<CoreTeam[]>([]);
  const [areaPage, setAreaPage] = useState(1);
  const [teamPage, setTeamPage] = useState(1);
  const [areaTotal, setAreaTotal] = useState(0);
  const [teamTotal, setTeamTotal] = useState(0);
  const [areaForm, setAreaForm] = useState({ code: "", name: "" });
  const [teamForm, setTeamForm] = useState({ area_public_id: "", code: "", name: "" });
  const [busy, setBusy] = useState(false);
  const [message, setMessage] = useState<string>();
  const [reloadKey, setReloadKey] = useState(0);
  const remoteAreas = useRemoteAreas(false);
  useEffect(() => {
    const controller = new AbortController();
    Promise.all([coreApi.listAreas({ include_disabled: true, page: areaPage, page_size: LIST_PAGE_SIZE }, controller.signal), coreApi.listTeams(undefined, true, controller.signal, teamPage, LIST_PAGE_SIZE)]).then(([areaResult, teamResult]) => { setAreas(areaResult.data.items); setTeams(teamResult.data.items); setAreaTotal(areaResult.data.total); setTeamTotal(teamResult.data.total); setTeamForm((current) => ({ ...current, area_public_id: current.area_public_id || areaResult.data.items[0]?.public_id || "" })); }).catch((error: unknown) => { if (!controller.signal.aborted) setMessage(readClientError(error)); });
    return () => controller.abort();
  }, [areaPage, reloadKey, teamPage]);
  async function createArea(event: FormEvent<HTMLFormElement>): Promise<void> { event.preventDefault(); setBusy(true); setMessage(undefined); try { await coreApi.createArea(areaForm); setAreaForm({ code: "", name: "" }); setAreaPage(1); setReloadKey((value) => value + 1); } catch (error) { setMessage(readClientError(error)); } finally { setBusy(false); } }
  async function createTeam(event: FormEvent<HTMLFormElement>): Promise<void> { event.preventDefault(); setBusy(true); setMessage(undefined); try { await coreApi.createTeam(teamForm); setTeamForm((current) => ({ area_public_id: current.area_public_id, code: "", name: "" })); setTeamPage(1); setReloadKey((value) => value + 1); } catch (error) { setMessage(readClientError(error)); } finally { setBusy(false); } }
  async function toggleArea(area: CoreArea): Promise<void> { setBusy(true); setMessage(undefined); try { await coreApi.updateArea(area.public_id, { enabled: !area.enabled }); setReloadKey((value) => value + 1); } catch (error) { setMessage(readClientError(error)); } finally { setBusy(false); } }
  async function toggleTeam(team: CoreTeam): Promise<void> { setBusy(true); setMessage(undefined); try { await coreApi.updateTeam(team.public_id, { enabled: !team.enabled }); setReloadKey((value) => value + 1); } catch (error) { setMessage(readClientError(error)); } finally { setBusy(false); } }
  async function editArea(area: CoreArea): Promise<void> {
    const code = window.prompt("区域编码", area.code);
    if (code === null) return;
    const name = window.prompt("区域名称", area.name);
    if (name === null) return;
    if (!code.trim() || !name.trim()) { setMessage("区域编码和名称不能为空"); return; }
    setBusy(true); setMessage(undefined);
    try { await coreApi.updateArea(area.public_id, { code: code.trim(), name: name.trim() }); setReloadKey((value) => value + 1); setMessage(`已更新区域“${name.trim()}”`); } catch (error) { setMessage(readClientError(error)); } finally { setBusy(false); }
  }
  async function editTeam(team: CoreTeam): Promise<void> {
    const code = window.prompt("班组编码", team.code);
    if (code === null) return;
    const name = window.prompt("班组名称", team.name);
    if (name === null) return;
    const areaPublicID = window.prompt("所属区域 public_id", team.area_public_id);
    if (areaPublicID === null) return;
    if (!code.trim() || !name.trim() || !areaPublicID.trim()) { setMessage("班组编码、名称和所属区域不能为空"); return; }
    setBusy(true); setMessage(undefined);
    try { await coreApi.updateTeam(team.public_id, { code: code.trim(), name: name.trim(), area_public_id: areaPublicID.trim() }); setReloadKey((value) => value + 1); setMessage(`已更新班组“${name.trim()}”`); } catch (error) { setMessage(readClientError(error)); } finally { setBusy(false); }
  }
  return <ManagementFrame eyebrow="MASTER DATA / ORGANIZATION" title="组织与班组" description="先维护运行区域和班组，再创建人员、任务模板和权限范围。停用操作由 Core 根据下级事实保护。" message={message || remoteAreas.error}>
    {canManage && <div className="management-grid"><section className="detail-card"><h2>新增运行区域</h2><form className="management-form" onSubmit={createArea}><input required placeholder="区域编码，例如 GATE" value={areaForm.code} onChange={(event) => setAreaForm({ ...areaForm, code: event.target.value })} /><input required placeholder="区域名称" value={areaForm.name} onChange={(event) => setAreaForm({ ...areaForm, name: event.target.value })} /><button className="primary-button" disabled={busy}>创建区域</button></form></section><section className="detail-card"><h2>新增班组</h2><form className="management-form" onSubmit={createTeam}><RemoteOptionSelect label="所属区域" value={teamForm.area_public_id} options={remoteAreas.items.filter((area) => area.enabled)} total={remoteAreas.total} query={remoteAreas.query} setQuery={remoteAreas.setQuery} loading={remoteAreas.loading} error={remoteAreas.error} getValue={(area) => area.public_id} getLabel={(area) => `${area.code} / ${area.name}`} onChange={(areaPublicID) => setTeamForm({ ...teamForm, area_public_id: areaPublicID })} required disabled={busy} /><input required placeholder="班组编码" value={teamForm.code} onChange={(event) => setTeamForm({ ...teamForm, code: event.target.value })} /><input required placeholder="班组名称" value={teamForm.name} onChange={(event) => setTeamForm({ ...teamForm, name: event.target.value })} /><button className="primary-button" disabled={busy || !teamForm.area_public_id}>创建班组</button></form></section></div>}
    <section className="task-panel"><div className="toolbar"><strong>区域与班组</strong><span className="scope-inline">{areaTotal} areas / {teamTotal} teams</span></div><div className="table-scroll"><table><thead><tr><th>类型</th><th>编码</th><th>名称</th><th>归属</th><th>状态</th><th /></tr></thead><tbody>{areas.map((area) => <tr key={area.public_id}><td>Area</td><td className="data-number">{area.code}</td><td>{area.name}</td><td>—</td><td>{area.enabled ? "启用" : "停用"}</td><td>{canManage && <><button className="text-button light-text-button" disabled={busy} onClick={() => editArea(area)}>编辑</button><button className="text-button light-text-button" disabled={busy} onClick={() => toggleArea(area)}>{area.enabled ? "停用" : "启用"}</button></>}</td></tr>)}{teams.map((team) => <tr key={team.public_id}><td>Team</td><td className="data-number">{team.code}</td><td>{team.name}</td><td>{team.area_name || team.area_public_id}</td><td>{team.enabled ? "启用" : "停用"}</td><td>{canManage && <><button className="text-button light-text-button" disabled={busy} onClick={() => editTeam(team)}>编辑</button><button className="text-button light-text-button" disabled={busy} onClick={() => toggleTeam(team)}>{team.enabled ? "停用" : "启用"}</button></>}</td></tr>)}</tbody></table></div><div className="organization-pagers"><div><span className="pager-label">运行区域</span><ListPager page={areaPage} pageSize={LIST_PAGE_SIZE} total={areaTotal} onPageChange={setAreaPage} disabled={busy} /></div><div><span className="pager-label">班组</span><ListPager page={teamPage} pageSize={LIST_PAGE_SIZE} total={teamTotal} onPageChange={setTeamPage} disabled={busy} /></div></div></section>
  </ManagementFrame>;
}

function FlightManagementPage(): JSX.Element {
  const [flights, setFlights] = useState<CoreFlight[]>([]);
  const [page, setPage] = useState(1);
  const [total, setTotal] = useState(0);
  const [message, setMessage] = useState<string>();
  const [reloadKey, setReloadKey] = useState(0);
  useEffect(() => { const controller = new AbortController(); coreApi.listFlights({ include_terminal: true, page, page_size: LIST_PAGE_SIZE }, controller.signal).then((response) => { setFlights(response.data.items); setPage(response.data.page); setTotal(response.data.total); }).catch((error: unknown) => { if (!controller.signal.aborted) setMessage(readClientError(error)); }); return () => controller.abort(); }, [page, reloadKey]);
  return <ManagementFrame eyebrow="OPERATIONS / FLIGHTS" title="航班运行" description="航班计划、状态和到达事件均来自外部航班 Provider；管理端只读 Core 已同步的航班事实。" message={message}><section className="detail-card"><p className="eyebrow">EXTERNAL SOURCE</p><h2>航班事实只读</h2><p className="page-description">当前环境允许在数据库中保留 development seed 航班，正式接入时由 Provider 同步计划和生命周期事件。浏览器不提供新增、到达、离港或取消入口。</p></section><section className="task-panel"><div className="toolbar"><strong>航班事实</strong><span className="scope-inline">{total} flights · 第 {page} 页</span><button className="secondary-button" onClick={() => setReloadKey((value) => value + 1)}>刷新</button></div><div className="table-scroll"><table><thead><tr><th>航班</th><th>来源</th><th>计划时间</th><th>状态</th><th>版本</th></tr></thead><tbody>{flights.map((flight) => <tr key={flight.public_id}><td><strong>{flight.flight_display_no}</strong><small>{flight.operating_date}</small></td><td><span>{flight.source_provider || "development"}</span><small>{flight.external_flight_id || "seed / pending provider"}</small></td><td>{formatDate(flight.scheduled_at)}</td><td><StatusBadge status={flight.status as TaskStatus} /></td><td className="data-number">v{flight.status_version}</td></tr>)}</tbody></table></div><ListPager page={page} pageSize={LIST_PAGE_SIZE} total={total} onPageChange={setPage} /></section></ManagementFrame>;
}

export function LegacyPersonnelManagementPage(): JSX.Element {
  const [personnel, setPersonnel] = useState<CorePersonnel[]>([]); const [teams, setTeams] = useState<CoreTeam[]>([]); const [page, setPage] = useState(1); const [total, setTotal] = useState(0); const [form, setForm] = useState({ employee_no: "", display_name: "", team_public_id: "", position_code: "", capabilities: "", password: "" }); const [busy, setBusy] = useState(false); const [message, setMessage] = useState<string>(); const [reloadKey, setReloadKey] = useState(0);
  useEffect(() => { const controller = new AbortController(); Promise.all([coreApi.listPersonnel({ page, page_size: LIST_PAGE_SIZE }, controller.signal), coreApi.listTeams(undefined, false, controller.signal)]).then(([personnelResult, teamResult]) => { setPersonnel(personnelResult.data.items); setTotal(personnelResult.data.total); setTeams(teamResult.data.items); setForm((current) => ({ ...current, team_public_id: current.team_public_id || teamResult.data.items[0]?.public_id || "" })); }).catch((error: unknown) => { if (!controller.signal.aborted) setMessage(readClientError(error)); }); return () => controller.abort(); }, [page, reloadKey]);
  async function createPersonnel(event: FormEvent<HTMLFormElement>): Promise<void> { event.preventDefault(); setBusy(true); setMessage(undefined); try { await coreApi.createPersonnel({ employee_no: form.employee_no, display_name: form.display_name, team_public_id: form.team_public_id, position_code: form.position_code, capabilities: form.capabilities.split(",").map((value) => value.trim()).filter(Boolean), password: form.password }); setForm({ employee_no: "", display_name: "", team_public_id: form.team_public_id, position_code: "", capabilities: "", password: "" }); setPage(1); setReloadKey((value) => value + 1); } catch (error) { setMessage(readClientError(error)); } finally { setBusy(false); } }
  async function toggle(person: CorePersonnel): Promise<void> { setBusy(true); setMessage(undefined); try { await coreApi.updatePersonnel(person.public_id, { enabled: !person.enabled }); setReloadKey((value) => value + 1); } catch (error) { setMessage(readClientError(error)); } finally { setBusy(false); } }
  async function resetPassword(person: CorePersonnel): Promise<void> { const password = window.prompt("输入新密码（至少 8 位）"); if (!password) return; setBusy(true); setMessage(undefined); try { await coreApi.resetPersonnelPassword(person.public_id, password); setMessage(`已重置 ${person.display_name} 的登录密码`); } catch (error) { setMessage(readClientError(error)); } finally { setBusy(false); } }
  return <ManagementFrame eyebrow="PEOPLE / PERSONNEL" title="人员档案" description="员工档案是员工 Web、个人微信小程序和企业微信小程序共用的 Core 身份事实。" message={message}><section className="detail-card"><h2>新增员工</h2><form className="management-form form-grid" onSubmit={createPersonnel}><input required pattern="[0-9]{4,12}" title="工号必须为 4-12 位数字" placeholder="工号（4-12 位数字）" value={form.employee_no} onChange={(event) => setForm({ ...form, employee_no: event.target.value })} /><input required placeholder="姓名" value={form.display_name} onChange={(event) => setForm({ ...form, display_name: event.target.value })} /><select required value={form.team_public_id} onChange={(event) => setForm({ ...form, team_public_id: event.target.value })}><option value="">选择班组</option>{teams.map((team) => <option key={team.public_id} value={team.public_id}>{team.area_name} / {team.name}</option>)}</select><input required pattern="[a-z][a-z0-9_-]{1,63}" title="岗位编码必须以小写字母开头，仅允许小写字母、数字、下划线和短横线" placeholder="岗位编码" value={form.position_code} onChange={(event) => setForm({ ...form, position_code: event.target.value })} /><input placeholder="能力（小写编码，逗号分隔）" pattern="[a-z][a-z0-9_.-]*(,[a-z][a-z0-9_.-]*)*" title="能力使用小写编码，多个能力用逗号分隔" value={form.capabilities} onChange={(event) => setForm({ ...form, capabilities: event.target.value })} /><input required type="password" minLength={8} placeholder="初始密码" value={form.password} onChange={(event) => setForm({ ...form, password: event.target.value })} /><button className="primary-button" disabled={busy || !form.team_public_id}>创建员工</button></form></section><section className="task-panel"><div className="toolbar"><strong>员工列表</strong><span className="scope-inline">{total} visible personnel</span></div><div className="table-scroll"><table><thead><tr><th>员工</th><th>班组 / 岗位</th><th>工作状态</th><th>状态</th><th /></tr></thead><tbody>{personnel.map((person) => <tr key={person.public_id}><td><strong>{person.display_name}</strong><small>{person.employee_no}</small></td><td><span>{person.team_public_id}</span><small>{person.position_code}</small></td><td>{person.work_state}</td><td>{person.enabled ? "可用" : "停用"}</td><td className="row-actions"><button className="text-button light-text-button" disabled={busy} onClick={() => toggle(person)}>{person.enabled ? "停用" : "启用"}</button><button className="text-button light-text-button" disabled={busy} onClick={() => resetPassword(person)}>重置密码</button></td></tr>)}</tbody></table></div><ListPager page={page} pageSize={LIST_PAGE_SIZE} total={total} onPageChange={setPage} disabled={busy} /></section></ManagementFrame>;
}

export function LegacyTemplateManagementPage(): JSX.Element {
  const [templates, setTemplates] = useState<CoreTemplate[]>([]); const [areas, setAreas] = useState<CoreArea[]>([]); const [teams, setTeams] = useState<CoreTeam[]>([]); const [page, setPage] = useState(1); const [total, setTotal] = useState(0); const [form, setForm] = useState({ name: "", area_public_id: "", team_public_id: "", required_position_code: "", required_capabilities: "", planned_offset_seconds: "0", default_message: "" }); const [busy, setBusy] = useState(false); const [message, setMessage] = useState<string>(); const [reloadKey, setReloadKey] = useState(0);
  useEffect(() => { const controller = new AbortController(); Promise.all([coreApi.listTemplates(true, controller.signal, page, LIST_PAGE_SIZE), coreApi.listAreas({ page: 1, page_size: LIST_PAGE_SIZE }, controller.signal), coreApi.listTeams(undefined, false, controller.signal, 1, LIST_PAGE_SIZE)]).then(([templateResult, areaResult, teamResult]) => { setTemplates(templateResult.data.items); setTotal(templateResult.data.total); setAreas(areaResult.data.items); setTeams(teamResult.data.items); setForm((current) => ({ ...current, area_public_id: current.area_public_id || areaResult.data.items[0]?.public_id || "", team_public_id: current.team_public_id || teamResult.data.items[0]?.public_id || "" })); }).catch((error: unknown) => { if (!controller.signal.aborted) setMessage(readClientError(error)); }); return () => controller.abort(); }, [page, reloadKey]);
  async function createTemplate(event: FormEvent<HTMLFormElement>): Promise<void> { event.preventDefault(); setBusy(true); setMessage(undefined); try { await coreApi.createTemplate({ name: form.name, template_version: 1, area_public_id: form.area_public_id, team_public_id: form.team_public_id, required_position_code: form.required_position_code, required_capabilities: form.required_capabilities.split(",").map((value) => value.trim()).filter(Boolean), planned_offset_seconds: Number(form.planned_offset_seconds) || 0, default_message: form.default_message }); setForm((current) => ({ ...current, name: "", required_position_code: "", required_capabilities: "", default_message: "" })); setPage(1); setReloadKey((value) => value + 1); } catch (error) { setMessage(readClientError(error)); } finally { setBusy(false); } }
  async function toggle(template: CoreTemplate): Promise<void> { setBusy(true); setMessage(undefined); try { await coreApi.updateTemplate(template.public_id, { enabled: !template.enabled }); setReloadKey((value) => value + 1); } catch (error) { setMessage(readClientError(error)); } finally { setBusy(false); } }
  const availableTeams = teams.filter((team) => !form.area_public_id || team.area_public_id === form.area_public_id);
  return <ManagementFrame eyebrow="TASKS / TEMPLATES" title="任务模板" description="模板定义航班到达后生成的任务内容、目标班组、岗位和能力要求。模板版本保持显式，历史任务使用快照。" message={message}><section className="detail-card"><h2>新增模板</h2><form className="management-form form-grid" onSubmit={createTemplate}><input required placeholder="模板名称" value={form.name} onChange={(event) => setForm({ ...form, name: event.target.value })} /><select required value={form.area_public_id} onChange={(event) => setForm({ ...form, area_public_id: event.target.value, team_public_id: "" })}><option value="">选择目标区域</option>{areas.map((area) => <option key={area.public_id} value={area.public_id}>{area.code} / {area.name}</option>)}</select><select required value={form.team_public_id} onChange={(event) => setForm({ ...form, team_public_id: event.target.value })}><option value="">选择目标班组</option>{availableTeams.map((team) => <option key={team.public_id} value={team.public_id}>{team.name}</option>)}</select><input required pattern="[a-z][a-z0-9_-]{1,63}" title="岗位编码必须以小写字母开头，仅允许小写字母、数字、下划线和短横线" placeholder="岗位编码" value={form.required_position_code} onChange={(event) => setForm({ ...form, required_position_code: event.target.value })} /><input placeholder="能力（小写编码，逗号分隔）" pattern="[a-z][a-z0-9_.-]*(,[a-z][a-z0-9_.-]*)*" title="能力使用小写编码，多个能力用逗号分隔" value={form.required_capabilities} onChange={(event) => setForm({ ...form, required_capabilities: event.target.value })} /><input type="number" min={0} placeholder="计划偏移秒数" value={form.planned_offset_seconds} onChange={(event) => setForm({ ...form, planned_offset_seconds: event.target.value })} /><textarea required placeholder="默认任务说明" value={form.default_message} onChange={(event) => setForm({ ...form, default_message: event.target.value })} /><button className="primary-button" disabled={busy || !form.team_public_id}>创建模板</button></form></section><section className="task-panel"><div className="toolbar"><strong>模板列表</strong><span className="scope-inline">{total} templates · 第 {page} 页</span></div><div className="table-scroll"><table><thead><tr><th>模板</th><th>目标</th><th>约束</th><th>状态</th><th /></tr></thead><tbody>{templates.map((template) => <tr key={template.public_id}><td><strong>{template.name}</strong><small>v{template.template_version} / {template.trigger_type}</small></td><td>{template.area_public_id}<small>{template.team_public_id}</small></td><td>{template.required_position_code}<small>{template.required_capabilities.join(", ") || "无额外能力"}</small></td><td>{template.enabled ? "启用" : "停用"}</td><td><button className="text-button light-text-button" disabled={busy} onClick={() => toggle(template)}>{template.enabled ? "停用" : "启用"}</button></td></tr>)}</tbody></table></div><ListPager page={page} pageSize={LIST_PAGE_SIZE} total={total} onPageChange={setPage} disabled={busy} /></section></ManagementFrame>;
}

function RulesPage(): JSX.Element {
  const [templates, setTemplates] = useState<CoreTemplate[]>([]);
  const [page, setPage] = useState(1);
  const [total, setTotal] = useState(0);
  const [loading, setLoading] = useState(true);
  const [message, setMessage] = useState<string>();
  const [reloadKey, setReloadKey] = useState(0);
  useEffect(() => {
    const controller = new AbortController();
    setLoading(true); setMessage(undefined);
    coreApi.listTemplates(true, controller.signal, page, LIST_PAGE_SIZE).then((response) => { setTemplates(response.data.items); setTotal(response.data.total); }).catch((error: unknown) => { if (!controller.signal.aborted) setMessage(readClientError(error)); }).finally(() => { if (!controller.signal.aborted) setLoading(false); });
    return () => controller.abort();
  }, [page, reloadKey]);
  const enabled = templates.filter((template) => template.enabled).length;
  return <ManagementFrame eyebrow="GOVERNANCE / RULES" title="规则配置" description="规则以 Core 任务模板版本保存；航班到达时使用启用的最高版本生成任务，历史任务保留当时的模板快照。" message={message}>
    <div className="metric-row"><Metric label="规则总数" value={loading ? "—" : String(total)} /><Metric label="当前页启用" value={loading ? "—" : String(enabled)} /><Metric label="当前页触发类型" value={loading ? "—" : String(new Set(templates.map((template) => template.trigger_type)).size)} /><Metric label="当前页最高版本" value={loading ? "—" : String(Math.max(0, ...templates.map((template) => template.template_version)))} /></div>
    <section className="task-panel"><div className="toolbar"><strong>规则版本</strong><button className="secondary-button" onClick={() => setReloadKey((value) => value + 1)}>刷新规则</button></div>{loading && <div className="panel-state">正在读取 Core 规则…</div>}{!loading && templates.length === 0 && <div className="panel-state">当前 Scope 没有任务模板规则。</div>}{!loading && templates.length > 0 && <div className="table-scroll"><table><thead><tr><th>规则</th><th>触发</th><th>目标 Scope</th><th>人员约束</th><th>计划偏移</th><th>状态</th></tr></thead><tbody>{templates.map((template) => <tr key={template.public_id}><td><strong>{template.name}</strong><small>{template.public_id}</small></td><td>{template.trigger_type}<small>v{template.template_version}</small></td><td>{template.area_public_id}<small>{template.team_public_id}</small></td><td>{template.required_position_code}<small>{template.required_capabilities.join(", ") || "无额外能力"}</small></td><td className="data-number">{template.planned_offset_seconds}s</td><td><span className={template.enabled ? "rule-enabled" : "rule-disabled"}>{template.enabled ? "启用" : "停用"}</span></td></tr>)}</tbody></table></div>}{!loading && <ListPager page={page} pageSize={LIST_PAGE_SIZE} total={total} onPageChange={setPage} disabled={loading} />}</section>
  </ManagementFrame>;
}

function ReportsPage(): JSX.Element {
  const [report, setReport] = useState<CoreReportOverview>();
  const [filters, setFilters] = useState({ from: "", to: "" });
  const [applied, setApplied] = useState({ from: "", to: "" });
  const [loading, setLoading] = useState(true);
  const [message, setMessage] = useState<string>();
  const [reloadKey, setReloadKey] = useState(0);
  useEffect(() => {
    const controller = new AbortController();
    setLoading(true); setMessage(undefined);
    coreApi.getReportOverview(applied, controller.signal).then((response) => setReport(response.data)).catch((error: unknown) => { if (!controller.signal.aborted) setMessage(readClientError(error)); }).finally(() => { if (!controller.signal.aborted) setLoading(false); });
    return () => controller.abort();
  }, [applied, reloadKey]);
  const taskTotal = sumCounts(report?.task_counts);
  const completed = report?.task_counts.completed ?? 0;
  const openExceptions = report?.exception_counts.open ?? 0;
  const busyPersonnel = (report?.personnel_counts.busy ?? 0) + (report?.personnel_counts.reserved ?? 0);
  function applyFilters(event: FormEvent<HTMLFormElement>): void { event.preventDefault(); setApplied(filters); }
  return <ManagementFrame eyebrow="GOVERNANCE / REPORTS" title="报表统计" description="报表由 Core 按当前管理 Scope 聚合事实；日期按航班 operating_date 过滤，人员指标反映当前状态。" message={message}>
    <form className="report-filter" onSubmit={applyFilters}><label>开始日期<input type="date" value={filters.from} onChange={(event) => setFilters({ ...filters, from: event.target.value })} /></label><label>结束日期<input type="date" value={filters.to} onChange={(event) => setFilters({ ...filters, to: event.target.value })} /></label><button className="primary-button" type="submit">应用范围</button><button className="secondary-button" type="button" onClick={() => setReloadKey((value) => value + 1)}>刷新</button></form>
    <div className="metric-row"><Metric label="任务总量" value={loading ? "—" : String(taskTotal)} /><Metric label="已完成任务" value={loading ? "—" : String(completed)} /><Metric label="处理中人员" value={loading ? "—" : String(busyPersonnel)} /><Metric label="待处理异常" value={loading ? "—" : String(openExceptions)} /></div>
    <div className="report-grid"><ReportCard title="任务状态" counts={report?.task_counts} labels={{ pending_dispatch: "待自动派发", awaiting_confirmation: "待确认（兼容）", assigned: "已派发/待收件", in_progress: "执行中", completed: "已完成", cancelled: "已取消" }} /><ReportCard title="航班状态" counts={report?.flight_counts} labels={{ scheduled: "计划", arrived: "已到达", departed: "已离港", cancelled: "已取消" }} /><ReportCard title="Assignment 状态" counts={report?.assignment_counts} labels={{ confirmed: "已派发", accepted: "已接受（兼容）", completed: "已完成", cancelled: "已取消" }} /><ReportCard title="人员当前状态" counts={report?.personnel_counts} labels={{ idle: "空闲", reserved: "预留", busy: "忙碌", unavailable: "不可用" }} /><ReportCard title="异常状态" counts={report?.exception_counts} labels={{ open: "待处理", acknowledged: "已确认", resolved: "已解决", rejected: "已驳回" }} /></div>
  </ManagementFrame>;
}

function ReportCard({ title, counts, labels }: { title: string; counts?: Record<string, number>; labels: Record<string, string> }): JSX.Element {
  return <section className="detail-card report-card"><h2>{title}</h2>{Object.keys(labels).map((key) => <div className="report-row" key={key}><span>{labels[key]}</span><strong>{counts?.[key] ?? 0}</strong></div>)}</section>;
}

function sumCounts(counts?: Record<string, number>): number { return counts ? Object.values(counts).reduce((total, value) => total + value, 0) : 0; }

function AssignmentManagementPage(): JSX.Element {
  const [assignments, setAssignments] = useState<CoreAssignment[]>([]); const [status, setStatus] = useState(""); const [page, setPage] = useState(1); const [total, setTotal] = useState(0); const [message, setMessage] = useState<string>(); const [reloadKey, setReloadKey] = useState(0);
  useEffect(() => { const controller = new AbortController(); coreApi.listAssignments({ status: status || undefined, page, page_size: LIST_PAGE_SIZE }, controller.signal).then((response) => { setAssignments(response.data.items); setTotal(response.data.total); }).catch((error: unknown) => { if (!controller.signal.aborted) setMessage(readClientError(error)); }); return () => controller.abort(); }, [page, status, reloadKey]);
  return <ManagementFrame eyebrow="TASKS / ASSIGNMENTS" title="任务分配" description="查看系统自动派发的 Assignment、员工收件确认和执行状态。员工不能拒绝任务；冲突通过异常申请进入值班经理/队长处理。" message={message}><section className="task-panel"><div className="toolbar"><label>状态 <select value={status} onChange={(event) => { setStatus(event.target.value); setPage(1); }}><option value="">全部</option><option value="confirmed">已派发</option><option value="accepted">执行中（兼容状态）</option><option value="completed">已完成</option><option value="cancelled">已取消</option></select></label><span className="scope-inline">{total} assignments</span><button className="secondary-button" onClick={() => setReloadKey((value) => value + 1)}>刷新</button></div><div className="table-scroll"><table><thead><tr><th>航班 / 任务</th><th>员工</th><th>状态</th><th>收件</th><th>计划时间</th><th>版本</th></tr></thead><tbody>{assignments.map((assignment) => <tr key={assignment.public_id}><td><strong>{assignment.flight_display_no}</strong><small>{assignment.task_public_id}</small></td><td><strong>{assignment.personnel_name}</strong><small>{assignment.personnel_employee_no}</small></td><td><StatusBadge status={assignment.status as TaskStatus} /></td><td>{assignment.receipt_status === "received" ? `已收到 · ${formatDate(assignment.received_at ?? "")}` : "待确认收件"}</td><td>{formatDate(assignment.planned_at)}</td><td className="data-number">v{assignment.status_version}</td></tr>)}</tbody></table></div><ListPager page={page} pageSize={LIST_PAGE_SIZE} total={total} onPageChange={setPage} /></section></ManagementFrame>;
}

function AdminIdentityManagementPage(): JSX.Element {
  const [identities, setIdentities] = useState<CoreAdminIdentity[]>([]);
  const [page, setPage] = useState(1);
  const [total, setTotal] = useState(0);
  const [form, setForm] = useState({ provider: "development", external_subject: "", display_name: "", role: "manager", global_scope: true, area_public_id: "", team_public_id: "" });
  const [busy, setBusy] = useState(false);
  const [message, setMessage] = useState<string>();
  const [reloadKey, setReloadKey] = useState(0);
  const remoteAreas = useRemoteAreas();
  const remoteTeams = useRemoteTeams(form.area_public_id || undefined);

  useEffect(() => {
    const controller = new AbortController();
    coreApi.listAdminIdentities(true, controller.signal, page, LIST_PAGE_SIZE).then((response) => { setIdentities(response.data.items); setTotal(response.data.total); }).catch((error: unknown) => { if (!controller.signal.aborted) setMessage(readClientError(error)); });
    return () => controller.abort();
  }, [page, reloadKey]);

  async function createIdentity(event: FormEvent<HTMLFormElement>): Promise<void> {
    event.preventDefault();
    if (!form.global_scope && !form.area_public_id && !form.team_public_id) { setMessage("非全局身份至少需要选择一个区域或班组"); return; }
    setBusy(true); setMessage(undefined);
    try {
      await coreApi.createAdminIdentity({ provider: form.provider, external_subject: form.external_subject, display_name: form.display_name, role: form.role, global_scope: form.global_scope, area_public_ids: form.global_scope || !form.area_public_id ? [] : [form.area_public_id], team_public_ids: form.global_scope || !form.team_public_id ? [] : [form.team_public_id] });
      setForm((current) => ({ ...current, external_subject: "", display_name: "", area_public_id: "", team_public_id: "" })); setPage(1); setReloadKey((value) => value + 1);
    } catch (error) { setMessage(readClientError(error)); } finally { setBusy(false); }
  }

  async function toggle(identity: CoreAdminIdentity): Promise<void> { setBusy(true); setMessage(undefined); try { await coreApi.updateAdminIdentity(identity.public_id, { enabled: !identity.enabled }); setReloadKey((value) => value + 1); } catch (error) { setMessage(readClientError(error)); } finally { setBusy(false); } }

  async function editIdentity(identity: CoreAdminIdentity): Promise<void> {
    const displayName = window.prompt("显示名称", identity.display_name);
    if (displayName === null) return;
    const role = window.prompt("角色（admin / manager / leader / supervisor）", identity.role);
    if (role === null) return;
    const globalScopeValue = window.prompt("全局 Scope（true / false）", String(identity.global_scope));
    if (globalScopeValue === null) return;
    const globalScope = globalScopeValue.trim().toLowerCase();
    if (globalScope !== "true" && globalScope !== "false") { setMessage("全局 Scope 只能填写 true 或 false"); return; }
    const areaValue = globalScope === "true" ? "" : window.prompt("管辖区域 public_id（多个用逗号分隔）", (identity.area_public_ids ?? []).join(","));
    if (areaValue === null) return;
    const teamValue = globalScope === "true" ? "" : window.prompt("管辖班组 public_id（多个用逗号分隔）", (identity.team_public_ids ?? []).join(","));
    if (teamValue === null) return;
    const areaPublicIDs = areaValue.split(",").map((value) => value.trim()).filter(Boolean);
    const teamPublicIDs = teamValue.split(",").map((value) => value.trim()).filter(Boolean);
    if (!displayName.trim() || !["admin", "manager", "leader", "supervisor"].includes(role.trim()) || (globalScope === "false" && areaPublicIDs.length === 0 && teamPublicIDs.length === 0)) { setMessage("显示名称不能为空；角色必须是 admin、manager、leader 或 supervisor；非全局身份至少需要一个区域或班组"); return; }
    setBusy(true); setMessage(undefined);
    try { await coreApi.updateAdminIdentity(identity.public_id, { display_name: displayName.trim(), role: role.trim(), global_scope: globalScope === "true", area_public_ids: areaPublicIDs, team_public_ids: teamPublicIDs }); setReloadKey((value) => value + 1); setMessage(`已更新身份“${displayName.trim()}”`); } catch (error) { setMessage(readClientError(error)); } finally { setBusy(false); }
  }

  return <ManagementFrame eyebrow="SYSTEM / ADMIN IDENTITIES" title="用户与角色" description="管理端 SSO 的外部身份映射到 Core 角色。队长可按区域或班组限制 Scope，主任和系统管理员可以使用全局 Scope；最终权限仍由 Core 服务端判定。" message={message || remoteAreas.error || remoteTeams.error}>
    <section className="detail-card"><h2>新增管理身份</h2><form className="management-form form-grid" onSubmit={createIdentity}><select value={form.provider} onChange={(event) => setForm({ ...form, provider: event.target.value })}><option value="development">development</option><option value="wecom">wecom</option><option value="oidc">oidc</option></select><input required placeholder="外部 subject" value={form.external_subject} onChange={(event) => setForm({ ...form, external_subject: event.target.value })} /><input required placeholder="显示名称" value={form.display_name} onChange={(event) => setForm({ ...form, display_name: event.target.value })} /><select value={form.role} onChange={(event) => setForm({ ...form, role: event.target.value })}><option value="admin">admin</option><option value="manager">manager</option><option value="leader">leader</option><option value="supervisor">supervisor（分管领导）</option></select><label className="checkbox-field"><input type="checkbox" checked={form.global_scope} onChange={(event) => setForm({ ...form, global_scope: event.target.checked })} /> 全局 Scope</label><RemoteOptionSelect label="管辖区域" value={form.area_public_id} options={remoteAreas.items.filter((area) => area.enabled)} total={remoteAreas.total} query={remoteAreas.query} setQuery={remoteAreas.setQuery} loading={remoteAreas.loading} error={remoteAreas.error} getValue={(area) => area.public_id} getLabel={(area) => `${area.code} / ${area.name}`} onChange={(value) => setForm({ ...form, area_public_id: value, team_public_id: "" })} disabled={busy || form.global_scope} /><RemoteOptionSelect label="管辖班组" value={form.team_public_id} options={remoteTeams.items.filter((team) => team.enabled)} total={remoteTeams.total} query={remoteTeams.query} setQuery={remoteTeams.setQuery} loading={remoteTeams.loading} error={remoteTeams.error} getValue={(team) => team.public_id} getLabel={(team) => team.name} onChange={(value) => setForm({ ...form, team_public_id: value })} disabled={busy || form.global_scope || !form.area_public_id} /><button className="primary-button" disabled={busy || (!form.global_scope && !form.area_public_id && !form.team_public_id)}>创建身份</button></form></section>
    <section className="task-panel"><div className="toolbar"><strong>已配置身份</strong><span className="scope-inline">{total} identities</span></div><div className="table-scroll"><table><thead><tr><th>身份</th><th>Provider / Subject</th><th>角色</th><th>Scope</th><th /></tr></thead><tbody>{identities.map((identity) => <tr key={identity.public_id}><td><strong>{identity.display_name}</strong><small>{identity.public_id}</small></td><td>{identity.provider}<small>{identity.external_subject}</small></td><td>{identity.role}</td><td>{identity.global_scope ? "全局" : `${identity.area_ids?.length ?? 0} area / ${identity.team_ids?.length ?? 0} team`}</td><td><button className="text-button light-text-button" disabled={busy} onClick={() => editIdentity(identity)}>编辑</button><button className="text-button light-text-button" disabled={busy} onClick={() => toggle(identity)}>{identity.enabled ? "停用" : "启用"}</button></td></tr>)}</tbody></table></div><ListPager page={page} pageSize={LIST_PAGE_SIZE} total={total} onPageChange={setPage} disabled={busy} /></section>
  </ManagementFrame>;
}
