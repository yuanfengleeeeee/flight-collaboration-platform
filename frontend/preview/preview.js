const page = document.body.dataset.page || "index";
const params = new URLSearchParams(window.location.search);

const taskSeed = [
  {
    id: "task-001",
    time: "07:40",
    flight: "MU2156",
    task: "国际航班值机准备",
    team: "国际值机服务组",
    area: "T2 / A区",
    status: "awaiting",
    candidates: 4,
    owner: "待确认",
    note: "航班到达后触发",
    description: "完成国际航班值机准备，复核证件查验和特殊文件提示。",
  },
  {
    id: "task-002",
    time: "08:15",
    flight: "CA1832",
    task: "到达行李地面派送",
    team: "行李地面服务组",
    area: "T1 / B区",
    status: "assigned",
    candidates: 0,
    owner: "张三",
    note: "已分配 · 待员工接受",
    description: "完成到达行李地面派送，记录异常行李并反馈处理进度。",
  },
  {
    id: "task-003",
    time: "09:05",
    flight: "ZH9210",
    task: "航班地面保障协调",
    team: "航班地面保障组",
    area: "T2 / C区",
    status: "in-progress",
    candidates: 0,
    owner: "李四",
    note: "执行中 · 已接受 08:54",
    description: "完成航班地面保障协调，确认机位、登机口和保障节点。",
  },
  {
    id: "task-004",
    time: "10:30",
    flight: "HO1188",
    task: "特殊旅客服务确认",
    team: "特殊旅客服务组",
    area: "T2 / D区",
    status: "completed",
    candidates: 0,
    owner: "王五",
    note: "已完成 · 10:12",
    description: "确认特殊旅客服务需求，安排陪同、轮椅或无陪儿童服务。",
  },
  {
    id: "task-005",
    time: "11:20",
    flight: "FM9450",
    task: "不正常航班旅客服务",
    team: "不正常航班服务组",
    area: "T2 / A区",
    status: "cancelled",
    candidates: 0,
    owner: "—",
    note: "已取消 · 航班调整",
    description: "因航班计划调整，当前旅客服务任务已取消。",
  },
];

const statusMeta = {
  awaiting: { label: "待确认", className: "awaiting" },
  assigned: { label: "已分配", className: "assigned" },
  "in-progress": { label: "执行中", className: "in-progress" },
  completed: { label: "已完成", className: "completed" },
  cancelled: { label: "已取消", className: "cancelled" },
};

const roleMeta = {
  manager: { label: "主任", scope: "全部团队 · 全部区域", initials: "主" },
  leader: { label: "队长", scope: "国际值机服务组 · T2", initials: "队" },
  admin: { label: "系统管理员", scope: "系统全局", initials: "管" },
};

const adminModules = {
  overview: { label: "运行总览", kicker: "OPERATIONS / OVERVIEW", description: "汇总今日航班、任务、人员状态与待处理提醒。" },
  flights: { label: "航班运行", kicker: "FLIGHTS / OPERATIONS", description: "查看航班计划、到达事实和任务触发上下文。" },
  tasks: { label: "任务工作台", kicker: "TASKS / WORKBENCH", description: "按角色 Scope 查找任务并进入正确的处理动作。" },
  "task-templates": { label: "任务模板", kicker: "TASKS / TEMPLATES", description: "维护任务模板、触发条件和默认岗位要求。" },
  assignments: { label: "任务分配", kicker: "TASKS / ASSIGNMENTS", description: "查看队长管辖、候选人和员工 Assignment。" },
  "task-history": { label: "任务历史", kicker: "TASKS / HISTORY", description: "按航班、任务和状态回溯已发生的业务事实。" },
  personnel: { label: "人员档案", kicker: "PEOPLE / DIRECTORY", description: "查看人员、团队与区域归属。" },
  positions: { label: "岗位与能力", kicker: "PEOPLE / CAPABILITY", description: "查看岗位定义、能力要求和匹配条件。" },
  "personnel-status": { label: "人员状态", kicker: "PEOPLE / AVAILABILITY", description: "查看现场可用、忙碌、停用等人员状态。" },
  rules: { label: "规则配置", kicker: "RULES / CONFIGURATION", description: "维护任务生成、候选计算和分配规则。" },
  events: { label: "事件与通知", kicker: "EVENTS / NOTIFICATIONS", description: "查看运行事件、通知投递和同步异常。" },
  audit: { label: "审计日志", kicker: "AUDIT / HISTORY", description: "追踪用户操作、状态变更和关键请求。" },
  reports: { label: "报表统计", kicker: "REPORTS / SIGNALS", description: "汇总任务、航班、人员和同步表现。" },
  users: { label: "用户与角色", kicker: "SYSTEM / USERS", description: "维护管理端用户和角色入口。" },
  scopes: { label: "权限范围", kicker: "SYSTEM / SCOPES", description: "维护团队、区域与角色可见范围。" },
  diagnostics: { label: "开发诊断", kicker: "SYSTEM / DIAGNOSTICS", description: "仅开发环境使用的请求、同步和状态诊断。" },
};

