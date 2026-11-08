package admin

const pageTemplates = `
{{define "css"}}
:root {
  color-scheme: light;
  --bg: #efe8dc;
  --card: #fffdf8;
  --ink: #1e1a16;
  --muted: #6d645b;
  --line: #e2d8c8;
  --accent: #0c5c56;
  --accent-ink: #f3fbfa;
  --danger: #8f2d1c;
  --ok: #1d6b3a;
  --info: #1e4f86;
  --warn: #8a4b08;
}
* { box-sizing: border-box; }
body {
  margin: 0;
  background: var(--bg);
  color: var(--ink);
  font: 16px/1.45 "Avenir Next", "Segoe UI", sans-serif;
}
.shell { width: min(1040px, calc(100% - 32px)); margin: 0 auto; padding-bottom: 48px; }
header.top {
  display: flex; justify-content: space-between; align-items: end; gap: 16px;
  padding: 28px 0 12px; border-bottom: 1px solid var(--line); margin-bottom: 18px;
}
nav { display: flex; flex-wrap: wrap; gap: 8px; margin: 0 0 18px; }
nav a {
  text-decoration: none; color: var(--ink); border: 1px solid var(--line);
  background: var(--card); border-radius: 999px; padding: 8px 12px; font-size: 0.92rem;
}
nav a.active { background: var(--accent); color: var(--accent-ink); border-color: var(--accent); }
h1, h2 { font-family: "Iowan Old Style", Palatino, Georgia, serif; font-weight: 600; }
h1 { margin: 0; font-size: 1.8rem; }
h2 { margin: 0 0 12px; font-size: 1.2rem; }
.mark {
  display: inline-grid; place-items: center; width: 36px; height: 36px; margin: 0 0 8px;
  border-radius: 8px; background: var(--accent); color: var(--accent-ink); font-weight: 700;
}
.lede, .help, .meta, .empty { color: var(--muted); }
.card, .stats article, .scroll li, .table-wrap {
  background: var(--card); border: 1px solid var(--line); border-radius: 14px;
}
.card { padding: 16px 18px; margin: 0 0 16px; }
.stats { display: grid; grid-template-columns: repeat(3, 1fr); gap: 10px; margin: 0 0 16px; }
.stats article { padding: 12px 14px; }
.stats span { display: block; color: var(--muted); font-size: 0.85rem; }
.stats strong { font-family: "Iowan Old Style", Palatino, Georgia, serif; font-size: 1.7rem; font-variant-numeric: tabular-nums; }
label { display: block; font-size: 0.85rem; color: var(--muted); margin-bottom: 6px; }
.row { display: flex; gap: 10px; align-items: end; flex-wrap: wrap; }
.grid2 { display: grid; grid-template-columns: 1fr 1fr; gap: 12px; }
.grow { flex: 1; min-width: 180px; }
input, select, textarea {
  width: 100%; border: 1px solid var(--line); background: #fff; border-radius: 10px;
  padding: 10px 12px; font: inherit; color: inherit;
}
button, summary {
  border: 0; border-radius: 10px; background: var(--accent); color: var(--accent-ink);
  font: inherit; padding: 10px 14px; cursor: pointer;
}
button.ghost { background: transparent; color: var(--ink); border: 1px solid var(--line); }
button.danger { background: var(--danger); }
.scroll {
  list-style: none; padding: 0; margin: 0; max-height: 460px; overflow-y: auto;
  border: 1px solid var(--line); border-radius: 14px; background: var(--card);
}
.scroll li {
  display: flex; justify-content: space-between; gap: 16px; padding: 12px 14px;
  border: 0; border-radius: 0; border-bottom: 1px solid var(--line); margin: 0;
}
.scroll li:last-child { border-bottom: 0; }
.scroll li.off { opacity: 0.55; }
.name { margin: 0 0 4px; font-family: ui-monospace, "SFMono-Regular", Menlo, Consolas, monospace; }
.actions { display: flex; gap: 8px; align-items: start; flex-wrap: wrap; }
.actions form { margin: 0; }
details summary { list-style: none; background: transparent; color: var(--danger); border: 1px solid var(--line); }
details summary::-webkit-details-marker { display: none; }
details form { margin-top: 8px; }
.tag { display: inline-block; margin-left: 6px; padding: 1px 7px; border-radius: 999px; font-size: 0.75rem; }
.tag.exact, .tag.dns { background: #e5f2f1; color: var(--accent); }
.tag.wild, .tag.proxy { background: #f8efe2; color: var(--warn); }
.tag.deny, .tag.error, .tag.blocked { background: #f8e8e4; color: var(--danger); }
.flag { color: var(--warn); margin: 4px 0; }
.result { padding: 10px 12px; border-radius: 10px; margin-top: 10px; }
.result.ok { background: #e7f5ec; color: var(--ok); }
.result.info { background: #e7f0fa; color: var(--info); }
.result.bad { background: #fbecea; color: var(--danger); }
.activity, .plain { list-style: none; padding: 0; margin: 0; }
.activity li { display: grid; grid-template-columns: auto auto 1fr; gap: 4px 10px; padding: 8px 0; border-bottom: 1px solid var(--line); }
.activity .help { grid-column: 2 / -1; margin: 0; }
.when { color: var(--muted); font-variant-numeric: tabular-nums; }
table { width: 100%; border-collapse: collapse; }
th, td { text-align: left; padding: 10px 12px; border-bottom: 1px solid var(--line); vertical-align: top; }
th { color: var(--muted); font-weight: 600; font-size: 0.85rem; }
.table-wrap { overflow-x: auto; }
.mode-box { display: flex; gap: 10px; flex-wrap: wrap; align-items: center; }
.narrow { width: min(420px, calc(100% - 32px)); margin: 8vh auto 48px; }
:focus-visible { outline: 2px solid var(--accent); outline-offset: 2px; }
@media (max-width: 720px) {
  .stats, .grid2 { grid-template-columns: 1fr; }
  .scroll li, header.top { display: block; }
  .actions { margin-top: 10px; }
}
{{end}}

{{define "nav"}}
<nav>
  <a href="/" {{if eq .Nav "dashboard"}}class="active"{{end}}>Dashboard</a>
  <a href="/domains" {{if eq .Nav "domains"}}class="active"{{end}}>Domains</a>
  <a href="/clients" {{if eq .Nav "clients"}}class="active"{{end}}>Clients</a>
  <a href="/traffic" {{if eq .Nav "traffic"}}class="active"{{end}}>Traffic</a>
  <a href="/account" {{if eq .Nav "account"}}class="active"{{end}}>Account</a>
</nav>
{{end}}

{{define "top"}}
<header class="top">
  <div>
    <p class="mark">53</p>
    <h1>SNI DNS Proxy</h1>
    <p class="meta">{{.Info.PublicIP}} · {{.Username}}</p>
  </div>
  <form method="post" action="/logout">
    <input type="hidden" name="csrf" value="{{.CSRF}}">
    <button type="submit" class="ghost">Sign out</button>
  </form>
</header>
{{template "nav" .}}
{{end}}

{{define "login"}}
<!doctype html>
<html lang="en">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<meta name="robots" content="noindex">
<title>Sign in · SNI DNS Proxy</title>
<style>{{template "css"}}</style>
</head>
<body>
<main class="narrow">
  <p class="mark">53</p>
  <h1>SNI DNS Proxy</h1>
  <p class="lede">Sign in with your admin username and password.</p>
  {{if .Error}}<p class="result bad">{{.Error}}</p>{{end}}
  <form method="post" action="/login" class="card">
    <label for="username">Username</label>
    <input id="username" name="username" value="{{.Username}}" autocomplete="username" required autofocus>
    <label for="password" style="margin-top:12px">Password</label>
    <input id="password" name="password" type="password" autocomplete="current-password" required>
    <p style="margin-top:14px"><button type="submit">Sign in</button></p>
  </form>
</main>
</body>
</html>
{{end}}

{{define "dashboard"}}
<!doctype html>
<html lang="en">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<title>Dashboard · SNI DNS Proxy</title>
<style>{{template "css"}}</style>
</head>
<body>
<div class="shell">
{{template "top" .}}
<p class="lede">Clients that use this DNS server are sent here for names on the list. TLS stays between the client and the real site.</p>
<p class="meta">Upstream {{.Info.Upstreams}} · DNS {{.Info.DNSAddr}} · HTTP {{.Info.HTTPAddr}} · HTTPS {{.Info.HTTPSAddr}} · Mode {{.ClientMode}} · {{.DomainCount}} domains · {{.ClientCount}} client rules</p>
<section class="stats">
  <article><span>Steered queries</span><strong>{{.Steered}}</strong></article>
  <article><span>Forwarded queries</span><strong>{{.Forwarded}}</strong></article>
  <article><span>DNS errors</span><strong>{{.DNSErrors}}</strong></article>
  <article><span>Proxied connections</span><strong>{{.Proxied}}</strong></article>
  <article><span>Refused</span><strong>{{.Refused}}</strong></article>
  <article><span>Proxy errors</span><strong>{{.ProxyErrors}}</strong></article>
</section>
<section class="card">
  <h2>Top talkers today</h2>
  {{if not .TopTalkers}}<p class="empty">No proxied traffic counted yet.</p>{{end}}
  {{if .TopTalkers}}
  <div class="table-wrap">
  <table>
    <tr><th>Client</th><th>Today</th><th>Download</th><th>Upload</th></tr>
    {{range .TopTalkers}}
    <tr><td class="name">{{.ClientKey}}</td><td>{{.TodaySum}}</td><td>{{.TodayIn}}</td><td>{{.TodayOut}}</td></tr>
    {{end}}
  </table>
  </div>
  {{end}}
</section>
<section class="card">
  <h2>Recent activity</h2>
  {{if not .Events}}<p class="empty">No steered queries or proxied connections yet.</p>{{end}}
  <ul class="activity">
    {{range .Events}}
    <li>
      <span class="when">{{.Time}}</span>
      <span class="tag {{.Kind}}">{{.Label}}</span>
      <span class="name">{{.Name}}</span>
      <span class="help">{{.Detail}}</span>
    </li>
    {{end}}
  </ul>
</section>
</div>
</body>
</html>
{{end}}

{{define "domains"}}
<!doctype html>
<html lang="en">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<title>Domains · SNI DNS Proxy</title>
<style>{{template "css"}}</style>
</head>
<body>
<div class="shell">
{{template "top" .}}
<section class="card">
  <h2>Check a name</h2>
  <form method="get" action="/domains" class="row">
    <div class="grow">
      <label for="check">Name</label>
      <input id="check" name="check" value="{{.CheckName}}" placeholder="www.example.com" spellcheck="false" maxlength="253">
    </div>
    <button type="submit">Check</button>
  </form>
  {{if .CheckMessage}}<p class="result {{.CheckClass}}">{{.CheckMessage}}</p>{{end}}
</section>
<section class="card">
  <h2>Add a domain</h2>
  <p class="help">example.com matches only that name. *.example.com matches every subdomain, not the apex itself.</p>
  <form method="post" action="/domains" class="row">
    <input type="hidden" name="csrf" value="{{.CSRF}}">
    <div class="grow">
      <label for="pattern">Pattern</label>
      <input id="pattern" name="pattern" value="{{.FormPattern}}" placeholder="example.com or *.example.com" spellcheck="false" maxlength="253" required>
    </div>
    <button type="submit">Add domain</button>
  </form>
  {{if .Error}}<p class="result bad">{{.Error}}</p>{{end}}
</section>
<section>
  <div class="row" style="margin-bottom:10px">
    <h2 style="margin:0;flex:1">Domain list <span class="help">({{.DomainCount}})</span></h2>
    <div class="grow" style="max-width:280px">
      <label for="filter">Filter</label>
      <input id="filter" type="search" placeholder="Filter domains" oninput="filterDomains(this.value)">
    </div>
  </div>
  {{if not .Domains}}<p class="empty">No domains yet.</p>{{end}}
  <ul class="scroll" id="domain-list">
    {{range .Domains}}
    <li class="{{if not .Enabled}}off{{end}}" data-pattern="{{.Pattern}}">
      <div>
        <p class="name">{{.Pattern}} <span class="tag {{if eq .Kind "Wildcard"}}wild{{else}}exact{{end}}">{{.Kind}}</span></p>
        <p class="help">{{.Coverage}}</p>
        {{if .Warning}}<p class="flag">{{.Warning}}</p>{{end}}
        <p class="help">{{if .Enabled}}Enabled{{else}}Disabled{{end}} · added {{.Added}}</p>
      </div>
      <div class="actions">
        <form method="post" action="/domains/{{.ID}}/toggle">
          <input type="hidden" name="csrf" value="{{$.CSRF}}">
          <button type="submit">{{if .Enabled}}Disable{{else}}Enable{{end}}</button>
        </form>
        <details>
          <summary>Delete</summary>
          <form method="post" action="/domains/{{.ID}}/delete">
            <input type="hidden" name="csrf" value="{{$.CSRF}}">
            <button type="submit" class="danger">Delete {{.Pattern}}</button>
          </form>
        </details>
      </div>
    </li>
    {{end}}
  </ul>
</section>
</div>
<script>
function filterDomains(q){
  q=(q||'').toLowerCase();
  document.querySelectorAll('#domain-list li').forEach(function(li){
    var p=li.getAttribute('data-pattern')||'';
    li.style.display=!q||p.indexOf(q)!==-1?'':'none';
  });
}
</script>
</body>
</html>
{{end}}

{{define "clients"}}
<!doctype html>
<html lang="en">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<title>Clients · SNI DNS Proxy</title>
<style>{{template "css"}}</style>
</head>
<body>
<div class="shell">
{{template "top" .}}
<section class="card">
  <h2>Default for unknown IPs</h2>
  <p class="help">Open lets every client use DNS and the proxy unless a rule blocks them. Locked allows only IPs you add.</p>
  <form method="post" action="/clients/mode" class="mode-box">
    <input type="hidden" name="csrf" value="{{.CSRF}}">
    <label><input type="radio" name="mode" value="open" {{if eq .ClientMode "open"}}checked{{end}}> Open</label>
    <label><input type="radio" name="mode" value="locked" {{if eq .ClientMode "locked"}}checked{{end}}> Locked</label>
    <button type="submit">Save mode</button>
  </form>
</section>
<section class="card">
  <h2>Add a client rule</h2>
  {{if .Error}}<p class="result bad">{{.Error}}</p>{{end}}
  <form method="post" action="/clients">
    <input type="hidden" name="csrf" value="{{.CSRF}}">
    <div class="grid2">
      <div>
        <label for="cidr">IP or CIDR</label>
        <input id="cidr" name="cidr" value="{{.FormCIDR}}" placeholder="203.0.113.10 or 203.0.113.0/24" required>
      </div>
      <div>
        <label for="access">Access</label>
        <select id="access" name="access">
          <option value="both" {{if eq .FormAccess "both"}}selected{{end}}>DNS + Proxy</option>
          <option value="dns" {{if eq .FormAccess "dns"}}selected{{end}}>DNS only</option>
          <option value="proxy" {{if eq .FormAccess "proxy"}}selected{{end}}>Proxy only</option>
          <option value="blocked" {{if eq .FormAccess "blocked"}}selected{{end}}>Blocked</option>
        </select>
      </div>
      <div>
        <label for="down_mbps">Download limit (Mbps, 0 = none)</label>
        <input id="down_mbps" name="down_mbps" value="{{if .FormDown}}{{.FormDown}}{{else}}0{{end}}" inputmode="decimal">
      </div>
      <div>
        <label for="up_mbps">Upload limit (Mbps, 0 = none)</label>
        <input id="up_mbps" name="up_mbps" value="{{if .FormUp}}{{.FormUp}}{{else}}0{{end}}" inputmode="decimal">
      </div>
    </div>
    <label for="notes" style="margin-top:12px">Notes</label>
    <input id="notes" name="notes" value="{{.FormNotes}}" placeholder="optional">
    <p style="margin-top:14px"><button type="submit">Add client</button></p>
  </form>
</section>
<section>
  <h2>Client rules</h2>
  {{if not .Clients}}<p class="empty">No client rules yet.</p>{{end}}
  {{if .Clients}}
  <div class="table-wrap">
  <table>
    <tr><th>CIDR</th><th>Access</th><th>Down</th><th>Up</th><th>Notes</th><th></th></tr>
    {{range .Clients}}
    <tr class="{{if not .Enabled}}off{{end}}">
      <td class="name">{{.CIDR}}</td>
      <td>{{accessLabel .Access}}</td>
      <td>{{.DownMbps}}</td>
      <td>{{.UpMbps}}</td>
      <td>{{.Notes}}</td>
      <td class="actions">
        <form method="post" action="/clients/{{.ID}}/toggle">
          <input type="hidden" name="csrf" value="{{$.CSRF}}">
          <button type="submit">{{if .Enabled}}Disable{{else}}Enable{{end}}</button>
        </form>
        <form method="post" action="/clients/{{.ID}}/delete" onsubmit="return confirm('Delete this rule?')">
          <input type="hidden" name="csrf" value="{{$.CSRF}}">
          <button type="submit" class="danger">Delete</button>
        </form>
      </td>
    </tr>
    {{end}}
  </table>
  </div>
  {{end}}
</section>
</div>
</body>
</html>
{{end}}

{{define "traffic"}}
<!doctype html>
<html lang="en">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<title>Traffic · SNI DNS Proxy</title>
<style>{{template "css"}}</style>
</head>
<body>
<div class="shell">
{{template "top" .}}
<section class="card">
  <h2>Traffic by client IP</h2>
  <p class="help">Download is bytes from the real site to the client. Upload is bytes from the client to the real site. Counts survive restarts.</p>
  {{if not .Traffic}}<p class="empty">No traffic recorded yet.</p>{{end}}
  {{if .Traffic}}
  <div class="table-wrap">
  <table>
    <tr><th>Client</th><th>Today</th><th>Today down</th><th>Today up</th><th>Total</th><th>Total down</th><th>Total up</th></tr>
    {{range .Traffic}}
    <tr>
      <td class="name">{{.ClientKey}}</td>
      <td>{{.TodaySum}}</td><td>{{.TodayIn}}</td><td>{{.TodayOut}}</td>
      <td>{{.TotalSum}}</td><td>{{.TotalIn}}</td><td>{{.TotalOut}}</td>
    </tr>
    {{end}}
  </table>
  </div>
  {{end}}
</section>
</div>
</body>
</html>
{{end}}

{{define "account"}}
<!doctype html>
<html lang="en">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<title>Account · SNI DNS Proxy</title>
<style>{{template "css"}}</style>
</head>
<body>
<div class="shell">
{{template "top" .}}
<section class="card">
  <h2>Admin account</h2>
  <p class="help">Leave the password fields blank to keep the current password.</p>
  {{if .Error}}<p class="result bad">{{.Error}}</p>{{end}}
  {{if .Success}}<p class="result ok">{{.Success}}</p>{{end}}
  <form method="post" action="/account">
    <input type="hidden" name="csrf" value="{{.CSRF}}">
    <label for="username">Username</label>
    <input id="username" name="username" value="{{.Username}}" autocomplete="username" required>
    <label for="password" style="margin-top:12px">New password</label>
    <input id="password" name="password" type="password" autocomplete="new-password">
    <label for="password2" style="margin-top:12px">Confirm new password</label>
    <input id="password2" name="password2" type="password" autocomplete="new-password">
    <p style="margin-top:14px"><button type="submit">Save account</button></p>
  </form>
</section>
</div>
</body>
</html>
{{end}}
`
