// GM command builder and local account administration. Shared UI helpers live in app.js.
const commandDefs = [
  ["gm help", "GM 帮助", "none", 0, 1],
  ["addexp", "增加经验", "amount", 1e9, 1],
  ["addmoney", "增加铜币", "amount", 1e12, 1],
  ["addgold", "增加金币", "amount", 1e12, 1],
  ["addsilver", "增加银币", "amount", 1e12, 1],
  ["addhonor", "增加名誉", "amount", 1e12, 1],
  ["takemoney", "扣除铜币", "amount", 1e12, 1],
  ["takegold", "扣除金币", "amount", 1e12, 1],
  ["takesilver", "扣除银币", "amount", 1e12, 1],
  ["takehonor", "扣除名誉", "amount", 1e12, 1],
  ["item", "发放物品（自己）", "item", 10000, 1],
  ["takeitem", "扣除物品（自己）", "item", 10000, 1],
  ["giveitem", "发放物品给角色", "target-item", 10000, 2],
  ["givemoney", "增加角色铜币", "target-amount", 1e12, 2],
  ["goto", "传送到地图 / 坐标", "map", 0, 1],
  ["god", "无敌状态", "state", 0, 1],
  ["oneshot", "一击击杀状态", "state", 0, 1],
  ["inspect", "查看在线角色", "target", 0, 2],
  ["mute", "禁言在线角色", "target-duration", 10080, 2],
  ["unmute", "解除禁言", "target", 0, 2],
  ["kick", "踢出角色", "target", 0, 3],
  ["ban", "封禁角色所属账号", "target-ban", 525600, 4],
  ["unban", "解封角色所属账号", "target", 0, 4],
  ["announce", "全服公告", "text", 0, 2],
  ["max", "补满生命 / 法力", "none", 0, 1],
  ["where", "查看当前位置", "none", 0, 1],
  ["online", "查看在线角色", "none", 0, 1],
];
const commandInput = (
  key,
  label,
  type = "text",
  min = "",
  max = "",
  value = "",
) =>
  `<label class="field"><span>${label}</span><input name="${key}" type="${type}" ${min !== "" ? `min="${min}"` : ""} ${max !== "" ? `max="${max}"` : ""} value="${esc(value)}" ${type === "number" ? 'step="1"' : 'maxlength="200"'}></label>`;
