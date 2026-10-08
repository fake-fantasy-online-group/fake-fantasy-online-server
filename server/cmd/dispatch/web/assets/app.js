const $ = (id) => document.getElementById(id);
const esc = (v) => escapeHTML(v ?? "");
const defaults = {
  section: "items",
  kind: "all",
  q: "",
  sort: "id-asc",
  page: 1,
  pageSize: 24,
};
let state = { ...defaults },
  meta = {},
  currentItems = [],
  currentRequest,
  requestVersion = 0,
  searchTimer;
let iconAtlas = null,
  iconIndex = new Map(),
  detailVersion = 0;
let view = readStorage("view", "list"),
  recent = readStorage("recent", []),
  saved = readStorage("saved", []);
const profs = [
  [0, "通用 / 其他"],
  [1, "战士"],
  [2, "剑客"],
  [3, "刺客"],
  [4, "药师"],
  [5, "术士"],
  [6, "职业编号 6"],
  [7, "职业编号 7"],
  [8, "职业编号 8"],
  [9, "职业编号 9"],
  [10, "职业编号 10"],
];
const skillTypes = [
  [1, "单体主动"],
  [2, "范围主动"],
  [17, "特殊技能"],
  [18, "被动技能"],
];
const gmLevels = [
  "普通玩家",
  "APP · 基础 GM",
  "WIZARD · 角色管理",
  "ARCH · 可踢出角色",
  "ADMIN · 可封禁账号",
];
const itemMode = () => state.section === "items";
function readStorage(key, fallback) {
  try {
    return JSON.parse(localStorage.getItem(`fantasy-gm-${key}`)) ?? fallback;
  } catch {
    return fallback;
  }
}
function saveStorage(key, value) {
  try {
    localStorage.setItem(`fantasy-gm-${key}`, JSON.stringify(value));
  } catch {
    /* Browsing and copying still work without storage. */
  }
}
function toast(message) {
  const dialog = document.querySelector("dialog[open]");
  (dialog || document.body).append($("toast"));
  $("toast").textContent = message;
  $("toast").hidden = false;
  clearTimeout(toast.timer);
  toast.timer = setTimeout(() => {
    $("toast").hidden = true;
  }, 3600);
}
function options(rows, current, first = "全部") {
  return (
    (first === null ? "" : `<option value="">${esc(first)}</option>`) +
    rows
      .map(
        ([id, name]) =>
          `<option value="${esc(id)}" ${String(id) === String(current) ? "selected" : ""}>${esc(name)}</option>`,
      )
      .join("")
  );
}
function selectField(key, label, rows, first = "全部") {
  return `<label class="field"><span>${label}</span><select name="${key}">${options(rows, state[key], first)}</select></label>`;
}
function numberField(key, label, max = "2147483647") {
  return `<label class="field"><span>${label}</span><input name="${key}" type="number" min="0" max="${max}" step="1" value="${esc(state[key])}" placeholder="不限"></label>`;
}
function readURL() {
  state = { ...defaults };
  const p = new URLSearchParams(location.search);
  const allowed = [
    "section",
    "kind",
    "q",
    "sort",
    "page",
    "pageSize",
    "category",
    "minLevel",
    "maxLevel",
    "minPrice",
    "maxPrice",
    "profession",
    "sex",
    "stat",
    "minStat",
    "tradable",
    "buyable",
    "droppable",
    "stackable",
    "mapId",
    "service",
    "status",
    "repeatable",
    "skillLevel",
    "npc",
  ];
  for (const key of allowed) if (p.has(key)) state[key] = p.get(key);
  if (!["items", "npc", "task", "skill", "gm"].includes(state.section))
    state.section = "items";
  state.page = Math.min(1000000, Math.max(1, Number(state.page) || 1));
  state.pageSize = [24, 48, 60].includes(Number(state.pageSize))
    ? Number(state.pageSize)
    : 24;
}
function params() {
  const p = new URLSearchParams();
  for (const [k, v] of Object.entries(state))
    if (v !== "" && v != null) p.set(k, String(v));
  if (!itemMode()) p.set("kind", state.section);
  return p;
}
function writeURL(push = false) {
  const p = new URLSearchParams();
  for (const [k, v] of Object.entries(state))
    if (v !== "" && v != null && v !== defaults[k]) p.set(k, String(v));
  history[push ? "pushState" : "replaceState"](
    null,
    "",
    p.size ? `?${p}` : location.pathname,
  );
}
function syncControls() {
  $("search").value = state.q;
  $("page-size").value = state.pageSize;
  document.querySelectorAll("[data-section]").forEach((b) => {
    const active = b.dataset.section === state.section;
    b.classList.toggle("active", active);
    if (active) b.setAttribute("aria-current", "page");
    else b.removeAttribute("aria-current");
  });
  $("catalog").hidden = state.section === "gm";
  $("gm-tools").hidden = state.section !== "gm";
  document.querySelector(".command-context").hidden = state.section === "gm";
  $("view-toggle").textContent = view === "list" ? "切换卡片" : "切换列表";
  $("view-toggle").setAttribute("aria-pressed", String(view === "cards"));
  $("item-grid").classList.toggle("list-view", view === "list");
  const commonSort = [
    ["id-asc", "编号 ↑"],
    ["id-desc", "编号 ↓"],
    ["name-asc", "名称"],
    ["level-asc", "等级 ↑"],
    ["level-desc", "等级 ↓"],
  ];
  if (itemMode())
    commonSort.push(
      ["price-asc", "价格 ↑"],
      ["price-desc", "价格 ↓"],
      ["attack-desc", "物攻 ↓"],
      ["defense-desc", "防御 ↓"],
    );
  if (!commonSort.some(([v]) => v === state.sort)) state.sort = "id-asc";
  $("sort").innerHTML = options(commonSort, state.sort, null);
  let fields = "";
  if (itemMode()) {
    fields += selectField(
      "kind",
      "物品类型",
      [
        ["all", "全部物品"],
        ["item", "普通物品"],
        ["equipment", "武器装备"],
      ],
      null,
    );
    fields += selectField(
      "category",
      "分类 / 装备部位",
      (meta.categories || [])
        .filter((c) => state.kind === "all" || c.kind === state.kind)
        .map((c) => [c.value, `${c.label} · ${c.count}`]),
    );
    fields += selectField(
      "profession",
      "装备适用职业",
      ["初行者", "战士", "剑客", "刺客", "药师", "术士"].map((p) => [p, p]),
    );
    fields += selectField("sex", "装备性别", [
      ["1", "男 / 不限"],
      ["2", "女 / 不限"],
    ]);
  }
  if (state.section === "npc") {
    fields += selectField(
      "mapId",
      "所在地图",
      (meta.maps || []).map((m) => [m.id, `${m.name} · ${m.id}`]),
    );
    fields += selectField("service", "NPC 功能", [
      ["shop", "有商店"],
      ["transport", "有传送菜单"],
      ["task", "有关联任务"],
    ]);
    fields += selectField("status", "可见性", [
      ["active", "正常出现"],
      ["hidden", "隐藏配置"],
    ]);
  } else {
    fields +=
      '<div class="filter-pair">' +
      numberField("minLevel", "最低等级") +
      numberField("maxLevel", "最高等级") +
      "</div>";
  }
  if (itemMode()) {
    fields +=
      '<details class="advanced" ' +
      ([
        "stat",
        "minStat",
        "minPrice",
        "maxPrice",
        "tradable",
        "buyable",
        "droppable",
        "stackable",
      ].some((k) => state[k])
        ? "open"
        : "") +
      "><summary>属性与流通</summary>";
    fields += selectField("stat", "装备属性", [
      ["attack", "物理攻击上限"],
      ["magicAttack", "魔法攻击"],
      ["defense", "防御"],
      ["magicDefense", "魔法防御"],
      ["str", "附加力量"],
      ["vit", "附加体质"],
      ["agi", "附加敏捷"],
      ["int", "附加智慧"],
      ["spi", "附加精神"],
      ["dex", "附加灵巧"],
      ["luk", "附加幸运"],
    ]);
    fields += numberField("minStat", "属性至少（默认 1）");
    fields +=
      '<div class="filter-pair">' +
      numberField("minPrice", "最低铜币价") +
      numberField("maxPrice", "最高铜币价") +
      "</div>";
    for (const [k, l] of [
      ["tradable", "可交易"],
      ["buyable", "允许购买"],
      ["droppable", "允许丢弃"],
      ["stackable", "可堆叠"],
    ])
      fields += selectField(k, l, [
        ["1", "是"],
        ["0", "否"],
      ]);
    fields += "</details>";
  }
  if (state.section === "task") {
    fields += selectField("status", "任务状态", [
      ["active", "已启用"],
      ["disabled", "已停用"],
    ]);
    fields += selectField("repeatable", "重复任务", [["1", "仅可重复"]]);
    if (state.npc) fields += `<p class="muted">发布人：${esc(state.npc)}</p>`;
  }
  if (state.section === "skill") {
    fields += selectField("profession", "职业", profs);
    fields += selectField("category", "技能形态", skillTypes);
    fields += numberField("skillLevel", "技能阶级");
    fields += selectField("status", "服务端收录", [
      ["active", "玩家技能范围"],
      ["reference", "其他资料（宠物 / 怪物等）"],
    ]);
  }
  $("filter-fields").innerHTML = fields;
}
function readFilters() {
  const form = new FormData($("filter-form"));
  for (const [k, v] of form) state[k] = v;
  state.q = $("search").value.trim();
  state.sort = $("sort").value;
  state.page = 1;
}
function activeFilters() {
  const names = {
    q: "搜索",
    kind: "类型",
    category: "分类",
    profession: "职业",
    sex: "性别",
    minLevel: "最低等级",
    maxLevel: "最高等级",
    mapId: "地图",
    status: "状态",
    service: "功能",
    stat: "属性",
    minStat: "属性至少",
    npc: "发布人",
    skillLevel: "技能阶级",
    repeatable: "重复",
    tradable: "可交易",
    buyable: "允许购买",
    droppable: "允许丢弃",
    stackable: "可堆叠",
    minPrice: "最低价格",
    maxPrice: "最高价格",
  };
  $("active-filters").innerHTML = Object.entries(state)
    .filter(([k, v]) => names[k] && v !== "" && v != null && v !== defaults[k])
    .map(([k, v]) => {
      const select = document.querySelector(`[name="${k}"]`);
      const text =
        select?.tagName === "SELECT"
          ? select.selectedOptions[0]?.textContent
          : v;
      return `<button data-remove-filter="${k}" class="filter-chip">${names[k]}：${esc(text ?? v)} ×</button>`;
    })
    .join("");
}
async function fetchJSON(path, opts = {}) {
  const r = await fetch(path, opts);
  let data;
  try {
    data = await r.json();
  } catch {
    throw new Error("服务返回了无效响应");
  }
  if (!r.ok) throw new Error(data.error || `请求失败 (${r.status})`);
  return data;
}
async function loadItems() {
  hideTooltip();
  currentRequest?.abort();
  const controller = new AbortController();
  currentRequest = controller;
  const version = ++requestVersion;
  if (state.section === "gm") return;
  $("error-state").hidden = true;
  $("empty-state").hidden = true;
  $("result-count").textContent = "—";
  $("pagination").innerHTML = "";
  $("item-grid").innerHTML = '<p class="loading">正在查找资料…</p>';
  currentItems = [];
  writeURL();
  activeFilters();
  try {
    const data = await fetchJSON(
      `/api/catalog/${itemMode() ? "items" : "entries"}?${params()}`,
      { signal: controller.signal },
    );
    if (version !== requestVersion) return;
    const pages = Math.max(1, Math.ceil(data.total / data.pageSize));
    if (state.page > pages) {
      state.page = pages;
      return loadItems();
    }
    currentItems = data.items;
    $("result-count").textContent = formatNumber(data.total);
    $("result-unit").textContent =
      state.section === "skill" ? "条技能阶级记录" : "条结果";
    $("empty-state").hidden = !!data.items.length;
    renderItems();
    renderPagination(data.total, data.pageSize);
  } catch (e) {
    if (e.name === "AbortError" || version !== requestVersion) return;
    $("item-grid").innerHTML = "";
    $("error-state").hidden = false;
    $("error-state").querySelector("p").textContent = e.message;
  }
}
function renderItems() {
  $("item-grid").innerHTML = currentItems
    .map((i, index) => entryCard(i, index))
    .join("");
}
function entryCard(i, index) {
  const item = ["item", "equipment"].includes(i.kind);
  const skill = i.kind === "skill";
  let facts = [],
    label = "",
    action = "",
    info = "";
  if (item) {
    label = i.typeLabel;
    facts = [
      ["等级", i.level || "不限"],
      ["属性", primaryStat(i)],
      ["购买", i.buyable ? formatCurrency(i.price) : "不可购买"],
    ];
    action = "复制发放";
    info = [
      i.tradable ? "可交易" : "不可交易",
      i.stackable ? "可堆叠" : "不可堆叠",
    ].join(" · ");
  } else if (i.kind === "npc") {
    label = i.hidden ? "隐藏 NPC" : i.shopId ? "商店 NPC" : "NPC";
    facts = [
      ["地图", `${i.mapName} · ${i.mapId}`],
      ["坐标", `${i.x}, ${i.y}`],
      ["关联任务", i.taskCount],
    ];
    action = "复制传送";
    info = `${i.transportId ? "提供传送 · " : ""}${i.hidden ? "隐藏配置，游戏中不出现" : "地图 NPC 编号"}`;
  } else if (i.kind === "task") {
    label = i.disabled ? "已停用" : "任务";
    facts = [
      ["等级", `${i.level}–${i.maxLevel || "不限"}`],
      ["发布人", i.npc],
      ["地区", i.mapName],
    ];
    action = "复制任务 ID";
    info = i.disabled
      ? i.disabledReason
      : i.repeatable
        ? "可重复"
        : "一次性任务";
  } else {
    label =
      skillTypes.find(([id]) => id === i.typeCode)?.[1] || `类型 ${i.typeCode}`;
    facts = [
      ["职业", profs.find(([id]) => id === i.profession)?.[1] || i.profession],
      ["学习等级", i.level],
      ["技能阶级", i.skillLevel],
    ];
    action = "复制技能 ID";
    info =
      i.status === "reference"
        ? "非玩家技能"
        : `冷却 ${i.cooldown}s · ${i.bookId ? "有技能书" : "无技能书要求"}`;
  }
  return `<article class="item-card" data-index="${index}" style="--card-accent:${item ? accentFor(i.typeCode, i.kind === "equipment") : "#62e5d2"}">
 <div class="card-head"><button class="icon-button" data-primary="${index}" aria-label="${action} ${esc(i.name)}">${item ? itemIcon(i) : `<span class="item-glyph">${i.kind === "npc" ? "人" : i.kind === "task" ? "!" : "✦"}</span>`}</button><div class="card-title"><h3><button class="name-button" data-primary="${index}" title="${action}">${esc(i.name)}</button></h3><p>ID ${i.id}${skill ? ` / 阶级 ${i.skillLevel}` : i.kind === "npc" ? ` / 地图 ${i.mapId}` : ""}</p></div><span class="kind-label">${esc(label)}</span></div>
 <div class="card-facts">${facts.map(([l, v]) => `<div><span>${l}</span><strong title="${esc(v)}">${esc(v)}</strong></div>`).join("")}</div>
 <div class="card-bottom"><span class="entry-note" title="${esc(info)}">${esc(info)}</span><button class="subtle" data-detail="${index}">详情</button></div>
 ${item ? `<template class="tooltip-template">${itemTooltip(i, i.kind === "equipment", `Lv.${i.level}`)}</template>` : ""}</article>`;
}
function itemCommand(id, quantity) {
  const n = quantity ?? Number($("quantity").value);
  if (!Number.isInteger(n) || n < 1 || n > 10000)
    throw new Error("发放数量必须是 1–10000 的整数");
  const target = $("target").value.trim();
  if (/[\r\n]/.test(target)) throw new Error("角色名不能包含换行");
  return target ? `/giveitem ${target} ${id} ${n}` : `/item ${id} ${n}`;
}
function primary(i) {
  try {
    if (["item", "equipment"].includes(i.kind))
      return copy(itemCommand(i.id), i.name);
    if (i.kind === "npc") return copy(`/goto ${i.mapId} ${i.x} ${i.y}`, i.name);
    return copy(String(i.id), i.name + " ID");
  } catch (e) {
    toast(e.message);
  }
}
async function copy(text, label = "GM 指令") {
  try {
    if (navigator.clipboard?.writeText)
      await navigator.clipboard.writeText(text);
    else {
      const input = document.createElement("textarea");
      input.value = text;
      document.body.append(input);
      input.select();
      const ok = document.execCommand("copy");
      input.remove();
      if (!ok) throw new Error("clipboard");
    }
    recent = [{ text, label }, ...recent.filter((r) => r.text !== text)].slice(
      0,
      20,
    );
    saveStorage("recent", recent);
    $("history-count").textContent = recent.length;
    toast(`已复制：${text}`);
  } catch {
    utility(
      "手动复制",
      `<p>请手动复制：</p><textarea readonly class="manual-copy">${esc(text)}</textarea>`,
    );
    $("utility-content").querySelector("textarea").select();
  }
}
function commandButton(text, label) {
  return `<button class="subtle" data-command="${esc(text)}">${esc(label)}</button>`;
}
function itemButton(i, label = "复制发放") {
  return `<button class="subtle" data-item-id="${i.id}" ${i.qty > 0 ? `data-qty="${i.qty}"` : ""}>${esc(label)} ${esc(i.name)}${i.qty > 0 ? ` ×${i.qty}` : ""}</button>`;
}
function jumpButton(section, q, label) {
  return `<button class="subtle" data-jump="${section}" data-query="${esc(q)}">${esc(label)}</button>`;
}
function utility(title, html) {
  document.body.append($("toast"));
  $("utility-title").textContent = title;
  $("utility-content").innerHTML = html;
  if (!$("utility-dialog").open) $("utility-dialog").showModal();
}
async function showDetail(i) {
  document.body.append($("toast"));
  hideTooltip();
  const version = ++detailVersion;
  $("detail-title").textContent = `${i.name} · ID ${i.id}`;
  let html = "";
  if (["item", "equipment"].includes(i.kind)) {
    html =
      itemTooltip(i, i.kind === "equipment", `Lv.${i.level}`) +
      `<div class="detail-actions">${itemButton(i)}${commandButton(`/takeitem ${i.id} 1`, "复制扣除 1 个（自己）")}${commandButton(String(i.id), "复制 ID")}</div>`;
  } else if (i.kind === "skill") {
    html = `<h2>${esc(i.name)} <small>阶级 ${i.skillLevel}</small></h2><p class="description">${esc(i.description)}</p><dl class="detail-facts">${[
      ["学习等级", i.level],
      ["最高阶级", i.maxLevel],
      ["职业", profs.find(([id]) => id === i.profession)?.[1] || i.profession],
      ["冷却", i.cooldown + " 秒"],
      ["吟唱", i.castTime + " 毫秒"],
      ["施法距离", i.distance],
      ["学习费用", formatCurrency(i.learnMoney || 0)],
    ]
      .map(([k, v]) => `<div><dt>${k}</dt><dd>${esc(v)}</dd></div>`)
      .join("")}</dl>`;
    html += `<div class="detail-actions">${commandButton(String(i.id), "复制技能 ID")}${jumpButton("skill", i.id, "查看全部阶级")}</div><h3>前置技能</h3><div class="relation-list">${(i.dependencies || []).map((d) => jumpButton("skill", d.id, `${d.id} · ${d.level} 级`)).join("") || '<p class="muted">无</p>'}</div>`;
    html +=
      '<h3>学习与关联装备</h3><div class="relation-list">' +
      (i.bookId
        ? itemButton({ id: i.bookId, name: "技能书", qty: 1 }) +
          jumpButton("items", i.bookId, "查看技能书")
        : '<p class="muted">技能书：无</p>') +
      (i.bindingArm
        ? itemButton({ id: i.bindingArm, name: "绑定装备", qty: 1 }) +
          jumpButton("items", i.bindingArm, "查看装备")
        : "") +
      "</div>";
  } else if (i.kind === "npc") {
    html = `<h2>${esc(i.name)}</h2><p class="description">${esc(i.mapName)} · 地图 ${i.mapId} · 坐标 ${i.x}, ${i.y}</p><p class="muted">${i.hidden ? "隐藏配置，游戏中不出现。" : ""}商店编号 ${i.shopId} · 传送菜单 ${i.transportId}</p><div class="detail-actions">${commandButton(`/goto ${i.mapId} ${i.x} ${i.y}`, "复制传送")}${commandButton(String(i.id), "复制地图内 NPC ID")}</div>`;
  } else {
    html = `<h2>${esc(i.name)}</h2><p class="description">${esc(i.description)}</p><p class="muted">等级 ${i.level}–${i.maxLevel || "不限"} · ${esc(i.npc)} · ${esc(i.mapName)}</p>${i.disabled ? `<p class="warning">已停用：${esc(i.disabledReason || "暂未启用")}</p>` : ""}<h3>奖励说明</h3><p class="description">${esc(i.reward || "无奖励说明")}</p><div class="detail-actions">${commandButton(String(i.id), "复制任务 ID")}</div>`;
  }
  $("detail-content").innerHTML =
    html +
    (i.kind === "skill"
      ? ""
      : '<div id="relations"><p class="loading">正在读取关联资料…</p></div>');
  if (!$("detail-dialog").open) $("detail-dialog").showModal();
  if (i.kind === "skill") return;
  try {
    const r = await fetchJSON(
      `/api/catalog/relations?kind=${i.kind}&id=${i.id}${i.mapId ? `&mapId=${i.mapId}` : ""}`,
    );
    if (version !== detailVersion || !$("detail-dialog").open) return;
    let content = "";
    const group = (title, body) =>
      body ? `<h3>${title}</h3><div class="relation-list">${body}</div>` : "";
    if (r.npcs)
      content += group(
        i.kind === "task" ? "发布 NPC / 所在地图" : "出售 NPC",
        r.npcs
          .map(
            (n) =>
              `<div class="relation-row"><span>${esc(n.name)} · ${esc(n.mapName)} ${n.hidden ? "（隐藏）" : ""}</span>${commandButton(`/goto ${n.mapId} ${n.x} ${n.y}`, "复制传送")}${jumpButton("npc", n.name, "查 NPC")}</div>`,
          )
          .join(""),
      );
    if (r.tasks)
      content += group(
        i.kind === "npc" ? "发布的任务" : "任务奖励来源",
        r.tasks
          .map((t) =>
            jumpButton(
              "task",
              t.id,
              `${t.name}${t.disabled ? "（停用）" : ""}`,
            ),
          )
          .join(""),
      );
    if (r.items)
      content += group(
        "商店物品",
        r.items
          .map(
            (t) =>
              `<div class="relation-row">${itemButton(t)}${jumpButton("items", t.id, "资料")}</div>`,
          )
          .join(""),
      );
    if (r.prerequisites)
      content += group(
        "前置任务",
        r.prerequisites.map((t) => jumpButton("task", t.id, t.name)).join(""),
      );
    if (r.rewards)
      content += group(
        "固定奖励物品",
        r.rewards.map((t) => itemButton(t)).join(""),
      );
    if (r.drops?.length)
      content += group(
        "历史掉落资料",
        r.drops.map((d) => `<span>${esc(d.name)} · ${d.id}</span>`).join(""),
      );
    if (r.steps)
      content += group(
        "任务流程",
        r.steps
          .map(
            (s) =>
              `<section class="quest-step"><h4>${s.number}. ${esc(s.npc || s.kind)}</h4><p class="description">${esc(s.text)}</p>${(s.kills || []).map((k) => `<p>击杀 ${esc(k.name)}（${k.id}）× ${k.qty}</p>`).join("")}${s.say ? `<p>对话：${esc(s.say)}</p>` : ""}${s.npc ? jumpButton("npc", s.npc, "查目标 NPC") : ""}<div class="relation-list">${s.items.map((t) => `<div><span>${{ collect: "收集", take: "交付", give: "获得" }[t.phase] || t.phase} · </span>${t.qty > 0 ? itemButton(t) : jumpButton("items", t.id, `${t.name}（按实际数量）`)}</div>`).join("")}</div></section>`,
          )
          .join(""),
      );
    $("relations").innerHTML = content;
  } catch (e) {
    if (version === detailVersion && $("relations"))
      $("relations").innerHTML =
        `<p class="warning">关联资料加载失败：${esc(e.message)}</p>`;
  }
}
function renderPagination(total, size) {
  const pages = Math.ceil(total / size);
  if (pages <= 1) {
    $("pagination").innerHTML = "";
    return;
  }
  const values = [
    ...new Set(
      [1, pages, state.page - 1, state.page, state.page + 1].filter(
        (n) => n > 0 && n <= pages,
      ),
    ),
  ].sort((a, b) => a - b);
  let last = 0;
  let html = `<button data-page="${state.page - 1}" ${state.page === 1 ? "disabled" : ""} aria-label="上一页">‹</button>`;
  for (const p of values) {
    if (last && p - last > 1) html += "<span>…</span>";
    html += `<button data-page="${p}" ${p === state.page ? 'class="active" aria-current="page"' : ""}>${p}</button>`;
    last = p;
  }
  html += `<button data-page="${state.page + 1}" ${state.page === pages ? "disabled" : ""} aria-label="下一页">›</button>`;
  $("pagination").innerHTML = html;
}
function navigate(section, q = "") {
  clearTimeout(searchTimer);
  $("detail-dialog").close();
  $("utility-dialog").close();
  detailVersion++;
  state = { ...defaults, section, q };
  syncControls();
  writeURL(true);
  loadItems();
}
function hideTooltip() {
  $("catalog-tooltip").hidden = true;
}
function showTooltip(card) {
  if ($("detail-dialog").open || matchMedia("(pointer: coarse)").matches)
    return;
  const template = card.querySelector("template");
  if (!template) return;
  const tip = $("catalog-tooltip");
  tip.innerHTML = template.innerHTML;
  tip.hidden = false;
  const r = card.getBoundingClientRect();
  tip.style.left = `${Math.max(12, Math.min(r.right + 10, innerWidth - tip.offsetWidth - 12))}px`;
  tip.style.top = `${Math.max(12, Math.min(r.top, innerHeight - tip.offsetHeight - 12))}px`;
}
function showHistory() {
  utility(
    "最近复制",
    recent.length
      ? `<div class="history-list">${recent.map((r) => `<div><p>${esc(r.label)}</p><code>${esc(r.text)}</code>${commandButton(r.text, "再次复制")}</div>`).join("")}</div><button id="clear-history" class="subtle">清空记录</button>`
      : '<p class="muted">暂无记录</p>',
  );
  if ($("clear-history"))
    $("clear-history").onclick = () => {
      recent = [];
      saveStorage("recent", recent);
      $("history-count").textContent = 0;
      showHistory();
    };
}
function showSaved() {
  utility(
    "收藏的筛选",
    saved.length
      ? `<div class="history-list">${saved.map((r, i) => `<div><button data-saved="${i}" class="subtle">${esc(r.label)}</button><button data-delete-saved="${i}" class="subtle">删除</button></div>`).join("")}</div>`
      : '<p class="muted">暂无收藏</p>',
  );
}