const adminNavGroups = [
  {
    label: "运行中枢",
    items: [["overview", "运行总览", "◈"], ["flights", "航班运行", "✦"], ["tasks", "任务工作台", "▦"]],
  },
  {
    label: "任务中心",
    items: [["task-templates", "任务模板", "□"], ["assignments", "任务分配", "⇄"], ["task-history", "任务历史", "↺"]],
  },
  {
    label: "人员与规则",
    items: [["personnel", "人员档案", "♙"], ["positions", "岗位与能力", "◇"], ["personnel-status", "人员状态", "◌"], ["rules", "规则配置", "⌘"]],
  },
  {
    label: "复盘与系统",
    items: [["events", "事件与通知", "!"], ["audit", "审计日志", "≡"], ["reports", "报表统计", "▥"], ["users", "用户与角色", "♙"], ["scopes", "权限范围", "◎"], ["diagnostics", "开发诊断", "⊙"]],
  },
];

const state = {
  mode: page === "employee" ? "employee" : page.startsWith("login") ? "auth" : "admin",
  adminView: "list",
  adminSection: params.get("section") || "tasks",
  adminRole: params.get("role") === "leader" ? "leader" : "manager",
  employeeView: "list",
  employeeSection: "tasks",
  selectedId: "task-001",
  adminFilter: "all",
  provider: "个人微信",
  employeeStatus: "assigned",
  commandStatus: "idle",
  adminTaskStatus: "awaiting",
  authRole: page === "login-employee" ? "employee" : "admin",
};

const app = document.querySelector("#app");
const toast = document.querySelector("#toast");
let toastTimer;

function statusBadge(status) {
  const meta = statusMeta[status] || statusMeta.awaiting;
  return `<span class="status-badge ${meta.className}">${meta.label}</span>`;
}

function effectiveTaskStatus(task) {
  return task.id === "task-001" ? state.adminTaskStatus : task.status;
}

function taskVisibleForAdmin(task) {
  if (state.adminRole !== "leader") return true;
  return task.team === "国际值机服务组" && task.area.startsWith("T2");
}

function getSelectedTask() {
  const task = taskSeed.find((item) => item.id === state.selectedId) || taskSeed[0];
  const status = effectiveTaskStatus(task);
  return task.id === "task-001"
    ? { ...task, status, owner: status === "assigned" ? "张三" : status === "cancelled" ? "—" : task.owner }
    : task;
}

function showToast(message) {
  toast.textContent = message;
  toast.classList.add("is-visible");
  clearTimeout(toastTimer);
  toastTimer = setTimeout(() => toast.classList.remove("is-visible"), 3200);
}