function syncCommandForm() {
  const def = commandDefs.find((d) => d[0] === $("command-action").value),
    type = def[2];
  let html = "";
  if (type.includes("target")) html += commandInput("target", "目标角色");
  if (type.includes("item"))
    html +=
      commandInput("id", "物品 ID", "number", 1, 2147483647) +
      commandInput("count", "数量", "number", 1, 10000, 1);
  if (type.includes("amount"))
    html += commandInput("amount", "数值", "number", 1, def[3], 100);
  if (type.includes("duration") || type.includes("ban"))
    html += commandInput(
      "duration",
      type.includes("ban") ? "封禁分钟（0 = 永久）" : "禁言分钟",
      "number",
      type.includes("ban") ? 0 : 1,
      def[3],
      10,
    );
  if (type === "text") html += commandInput("text", "公告内容");
  if (type === "state")
    html +=
      '<label class="field"><span>状态</span><select name="state"><option value="status">查询状态</option><option value="on">开启</option><option value="off">关闭</option></select></label>';
  if (type === "map")
    html +=
      commandInput("map", "地图 ID", "number", 1, 2147483647) +
      '<div class="filter-pair">' +
      commandInput("x", "X（可选）", "number", 0, 2147483647) +
      commandInput("y", "Y（可选）", "number", 0, 2147483647) +
      '</div><p class="muted">X、Y 同填或留空。</p>';
  $("command-fields").innerHTML = html;
  $("command-permission").textContent = `权限：${gmLevels[def[4]]}`;
  previewCommand();
  markBookCommand();
}
function buildCommand() {
  const def = commandDefs.find((d) => d[0] === $("command-action").value),
    type = def[2],
    f = Object.fromEntries(new FormData($("command-form")));
  let args = [];
  const text = (key) => {
    const value = (f[key] || "").trim();
    if (!value || /[\r\n]/.test(value))
      throw new Error("请填写完整参数，文本不能包含换行");
    return value;
  };
  const num = (key, min, max) => {
    const raw = f[key];
    const value = Number(raw);
    if (
      raw === "" ||
      !Number.isSafeInteger(value) ||
      value < min ||
      value > max
    )
      throw new Error(`请输入 ${min}–${max} 的整数`);
    return String(value);
  };
  if (type.includes("target")) args.push(text("target"));
  if (type.includes("item"))
    args.push(num("id", 1, 2147483647), num("count", 1, 10000));
  if (type.includes("amount")) args.push(num("amount", 1, def[3]));
  if (type.includes("duration") || type.includes("ban")) {
    const d = num("duration", type.includes("ban") ? 0 : 1, def[3]);
    if (d !== "0") args.push(d);
  }
  if (type === "text") {
    const message = text("text");
    if (new TextEncoder().encode(message).length > 300)
      throw new Error("公告内容最多 300 字节");
    args.push(message);
  }
  if (type === "state") args.push(f.state);
  if (type === "map") {
    args.push(num("map", 1, 2147483647));
    if (f.x !== "" || f.y !== "")
      args.push(num("x", 0, 2147483647), num("y", 0, 2147483647));
  }
  const command = "/" + def[0] + (args.length ? " " + args.join(" ") : "");
  if (new TextEncoder().encode(command).length > 512)
    throw new Error("命令超出 512 字节，请缩短文本");
  return command;
}
function previewCommand() {
  try {
    $("command-preview").textContent = buildCommand();
    $("command-preview").classList.remove("invalid");
  } catch (e) {
    $("command-preview").textContent = e.message;
    $("command-preview").classList.add("invalid");
  }
}
let accounts = [],
  accountVersion = 0;
async function loadAccounts() {
  const version = ++accountVersion;
  $("account-message").textContent = "正在查询账号…";
  $("accounts").innerHTML = "";
  try {
    const data = await fetchJSON(
      `/api/gm/accounts?q=${encodeURIComponent($("account-search").value.trim())}`,
    );
    if (version !== accountVersion) return;
    accounts = data.accounts;
    $("account-message").textContent =
      accounts.length >= 100
        ? "仅显示前 100 个账号"
        : accounts.length
          ? ""
          : "没有找到账号";
    $("accounts").innerHTML = accounts
      .map(
        (a, index) =>
          `<form class="account-row" data-account="${index}"><div><strong>${esc(a.username)} <small>#${a.id}</small></strong><p>${a.characters.map((c) => `${esc(c.name)} Lv.${c.level}`).join(" · ") || "尚无角色"}${a.banned ? " · 已封禁" : ""}</p><span class="account-level">当前：${gmLevels[a.gmLevel]}</span></div><div class="account-controls"><select name="level" aria-label="${esc(a.username)} 的 GM 权限">${options(
            gmLevels.map((l, i) => [i, l]),
            a.gmLevel,
            null,
          )}</select><button class="subtle" type="submit">保存</button></div></form>`,
      )
      .join("");
  } catch (e) {
    if (version === accountVersion)
      $("account-message").textContent = e.message;
  }
}
function reviewAccount(form) {
  const a = accounts[Number(form.dataset.account)],
    level = Number(new FormData(form).get("level"));
  if (level === a.gmLevel) {
    toast("权限未变化");
    return;
  }
  utility(
    "确认账号权限",
    `<p>账号 <strong>${esc(a.username)}（#${a.id}）</strong></p><p>${gmLevels[a.gmLevel]} → <strong>${gmLevels[level]}</strong></p><p class="muted">保存后需退出账号重新登录。</p><button id="confirm-account" class="apply-button">确认保存权限</button><p id="save-account-status" role="status"></p>`,
  );
  $("confirm-account").onclick = async () => {
    const button = $("confirm-account");
    button.disabled = true;
    try {
      await fetchJSON("/api/gm/accounts", {
        method: "POST",
        headers: {
          "Content-Type": "application/json",
          "X-GM-Request": "account-level",
        },
        body: JSON.stringify({
          id: a.id,
          gmLevel: level,
          expectedLevel: a.gmLevel,
        }),
      });
      $("save-account-status").textContent = "已保存，请退出账号重新登录。";
      button.textContent = "已保存";
      await loadAccounts();
    } catch (e) {
      $("save-account-status").textContent = e.message;
      button.disabled = false;
    }
  };
}