document.addEventListener("click", (e) => {
  const b = e.target.closest("button");
  if (!b) return;
  if (b.hasAttribute("data-close")) {
    b.closest("dialog").close();
    hideTooltip();
    return;
  }
  if (b.dataset.section) {
    navigate(b.dataset.section);
    return;
  }
  if (b.dataset.primary !== undefined) {
    primary(currentItems[Number(b.dataset.primary)]);
    return;
  }
  if (b.dataset.detail !== undefined) {
    showDetail(currentItems[Number(b.dataset.detail)]);
    return;
  }
  if (b.dataset.command !== undefined) {
    copy(b.dataset.command, b.textContent);
    return;
  }
  if (b.dataset.itemId) {
    try {
      copy(
        itemCommand(
          b.dataset.itemId,
          b.dataset.qty ? Number(b.dataset.qty) : undefined,
        ),
        b.textContent,
      );
    } catch (error) {
      toast(error.message);
    }
    return;
  }
  if (b.dataset.jump) {
    navigate(b.dataset.jump, b.dataset.query);
    return;
  }
  if (b.dataset.page) {
    clearTimeout(searchTimer);
    state.page = Number(b.dataset.page);
    writeURL(true);
    loadItems();
    return;
  }
  if (b.dataset.removeFilter) {
    delete state[b.dataset.removeFilter];
    if (b.dataset.removeFilter === "kind") {
      state.kind = "all";
      delete state.category;
    }
    if (b.dataset.removeFilter === "stat") delete state.minStat;
    state.page = 1;
    syncControls();
    loadItems();
    return;
  }
  if (b.dataset.saved !== undefined) {
    state = { ...defaults, ...saved[Number(b.dataset.saved)].state, page: 1 };
    $("utility-dialog").close();
    syncControls();
    writeURL(true);
    loadItems();
    return;
  }
  if (b.dataset.deleteSaved !== undefined) {
    saved.splice(Number(b.dataset.deleteSaved), 1);
    saveStorage("saved", saved);
    showSaved();
  }
});
$("toggle-filters").onclick = () => {
  const expanded = document
    .querySelector(".filters")
    .classList.toggle("expanded");
  $("toggle-filters").setAttribute("aria-expanded", String(expanded));
  $("toggle-filters").textContent = expanded ? "收起筛选" : "展开筛选";
};
$("filter-form").addEventListener("submit", (e) => {
  e.preventDefault();
  clearTimeout(searchTimer);
  readFilters();
  loadItems();
});
$("filter-form").addEventListener("change", (e) => {
  if (e.target.name === "kind") {
    readFilters();
    state.category = "";
    syncControls();
    loadItems();
  }
});
$("search").addEventListener("input", () => {
  clearTimeout(searchTimer);
  searchTimer = setTimeout(() => {
    readFilters();
    loadItems();
  }, 250);
});
$("sort").addEventListener("change", () => {
  readFilters();
  loadItems();
});
$("page-size").addEventListener("change", () => {
  state.pageSize = Number($("page-size").value);
  state.page = 1;
  loadItems();
});
const reset = () => navigate(state.section);
$("clear-filters").onclick = reset;
$("empty-reset").onclick = reset;
$("retry").onclick = loadItems;
$("view-toggle").onclick = () => {
  view = view === "list" ? "cards" : "list";
  saveStorage("view", view);
  $("item-grid").classList.toggle("list-view", view === "list");
  $("view-toggle").textContent = view === "list" ? "切换卡片" : "切换列表";
  $("view-toggle").setAttribute("aria-pressed", String(view === "cards"));
  renderItems();
};
$("show-history").onclick = showHistory;
$("saved-searches").onclick = showSaved;
$("save-search").onclick = () => {
  readFilters();
  const label = [
    { items: "物品", npc: "NPC", task: "任务", skill: "技能" }[state.section],
    state.q || "全部",
    ...Array.from(new FormData($("filter-form")))
      .filter(([, v]) => v && v !== "all")
      .map(
        ([k, v]) =>
          document.querySelector(`[name="${k}"]`)?.selectedOptions?.[0]
            ?.textContent || `${k}=${v}`,
      ),
  ].join(" · ");
  const entry = { label, state: { ...state, page: 1 } };
  saved = [
    entry,
    ...saved.filter(
      (s) => JSON.stringify(s.state) !== JSON.stringify(entry.state),
    ),
  ].slice(0, 20);
  saveStorage("saved", saved);
  toast("已收藏当前筛选");
  loadItems();
};
$("command-action").innerHTML = options(
  commandDefs.map((d) => [d[0], d[1]]),
  "addexp",
  null,
);
$("command-action").onchange = syncCommandForm;
$("command-fields").addEventListener("input", previewCommand);
$("command-form").addEventListener("submit", (e) => {
  e.preventDefault();
  try {
    copy(buildCommand(), $("command-action").selectedOptions[0].textContent);
  } catch (error) {
    toast(error.message);
  }
});
initCommandBook();
$("account-panel").addEventListener("toggle", () => {
  if ($("account-panel").open) loadAccounts();
});
$("account-search-form").addEventListener("submit", (e) => {
  e.preventDefault();
  loadAccounts();
});
$("accounts").addEventListener("submit", (e) => {
  e.preventDefault();
  reviewAccount(e.target);
});
$("item-grid").addEventListener("pointerover", (e) => {
  const card = e.target.closest(".item-card");
  if (card && !card.contains(e.relatedTarget)) showTooltip(card);
});
$("item-grid").addEventListener("pointerout", (e) => {
  const card = e.target.closest(".item-card");
  if (card && !card.contains(e.relatedTarget)) hideTooltip();
});
$("item-grid").addEventListener("focusin", (e) => {
  const card = e.target.closest(".item-card");
  if (card) showTooltip(card);
});
$("item-grid").addEventListener("focusout", hideTooltip);
addEventListener("scroll", hideTooltip, { capture: true, passive: true });
addEventListener("resize", hideTooltip);
document.addEventListener("keydown", (e) => {
  if (e.key === "Escape") hideTooltip();
  if ((e.metaKey || e.ctrlKey) && e.key.toLowerCase() === "k") {
    e.preventDefault();
    if (state.section === "gm") $("command-search").focus();
    else $("search").focus();
  }
});
addEventListener("popstate", () => {
  clearTimeout(searchTimer);
  readURL();
  syncControls();
  loadItems();
});
async function init() {
  readURL();
  syncControls();
  syncCommandForm();
  $("history-count").textContent = recent.length;
  const results = await Promise.allSettled([
    fetchJSON("/api/catalog/meta"),
    fetchJSON("assets/item-icons.json"),
  ]);
  if (results[0].status === "fulfilled") {
    meta = results[0].value;
    $("stats").innerHTML = [
      [meta.total, "物品 / 装备"],
      [meta.npcCount, "NPC 落点"],
      [meta.taskCount, "任务"],
      [meta.skillCount, "技能"],
    ]
      .map(
        ([n, l]) =>
          `<div><strong>${formatNumber(n)}</strong><span>${l}</span></div>`,
      )
      .join("");
  } else {
    $("stats").textContent = "资料统计暂不可用";
  }
  if (results[1].status === "fulfilled") {
    iconAtlas = results[1].value;
    iconIndex = new Map(iconAtlas.ids.map((id, index) => [Number(id), index]));
  }
  syncControls();
  loadItems();
}