function renderAdminSidebar() {
  const role = roleMeta[state.adminRole];
  const nav = adminNavGroups.map((group) => `<div class="nav-group"><span class="nav-label">${group.label}</span>${group.items.map(([key, label, icon]) => {
    const restricted = state.adminRole === "leader" && ["users", "scopes"].includes(key);
    return `<button class="nav-link ${state.adminSection === key ? "is-active" : ""} ${restricted ? "is-restricted" : ""}" data-action="admin-section" data-section="${key}" ${restricted ? `title="${label}仅限主任或系统管理员" disabled` : ""}><span class="nav-icon">${icon}</span><span>${label}</span>${restricted ? `<small>需授权</small>` : ""}</button>`;
  }).join("")}</div>`).join("");

  return `<aside class="admin-sidebar"><div class="brand-lockup"><span class="brand-mark">A</span><strong>航空客运地面代理协同</strong><small>OPERATIONS / CONTROL</small></div><nav class="admin-nav" aria-label="管理端工作区">${nav}</nav><div class="scope-card"><span>当前可见范围</span><strong>${role.scope}</strong><small>由 Core Scope 控制 · ${role.label}视图</small></div><div class="sidebar-footer"><div class="operator"><span class="avatar">${role.initials}</span><span>${role.label}<br /><small>${state.adminRole === "leader" ? "Leader · T2" : state.adminRole === "admin" ? "System · Global" : "Manager · Global"}</small></span></div><button class="role-preview" data-action="toggle-admin-role">Mock：切换为${state.adminRole === "leader" ? "主任" : "队长"}视图</button></div></aside>`;
}

function adminMetric(label, value, tone = "") {
  return `<article class="metric-card ${tone}"><span>${label}</span><strong>${value}</strong></article>`;
}

function renderAdminList() {
  const visibleTasks = taskSeed.filter((task) => taskVisibleForAdmin(task)).filter((task) => state.adminFilter === "all" || effectiveTaskStatus(task) === state.adminFilter);
  const role = roleMeta[state.adminRole];

  return `<div class="admin-shell">${renderAdminSidebar()}<section class="admin-content"><div class="content-topline"><div class="breadcrumb">运行管理台 <span>/</span> <strong>任务工作台</strong></div><div class="topline-actions"><span class="sync-pill">Core 已同步 07:42</span><button class="text-button" data-action="refresh">刷新</button></div></div><div class="page-heading"><div><span class="eyebrow">SHIFT / 09.01 · ${role.scope}</span><h1>任务工作台</h1><p>${state.adminRole === "leader" ? "只显示当前队长所属团队与区域的任务。" : "主任视图：查看所有队长、团队、区域和任务状态。"}</p></div><div class="live-indicator"><strong>运行状态正常</strong><small>最后读取 07:42:18 · ${role.label}视图</small></div></div><div class="metric-grid">${adminMetric("待确认", state.adminRole === "leader" ? "04" : "12", "")}${adminMetric("已分配", state.adminRole === "leader" ? "03" : "08", "blue")}${adminMetric("执行中", state.adminRole === "leader" ? "01" : "04", "blue")}${adminMetric("今日已完成", state.adminRole === "leader" ? "11" : "36", "teal")}</div><section class="workbench-card"><div class="scope-banner"><div><span class="section-kicker">SCOPE FILTER</span><strong>${role.label} · ${role.scope}</strong></div><span>${state.adminRole === "leader" ? "团队/区域自动授权" : "全局可见"}</span></div><div class="toolbar-row"><div class="toolbar-group"><label class="input-wrap"><span>⌕</span><input aria-label="搜索航班或任务" placeholder="搜索航班或任务" /></label><select class="select-control" aria-label="选择团队"><option>${state.adminRole === "leader" ? "国际值机服务组" : "全部团队"}</option><option>值机服务组</option><option>行李地面服务组</option><option>航班地面保障组</option></select><select class="select-control" aria-label="选择区域"><option>${state.adminRole === "leader" ? "T2" : "全部区域"}</option><option>T1</option><option>T2</option></select><select class="select-control" aria-label="选择队长" ${state.adminRole === "leader" ? "disabled" : ""}><option>${state.adminRole === "leader" ? "当前队长" : "全部队长"}</option><option>李队长</option><option>王队长</option></select></div><button class="secondary-button" data-action="open-auth">查看登录流程</button></div><div class="filter-tabs" role="tablist" aria-label="任务状态筛选">${renderFilterButton("all", "全部任务")}${renderFilterButton("awaiting", "待确认")}${renderFilterButton("assigned", "已分配")}${renderFilterButton("in-progress", "执行中")}${renderFilterButton("completed", "已完成")}</div><div class="table-scroll"><table class="task-table"><thead><tr><th>计划时间</th><th>航班 / 任务</th><th>团队</th><th>区域</th><th>状态</th><th>队长 / 执行人</th><th></th></tr></thead><tbody>${visibleTasks.map(renderAdminTaskRow).join("") || renderEmptyAdminRow()}</tbody></table></div><div class="list-footer"><span>显示 ${visibleTasks.length} 条当前 Scope 任务</span><span>Projection lag &lt; 30s</span></div></section></section></div>`;
}