// Usage and aliases match the whitelist in internal/session/gm.go. Parameter
// limits and permissions come from commandDefs, shared with the builder.
const commandChapters = {
  query: { name: "查询", commands: ["gm help", "where", "online", "inspect"] },
  values: {
    name: "经验与货币",
    commands: [
      "addexp",
      "addmoney",
      "addgold",
      "addsilver",
      "addhonor",
      "takemoney",
      "takegold",
      "takesilver",
      "takehonor",
      "givemoney",
    ],
  },
  items: { name: "物品", commands: ["item", "takeitem", "giveitem"] },
  state: { name: "传送与状态", commands: ["goto", "max", "god", "oneshot"] },
  players: {
    name: "玩家管理",
    commands: ["mute", "unmute", "kick", "ban", "unban", "announce"],
  },
};
const commandUsage = {
  "gm help": ["", "显示服务端指令列表。", ["/help", "/gmhelp"]],
  addexp: ["<经验>", "增加自己的经验。"],
  addmoney: ["<铜币>", "增加自己的铜币。"],
  addgold: ["<金币>", "增加自己的金币。"],
  addsilver: ["<银币>", "增加自己的银币。"],
  addhonor: ["<名誉>", "增加自己的名誉。"],
  takemoney: ["<铜币>", "扣除自己的铜币；余额不足时不执行。"],
  takegold: ["<金币>", "扣除自己的金币；余额不足时不执行。"],
  takesilver: ["<银币>", "扣除自己的银币；余额不足时不执行。"],
  takehonor: ["<名誉>", "扣除自己的名誉；余额不足时不执行。"],
  item: ["<物品ID> [数量]", "发放到自己的背包。"],
  takeitem: ["<物品ID> [数量]", "从自己的背包扣除；数量不足时不执行。"],
  giveitem: ["<角色名> <物品ID> [数量]", "发放给在线角色。"],
  givemoney: ["<角色名> <铜币>", "给在线角色增加铜币。"],
  goto: [
    "<地图ID> [X Y]",
    "坐标留空使用安全落点；填写时 X、Y 必须同时提供。",
    ["/teleport"],
  ],
  max: ["", "补满生命与法力；死亡时复活。"],
  god: ["[on|off|status]", "无敌开关；默认查询。切图保留，退出登录清除。"],
  oneshot: [
    "[on|off|status]",
    "一击击杀开关；默认查询。切图保留，退出登录清除。",
  ],
  where: ["", "显示当前地图、坐标和场景实例。"],
  online: ["", "显示在线人数与角色列表。"],
  inspect: ["<角色名>", "查看在线角色的等级、资源、背包和位置。"],
  mute: ["<角色名> <分钟>", "禁言在线角色。"],
  unmute: ["<角色名>", "解除在线角色的禁言。"],
  kick: ["<角色名>", "断开在线角色的连接。"],
  ban: [
    "<角色名> [分钟]",
    "封禁角色所属账号；省略分钟为永久封禁。定时封禁要求角色在线。",
  ],
  unban: ["<角色名>", "解除角色所属账号的封禁。"],
  announce: ["<内容>", "向全服在线角色发送公告。"],
};
let commandChapter = "all";

