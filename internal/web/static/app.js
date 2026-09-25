const app = document.getElementById("app");
const S = { me: null, group: null, role: "", code: null, tab: "markets", es: null, esCode: null, members: [], coins: null };
const CAMPUS_EXAMPLES = [
  "Wildcats cover the spread this Saturday",
  "Dillo Day headliner is announced before March 1",
  "It snows in Evanston before Halloween",
  "ASG's dining referendum passes",
  "Norris Starbucks line is over 20 people at 10am Monday",
  "The Lakefill freezes enough to walk on by February",
];
const campus = () => S.group && S.group.kind === "campus";

const EXAMPLES = [
  "Does Jake text her back by Friday?",
  "Priya gets the Stripe offer",
  "Marcus orders DoorDash 5+ times this week",
  "Sam actually shows up to the gym tomorrow",
  "Chris and Taylor are official by Halloween",
  "Alex gets cooked in the group chat for this",
  "Nick finishes the half marathon",
  "Dev ships his side project before the semester ends",
];
const BUY_LINES = {
  yes: ["believes", "is all in on YES", "is feeling it", "smells a W"],
  no: ["is fading it", "doesn't believe", "is betting against the homie", "smells an L"],
};

const esc = (s) => String(s ?? "").replace(/[&<>"']/g, (c) => ({ "&": "&amp;", "<": "&lt;", ">": "&gt;", '"': "&quot;", "'": "&#39;" }[c]));
const pct = (p) => Math.round(p * 100);
const fmt = (n) => (Math.abs(n) >= 10000 ? (n / 1000).toFixed(1) + "k" : Math.round(n).toLocaleString());
const pick = (a) => a[Math.floor(Math.random() * a.length)];
const ago = (iso) => {
  const s = Math.max(0, (Date.now() - new Date(iso)) / 1000);
  if (s < 60) return "just now";
  if (s < 3600) return Math.floor(s / 60) + "m ago";
  if (s < 86400) return Math.floor(s / 3600) + "h ago";
  return Math.floor(s / 86400) + "d ago";
};
const until = (iso) => {
  const s = (new Date(iso) - Date.now()) / 1000;
  if (s <= 0) return "closed";
  if (s < 3600) return Math.ceil(s / 60) + "m left";
  if (s < 86400) return Math.ceil(s / 3600) + "h left";
  return Math.ceil(s / 86400) + "d left";
};

async function api(method, path, body) {
  const r = await fetch(path, {
    method,
    headers: body ? { "Content-Type": "application/json" } : {},
    body: body ? JSON.stringify(body) : undefined,
  });
  const data = await r.json().catch(() => ({}));
  if (!r.ok) throw Object.assign(new Error(data.error || "something broke"), { status: r.status });
  return data;
}

function toast(msg, bad) {
  const t = document.createElement("div");
  t.className = "toast" + (bad ? " bad" : "");
  t.textContent = msg;
  document.getElementById("toasts").append(t);
  setTimeout(() => t.remove(), 3200);
  if (navigator.vibrate && !bad) navigator.vibrate(12);
}

function go(path) {
  history.pushState({}, "", path);
  route();
}
document.addEventListener("click", (e) => {
  const a = e.target.closest("a[data-link]");
  if (a) { e.preventDefault(); go(a.getAttribute("href")); }
});
window.addEventListener("popstate", route);

async function loadMe() {
  if (S.me) return S.me;
  try { S.me = await api("GET", "/api/me"); } catch { S.me = null; }
  if (S.me && !S.me.id) S.me = null;
  return S.me;
}

async function ensureName(prompt) {
  if (await loadMe()) return S.me;
  return new Promise((resolve) => {
    sheet(`<h2>${esc(prompt || "What do your friends call you?")}</h2>
      <p class="muted small">This is how you show up on the leaderboard.</p>
      <input class="field" id="nm" maxlength="24" placeholder="Your name" autocomplete="nickname">
      <div style="height:12px"></div><button class="btn" id="nmgo">Let's go</button>`, (el, close) => {
      const input = el.querySelector("#nm");
      input.focus();
      const submit = async () => {
        try { S.me = await api("POST", "/api/me", { name: input.value }); close(); resolve(S.me); }
        catch (e) { toast(e.message, true); }
      };
      el.querySelector("#nmgo").onclick = submit;
      input.onkeydown = (e) => e.key === "Enter" && submit();
    }, () => resolve(null));
  });
}

function sheet(html, onOpen, onDismiss) {
  const bg = document.createElement("div");
  bg.className = "sheet-bg";
  const el = document.createElement("div");
  el.className = "sheet";
  el.innerHTML = html;
  const close = () => { bg.remove(); el.remove(); };
  bg.onclick = () => { close(); onDismiss && onDismiss(); };
  document.body.append(bg, el);
  onOpen && onOpen(el, close);
  return close;
}

function home() {
  disconnect();
  document.title = "sidebet: put odds on your friends";
  app.innerHTML = `<div class="wrap">
    <div class="hero">
      <div class="brand" style="font-size:1.4rem">sidebet</div>
      <h1>Put odds on your <em>friends.</em></h1>
      <p>Prediction markets for your group chat. Bet play money on each other's lives, watch the odds move live, and find out who really believes in you.</p>
      <button class="btn" id="start">Start a group</button>
      <a class="btn ghost" data-link href="/g/nu" style="margin-top:10px">At Northwestern? Campus markets →</a>
      <div class="ticker">
        <div class="row"><span>Jake texts her back by Friday</span><b class="no">18%</b></div>
        <div class="row"><span>Priya gets the Stripe offer</span><b class="yes">74%</b></div>
        <div class="row"><span>Marcus orders DoorDash 5+ times this week</span><b class="yes">91%</b></div>
      </div>
      <p class="small muted">Play money only. No cash, no crypto, no parlays with your rent. Just vibes and public humiliation.</p>
    </div></div>`;
  document.getElementById("start").onclick = async () => {
    if (!(await ensureName())) return;
    sheet(`<h2>Name the group</h2><p class="muted small">Same energy as the group chat name.</p>
      <input class="field" id="gn" maxlength="40" placeholder="the boys, apt 4b, cs 211 survivors…">
      <div style="height:12px"></div><button class="btn" id="gngo">Create</button>`, (el, close) => {
      const input = el.querySelector("#gn");
      input.focus();
      const submit = async () => {
        try { const g = await api("POST", "/api/groups", { name: input.value }); close(); go(`/g/${g.code}?new=1`); }
        catch (e) { toast(e.message, true); }
      };
      el.querySelector("#gngo").onclick = submit;
      input.onkeydown = (e) => e.key === "Enter" && submit();
    });
  };
}

function connect(code) {
  if (S.esCode === code && S.es) return;
  disconnect();
  S.es = new EventSource(`/api/groups/${code}/events`);
  S.esCode = code;
  S.es.onmessage = (e) => onEvent(JSON.parse(e.data));
}
function disconnect() {
  if (S.es) S.es.close();
  S.es = null;
  S.esCode = null;
}

function onEvent(ev) {
  const mine = S.me && ev.data && ev.data.trade && ev.data.trade.user_id === S.me.id;
  if (ev.type === "trade" && !mine) {
    const t = ev.data.trade;
    const side = t.side.endsWith("yes") ? "yes" : "no";
    const q = ev.data.market.question;
    const who = t.insider ? `🚨 INSIDER: ${t.user}` : t.user;
    if (t.side.startsWith("buy")) toast(`${who} ${pick(BUY_LINES[side])}: ${fmt(t.coins)} on ${side.toUpperCase()} · "${q}" → ${pct(ev.data.market.chance)}%`);
    else toast(`${who} cashed out of "${q}"`);
  } else if (ev.type === "market" && ev.data.creator_id !== S.me?.id) {
    toast(`New bet dropped: "${ev.data.question}"`);
  } else if (ev.type === "resolve") {
    toast(`Settled: "${ev.data.question}" → ${ev.data.outcome.toUpperCase()}`);
  } else if (ev.type === "member") {
    toast(`${ev.data.name} joined. Fresh meat.`);
  } else if (ev.type === "bailout" && ev.data.user.id !== S.me?.id) {
    toast(`${ev.data.user.name} took a bailout 💀`);
  }
  refreshView(ev);
}

let refreshTimer;
function refreshView(ev) {
  clearTimeout(refreshTimer);
  refreshTimer = setTimeout(() => {
    const p = location.pathname;
    if (/^\/g\/[^/]+\/m\/\d+$/.test(p)) renderMarket(true);
    else if (/^\/g\/[^/]+$/.test(p)) renderGroup(true, ev);
  }, 120);
}

async function groupPage(code) {
  S.code = code;
  let p;
  try { p = await api("GET", `/api/groups/${code}`); }
  catch { app.innerHTML = `<div class="wrap"><div class="empty"><b>That group doesn't exist.</b>Maybe the link got cooked.</div><a class="btn ghost" data-link href="/">Go home</a></div>`; return; }
  S.group = p.group;
  S.role = p.role || "";
  document.title = `${p.group.name} · sidebet`;
  if (!p.member) return p.group.kind === "campus" ? campusJoin(p) : joinScreen(p);
  connect(code);
  await renderGroup();
  if (new URLSearchParams(location.search).get("new")) {
    history.replaceState({}, "", `/g/${code}`);
    invite();
  }
}

function joinScreen(p) {
  const h = p.hottest;
  app.innerHTML = `<div class="wrap"><div class="hero">
    <div class="brand" style="font-size:1.2rem">sidebet</div>
    <h1>${esc(p.group.name)}</h1>
    <p>${p.members} ${p.members === 1 ? "friend is" : "friends are"} putting odds on each other. Get in before they bet on you.</p>
    ${h ? `<div class="mk"><div class="q">${esc(h.question)}</div><div class="meta"><span class="muted small">${h.traders} betting</span>
      <span class="pct ${h.chance >= 0.5 ? "yes" : "no"}">${pct(h.chance)}%</span></div></div>` : ""}
    <button class="btn" id="join">I'm in</button>
    <p class="small muted" style="margin-top:14px">You get 1,000 coins. Play money. The shame is real.</p>
  </div></div>`;
  document.getElementById("join").onclick = async () => {
    if (!(await ensureName("What does the group call you?"))) return;
    try { await api("POST", `/api/groups/${p.group.code}/join`); groupPage(p.group.code); }
    catch (e) { toast(e.message, true); }
  };
}

function myCoins(board) {
  return board.me ? board.me.coins : 0;
}

function campusJoin(p) {
  const top = p.top || [];
  const domain = (p.group.domains || [])[0] || "school";
  app.innerHTML = `<div class="wrap"><div class="hero">
    <div class="brand" style="font-size:1.2rem">sidebet · campus</div>
    <h1>Kalshi for <em>${esc(p.group.name)}.</em></h1>
    <p>${p.members.toLocaleString()} ${p.members === 1 ? "student is" : "students are"} betting play money on what happens on campus. Odds move live. Winners run the leaderboard.</p>
    <div class="ticker">${top.length ? top.map((m) => `<div class="row"><span>${esc(m.question)}</span><b class="${m.chance >= 0.5 ? "yes" : "no"}">${pct(m.chance)}%</b></div>`).join("")
      : `<div class="row"><span>First markets drop soon</span><b class="yes">—</b></div>`}</div>
    <button class="btn" id="verify">Get in with your @${esc(domain)} email</button>
    <p class="small muted" style="margin-top:14px">Verified students only. 1,000 coins to start. Play money, no cash. Bets are about events, never about individual people.</p>
  </div></div>`;
  document.getElementById("verify").onclick = () => verifySheet(p.group);
}

function verifySheet(group) {
  const domain = (group.domains || [])[0] || "school";
  let email = "";
  sheet(`<h2>Verify you're a student</h2><p class="muted small">We'll email a 6-digit code to your school address. It's never shown to anyone.</p>
    <div id="step1"><input class="field" id="em" type="email" inputmode="email" autocomplete="email" placeholder="you@${esc(domain)}">
    <div style="height:12px"></div><button class="btn" id="send">Send code</button></div>
    <div id="step2" style="display:none"><input class="field" id="cd" inputmode="numeric" autocomplete="one-time-code" maxlength="6" placeholder="6-digit code">
    <div id="namewrap" style="margin-top:10px"><input class="field" id="nm" maxlength="24" placeholder="Name on the leaderboard" autocomplete="nickname"></div>
    <div style="height:12px"></div><button class="btn" id="finish">Verify and join</button><p class="small muted" id="hint"></p></div>`, (el, close) => {
    const em = el.querySelector("#em");
    em.focus();
    if (S.me) el.querySelector("#namewrap").style.display = "none";
    el.querySelector("#send").onclick = async () => {
      try {
        const r = await api("POST", "/api/verify/start", { email: em.value, group: group.code });
        email = em.value;
        el.querySelector("#step1").style.display = "none";
        el.querySelector("#step2").style.display = "block";
        el.querySelector("#hint").textContent = r.dev_code ? `Dev mode code: ${r.dev_code}` : `Sent to ${email}. Check spam if it's not there in a minute.`;
        el.querySelector("#cd").focus();
      } catch (e) { toast(e.message, true); }
    };
    el.querySelector("#finish").onclick = async () => {
      try {
        const r = await api("POST", "/api/verify/finish", { email, code: el.querySelector("#cd").value, name: el.querySelector("#nm").value });
        S.me = r.user;
        await api("POST", `/api/groups/${group.code}/join`);
        close();
        toast(r.returning ? "Welcome back." : "You're in. 1,000 coins. Don't fumble them.");
        groupPage(group.code);
      } catch (e) { toast(e.message, true); }
    };
  });
}

async function renderGroup(soft, ev) {
  const code = S.code;
  const [markets, board] = await Promise.all([api("GET", `/api/groups/${code}/markets`), api("GET", `/api/groups/${code}/leaderboard`)]);
  S.members = board.top;
  S.board = board;
  S.coins = myCoins(board);
  const flashId = ev && ev.data && (ev.data.market?.id || ev.data.id);
  let body = "";
  if (S.tab === "markets") body = marketsHTML(markets, flashId);
  else if (S.tab === "board") body = boardHTML(board);
  else if (S.tab === "mod") body = `<div class="panel list" id="mod"><div class="muted small">Loading…</div></div>`;
  else body = `<div class="panel feed list" id="feed"><div class="muted small">Loading…</div></div>`;
  const scroll = window.scrollY;
  app.innerHTML = `<div class="wrap">
    <div class="top"><div style="min-width:0"><a class="back small" data-link href="/">sidebet</a><h1>${esc(S.group.name)}</h1></div>
      <span class="coins"><i>●</i>${fmt(S.coins)}</span></div>
    ${S.coins < 50 ? `<button class="btn ghost" id="bail" style="margin-bottom:12px">Broke? Take the daily bailout (+100)</button>` : ""}
    <div class="tabs">
      <button data-tab="markets" class="${S.tab === "markets" ? "on" : ""}">Bets</button>
      <button data-tab="board" class="${S.tab === "board" ? "on" : ""}">Leaderboard</button>
      <button data-tab="feed" class="${S.tab === "feed" ? "on" : ""}"><span class="live-dot"></span>Live</button>
      ${S.role === "admin" ? `<button data-tab="mod" class="${S.tab === "mod" ? "on" : ""}">Mod</button>` : ""}
    </div>
    ${body}
    <div style="display:flex;gap:10px;margin-top:16px"><button class="btn ghost" id="inv">${campus() ? "Send to a friend on campus" : "Invite friends"}</button></div>
  </div>
  <button class="btn fab" id="new">+ New bet</button>`;
  if (soft) window.scrollTo(0, scroll);
  app.querySelectorAll("[data-tab]").forEach((b) => (b.onclick = () => { S.tab = b.dataset.tab; renderGroup(); }));
  document.getElementById("new").onclick = newMarket;
  document.getElementById("inv").onclick = invite;
  const bail = document.getElementById("bail");
  if (bail) bail.onclick = async () => {
    try { await api("POST", `/api/groups/${code}/bailout`); toast("Bailout secured. Don't blow it."); renderGroup(true); }
    catch (e) { toast(e.message, true); }
  };
  if (S.tab === "feed") loadFeed();
  if (S.tab === "mod") loadMod();
}

async function loadMod() {
  const el = document.getElementById("mod");
  try {
    const list = await api("GET", `/api/groups/${S.code}/reports`);
    el.innerHTML = list.length ? list.map((r) => `<div class="it"><div class="who"><a data-link href="/g/${S.code}/m/${r.market.id}"><b>${esc(r.market.question)}</b></a>
      <div class="muted small">${r.reports} report${r.reports === 1 ? "" : "s"}: ${esc(r.reasons.join(" · "))}</div></div>
      <button class="btn ghost" style="width:auto;padding:8px 12px" data-void="${r.market.id}">Void</button></div>`).join("")
      : `<div class="empty"><b>Queue is clean.</b>Reported markets show up here.</div>`;
    el.querySelectorAll("[data-void]").forEach((b) => (b.onclick = async () => {
      if (!confirm("Void this market and refund everyone?")) return;
      try { await api("POST", `/api/groups/${S.code}/markets/${b.dataset.void}/resolve`, { outcome: "void" }); toast("Voided."); loadMod(); }
      catch (e) { toast(e.message, true); }
    }));
  } catch (e) { el.innerHTML = `<div class="empty">${esc(e.message)}</div>`; }
}

function marketsHTML(list, flashId) {
  if (!list.length) return `<div class="empty"><b>No bets yet.</b>Someone has to start the chaos. Might as well be you.</div>`;
  return list.map((m) => {
    const open = m.status === "open" && new Date(m.closes_at) > Date.now();
    const pos = m.mine && (m.mine.yes > 0.01 || m.mine.no > 0.01) ? `<span class="chip pos">you: ${m.mine.yes > m.mine.no ? "YES" : "NO"} · ${fmt(m.mine.value)}</span>` : "";
    const status = m.status === "resolved" ? `<span class="chip">${m.outcome.toUpperCase()}</span>` : m.status === "void" ? `<span class="chip">void</span>` : `<span class="chip">${until(m.closes_at)}</span>`;
    const pctEl = m.status === "resolved" ? `<span class="pct ${m.outcome}">${m.outcome.toUpperCase()}</span>`
      : `<span class="pct ${m.chance >= 0.5 ? "yes" : "no"}">${pct(m.chance)}%<small>chance</small></span>`;
    return `<a class="mk ${flashId === m.id ? "flash" : ""}" data-link href="/g/${S.code}/m/${m.id}" style="${open ? "" : "opacity:.6"}">
      <div class="q">${esc(m.question)}</div>
      <div class="chips">${m.subject ? `<span class="chip subj">about ${esc(m.subject)}</span>` : ""}${status}${pos}</div>
      <div class="meta"><span class="muted small">${m.traders} betting · ${fmt(m.volume)} in</span>${pctEl}</div>
      ${m.status === "open" ? `<div class="bar"><span style="width:${pct(m.chance)}%"></span></div>` : ""}
    </a>`;
  }).join("");
}

function boardHTML(board) {
  const inTop = board.me && board.top.some((m) => m.id === board.me.id);
  const meRow = board.me && !inTop ? `<div class="panel list" style="margin-top:10px"><div class="it"><span class="rank">${board.me.rank}</span>
    <span class="who"><b>You</b><span class="muted small">of ${board.members.toLocaleString()}</span></span><span class="num">${fmt(board.me.net_worth)}</span></div></div>` : "";
  return `<div class="panel list">${board.top.map((m, i) => `<div class="it">
    <span class="rank">${m.rank || i + 1}</span>
    <span class="who"><b>${esc(m.name)}${m.id === S.me?.id ? " (you)" : ""}</b>
      ${m.title ? `<span class="title-tag ${m.title === "Top Degen" ? "top" : "bad"}">${m.title}</span>` : `<span class="muted small">${m.open_bets} open bet${m.open_bets === 1 ? "" : "s"}</span>`}</span>
    <span class="num">${fmt(m.net_worth)}<div class="muted small">net worth</div></span></div>`).join("")}</div>${meRow}`;
}

async function loadFeed() {
  const list = await api("GET", `/api/groups/${S.code}/feed`);
  const el = document.getElementById("feed");
  if (!el) return;
  el.innerHTML = list.length ? list.map((t) => {
    const side = t.side.endsWith("yes") ? "yes" : "no";
    const verb = t.side.startsWith("buy") ? `bet <b class="${side}">${fmt(t.coins)}</b> on <b class="${side}">${side.toUpperCase()}</b>` : `cashed out <b>${fmt(-t.coins)}</b> from ${side.toUpperCase()}`;
    return `<a class="it" data-link href="/g/${S.code}/m/${t.market_id}"><div>${t.insider ? "🚨 " : ""}<b>${esc(t.user)}</b> ${verb}</div>
      <div class="muted">${esc(t.question)} · now ${pct(t.price_after)}% · ${ago(t.at)}${t.insider ? " · insider trading" : ""}</div></a>`;
  }).join("") : `<div class="empty"><b>Dead quiet.</b>Place the first bet and start the feed.</div>`;
}

async function invite() {
  const url = `${location.origin}/g/${S.code}`;
  const text = campus() ? `${S.group.name} has a prediction market now. Verify your school email and bet on campus.` : `Join ${S.group.name} on sidebet. We're putting odds on each other.`;
  if (navigator.share) {
    try { await navigator.share({ title: "sidebet", text, url }); return; } catch { }
  }
  try { await navigator.clipboard.writeText(url); toast("Invite link copied. Drop it in the chat."); }
  catch { prompt("Copy this link", url); }
}

function newMarket() {
  const others = S.members.filter((m) => m.id !== S.me?.id);
  let closes = 72, subject = null;
  const subjects = campus() ? "" : `<label class="l">Who's it about?</label>
    <div class="seg" id="subj"><button data-id="" class="on">Nobody specific</button>${others.map((m) => `<button data-id="${m.id}">${esc(m.name)}</button>`).join("")}${S.me ? `<button data-id="${S.me.id}">Me (bold)</button>` : ""}</div>`;
  sheet(`<h2>New bet</h2><p class="muted small">${campus() ? "A yes/no question about something happening on campus. Events, not people. A campus mod settles it." : "Ask a yes/no question about someone in the group."}</p>
    <label class="l">The question</label>
    <input class="field" id="q" maxlength="140" placeholder="${esc(pick(campus() ? CAMPUS_EXAMPLES : EXAMPLES))}">
    ${subjects}
    <label class="l">Betting closes</label>
    <div class="seg" id="close"><button data-h="6">Tonight</button><button data-h="24">Tomorrow</button><button data-h="72" class="on">3 days</button><button data-h="168">1 week</button><button data-h="720">1 month</button></div>
    <div style="height:16px"></div><button class="btn" id="mkgo">Drop it in the group</button>`, (el, close) => {
    const q = el.querySelector("#q");
    q.focus();
    const seg = (id, fn) => el.querySelectorAll(`#${id} button`).forEach((b) => (b.onclick = () => {
      el.querySelectorAll(`#${id} button`).forEach((x) => x.classList.remove("on"));
      b.classList.add("on");
      fn(b);
    }));
    seg("subj", (b) => (subject = b.dataset.id ? Number(b.dataset.id) : null));
    seg("close", (b) => (closes = Number(b.dataset.h)));
    el.querySelector("#mkgo").onclick = async () => {
      try {
        const m = await api("POST", `/api/groups/${S.code}/markets`, { question: q.value, subject_id: subject, closes_in_hours: closes });
        close();
        go(`/g/${S.code}/m/${m.id}?new=1`);
      } catch (e) { toast(e.message, true); }
    };
  });
}

function chartSVG(points, status) {
  const w = 600, h = 150, pad = 8;
  const n = points.length + (status === "open" ? 1 : 0);
  const x = (i) => pad + (n <= 1 ? 0 : (i / (n - 1)) * (w - pad * 2));
  const y = (p) => pad + (1 - p) * (h - pad * 2);
  let d = `M${x(0)},${y(points[0].chance)}`;
  for (let i = 1; i < points.length; i++) d += ` L${x(i)},${y(points[i].chance)}`;
  const last = points[points.length - 1].chance;
  if (status === "open") d += ` L${x(n - 1)},${y(last)}`;
  const col = last >= 0.5 ? "var(--yes)" : "var(--no)";
  const end = status === "open" ? n - 1 : points.length - 1;
  return `<svg class="chart" viewBox="0 0 ${w} ${h}" preserveAspectRatio="none" role="img" aria-label="Odds after each bet">
    <line x1="0" x2="${w}" y1="${y(0.5)}" y2="${y(0.5)}" stroke="var(--line)" stroke-dasharray="4 6" vector-effect="non-scaling-stroke"/>
    <path d="${d} L${x(end)},${h} L${x(0)},${h} Z" fill="${col}" opacity=".1"/>
    <path d="${d}" fill="none" stroke="${col}" stroke-width="3" vector-effect="non-scaling-stroke" stroke-linejoin="round" stroke-linecap="round"/></svg>`;
}

let tradeState = { side: "yes", amount: 50 };

async function renderMarket(soft) {
  const [, code, id] = location.pathname.match(/^\/g\/([^/]+)\/m\/(\d+)/);
  S.code = code;
  let data, board;
  try {
    [data, board] = await Promise.all([api("GET", `/api/groups/${code}/markets/${id}`), api("GET", `/api/groups/${code}/leaderboard`)]);
  } catch (e) {
    if (e.status === 403 || e.status === 401) return go(`/g/${code}`);
    app.innerHTML = `<div class="wrap"><div class="empty"><b>Bet not found.</b></div><a class="btn ghost" data-link href="/g/${code}">Back</a></div>`;
    return;
  }
  if (!S.group || S.group.code !== code) {
    const p = await api("GET", `/api/groups/${code}`);
    S.group = p.group;
    S.role = p.role || "";
  }
  connect(code);
  S.members = board.top;
  S.coins = myCoins(board);
  const m = data.market;
  document.title = `${pct(m.chance)}% · ${m.question}`;
  const open = m.status === "open" && new Date(m.closes_at) > Date.now();
  const col = m.chance >= 0.5 ? "yes" : "no";
  const head = m.status === "resolved" ? `<div class="big ${m.outcome}">${m.outcome.toUpperCase()}</div><div class="muted">settled ${ago(m.resolved_at)}</div>`
    : m.status === "void" ? `<div class="big muted">VOID</div><div class="muted">everyone got refunded</div>`
    : `<div class="big ${col}">${pct(m.chance)}%</div><div class="muted">chance · ${until(m.closes_at)}</div>`;
  const mine = m.mine && (m.mine.yes > 0.01 || m.mine.no > 0.01) ? m.mine : null;
  const scroll = window.scrollY;
  app.innerHTML = `<div class="wrap">
    <div class="top"><a class="back" data-link href="/g/${code}">‹ ${esc(S.group.name)}</a><span class="coins"><i>●</i>${fmt(S.coins)}</span></div>
    <div class="chips">${m.subject ? `<span class="chip subj">about ${esc(m.subject)}</span>` : ""}<span class="chip">by ${esc(m.creator)}</span><span class="chip">${m.traders} betting · ${fmt(m.volume)} in</span></div>
    <div class="question">${esc(m.question)}</div>
    ${head}
    ${chartSVG(data.history, m.status)}
    ${open ? `<div class="panel">
      <div class="two"><button class="btn yes" data-side="yes">YES ${pct(m.chance)}¢</button><button class="btn no" data-side="no">NO ${100 - pct(m.chance)}¢</button></div>
      <div class="amounts">${[10, 50, 100, 250].map((a) => `<button data-amt="${a}">${a}</button>`).join("")}<button data-amt="all" class="yolo">ALL IN</button></div>
      <div class="quote" id="quote"></div>
      <button class="btn" id="place" style="margin-top:10px">Place bet</button>
      ${m.subject_id === S.me?.id ? `<p class="small" style="color:var(--warn);margin:10px 0 0">🚨 This bet is about you. Anything you do gets flagged as insider trading. Everyone will see.</p>` : ""}
    </div>` : ""}
    ${mine ? `<div class="panel"><h3>Your position</h3>
      <div class="list">${mine.yes > 0.01 ? `<div class="it"><span><b class="yes">${fmt(mine.yes)} YES</b> shares</span>${open ? `<button class="btn ghost" style="width:auto;padding:8px 12px" data-sell="yes" data-sh="${mine.yes}">Cash out</button>` : ""}</div>` : ""}
      ${mine.no > 0.01 ? `<div class="it"><span><b class="no">${fmt(mine.no)} NO</b> shares</span>${open ? `<button class="btn ghost" style="width:auto;padding:8px 12px" data-sell="no" data-sh="${mine.no}">Cash out</button>` : ""}</div>` : ""}</div>
      <div class="muted small" style="margin-top:6px">Worth ${fmt(mine.value)} now · pays ${fmt(Math.max(mine.yes, mine.no))} if you're right</div></div>` : ""}
    ${open && (campus() ? S.role === "admin" : m.creator_id === S.me?.id) ? `<div class="panel"><h3>${campus() ? "Campus mod: settle it when it's decided." : "You made this bet. Settle it when it's decided."}</h3>
      <div class="two"><button class="btn ghost" data-res="yes">It happened</button><button class="btn ghost" data-res="no">It didn't</button></div>
      <button class="btn ghost" data-res="void" style="margin-top:10px">Void and refund everyone</button></div>` : ""}
    ${open && !campus() && m.creator_id !== S.me?.id && S.group?.created_by === S.me?.id ? `<div class="panel"><h3>Group owner</h3>
      <p class="muted small" style="margin:0 0 10px">Too far? Void it and everyone gets their coins back.</p>
      <button class="btn ghost" data-res="void">Void this bet</button></div>` : ""}
    <div class="panel"><h3>Who's betting</h3><div class="list feed">${data.trades.length ? data.trades.map((t) => {
      const side = t.side.endsWith("yes") ? "yes" : "no";
      return `<div class="it"><div>${t.insider ? "🚨 " : ""}<b>${esc(t.user)}</b> ${t.side.startsWith("buy") ? `bet ${fmt(t.coins)} on <b class="${side}">${side.toUpperCase()}</b>` : `cashed out ${fmt(-t.coins)}`}</div>
        <div class="muted">→ ${pct(t.price_after)}% · ${ago(t.at)}${t.insider ? " · insider trading" : ""}</div></div>`;
    }).join("") : `<div class="muted small">Nobody yet. First bet sets the line.</div>`}</div></div>
    ${campus() ? `<button class="btn ghost small" id="report" style="margin-top:14px">Report this market</button>` : ""}
    ${campus() && open ? `<p class="small muted">A campus mod settles this when the result is public.</p>` : ""}
  </div>
  <button class="btn fab" id="share">Send to the group chat</button>`;
  if (soft) window.scrollTo(0, scroll);
  const report = document.getElementById("report");
  if (report) report.onclick = () => {
    sheet(`<h2>Report this market</h2><p class="muted small">Mods review reports. Markets about a specific person, harassment or spam get voided.</p>
      <input class="field" id="why" maxlength="200" placeholder="What's wrong with it?"><div style="height:12px"></div><button class="btn" id="rp">Send report</button>`, (el, close) => {
      el.querySelector("#rp").onclick = async () => {
        try { await api("POST", `/api/groups/${code}/markets/${m.id}/report`, { reason: el.querySelector("#why").value || "reported" }); close(); toast("Reported. A mod will look."); }
        catch (e) { toast(e.message, true); }
      };
    });
  };
  document.getElementById("share").onclick = async () => {
    const url = `${location.origin}/g/${code}/m/${m.id}`;
    const text = m.status === "open" ? `${pct(m.chance)}% chance: ${m.question}` : `${m.question} → ${m.outcome.toUpperCase()}`;
    if (navigator.share) { try { await navigator.share({ text, url }); return; } catch { } }
    try { await navigator.clipboard.writeText(`${text} ${url}`); toast("Copied. Go stir the pot."); } catch { prompt("Copy this", url); }
  };
  if (open) wireTrade(m, code);
  app.querySelectorAll("[data-sell]").forEach((b) => (b.onclick = async () => {
    try {
      const r = await api("POST", `/api/groups/${code}/markets/${m.id}/sell`, { side: b.dataset.sell, shares: Number(b.dataset.sh) });
      toast(`Cashed out ${fmt(-r.trade.coins)} coins. Coward, but okay.`);
      renderMarket(true);
    } catch (e) { toast(e.message, true); }
  }));
  app.querySelectorAll("[data-res]").forEach((b) => (b.onclick = async () => {
    const o = b.dataset.res;
    if (!confirm(o === "void" ? "Void this bet and refund everyone?" : `Settle as ${o.toUpperCase()}? This pays out and can't be undone.`)) return;
    try { await api("POST", `/api/groups/${code}/markets/${m.id}/resolve`, { outcome: o }); toast("Settled. Winners paid."); renderMarket(true); }
    catch (e) { toast(e.message, true); }
  }));
  if (new URLSearchParams(location.search).get("new")) {
    history.replaceState({}, "", `/g/${code}/m/${m.id}`);
    toast("Bet is live. Now send it to the group chat.");
  }
}