function renderFilterButton(filter, label) {
  return `<button class="filter-tab ${state.adminFilter === filter ? "is-active" : ""}" data-filter="${filter}" role="tab">${label}</button>`;
}

function renderAdminTaskRow(task) {
  const status = effectiveTaskStatus(task);
  const meta = statusMeta[status];
  const candidateText = status === "awaiting" ? `${task.candidates} 人候选` : task.owner;
  return `<tr class="is-${meta.className}" data-task-id="${task.id}"><td><div class="time-cell"><strong class="data-number">${task.time}</strong><small>计划开始</small></div></td><td><div class="task-name-cell"><strong>${task.task}</strong><small class="flight-no">${task.flight}</small></div></td><td>${task.team}</td><td>${task.area}</td><td>${statusBadge(status)}</td><td><div class="candidate-count"><strong>${candidateText}</strong><small>${task.note}</small></div></td><td><button class="text-button" data-task-id="${task.id}">详情 →</button></td></tr>`;
}

function renderEmptyAdminRow() {
  const message = state.adminRole === "leader" ? "当前管辖范围暂无任务" : "当前筛选下没有任务";
  return `<tr><td colspan="7"><div class="empty-state"><strong>${message}</strong><span>权限范围由服务端控制，清除状态筛选后继续查看。</span><button class="secondary-button small-button" data-filter="all">清除筛选</button></div></td></tr>`;
}

function renderAdminDetail() {
  const task = getSelectedTask();
  const status = task.status;
  const canConfirm = status === "awaiting" && taskVisibleForAdmin(task);
  return `<div class="admin-shell">${renderAdminSidebar()}<section class="admin-content"><div class="content-topline"><button class="back-button" data-action="admin-list">← 返回任务工作台</button><div class="topline-actions"><span class="sync-pill">Core 已同步 07:42</span><button class="text-button" data-action="refresh">刷新详情</button></div></div><div class="detail-layout"><div class="detail-main"><article class="detail-card"><div class="detail-heading"><div><span class="eyebrow">FLIGHT STRIP / ${task.flight}</span><h1>${task.task}</h1><p>${task.team} · ${task.area} · 管辖队长：${task.team === "国际值机服务组" ? "李队长" : "王队长"}</p></div>${statusBadge(status)}</div><div class="detail-facts"><div class="fact"><span>计划开始</span><strong class="data-number">${task.time}</strong></div><div class="fact"><span>触发来源</span><strong>航班到达</strong></div><div class="fact"><span>当前执行人</span><strong>${task.owner}</strong></div><div class="fact"><span>Task 版本</span><strong class="data-number">v12</strong></div></div></article>${canConfirm ? renderCandidateCard(task) : renderAssignmentCard(task)}${renderTimeline(task)}</div><aside class="detail-side"><div class="side-card"><span class="section-kicker">NEXT DECISION</span><h2>${canConfirm ? "确认一名候选人" : "任务状态"}</h2><p>${canConfirm ? "确认后将创建 Assignment，并把任务发送到员工端。队长只能操作当前 Scope 内任务。" : "所有最终状态来自 Core 事实源，当前页面只做展示。"}</p>${canConfirm ? `<button class="primary-button" data-action="confirm-task">确认分配</button><button class="secondary-button" data-action="cancel-task">取消任务</button>` : `<button class="secondary-button" data-action="admin-list">返回任务工作台</button>`}</div><div class="diagnostic-card"><span class="section-kicker">ACCESS CONTEXT</span><code>role: ${state.adminRole}</code><code>scope: ${roleMeta[state.adminRole].scope}</code><code>request_id: req_mock_8f42</code><code>expected_version: 12</code></div></aside></div></section></div>`;
}