function commandLimit(def) {
  const type = def[2];
  if (type.includes("item")) return "数量 1–10,000，默认 1";
  if (type.includes("amount")) return `数值 1–${formatNumber(def[3])}`;
  if (type === "target-duration" || type === "target-ban")
    return `分钟 1–${formatNumber(def[3])}`;
  if (type === "text") return "内容最多 300 字节";
  return "";
}
function renderCommandBook() {
  const query = $("command-search").value.trim().toLowerCase();
  const matches = commandDefs.filter((def) => {
    const usage = commandUsage[def[0]];
    return [def[0], def[1], ...usage].join(" ").toLowerCase().includes(query);
  });
  const chapters = [
    ["all", "全部", matches.length],
    ...Object.entries(commandChapters).map(([key, chapter]) => [
      key,
      chapter.name,
      matches.filter((def) => chapter.commands.includes(def[0])).length,
    ]),
  ];
  $("command-categories").innerHTML = chapters
    .map(
      ([key, name, count]) =>
        `<button type="button" data-chapter="${key}" aria-pressed="${key === commandChapter}">${name}<span>${count}</span></button>`,
    )
    .join("");
  const selected =
    commandChapter === "all"
      ? matches
      : matches.filter((def) =>
          commandChapters[commandChapter].commands.includes(def[0]),
        );
  $("command-count").textContent = `${selected.length} / ${commandDefs.length}`;
  $("command-entries").innerHTML = selected.length
    ? Object.values(commandChapters)
        .map((chapter) => {
          const rows = selected.filter((def) =>
            chapter.commands.includes(def[0]),
          );
          if (!rows.length) return "";
          return `<section class="command-chapter"><h3>${chapter.name}</h3>${rows
            .map((def) => {
              const [name, label, type, , level] = def;
              const [args, note, aliases = []] = commandUsage[name];
              const syntax = `/${name}${args ? " " + args : ""}`;
              const buttons =
                type === "none"
                  ? commandButton(syntax, "复制")
                  : type === "state"
                    ? [
                        ["on", "开启"],
                        ["off", "关闭"],
                        ["status", "状态"],
                      ]
                        .map(([value, title]) =>
                          commandButton(`/${name} ${value}`, title),
                        )
                        .join("")
                    : `${commandButton(syntax, "复制格式")}<button type="button" class="subtle" data-book-command="${name}">填参数</button>`;
              return `<article class="command-entry" data-entry-command="${name}">
        <div class="command-entry-heading"><button type="button" class="command-name" data-book-command="${name}">${esc(label)}</button><span class="command-role">${["", "APP", "WIZARD", "ARCH", "ADMIN"][level]}</span></div>
        <code>${esc(syntax)}</code>
        <div class="command-entry-actions">${buttons}</div>
        <details class="command-reference"><summary>参数说明</summary><p>${esc(note)}</p>${commandLimit(def) ? `<p>${esc(commandLimit(def))}</p>` : ""}${aliases.length ? `<p>别名：${aliases.map(esc).join("、")}</p>` : ""}</details>
      </article>`;
            })
            .join("")}</section>`;
        })
        .join("")
    : '<p class="muted">没有匹配的指令</p>';
  markBookCommand();
}
function markBookCommand() {
  const name = $("command-action").value;
  document.querySelectorAll("[data-entry-command]").forEach((entry) => {
    const active = entry.dataset.entryCommand === name;
    entry.classList.toggle("active", active);
    entry
      .querySelector(".command-name")
      .setAttribute("aria-pressed", String(active));
  });
}
function initCommandBook() {
  $("command-search").addEventListener("input", renderCommandBook);
  $("command-categories").addEventListener("click", (event) => {
    const button = event.target.closest("[data-chapter]");
    if (!button) return;
    commandChapter = button.dataset.chapter;
    renderCommandBook();
    $("command-entries").scrollTop = 0;
  });
  $("command-entries").addEventListener("click", (event) => {
    const button = event.target.closest("[data-book-command]");
    if (!button) return;
    $("command-action").value = button.dataset.bookCommand;
    syncCommandForm();
    if (matchMedia("(max-width: 980px)").matches)
      $("command-editor").scrollIntoView({ block: "start" });
    (
      $("command-fields").querySelector("input, select") ||
      $("command-form").querySelector('[type="submit"]')
    ).focus({ preventScroll: true });
  });
  $("book-history").onclick = showHistory;
  renderCommandBook();
}
