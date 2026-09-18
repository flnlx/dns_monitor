# Internal implementation contract

All timestamps Unix milliseconds. JSON field names defined in internal/model/model.go. Percentages 0..100, latency milliseconds. No external frontend dependencies/CDN. API JSON errors {"error":"..."}.

- GET /api/state?range=24h|7d|30d -> {servers:ServerSummary[],config:Config,runtime:RuntimeStatus,service:{installed:bool,state:string,can_manage:bool},version:string,now:number,listen_restart_required:bool}
- POST /api/servers (Server input) -> Server; PUT /api/servers/{id} -> Server; DELETE -> {ok:true}
- GET /api/servers/{id}/history?range=24h|7d|30d -> {points:HistoryPoint[],metrics:Metrics}
- GET /api/servers/{id}/results?limit=100&before=timestamp -> {results:ProbeResult[]}; capped limit 200, desc timestamp/id
- POST /api/servers/{id}/probe -> {ok:true} queued within global concurrency; 409 when paused/disabled/already queued
- PUT /api/config (Config) -> {config:Config,restart_required:bool}; listen change effective next startup. concurrency=0 pauses, max_backoff_hours=0 disables backoff; 0..24 hours, concurrency 0..50.
- PUT /api/overrides -> Override {server_id,domain,type,verdict:"auto"|"clean"|"polluted",note}; applies persistently to server/domain/type until auto reset. Retain raw detected verdict separately. Domain must be valid.
- GET /api/service -> service; POST /api/service/{install|uninstall|start|stop|restart} -> {ok:true,message:string}; Windows elevation via service helper command. Do not install automatically.
- GET /api/export?server_id=...&range=... streams CSV of raw records.
- GET /api/session -> {token:string}; token in X-DNSMonitor-Token for all API except session and health; browser stores token only in memory; same-origin checks; login via optional app access key if remote. Endpoint /api/session requires Basic Auth (username admin) if key exists; served UI can prompt key through session fetch Authorization header. Key generated on first launch and written data/access-key.txt. Main prints path/key instructions, not values to logs. Local UI still requires key, safer consistent behavior with 0.0.0.0.

Store implementation owns internal/store; API desired: Open(path string)(*Store,error), Close()error; GetConfig()(model.Config,error), SaveConfig(model.Config)error; ListServers()([]model.Server,error), GetServer(id int64)(model.Server,error), SaveServer(model.Server)(model.Server,error), DeleteServer(id int64)error; SaveRound(model.Round)error; Summary(since,now int64)([]model.ServerSummary,error); History(serverID,since,now,stepMS int64)([]model.HistoryPoint,model.Metrics,error); Results(serverID int64,limit int,before int64)([]model.ProbeResult,error); SaveOverride(model.Override)error; Cleanup(before int64)error; WalkResults(serverID,since int64,fn func(model.ProbeResult)error)error. Store may add helpers and explain.

Monitor owns internal/monitor; depends store/model. New(st *store.Store,doggoPath string)*Monitor; Run(ctx context.Context); Wake(); Queue(serverID int64)error; Status()model.RuntimeStatus. Config obtained from store on wake/tick. ValidateAddress(address string)(canonical,protocol string,err error); ValidateConfig(*model.Config)error. Global concurrency includes reference lookups; per-server in-flight exclusion; bounded queue; no unbounded goroutines. DNS comparison uses enabled trusted servers except target itself, uses the TTL/history IPv4 policy below and must not treat missing references or failures as prefix-mismatch evidence. Save all actual trusted probe results/evidence as well as target results. Forced IPv4; no subprocess shell.

Main/root owns HTTP API, cmd/dns-monitor, Windows service manager, build/package docs/integration. UI agent owns internal/web only; provide embed FS (package web, var FS embed.FS containing assets/*). Monitor agent and store agent must not edit model or API_CONTRACT; coordinate changes with parent.

## v1.1 additions
- POST /api/probes queues all enabled servers; returns RefreshStatus {id,total,completed,pending:boolean,error?}; GET /api/state runtime.refresh exposes progress. Repeated requests join the active batch. Pending=false is terminal even when completed<total (e.g. pause).
- GET /api/servers/template and GET /api/servers/export download server configuration CSV, separate from raw /api/export. Overview summary CSV is generated from the exact current filtered/sorted UI snapshot.
- POST /api/servers/import/preview {csv} -> {snapshot,rows:[{row,server,key,duplicate,existing,duplicate_of_row?,error?}],errors}. POST /api/servers/import {csv,snapshot,decisions:[{row,action:skip|overwrite}]} -> {created,updated,skipped}. Preview snapshot stale ->409, validation errors ->400; atomic mutation. Rows are1-based data records, not physical lines.
- Current trusted servers are exempt from F/pollution even for historical records; ProbeResult.trusted flags current policy while historical raw Pollution/Override remain evidence. RawCSV now also includes trusted and effective_pollution. PUT /api/overrides polluted on trusted ->400.
- Default fresh configuration is youtube.com A; existing saved domains are not replaced. Rating sorting keeps pending at end.
## v1.2 additions
- Fresh configurations use www.youtube.com A. Existing configured domains stay intact.
- Config.reference_history_hours: finite float 0..720, default1, 0 disables historical matching. Old saved configurations default to1 when the field is absent; old API clients omitting it preserve its saved value.
- AnswerRecord: {value,ttl_seconds,observed_at,expires_at}; timestamps are Unix milliseconds. ProbeResult.records and Reference.records contain per-IP freshness/history evidence. ProbeResult.compared_at records the actual classification instant; ProbeResult.policy_version=2 for new probes; effective_pollution is the current policy/override/trust verdict, while pollution remains original detected evidence.
- Pollution states: matched (all fresh), clean (fresh/history match or manual clear), suspicious (unseen IPv4s all match a reference /16), polluted (at least one unseen IPv4 also misses all reference /16s), unknown (insufficient evidence). The worst IP wins. Prefixes are per domain+A, from fresh plus in-window historical trusted observations.
- Grades: F overrides E, then performance A-D/pending. Trusted servers are exempt from E/F. Old pre-v2 automatic F becomes effectively unknown; original evidence and manual overrides are retained.
- Trust history is bounded and persistent. Removing trust, disabling, or changing an address clears source history; internal TrustEpoch prevents old in-flight observations repopulating it. This epoch is not an API/CSV field.
- Raw CSV appends records_json, policy_version and compared_at, and effective_pollution comes from the same store policy as the UI. Overview CSV still exports exactly the filtered/sorted summary.