function itemIcon(item, extraClass = "") {
  const index = iconIndex.get(Number(item.iconId));
  if (iconAtlas && index !== undefined) {
    const x = (index % iconAtlas.columns) * iconAtlas.cell;
    const y = Math.floor(index / iconAtlas.columns) * iconAtlas.cell;
    const style = `--icon-x:-${x}px;--icon-y:-${y}px;--atlas-w:${iconAtlas.width}px;--atlas-h:${iconAtlas.height}px`;
    return `<span class="item-icon ${extraClass}" style="${style}" aria-hidden="true"></span>`;
  }
  const glyph =
    item.kind === "equipment"
      ? "✦"
      : item.typeLabel === "宝石"
        ? "◆"
        : item.typeLabel === "卡片"
          ? "▧"
          : "◇";
  return `<span class="item-glyph ${extraClass}" aria-hidden="true">${glyph}</span>`;
}

function itemTooltip(item, equipment, level) {
  const attributes = [...(item.attributes || [])];
  if (item.level > 0) {
    attributes.unshift({
      label: equipment ? "需要等级" : "物品等级",
      value: level,
      kind: equipment ? "requirement" : "",
    });
  }
  const attributeSections = [
    [
      "requirement",
      "装备要求",
      attributes.filter((attribute) => attribute.kind === "requirement"),
    ],
    [
      "base",
      "基础属性",
      attributes.filter(
        (attribute) => !["requirement", "bonus"].includes(attribute.kind),
      ),
    ],
    [
      "bonus",
      "附加属性",
      attributes.filter((attribute) => attribute.kind === "bonus"),
    ],
  ]
    .filter(([, , rows]) => rows.length)
    .map(
      ([kind, title, rows]) => `
    <section class="tooltip-property-section ${kind}">
      <div class="tooltip-section-title">${title}</div>
      <div class="tooltip-attributes">${rows.map((attribute) => `<div class="tooltip-attribute ${escapeHTML(attribute.kind || "")}"><span>${escapeHTML(attribute.label)}</span><strong>${escapeHTML(attribute.value)}</strong></div>`).join("")}</div>
    </section>`,
    )
    .join("");
  const attributeContent = attributeSections;
  const description = item.description
    ? `<p class="tooltip-description">${escapeHTML(item.description).replace(/\n/g, "<br>")}</p>`
    : "";
  const stateTags = [
    [item.tradable, "可交易"],
    [item.buyable, "允许购买"],
    [item.droppable, "允许丢弃"],
    [item.stackable, "可堆叠"],
  ]
    .map(
      ([on, label]) =>
        `<span class="${on ? "on" : "off"}">${on ? "" : "不"}${label}</span>`,
    )
    .join("");
  return `<div class="tooltip-head">
      <div class="tooltip-icon-shell">${itemIcon(item, "tooltip-icon")}</div>
      <div><p>${escapeHTML(item.typeLabel)} · ID ${item.id}</p><h3>${escapeHTML(item.name)}</h3></div>
    </div>
    ${description}
    ${attributeContent}
    <div class="tooltip-economy">
      <span>重量 <strong>${formatNumber(item.weight)}</strong></span>
      <span>购买 <strong>${item.price > 0 ? formatCurrency(item.price) : "—"}</strong></span>
      <span>出售 <strong>${item.sellPrice > 0 ? formatCurrency(item.sellPrice) : "—"}</strong></span>
    </div>
    <div class="tooltip-tags">${stateTags}</div>`;
}