function renderCandidateCard(task) {
  const candidates = [["张三", "值机服务岗位", "07:30 可用"], ["李四", "值机服务岗位", "07:35 可用"], ["王五", "值机服务岗位", "07:38 可用"]];
  return `<section class="candidate-card"><div class="section-title"><h2>推荐人员（${task.candidates}）</h2><p>按资格与计划时间排序</p></div><div class="candidate-list">${candidates.map(([name, role, time]) => `<div class="candidate-row"><span class="candidate-radio">○</span><strong>${name}</strong><small>${role}</small><span class="mini-chip">能力匹配</span><small>${time}</small><button class="secondary-button small-button">选择</button></div>`).join("")}</div></section>`;
}

function renderAssignmentCard(task) {
  const isCancelled = task.status === "cancelled";
  return `<section class="candidate-card"><div class="section-title"><h2>${isCancelled ? "取消记录" : "当前 Assignment"}</h2><p>${isCancelled ? "保留业务审计事实" : "服务端事实"}</p></div><div class="account-row"><div><strong>${isCancelled ? "任务已取消" : task.owner}</strong><small>${isCancelled ? "原因：航班计划调整" : "员工已进入 Edge Projection"}</small></div>${statusBadge(task.status)}</div></section>`;
}

function renderTimeline(task) {
  const items = task.status === "awaiting" ? [["任务生成", "07:31", true], ["候选计算", "07:31", true], ["等待确认", "现在", false]] : [["任务生成", "07:31", true], ["确认分配", "07:35", true], [task.status === "cancelled" ? "任务取消" : "员工端已收到", task.status === "cancelled" ? "07:45" : "07:42", true]];
  return `<section class="timeline-card"><div class="section-title"><h2>状态时间线</h2><p>Core event → Edge projection</p></div><div class="timeline">${items.map(([label, time, done]) => `<div class="timeline-item ${done ? "is-done" : ""}"><span class="timeline-dot"></span><strong>${label}</strong><small>${time}</small></div>`).join("")}</div></section>`;
}

function renderAdminModule() {
  const module = adminModules[state.adminSection] || adminModules.overview;
  const role = roleMeta[state.adminRole];
  return `<div class="admin-shell">${renderAdminSidebar()}<section class="admin-content"><div class="content-topline"><div class="breadcrumb">运行管理台 <span>/</span> <strong>${module.label}</strong></div><span class="sync-pill">页面骨架已注册</span></div><div class="page-heading"><div><span class="eyebrow">${module.kicker}</span><h1>${module.label}</h1><p>${module.description}</p></div><div class="live-indicator"><strong>首期工作区</strong><small>${role.label} · ${role.scope}</small></div></div><section class="module-empty"><div class="module-empty-mark">${state.adminSection === "overview" ? "◈" : "□"}</div><span class="section-kicker">MODULE SCAFFOLD</span><h2>导航与权限骨架已就位</h2><p>这个模块已纳入首期管理端工作区。当前先确认页面边界、角色可见范围和空态，真实字段与业务内容将在对应 API 和业务合同确认后接入。</p><div class="module-empty-grid"><div><span>当前角色</span><strong>${role.label}</strong></div><div><span>当前范围</span><strong>${role.scope}</strong></div><div><span>数据状态</span><strong>暂无 Mock 内容</strong></div></div></section></section></div>`;
}

function renderEmployee() {
  const task = taskSeed.find((item) => item.id === "task-002");
  if (state.employeeSection === "account") return renderEmployeeAccount();
  if (["notifications", "exceptions", "history"].includes(state.employeeSection)) return renderEmployeeSection(state.employeeSection);
  return `<section class="employee-stage"><div class="employee-stage-inner"><div class="employee-intro"><span class="eyebrow">EMPLOYEE / ${state.provider.toUpperCase()}</span><h1>把下一项任务，<em>带到现场。</em></h1><p>员工端只展示服务端分配给你的任务。个人微信与企业微信共用同一份业务数据，操作状态以服务端同步为准。</p><div class="intro-points"><span class="intro-point">单手查看航班与区域</span><span class="intro-point">接受与完成都有同步反馈</span><span class="intro-point">通知、异常和历史独立可达</span></div></div><div class="phone"><div class="phone-screen"><div class="phone-notch"></div>${state.employeeView === "detail" ? renderEmployeeDetail(task) : renderEmployeeList(task)}</div></div></div></section>`;
}

