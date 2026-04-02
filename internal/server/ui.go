package server

import (
	"net/http"
)

// uiHTML is the self-contained dashboard for Trough.
// Served at GET /ui — no build step, no external files.
const uiHTML = `<!DOCTYPE html><html lang="en"><head>
<meta charset="UTF-8"><meta name="viewport" content="width=device-width,initial-scale=1">
<title>Trough — Stockyard</title>
<link rel="preconnect" href="https://fonts.googleapis.com">
<link href="https://fonts.googleapis.com/css2?family=Libre+Baskerville:ital,wght@0,400;0,700;1,400&family=JetBrains+Mono:wght@400;600&display=swap" rel="stylesheet">
<style>:root{
  --bg:#1a1410;--bg2:#241e18;--bg3:#2e261e;
  --rust:#c45d2c;--rust-light:#e8753a;--rust-dark:#8b3d1a;
  --leather:#a0845c;--leather-light:#c4a87a;
  --cream:#f0e6d3;--cream-dim:#bfb5a3;--cream-muted:#7a7060;
  --gold:#d4a843;--green:#5ba86e;--red:#c0392b;
  --font-serif:'Libre Baskerville',Georgia,serif;
  --font-mono:'JetBrains Mono',monospace;
}
*{margin:0;padding:0;box-sizing:border-box}
body{background:var(--bg);color:var(--cream);font-family:var(--font-serif);min-height:100vh;overflow-x:hidden}
a{color:var(--rust-light);text-decoration:none}a:hover{color:var(--gold)}
.hdr{background:var(--bg2);border-bottom:2px solid var(--rust-dark);padding:.9rem 1.8rem;display:flex;align-items:center;justify-content:space-between;gap:1rem}
.hdr-left{display:flex;align-items:center;gap:1rem}
.hdr-brand{font-family:var(--font-mono);font-size:.75rem;color:var(--leather);letter-spacing:3px;text-transform:uppercase}
.hdr-title{font-family:var(--font-mono);font-size:1.1rem;color:var(--cream);letter-spacing:1px}
.badge{font-family:var(--font-mono);font-size:.6rem;padding:.2rem .6rem;letter-spacing:1px;text-transform:uppercase;border:1px solid}
.badge-free{color:var(--green);border-color:var(--green)}
.badge-pro{color:var(--gold);border-color:var(--gold)}
.badge-ok{color:var(--green);border-color:var(--green)}
.badge-err{color:var(--red);border-color:var(--red)}
.main{max-width:1000px;margin:0 auto;padding:2rem 1.5rem}
.cards{display:grid;grid-template-columns:repeat(auto-fit,minmax(160px,1fr));gap:1rem;margin-bottom:2rem}
.card{background:var(--bg2);border:1px solid var(--bg3);padding:1.2rem 1.5rem}
.card-val{font-family:var(--font-mono);font-size:1.8rem;font-weight:700;color:var(--cream);display:block}
.card-lbl{font-family:var(--font-mono);font-size:.62rem;letter-spacing:2px;text-transform:uppercase;color:var(--leather);margin-top:.3rem}
.section{margin-bottom:2.5rem}
.section-title{font-family:var(--font-mono);font-size:.68rem;letter-spacing:3px;text-transform:uppercase;color:var(--rust-light);margin-bottom:.8rem;padding-bottom:.5rem;border-bottom:1px solid var(--bg3)}
table{width:100%;border-collapse:collapse;font-family:var(--font-mono);font-size:.78rem}
th{background:var(--bg3);padding:.5rem .8rem;text-align:left;color:var(--leather-light);font-weight:400;letter-spacing:1px;font-size:.65rem;text-transform:uppercase}
td{padding:.5rem .8rem;border-bottom:1px solid var(--bg3);color:var(--cream-dim);vertical-align:top;word-break:break-all}
tr:hover td{background:var(--bg2)}
.empty{color:var(--cream-muted);text-align:center;padding:2rem;font-style:italic}
.btn{font-family:var(--font-mono);font-size:.75rem;padding:.4rem 1rem;border:1px solid var(--leather);background:transparent;color:var(--cream);cursor:pointer;transition:all .2s}
.btn:hover{border-color:var(--rust-light);color:var(--rust-light)}
.btn-rust{border-color:var(--rust);color:var(--rust-light)}.btn-rust:hover{background:var(--rust);color:var(--cream)}
.pill{display:inline-block;font-family:var(--font-mono);font-size:.6rem;padding:.1rem .4rem;border-radius:2px;text-transform:uppercase}
.pill-get{background:#1a3a2a;color:var(--green)}.pill-post{background:#2a1f1a;color:var(--rust-light)}
.pill-del{background:#2a1a1a;color:var(--red)}.pill-ok{background:#1a3a2a;color:var(--green)}
.pill-err{background:#2a1a1a;color:var(--red)}
.mono{font-family:var(--font-mono);font-size:.78rem}
.lbl{font-family:var(--font-mono);font-size:.62rem;letter-spacing:1px;text-transform:uppercase;color:var(--leather)}
.upgrade{background:var(--bg2);border:1px solid var(--rust-dark);border-left:3px solid var(--rust);padding:.8rem 1.2rem;font-size:.82rem;color:var(--cream-dim);margin-bottom:1.5rem}
.upgrade a{color:var(--rust-light)}
pre{background:var(--bg3);padding:.8rem 1rem;font-family:var(--font-mono);font-size:.75rem;color:var(--cream-dim);overflow-x:auto;max-width:100%}
input,select{font-family:var(--font-mono);font-size:.78rem;background:var(--bg3);border:1px solid var(--bg3);color:var(--cream);padding:.4rem .7rem;outline:none}
input:focus,select:focus{border-color:var(--leather)}
.row{display:flex;gap:.8rem;align-items:flex-end;flex-wrap:wrap;margin-bottom:1rem}
.field{display:flex;flex-direction:column;gap:.3rem}
.sserow{padding:.4rem .8rem;border-bottom:1px solid var(--bg3);font-family:var(--font-mono);font-size:.72rem;color:var(--cream-dim);display:grid;grid-template-columns:120px 60px 1fr;gap:.5rem}
.sserow:nth-child(odd){background:var(--bg2)}
</style></head><body>
<div class="hdr">
  <div class="hdr-left">
    <svg viewBox="0 0 64 64" width="22" height="22" fill="none"><rect x="8" y="8" width="8" height="48" rx="2.5" fill="#e8753a"/><rect x="28" y="8" width="8" height="48" rx="2.5" fill="#e8753a"/><rect x="48" y="8" width="8" height="48" rx="2.5" fill="#e8753a"/><rect x="8" y="27" width="48" height="7" rx="2.5" fill="#c4a87a"/></svg>
    <span class="hdr-brand">Stockyard</span>
    <span class="hdr-title">Trough</span>
  </div>
  <div style="display:flex;gap:.8rem;align-items:center">
    <span id="tier-badge" class="badge badge-free">Free</span>
    <a href="/api/stats" class="lbl" style="color:var(--leather)">API</a>
    <a href="https://stockyard.dev/trough/" class="lbl" style="color:var(--leather)">Docs</a>
  </div>
</div>
<div class="main">

<div class="cards">
  <div class="card"><span class="card-val" id="s-reqs">—</span><span class="card-lbl">Total Requests</span></div>
  <div class="card"><span class="card-val" id="s-today-r">—</span><span class="card-lbl">Requests Today</span></div>
  <div class="card"><span class="card-val" id="s-cost">—</span><span class="card-lbl">Total Cost</span></div>
  <div class="card"><span class="card-val" id="s-today-c">—</span><span class="card-lbl">Cost Today</span></div>
</div>

<div class="section">
  <div class="section-title">Upstream Services</div>
  <div class="row">
    <div class="field"><span class="lbl">Name</span><input id="up-name" placeholder="twilio" style="width:140px"></div>
    <div class="field"><span class="lbl">Base URL</span><input id="up-url" placeholder="https://api.twilio.com" style="width:260px"></div>
    <button class="btn btn-rust" onclick="createUpstream()">+ Add</button>
  </div>
  <table><thead><tr><th>ID</th><th>Name</th><th>Base URL</th><th>Proxy URL</th><th></th></tr></thead>
  <tbody id="up-list"><tr><td colspan="5" class="empty">Loading...</td></tr></tbody></table>
</div>

<div class="section">
  <div class="section-title">Spend Summary <span class="lbl">(30 days)</span></div>
  <div style="display:grid;grid-template-columns:1fr 1fr;gap:2rem">
    <div>
      <div class="lbl" style="margin-bottom:.6rem">By Day</div>
      <table><thead><tr><th>Date</th><th>Requests</th><th>Cost</th></tr></thead>
      <tbody id="day-list"><tr><td colspan="3" class="empty">Loading...</td></tr></tbody></table>
    </div>
    <div>
      <div class="lbl" style="margin-bottom:.6rem">By Endpoint</div>
      <table><thead><tr><th>Path</th><th>Method</th><th>Reqs</th><th>Cost</th></tr></thead>
      <tbody id="ep-list"><tr><td colspan="4" class="empty">Loading...</td></tr></tbody></table>
    </div>
  </div>
</div>

<div class="section">
  <div class="section-title">Recent Requests <span class="lbl">(last 30)</span></div>
  <table><thead><tr><th>Service</th><th>Method</th><th>Path</th><th>Status</th><th>Latency</th><th>Cost</th><th>Time</th></tr></thead>
  <tbody id="req-list"><tr><td colspan="7" class="empty">Loading...</td></tr></tbody></table>
</div>

</div>
<script>
let _timer=null;
function autoReload(fn,ms=8000){if(_timer)clearInterval(_timer);_timer=setInterval(fn,ms)}
function ts(s){if(!s)return'-';const d=new Date(s);return d.toLocaleString()}
function rel(s){if(!s)return'-';const d=new Date(s),n=new Date(),diff=Math.round((n-d)/1000);if(diff<60)return diff+'s ago';if(diff<3600)return Math.round(diff/60)+'m ago';return Math.round(diff/3600)+'h ago'}
function fmt(n){return n===undefined||n===null?'-':n.toLocaleString()}
function pill(m){const c={'GET':'pill-get','POST':'pill-post','DELETE':'pill-del'}[m]||'';return '<span class="pill '+c+'">'+m+'</span>'}
function status(s){const ok=s>=200&&s<300;return '<span class="pill '+(ok?'pill-ok':'pill-err')+'">'+s+'</span>'}

const API='/api';
function cents(c){if(!c)return'$0.00';return'$'+(c/100).toFixed(c<100?4:2)}

async function loadStats(){
  const r=await(await fetch(API+'/stats')).json().catch(()=>({}));
  document.getElementById('s-reqs').textContent=fmt(r.total_requests);
  document.getElementById('s-today-r').textContent=fmt(r.requests_today);
  document.getElementById('s-cost').textContent=cents(r.total_cost_cents);
  document.getElementById('s-today-c').textContent=cents(r.cost_today_cents);
}

async function loadUpstreams(){
  const r=await(await fetch(API+'/upstreams')).json().catch(()=>({upstreams:[]}));
  const us=r.upstreams||[];
  document.getElementById('up-list').innerHTML=us.length?us.map(u=>
    ` + "`" + `<tr>
      <td class="mono" style="font-size:.7rem;color:var(--leather-light)">${u.id}</td>
      <td style="color:var(--cream)">${u.name}</td>
      <td class="mono" style="font-size:.72rem">${u.base_url}</td>
      <td class="mono" style="font-size:.7rem;color:var(--rust-light)">/proxy/${u.id}/</td>
      <td><button class="btn" style="font-size:.65rem;padding:.2rem .5rem" onclick="deleteUpstream('${u.id}')">Remove</button></td>
    </tr>` + "`" + `).join(''):'<tr><td colspan="5" class="empty">No services registered yet.</td></tr>';
}

async function loadSpend(){
  const r=await(await fetch(API+'/spend?days=30')).json().catch(()=>({}));
  const days=r.by_day||[];
  document.getElementById('day-list').innerHTML=days.length?days.slice(0,10).map(d=>
    ` + "`" + `<tr><td class="mono">${d.date}</td><td>${fmt(d.requests)}</td><td style="color:var(--gold)">${cents(d.cost_cents)}</td></tr>` + "`" + `
  ).join(''):'<tr><td colspan="3" class="empty">No data yet.</td></tr>';
  const eps=r.by_endpoint||[];
  document.getElementById('ep-list').innerHTML=eps.length?eps.slice(0,10).map(e=>
    ` + "`" + `<tr>
      <td class="mono" style="font-size:.7rem;max-width:160px;overflow:hidden;text-overflow:ellipsis">${e.path}</td>
      <td>${pill(e.method)}</td>
      <td>${fmt(e.requests)}</td>
      <td style="color:var(--gold)">${cents(e.cost_cents)}</td>
    </tr>` + "`" + `
  ).join(''):'<tr><td colspan="4" class="empty">No data yet.</td></tr>';
}

async function loadRequests(){
  const r=await(await fetch(API+'/requests?limit=30')).json().catch(()=>({requests:[]}));
  const reqs=r.requests||[];
  document.getElementById('req-list').innerHTML=reqs.length?reqs.map(q=>
    ` + "`" + `<tr>
      <td class="mono" style="font-size:.7rem;color:var(--leather-light)">${q.upstream_name}</td>
      <td>${pill(q.method)}</td>
      <td class="mono" style="font-size:.7rem;max-width:200px;overflow:hidden;text-overflow:ellipsis">${q.path}</td>
      <td>${status(q.status)}</td>
      <td class="mono">${q.latency_ms}ms</td>
      <td style="color:var(--gold)">${cents(q.cost_cents)}</td>
      <td>${rel(q.created_at)}</td>
    </tr>` + "`" + `).join(''):'<tr><td colspan="7" class="empty">No requests yet. Route traffic through /proxy/{id}/...</td></tr>';
}

async function createUpstream(){
  const name=document.getElementById('up-name').value.trim();
  const base_url=document.getElementById('up-url').value.trim();
  if(!name||!base_url)return;
  const r=await fetch(API+'/upstreams',{method:'POST',headers:{'Content-Type':'application/json'},body:JSON.stringify({name,base_url})}).catch(()=>null);
  if(!r)return;
  if(r.status===402){alert('Free tier: 1 service limit. Upgrade to Pro at stockyard.dev/trough/');return;}
  document.getElementById('up-name').value='';document.getElementById('up-url').value='';
  loadUpstreams();
}

async function deleteUpstream(id){if(!confirm('Remove upstream?'))return;await fetch(API+'/upstreams/'+id,{method:'DELETE'});loadUpstreams();}

async function refresh(){await Promise.all([loadStats(),loadUpstreams(),loadSpend(),loadRequests()]);}
refresh();autoReload(refresh,8000);
</script></body></html>`

func (s *Server) handleUI(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-cache")
	w.Write([]byte(uiHTML))
}