function primaryStat(item) {
  if (item.maxAttack || item.minAttack)
    return `物攻 ${item.minAttack}–${item.maxAttack}`;
  if (item.magicAttack) return `魔攻 ${formatNumber(item.magicAttack)}`;
  if (item.defense) return `防御 ${formatNumber(item.defense)}`;
  if (item.magicDefense) return `魔防 ${formatNumber(item.magicDefense)}`;
  if (item.durability) return `耐久 ${formatNumber(item.durability)}`;
  return "—";
}

function accentFor(code, equipment) {
  const palette = equipment
    ? ["#6ee7d5", "#73b9ff", "#b598ff", "#f2bf6d", "#e67ca7", "#8ed477"]
    : ["#6ee7d5", "#f2bf6d", "#b598ff", "#73b9ff"];
  return palette[Math.abs(Number(code)) % palette.length];
}

function escapeHTML(value) {
  return String(value).replace(
    /[&<>'"]/g,
    (char) =>
      ({ "&": "&amp;", "<": "&lt;", ">": "&gt;", "'": "&#39;", '"': "&quot;" })[
        char
      ],
  );
}
function formatNumber(value) {
  return new Intl.NumberFormat("zh-CN").format(value);
}
function formatCurrency(value) {
  const totalCopper = Math.max(0, Math.trunc(Number(value) || 0));
  const gold = Math.floor(totalCopper / 1_000_000);
  const silver = Math.floor((totalCopper % 1_000_000) / 1_000);
  const copper = totalCopper % 1_000;
  const parts = [];
  if (gold) parts.push(`${formatNumber(gold)}金币`);
  if (silver) parts.push(`${formatNumber(silver)}银币`);
  if (copper || !parts.length) parts.push(`${formatNumber(copper)}铜币`);
  return parts.join(" ");
}
function compactNumber(value) {
  return new Intl.NumberFormat("zh-CN", {
    notation: "compact",
    maximumFractionDigits: 1,
  }).format(value);
}

init();