function renderEmployeeList() {
  const other = taskSeed.filter((item) => ["task-002", "task-003", "task-004"].includes(item.id));
  return `<div class="phone-topbar"><div><span class="eyebrow">09 月 01 日 · 今日任务</span><h1>我的任务</h1><p>已同步 07:42 · ${state.commandStatus === "pending" ? "操作等待同步" : "数据正常"}</p></div><span class="provider-pill">${state.provider}</span></div><div class="phone-content"><div class="phone-section-heading"><h2>当前任务</h2><span>3 项</span></div><div class="employee-task-list">${other.map((item) => { const status = item.id === "task-002" ? state.employeeStatus : item.status; return `<article class="employee-task-card ${status === "in-progress" ? "is-progress" : ""} ${status === "completed" ? "is-completed" : ""} ${status === "cancelled" ? "is-cancelled" : ""}" data-task-id="${item.id}"><div class="task-card-top"><div><small>${item.time} · ${item.area}</small><strong>${item.flight}</strong></div>${statusBadge(status)}</div><div class="task-card-bottom"><span>${item.task}</span><span class="task-card-arrow">›</span></div></article>`; }).join("")}</div></div>${renderEmployeeNav("tasks")}`;
}

function renderEmployeeDetail(task) {
  const status = state.employeeStatus;
  const meta = statusMeta[status];
  const commandCopy = state.commandStatus === "pending" ? "已提交，等待任务状态同步" : state.commandStatus === "confirmed" ? "已同步到最新任务状态" : "操作将进入 Edge Command";
  const action = status === "assigned" ? `<button class="primary-button" data-action="accept-task">接受任务</button>` : status === "in-progress" ? `<button class="primary-button" data-action="complete-task">标记完成</button>` : `<button class="secondary-button" data-action="employee-list">返回我的任务</button>`;
  return `<div class="phone-content"><div class="phone-detail-header"><button class="phone-back" data-action="employee-list">←</button><div><h2>任务详情</h2><p>${state.provider} · ${meta.label}</p></div></div><div class="phone-flight-panel"><span class="flight-no">${task.time} · ${task.flight}</span><strong>${task.task}</strong><span>${task.area} · 计划开始 ${task.time}</span></div><div class="phone-info-card"><h3>任务说明</h3><p>${task.description}</p></div><div class="phone-info-card"><h3>当前状态</h3><p>${meta.label} · 最后同步 07:42</p></div>${state.commandStatus !== "idle" ? `<div class="command-row"><strong>${state.commandStatus === "pending" ? "同步中" : "已确认"}</strong><span>${commandCopy}</span></div>` : ""}<div class="phone-action-dock">${action}</div></div>${renderEmployeeNav("tasks")}`;
}

function renderEmployeeSection(section) {
  const content = {
    notifications: ["通知", "NOTIFICATIONS", "任务变更、同步提醒和系统通知会显示在这里。", "暂无新通知"],
    exceptions: ["异常", "EXCEPTIONS", "现场异常和需要跟进的问题会在这里集中处理。", "暂无待处理异常"],
    history: ["历史任务", "TASK HISTORY", "已完成与已取消的本人任务将在这里留下记录。", "暂无历史任务"],
  }[section];
  return `<section class="employee-stage"><div class="employee-stage-inner"><div class="employee-intro"><span class="eyebrow">EMPLOYEE / ${content[1]}</span><h1>把现场信息，<em>留在同一条线上。</em></h1><p>${content[2]}</p><div class="intro-points"><span class="intro-point">个人微信与企业微信共用</span><span class="intro-point">状态来自 Edge Projection</span></div></div><div class="phone"><div class="phone-screen"><div class="phone-notch"></div><div class="phone-topbar"><div><span class="eyebrow">EMPLOYEE / ${content[1]}</span><h1>${content[0]}</h1><p>张三 · E000123</p></div><span class="provider-pill">${state.provider}</span></div><div class="phone-content"><div class="mobile-empty"><span class="mobile-empty-mark">${section === "exceptions" ? "!" : section === "history" ? "↺" : "◌"}</span><strong>${content[3]}</strong><p>${content[2]}</p></div></div>${renderEmployeeNav(section)}</div></div></div></section>`;
}