const LIQUIDITY = 150;

function lmsrShares(m, side, spend) {
  const p = side === "yes" ? m.chance : 1 - m.chance;
  return LIQUIDITY * Math.log((Math.exp(spend / LIQUIDITY) - (1 - p)) / p);
}

function wireTrade(m, code) {
  const sides = app.querySelectorAll("[data-side]");
  const amts = app.querySelectorAll("[data-amt]");
  const quote = document.getElementById("quote");
  const paint = () => {
    sides.forEach((b) => (b.style.opacity = b.dataset.side === tradeState.side ? "1" : ".35"));
    const amount = tradeState.amount === "all" ? Math.floor(S.coins) : tradeState.amount;
    amts.forEach((b) => b.classList.toggle("on", String(tradeState.amount) === b.dataset.amt));
    if (amount < 1) { quote.textContent = "You're broke. Take the bailout on the group page."; return; }
    const shares = lmsrShares(m, tradeState.side, amount);
    quote.innerHTML = `Bet <b>${fmt(amount)}</b> on <b class="${tradeState.side}">${tradeState.side.toUpperCase()}</b> → win <b>${fmt(shares)}</b> if right (${(shares / amount).toFixed(2)}x)`;
  };
  sides.forEach((b) => (b.onclick = () => { tradeState.side = b.dataset.side; paint(); }));
  amts.forEach((b) => (b.onclick = () => { tradeState.amount = b.dataset.amt === "all" ? "all" : Number(b.dataset.amt); paint(); }));
  paint();
  document.getElementById("place").onclick = async (e) => {
    const amount = tradeState.amount === "all" ? Math.floor(S.coins) : tradeState.amount;
    e.target.disabled = true;
    try {
      const r = await api("POST", `/api/groups/${code}/markets/${m.id}/buy`, { side: tradeState.side, amount });
      toast(tradeState.amount === "all" ? "ALL IN. Legendary or tragic. No in between." : `Locked in. Odds now ${pct(r.market.chance)}%.`);
      renderMarket(true);
    } catch (err) { toast(err.message, true); e.target.disabled = false; }
  };
}

async function route() {
  const p = location.pathname;
  await loadMe();
  let m;
  if ((m = p.match(/^\/g\/([^/]+)\/m\/(\d+)$/))) return renderMarket();
  if ((m = p.match(/^\/g\/([^/]+)$/))) { S.tab = "markets"; return groupPage(m[1]); }
  home();
}

route();
