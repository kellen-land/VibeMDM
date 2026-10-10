package agent

import (
	"context"
	"encoding/json"
	"html/template"
	"net"
	"net/http"
	"time"

	"github.com/vibemdm/vibemdm/internal/model"
)

// SelfServicePort is the local port the daemon's Self Service portal listens on.
const SelfServicePort = "127.0.0.1:8571"

var ssPage = template.Must(template.New("ss").Parse(`<!doctype html>
<html lang="en"><head><meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1">
<title>{{.Org}} Self Service</title>
<style>
:root{--bg:#0a0e10;--panel:#11171a;--line:#222b30;--ink:#e6edf0;--muted:#8a97a0;--teal:#0fb5ae;--teal-b:#22c3b8}
@media (prefers-color-scheme:light){:root{--bg:#f5f7f8;--panel:#fff;--line:#dde3e6;--ink:#1a2226;--muted:#5d6a72}}
*{box-sizing:border-box}body{margin:0;background:var(--bg);color:var(--ink);font:14px/1.5 -apple-system,BlinkMacSystemFont,"SF Pro Text",system-ui,sans-serif}
main{max-width:860px;margin:0 auto;padding:40px 20px}
h1{font-size:22px;margin:0}h1 b{color:var(--teal-b)}p.sub{color:var(--muted);margin:6px 0 28px}
.grid{display:grid;grid-template-columns:repeat(auto-fill,minmax(250px,1fr));gap:14px}
.card{background:var(--panel);border:1px solid var(--line);border-radius:12px;padding:16px;display:flex;flex-direction:column;gap:8px}
.card h2{font-size:15px;margin:0}.card .d{color:var(--muted);font-size:12.5px;flex:1}
button{background:var(--teal);border:0;color:#04211f;font-weight:600;padding:8px 14px;border-radius:8px;cursor:pointer;font-size:13px}
button:disabled{opacity:.5;cursor:default}
.status{font-size:12px;color:var(--muted);min-height:16px}
.ok{color:#3fb950}.err{color:#f85149}
.empty{border:1px dashed var(--line);border-radius:12px;padding:40px;text-align:center;color:var(--muted)}
</style></head><body><main>
<h1><b>{{.Org}}</b> Self Service</h1>
<p class="sub">Software and tasks your administrator has made available for this Mac.</p>
<div id="list" class="grid"></div>
<script>
async function load(){
  const r = await fetch('/api/items'); const d = await r.json();
  const el = document.getElementById('list');
  if(!d.items || !d.items.length){ el.outerHTML = '<div class="empty">Nothing is available right now. Check back later.</div>'; return; }
  el.innerHTML='';
  for(const it of d.items){
    const c=document.createElement('div'); c.className='card';
    c.innerHTML='<h2></h2><div class="d"></div><button></button><div class="status"></div>';
    c.querySelector('h2').textContent=it.name;
    c.querySelector('.d').textContent=it.description||'';
    const b=c.querySelector('button'); b.textContent=it.button_text||'Install';
    const st=c.querySelector('.status');
    b.onclick=async()=>{ b.disabled=true; st.textContent='Running…'; st.className='status';
      try{ const rr=await fetch('/api/run',{method:'POST',headers:{'Content-Type':'application/json'},body:JSON.stringify({kind:it.kind,id:it.id})});
        const out=await rr.json();
        if(rr.ok && out.ok){ st.textContent='Done'; st.className='status ok'; }
        else { st.textContent=out.error||'Failed'; st.className='status err'; }
      }catch(e){ st.textContent='Failed: '+e; st.className='status err'; }
      b.disabled=false; };
    el.appendChild(c);
  }
}
load();
</script></main></body></html>`))

// serveSelfService hosts the local Self Service portal (loopback only).
func (d *Daemon) serveSelfService(ctx context.Context) {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		ssPage.Execute(w, map[string]string{"Org": d.cfg.OrgName})
	})
	mux.HandleFunc("GET /api/items", func(w http.ResponseWriter, r *http.Request) {
		items, org, err := d.client.SelfServiceList(r.Context())
		w.Header().Set("Content-Type", "application/json")
		if err != nil {
			w.WriteHeader(502)
			json.NewEncoder(w).Encode(map[string]string{"error": "cannot reach the management server"})
			return
		}
		if items == nil {
			items = []model.SelfServiceItem{}
		}
		json.NewEncoder(w).Encode(map[string]any{"items": items, "org": org})
	})
	mux.HandleFunc("POST /api/run", func(w http.ResponseWriter, r *http.Request) {
		var req struct{ Kind, ID string }
		json.NewDecoder(r.Body).Decode(&req)
		w.Header().Set("Content-Type", "application/json")
		pol, app, err := d.client.SelfServiceRun(r.Context(), req.Kind, req.ID)
		if err != nil {
			w.WriteHeader(502)
			json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
			return
		}
		ok := true
		d.runLocked(ctx, func(ctx context.Context) {
			switch {
			case pol != nil:
				sub := d.exec.RunPolicy(ctx, *pol)
				ok = sub.Status == "completed"
			case app != nil:
				d.exec.InstallApp(ctx, *app)
			}
		})
		json.NewEncoder(w).Encode(map[string]any{"ok": ok})
	})
	srv := &http.Server{Addr: SelfServicePort, Handler: mux, ReadHeaderTimeout: 10 * time.Second}
	ln, err := net.Listen("tcp", SelfServicePort)
	if err != nil {
		d.log.Warn("self service portal not started", "err", err)
		return
	}
	go func() {
		<-ctx.Done()
		srv.Close()
	}()
	d.log.Info("self service portal available", "url", "http://"+SelfServicePort)
	srv.Serve(ln)
}