function renderEmployeeAccount() {
  return `<div class="employee-account-stage"><div class="phone phone-account"><div class="phone-screen"><div class="phone-notch"></div><div class="phone-topbar"><div><span class="eyebrow">ACCOUNT / SECURITY</span><h1>账号</h1><p>当前设备会话</p></div><span class="provider-pill">${state.provider}</span></div><div class="phone-content account-panel"><section class="account-card"><h2>当前账号</h2><div class="account-row"><div><strong>张三</strong><small>工号 E000123</small></div><span class="status-badge completed">已认证</span></div></section><section class="account-card"><h2>身份入口</h2><div class="account-row"><div><strong>个人微信</strong><small>${state.provider === "个人微信" ? "当前入口 · 已绑定" : "已绑定，可快捷登录"}</small></div><span class="status-badge completed">已绑定</span></div><div class="account-row"><div><strong>企业微信</strong><small>绑定后可从企业工作台进入</small></div><button class="secondary-button small-button" data-action="bind-wecom">绑定</button></div></section><section class="account-card"><h2>会话</h2><div class="account-row"><div><strong>本次登录</strong><small>今天 07:38 · Edge Session</small></div><button class="danger-button small-button" data-action="logout">退出</button></div></section></div>${renderEmployeeNav("account")}</div></div><div class="account-copy"><span class="eyebrow">EMPLOYEE / ACCOUNT</span><h1>身份入口和会话，<em>由你掌控。</em></h1><p>个人微信、企业微信最终绑定同一个员工账号。退出只撤销当前设备会话，不删除任务数据。</p></div></div>`;
}

function renderEmployeeNav(active) {
  const items = [["tasks", "任务", "⌁", "employee-list"], ["notifications", "通知", "◌", "employee-notifications"], ["exceptions", "异常", "!", "employee-exceptions"], ["history", "历史", "↺", "employee-history"], ["account", "账号", "♙", "employee-account"]];
  return `<nav class="employee-nav" aria-label="员工端导航">${items.map(([key, label, icon, action]) => `<button class="${active === key ? "is-active" : ""}" data-action="${action}"><span>${icon}</span>${label}${key === "notifications" ? `<i class="nav-unread">2</i>` : ""}</button>`).join("")}</nav>`;
}

function renderAuth() {
  const isAdmin = state.authRole === "admin";
  return `<section class="login-stage"><div class="login-brand-panel"><span class="brand-mark">A</span><span class="eyebrow login-kicker">${isAdmin ? "ADMIN ACCESS / CORE" : "EMPLOYEE ACCESS / EDGE"}</span><h1>${isAdmin ? "进入运行台，<span>看清每一个决定。</span>" : "把下一项任务，<span>带到现场。</span>"}</h1><p>${isAdmin ? "面向主任、队长和系统管理员的独立管理端入口。" : "工号首次认证，绑定个人微信或企业微信后下次快捷进入。"}</p><div class="login-signal-line"><i></i><b></b><i></i></div></div><div class="login-form-panel"><div class="login-header"><div><span class="eyebrow">${isAdmin ? "MANAGEMENT / LOGIN" : "STAFF / LOGIN"}</span><h1>${isAdmin ? "管理端登录" : "员工登录"}</h1><p>${isAdmin ? "使用管理账号进入 Core 运行台" : "首次使用请通过工号与密码认证"}</p></div>${!isAdmin ? `<span class="provider-pill">${state.provider}</span>` : ""}</div><form class="login-form" data-form="login"><div class="login-field"><label for="login-account">${isAdmin ? "管理账号" : "工号"}</label><input id="login-account" value="${isAdmin ? "manager.demo" : "E000123"}" autocomplete="username" /></div><div class="login-field"><label for="login-password">密码</label><input id="login-password" type="password" value="demo-password" autocomplete="current-password" /></div><button class="primary-button" type="submit">${isAdmin ? "进入管理端" : "登录并继续"}</button><div class="login-footnote">${isAdmin ? "当前为 Mock 页面，不请求真实 Core。" : "登录后可绑定个人微信或企业微信，绑定成功后使用可撤销的长会话。"}</div></form><div class="login-route-links"><span>切换独立入口</span>${isAdmin ? `<a href="./login-employee.html">员工登录 →</a>` : `<a href="./login-admin.html">管理端登录 →</a>`}</div></div></section>`;
}

function render() {
  if (!app) return;
  if (state.mode === "auth") {
    app.innerHTML = renderAuth();
    return;
  }
  if (state.mode === "employee") {
    app.innerHTML = renderEmployee();
    return;
  }
  if (state.adminSection === "tasks" && state.adminView === "detail") {
    app.innerHTML = renderAdminDetail();
    return;
  }
  app.innerHTML = state.adminSection === "tasks" ? renderAdminList() : renderAdminModule();
}

function handleAction(action, target) {
  switch (action) {
    case "admin-section":
      state.adminSection = target?.dataset.section || "tasks";
      state.adminView = "list";
      render();
      break;
    case "admin-list":
      state.adminSection = "tasks";
      state.adminView = "list";
      render();
      break;
    case "open-auth":
      window.location.href = "./login-admin.html";
      break;
    case "refresh":
      showToast("Mock：已重新读取当前页面数据");
      break;
    case "toggle-admin-role":
      state.adminRole = state.adminRole === "leader" ? "manager" : "leader";
      state.adminSection = "tasks";
      state.adminView = "list";
      showToast(`Mock：已切换为${roleMeta[state.adminRole].label}视图`);
      render();
      break;
    case "confirm-task":
      state.adminTaskStatus = "assigned";
      showToast("Mock：确认分配已提交，员工端将等待 Projection 同步");
      render();
      break;
    case "cancel-task":
      state.adminTaskStatus = "cancelled";
      showToast("Mock：取消任务已提交，保留 Audit 与状态时间线");
      render();
      break;
    case "employee-list":
      state.employeeSection = "tasks";
      state.employeeView = "list";
      render();
      break;
    case "employee-account":
      state.employeeSection = "account";
      state.employeeView = "list";
      render();
      break;
    case "employee-notifications":
      state.employeeSection = "notifications";
      state.employeeView = "list";
      render();
      break;
    case "employee-exceptions":
      state.employeeSection = "exceptions";
      state.employeeView = "list";
      render();
      break;
    case "employee-history":
      state.employeeSection = "history";
      state.employeeView = "list";
      render();
      break;
    case "accept-task":
      state.commandStatus = "pending";
      showToast("Mock：Accept Command 已进入 pending，等待 Core Event");
      render();
      window.setTimeout(() => {
        state.employeeStatus = "in-progress";
        state.commandStatus = "confirmed";
        showToast("Mock：Projection 已同步，任务进入执行中");
        render();
      }, 900);
      break;
    case "complete-task":
      state.commandStatus = "pending";
      showToast("Mock：Complete Command 已进入 pending，等待 Core Event");
      render();
      window.setTimeout(() => {
        state.employeeStatus = "completed";
        state.commandStatus = "confirmed";
        showToast("Mock：Projection 已同步，任务已完成");
        render();
      }, 900);
      break;
    case "bind-wecom":
      showToast("Mock：企业微信绑定流程已打开");
      break;
    case "logout":
      window.location.href = "./login-employee.html";
      break;
    default:
      break;
  }
}

document.addEventListener("click", (event) => {
  const filterButton = event.target.closest("[data-filter]");
  if (filterButton && state.mode === "admin") {
    state.adminFilter = filterButton.dataset.filter;
    render();
    return;
  }

  const taskTarget = event.target.closest("[data-task-id]");
  if (taskTarget) {
    state.selectedId = taskTarget.dataset.taskId;
    if (state.mode === "admin" && state.adminSection === "tasks") {
      state.adminView = "detail";
      render();
    } else if (state.mode === "employee" && state.employeeSection === "tasks") {
      state.employeeView = "detail";
      render();
    }
    return;
  }

  const actionTarget = event.target.closest("[data-action]");
  if (actionTarget) handleAction(actionTarget.dataset.action, actionTarget);
});

document.addEventListener("submit", (event) => {
  const form = event.target.closest("[data-form='login']");
  if (!form || state.mode !== "auth") return;
  event.preventDefault();
  showToast("Mock：登录成功，正在进入独立页面");
  window.setTimeout(() => {
    window.location.href = state.authRole === "admin" ? "./admin.html" : "./employee.html";
  }, 450);
});

render();
