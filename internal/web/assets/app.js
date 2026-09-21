'use strict';

(() => {
  const $ = (selector, root = document) => root.querySelector(selector);
  const $$ = (selector, root = document) => [...root.querySelectorAll(selector)];
  const langKey = 'dns-monitor.lang.v1';
  const zhTitle = 'DNS Monitor · 本地 DNS 观测台';
  const detectLang = () => {
    try {
      if (typeof localStorage === 'undefined') return 'zh';
      const stored = localStorage.getItem(langKey);
      if (stored === 'en' || stored === 'zh') return stored;
    } catch { /* storage unavailable */ }
    if (typeof navigator === 'undefined') return 'zh';
    return String(navigator.language || '').toLowerCase().indexOf('en') === 0 ? 'en' : 'zh';
  };
  let currentLang = detectLang();
  const I18N = {
    en: {
      'docTitle': 'DNS Monitor · Local DNS Observatory',
      // --- login screen ---
      'login.artH1': 'Every resolution,<br>deserves to be seen.',
      'login.artP': 'Continuously observe DNS availability, response speed, and resolution quality.',
      'login.brandSmall': 'Local DNS Observatory',
      'login.h2': 'Connect to your observatory',
      'login.muted': 'Enter the local access key to view and manage DNS monitoring.',
      'login.keyLabel': 'Access key',
      'login.keyPlaceholder': 'Paste the key from data/access-key.txt',
      'login.help': 'The key is stored in <code>data/access-key.txt</code> inside the program folder. Open that file on the machine running the program.',
      'login.submit': 'Enter the observatory ',
      'login.footnote': 'Session credentials live in this tab; refreshing keeps you signed in. Click “Sign out” in the top-right to end the session.',
      'login.bottom': 'Local · IPv4 · Windows portable',
      // --- shared ---
      'close': 'Close',
      'cancel': 'Cancel',
      'confirm': 'Confirm',
      'langToggle.title': 'Switch language',
      'brand.aria': 'DNS Monitor home',
      'brand.small': 'Local DNS Observatory',
      'logout': 'Sign out',
      // --- navigation ---
      'nav.label': 'Workspace',
      'nav.aria': 'Main navigation',
      'nav.overview': 'Overview',
      'nav.config': 'Probing setup',
      'nav.service': 'Windows service',
      'nav.guide': 'User guide',
      'runtime.waiting': 'Waiting to connect',
      'runtime.retention': 'Data retention',
      'runtime.retentionValue': '30 days',
      'runtime.network': 'Probe network',
      // --- topbar ---
      'refresh.title': 'Probe all enabled DNS servers now and load fresh results',
      'refresh.label': 'Refresh',
      'addServer': 'Add DNS',
      'restartNotice': 'Listen address saved. Restart the program or Windows service for the new address to take effect.',
      'page.overview': 'Overview',
      'page.config': 'Probing setup',
      'page.service': 'Windows service',
      'page.guide': 'User guide',
      // --- overview ---
      'intro.h2': 'Every DNS performs, and the records show it.',
      'intro.p': 'Send real resolution requests on schedule and record availability, latency, and query results. Trusted-DNS comparison and manual review together surface resolution quality.',
      'intro.chip': 'Continuous monitoring<small>local storage / last 30 days</small>',
      'overview.serversTitle': 'DNS servers ',
      'overview.sectionP': 'Performance metrics and status blocks follow the selected range; blocks preserve the resolution quality and rating at the time.',
      'overview.transferLabel': 'DNS server inventory',
      'overview.importServers': 'Import servers from CSV',
      'overview.exportServers': 'Export servers CSV ↗',
      'overview.downloadTemplate': 'Download template',
      'overview.howItWorks': 'How verdicts work →',
      'overview.exportCurrent': 'Export current results',
      'range.aria': 'Statistic range',
      'range.24h': '24 hours',
      'range.7d': '7 days',
      'range.30d': '30 days',
      // --- overview cards & explanation (dynamic) ---
      'ov.cardServers': 'Monitored servers',
      'ov.cardServersCaption': '{enabled} enabled · {trusted} trusted',
      'ov.cardOnline': 'Recent queries OK',
      'ov.cardOnlineCaption': '{failed} failing · {awaiting} awaiting first probe',
      'ov.cardAnomaly': 'Resolution anomalies',
      'ov.cardAnomalyCaption': '{f} F · {e} E · {unknown} pending',
      'ov.cardLatency': 'Average server latency',
      'ov.cardLatencyCaption': 'avg of {responding} responding servers · {samples} samples',
      'ov.ratingExplanation': 'Latency, availability, and success rate are aggregated over the selected range; current resolution quality comes from the latest formal probe and the current rating uses the last {window} minutes. A pending verdict no longer carries an old clean rating; click a domain for reasons and manual review.',
      // --- search & filters ---
      'search.placeholder': 'Search name, provider, or address',
      'search.aria': 'Search DNS servers',
      'filter.allProtocols': 'All protocols',
      'filter.allPollution': 'All resolution states',
      'filter.allGrades': 'All ratings',
      'filter.all': 'All',
      'filter.hint': 'Multi-select',
      'filter.protocolAria': 'Filter by protocol (multi-select)',
      'filter.protocolGroup': 'Protocol',
      'filter.pollutionAria': 'Filter by resolution state (multi-select)',
      'filter.pollutionGroup': 'Resolution state',
      'filter.gradeAria': 'Filter by rating (multi-select)',
      'filter.gradeGroup': 'Rating',
      // --- table ---
      'table.serverProvider': 'Server / provider',
      'table.status': 'Status',
      'table.protocol': 'Protocol',
      'table.avgLatency': 'Avg latency ',
      'table.availability': 'Availability ',
      'table.successRate': 'Success rate ',
      'table.currentResolution': 'Current resolution',
      'table.currentRating': 'Current rating ',
      'table.actions': 'Actions',
      'table.footerHint': 'Reads state every 15 s · refresh at the top re-probes',
      // --- config ---
      'cfg.scheduleTitle': 'Probe scheduling',
      'cfg.scheduleSub': 'Balance sampling frequency against resource usage.',
      'cfg.intervalLabel': 'Base probe interval',
      'cfg.intervalHelp': 'Servers are staggered automatically; domains are queried in sequence.',
      'cfg.timeoutLabel': 'Per-query timeout',
      'cfg.timeoutHelp': 'Timeouts count as query failures.',
      'cfg.concurrencyLabel': 'Probe concurrency',
      'cfg.concurrencyHelp': 'Default 2; set to 0 to pause all probing.',
      'cfg.ttlLabel': 'Trusted query cache cap',
      'cfg.ttlHelp': 'Reuse never exceeds the real DNS TTL; expiry does not trigger queries on its own.',
      'cfg.historyLabel': 'Trusted history retention',
      'cfg.historyHelp': 'How long expired trusted answers stay usable as historical references.',
      'cfg.backoffSwitch': 'Smart offline backoff',
      'cfg.backoffSwitchHelp': 'After repeated failures, probe gaps grow longer and return to normal once recovered.',
      'cfg.backoffLabel': 'Maximum backoff time',
      'cfg.backoffHelp': 'Range 0-24; default 1 hour. Set to 0 to disable backoff.',
      'cfg.ratingTitle': 'Current status & rating',
      'cfg.ratingSub': 'Shorten observation and waiting so recent changes surface faster.',
      'cfg.windowLabel': 'Rating observation window',
      'cfg.windowHelp': 'Used for current status and rating; a window without samples waits for a new probe while historical charts still follow the selected range.',
      'cfg.samplesLabel': 'Minimum rating samples',
      'cfg.samplesHelp': 'Default 3 formal samples; auxiliary trusted queries do not count.',
      'cfg.coverageLabel': 'Minimum rating coverage',
      'cfg.coverageHelp': 'Default 5 minutes; 0 adds no extra coverage wait and cannot exceed the observation window.',
      'cfg.ratingHelp': 'Current resolution quality updates with the latest formal round in the window; without recent samples a new probe is awaited. With insufficient trusted references or answers the state shows pending and never falls back to an old clean rating; hover to see the pending reason. With a single domain and a 5-minute cycle a rating usually forms in about 10 minutes.',
      'cfg.domainsTitle': 'Probe domains',
      'cfg.domainsSub': 'Each probe round queries all of the domains below.',
      'cfg.domainCol': 'Domain',
      'cfg.typeCol': 'Record type',
      'cfg.addDomain': '＋ Add domain',
      'cfg.domainsHelp': 'Enter full domain names, e.g. www.youtube.com. Fresh installs default to www.youtube.com; existing custom configs are kept. Keep at least one. IPv4 transport only uses IPv4 server addresses.',
      'cfg.localTitle': 'Local access',
      'cfg.localSub': 'Controls how browsers and services reach the console.',
      'cfg.listenLabel': 'Listen address',
      'cfg.listenHelp': 'Default 0.0.0.0:8080 allows LAN access. Use 127.0.0.1:8080 for local-only. Requires a restart.',
      'cfg.saveTitle': 'Start with the next cycle',
      'cfg.saveHint': 'Except for the listen address, probe config applies as soon as it is saved.',
      'cfg.saveButton': 'Save config',
      'cfg.tip1Title': 'Small concurrency, lighter load',
      'cfg.tip1': 'Start with the defaults (concurrency 2, 300 s cycle). Enable backoff for long-offline servers to reduce wasted requests.',
      'cfg.tip2Title': 'Raw records roll over after 30 days',
      'cfg.tip2': 'Expired records are deleted automatically. For long-term archives use “Export all raw records” in the guide or export raw records from a server’s details.',
      'unit.seconds': 'seconds',
      'unit.minutes': 'minutes',
      'unit.hours': 'hours',
      'unit.samples': 'samples',
      'unit.count': 'servers',
      // --- service page ---
      'svc.title': 'Run as a Windows service',
      'svc.sub': 'Keep monitoring running after you close the browser or sign out.',
      'svc.current': 'Current service state',
      'svc.notice': 'Service install and management may trigger a Windows admin prompt. Stopping or restarting the service briefly disconnects the web UI.',
      'svc.tip1Title': 'Before moving the portable folder',
      'svc.tip1': 'If a service is installed, stop and uninstall it first, then quit the program and move the whole folder. Reinstall at the new location so the service does not keep pointing at the old path.',
      'svc.tip2Title': 'Works without the service',
      'svc.tip2': 'Run dns-monitor.exe directly to monitor; config, database, and the access key all live in the program folder.',
      // --- guide ---
      'guide.heroH2': 'Your DNS, observed over the long term.',
      'guide.heroP': 'DNS Monitor is a portable monitoring tool for Windows. By sending resolution requests on a schedule it helps you pick stable, fast, and trustworthy IPv4 DNS.',
      'guide.g1Title': 'Getting started',
      'guide.g1': 'Add servers and fill in the provider and address. Fresh installs probe www.youtube.com by default; you can also import a server CSV and preview it, choosing skip or overwrite per duplicate. Supported protocols follow address validation; common forms are below.',
      'guide.g1Help': 'Encrypted protocols may use domain addresses, but connections only use IPv4. Incompatible address formats are flagged clearly on save.',
      'guide.g2Title': 'How resolution quality is judged',
      'guide.g2': 'Check the “trusted DNS” boxes you recognize. The system merges these servers’ IPv4 answers per domain and record type: answers within their TTL are fresh references, and expired answers remain usable as historical references within the configured window. Only trusted DNS can extend the reference set.',
      'pollution.matched': 'Matches reference',
      'pollution.matchedDesc': 'All returned IPs appear in fresh references.',
      'pollution.clean': 'Clean',
      'pollution.cleanDesc': 'All IPs have appeared in valid references, with at least one matching only recent history; manually clearing an anomaly also shows clean.',
      'pollution.suspicious': 'Suspicious · E',
      'pollution.suspiciousDesc': 'Some IP is not in valid references, but all unseen IPs match the effective references on their first two octets.',
      'pollution.polluted': 'Polluted · F',
      'pollution.pollutedDesc': 'At least one IP is neither in valid references nor matching on the first two octets. This is a rule-based suspicion, not proof of pollution.',
      'guide.g2P2': 'Matching the first two octets is a /16 match, e.g. 57.144.152.1 and 57.144.186.1. IPs are compared one by one and the whole answer takes the worst grade; with no valid reference, a query failure, or no IPv4 answer the state is pending. The current fresh and historical sets are used directly without waiting for every trusted DNS; an offline source or expired reference never blocks the rest. Periodic trusted probing keeps the set filled, and replenishment is only attempted when the set is empty.',
      'guide.g2P3': 'Trust only exempts the pollution verdict; it does not mean the server is usable. A trusted DNS itself always shows “Trusted / clean” and is never graded E or F. Clicking a domain mark in the results lets you <strong>confirm pollution, mark it clean, or restore automatic judgment</strong>. Manual verdicts persist for that server, domain, and record type, while the raw evidence is kept. Legacy automatic polluted records are kept as evidence only and play no part in current E/F ratings.',
      'guide.g3Title': 'Reading the metrics',
      'metric.availability': 'Availability',
      'metric.availabilityDesc': 'Response share weighted by sampled coverage time; an error response still proves reachability, and offline backoff is not counted as success.',
      'metric.success': 'Query success rate',
      'metric.successDesc': 'Share of completed resolutions; timeouts and resolution errors count as failures.',
      'metric.latency': 'Average / P95 latency',
      'metric.latencyDesc': 'Average response speed of successful queries and the latency level reached by 95% of them.',
      'metric.coverage': 'Coverage',
      'metric.coverageDesc': 'How well the selected time range is sampled; don’t rush ratings while monitoring is sparse.',
      'guide.g4Title': 'Ratings & data retention',
      'guide.g4P1': 'The overview’s current resolution quality comes from the latest formal round in the recent window; without samples in the window, or when the latest result is pending, an old clean state is not kept. If every query in the latest formal round fails, the current rating immediately shows “Unavailable” — trusted DNS included — and sorts to the bottom in every direction; ratings return to normal rules once a round has successes. A window without samples still shows “Pending”. The current rating uses a configurable recent window (default 60 minutes) and scores after 3 formal samples and 5 minutes of valid coverage.',
      'guide.g4P2': 'The quality score weighs availability (45%), query success rate (35%), and P95 latency (20%). A ≥ 95, B ≥ 85, C ≥ 70, D &lt; 70; matches or clean state follow that performance score, suspicious rates E, and suspected or manually confirmed pollution rates F first.',
      'guide.g4P3': 'P95 latency ≤ 50 ms earns the full latency share, ≥ 2000 ms earns zero, decreasing linearly in between. With too few samples, insufficient coverage, or insufficient resolution evidence the state shows “Pending”; hover for the reason and progress. Detail metrics and charts keep the selected historical range; probes that backoff skipped are never faked as successes.',
      'guide.g4P4': 'Raw records from every actual probe roll over after 30 days and are deleted automatically. Charts are time-aggregated. “Export current results” in the overview keeps the current range, filters, and sorting; the entry below exports raw records for all servers in the selected range.',
      'guide.exportAll': 'Export all raw records ↗',
      'guide.portableTitle': 'The whole folder is your workspace.',
      'guide.portableP': 'Quit the program, then copy the entire folder to migrate. If a Windows service is installed, stop and uninstall it first, move, then reinstall. Never copy the database while the program is running.',
      // --- footer ---
      'footer.subtitle': 'Local DNS Observatory',
      'footer.note': 'Your data belongs to you; monitoring stays local.',
      'export.title': 'Export',
      // --- detail dialog ---
      'dtl.rangeAria': 'Detail statistics range',
      'dtl.historyHeading': 'Historical performance',
      'dtl.statusAria': 'Resolution quality and rating history',
      'dtl.chart1Title': 'Available & success rate',
      'dtl.legendAvail': 'Availability',
      'dtl.legendSuccess': 'Success rate',
      'dtl.chart1Aria': 'Availability and query success rate history',
      'dtl.chart2Title': 'Response latency <small>ms</small>',
      'dtl.legendAvg': 'Average',
      'dtl.chart2Aria': 'Average and P95 response latency history',
      'dtl.probe': 'Probe now',
      'dtl.export': 'Export raw records ↗',
      'dtl.edit': 'Edit',
      'dtl.rawHeading': 'Raw probe results',
      'dtl.rawHint': 'Click a domain to view answers, trusted references, and the verdict menu.',
      'dtl.refreshRecords': '↻ Refresh records',
      'dtl.probeTime': 'Probe time',
      'dtl.domainType': 'Domain / type',
      'dtl.queryResult': 'Query result',
      'dtl.latency': 'Latency',
      'dtl.resolution': 'Resolution',
      'dtl.loadMore': 'Load more',
      // --- server dialog ---
      'srv.nameLabel': 'Name',
      'srv.namePlaceholder': 'e.g. Cloudflare primary node',
      'srv.providerLabel': 'Provider',
      'srv.providerPlaceholder': 'e.g. Cloudflare',
      'srv.addressLabel': 'Server address',
      'srv.addressPlaceholder': '1.1.1.1 or https://dns.example.com/dns-query',
      'srv.addressHelp': 'Accepts supported AdGuard Home upstream syntax; IPv4 only.',
      'srv.notesLabel': 'Notes',
      'srv.notesPlaceholder': 'Line, region, or purpose (optional)',
      'srv.enableTitle': 'Enable monitoring',
      'srv.enableHelp': 'Samples automatically per the current probe config after saving.',
      'srv.trustedTitle': 'Mark as trusted user DNS',
      'srv.trustedHelp': 'Use this server’s successful answers as pollution references for other DNS.',
      'srv.save': 'Save server',
      // --- verdict dialog ---
      'vd.title': 'Manual review',
      'vd.help': 'The choice persists for this server, domain, and record type; the original probe verdict and evidence are kept.',
      'vd.noteLabel': 'Review note (optional)',
      'vd.notePlaceholder': 'e.g. Confirmed a normal CDN regional difference',
      'vd.polluted': 'Confirm pollution',
      'vd.clean': 'Mark clean',
      'vd.auto': 'Restore automatic',
      // --- import dialog ---
      'imp.title': 'Import DNS servers',
      'imp.subtitle': 'Preview each row, then decide how to handle duplicate servers.',
      'imp.fileLabel': 'Choose CSV file',
      'imp.template': 'Download template ↗',
      'imp.preview': 'Preview again',
      'imp.help': 'UTF-8 with optional BOM; files up to 900 KiB and 1000 records, full import request under 1 MiB. The template has two example rows; replace them as needed. Headers are <code>name,provider,address,enabled,trusted,notes</code>. Preview row numbers exclude the header.',
      'imp.bulkLabel': 'Duplicate handling',
      'imp.overwriteAll': 'Overwrite all',
      'imp.skipAll': 'Skip all',
      'imp.row': 'Row',
      'imp.nameProvider': 'Name / provider',
      'imp.address': 'Server address',
      'imp.previewResult': 'Preview result',
      'imp.importAction': 'Import action',
      'imp.detailHelp': 'New servers are created automatically. Duplicates default to skip; with overwrite only the columns provided in the CSV change, and missing columns keep their current values. An empty notes column clears notes; blank enabled/trusted columns default to true/false respectively. Check the enabled and trusted marks in the preview. In-file duplicates point to the data row they reference.',
      'imp.commit': 'Confirm import',
      // --- dynamic state & status text ---
      'time.neverProbed': 'Not probed yet',
      'state.matched': 'Matches reference',
      'state.clean': 'Clean',
      'state.suspicious': 'Suspicious',
      'state.polluted': 'Polluted',
      'state.unknown': 'Pending',
      'state.confirmedPolluted': 'Confirmed polluted',
      'state.trustedGood': 'Trusted / clean',
      'state.noData': 'No data',
      'state.unavailable': 'Unavailable',
      'state.pending': 'Pending',
      'state.quality': 'resolution quality',
      'state.rating': 'rating',
      'trust.badge': '✓ Trusted / clean',
      'trust.title': 'User-designated trusted DNS; exempt from pollution verdicts. Performance ratings still require enough sampling.',
      'hint.matched': 'All comparable returned IPs hit fresh trusted references within TTL.',
      'hint.clean': 'Returned IPs hit valid trusted history, or the anomaly was manually cleared.',
      'hint.suspicious': 'Some IP missed valid trusted references, but every unseen IP matches the reference addresses on its first two octets.',
      'hint.polluted': 'Some IP missed valid trusted references and its first two octets do not match either; this is a suspicion, not proof.',
      'hint.unknown': 'The query failed, no comparable IPv4 answer, or no valid trusted reference; a verdict is not possible yet.',
      'hint.confirmedPolluted': 'Manually confirmed pollution; change it from the domain menu.',
      'grade.A': 'Excellent: overall score ≥ 95, from availability, query success rate, and P95 latency.',
      'grade.B': 'Good: overall score ≥ 85 and < 95, from availability, query success rate, and P95 latency.',
      'grade.C': 'Fair: overall score ≥ 70 and < 85, from availability, query success rate, and P95 latency.',
      'grade.D': 'Poor: overall score < 70, from availability, query success rate, and P95 latency.',
      'grade.E': 'Resolution results are suspicious, rating E takes priority; unseen IPs match the trusted reference on the first two octets.',
      'grade.F': 'Suspected or manually confirmed pollution; rating F takes priority.',
      'grade.unavailable': 'Unavailable: every query in the latest formal round failed (connectivity failure, timeout, or resolution error); trusted marks do not exempt. Ratings resume after recovery.',
      'grade.pending': 'Waiting for enough formal samples and valid coverage; thresholds are adjustable in the probe config.',
      'grade.windowPrefix': 'Last {minutes} minutes. ',
      'grade.noSamples': 'No formal samples in the rating window; waiting for the next probe round.',
      'grade.qualityUnknown': 'The latest formal probe’s resolution quality is pending; see the reason in the domain record.',
      'grade.insufficientSamples': 'Formal samples have not yet reached the rating threshold.',
      'grade.insufficientCoverage': 'Valid monitoring coverage has not yet reached the rating threshold.',
      'grade.pendingSummary': '{reason}samples {samples}/{minSamples}, coverage {coverage}/{minCoverage} minutes.',
      // --- status history ---
      'status.noSnapshots': ': no data (no formal probe snapshots in this window)',
      'status.distItem': '{label} ×{count}',
      'status.summary': '{heading}: worst record {label}. {count} probe snapshots; {distribution}.',
      'status.latest': 'Last snapshot of this window ({time}): resolution quality {pollution}, rating {grade}{trustedSuffix}.',
      'status.trustedSuffix': ', then trusted as user DNS',
      'status.latestDetail': 'Last {window} min: {samples} samples, success rate {rate}, average latency {latency}.',
      'status.empty': 'No history yet',
      'status.ariaHistory': 'View resolution quality history of {name}',
      'status.historyCaption': '{range} history ↗',
      'range.24hShort': '24h',
      'range.7dShort': '7d',
      'range.30dShort': '30d',
      'status.statsEmpty': 'No status history yet.',
      'status.historyHeading': 'Resolution quality & rating history',
      'status.pastNow': 'past → now',
      'status.cellHelp': 'Each cell covers {interval}, showing the worst state in that window’s probe snapshots; hover, focus, or click for details.',
      'status.qualityTitle': 'Resolution quality',
      'status.ratingTitle': 'Rating at probe time',
      'status.inspect': 'Select a block to see the status distribution and the rating metrics at that time.',
      'status.note': 'Only states captured when a formal probe finished are recorded; they don’t imply the state persisted for the whole window. Ratings use the observation window, config, and trusted settings at that time; later changes never rewrite history. Periods before an upgrade or with no probe show as no data.',
      // --- dynamic interactions ---
      'session.expired': 'Session expired; enter the access key again.',
      'api.failed': 'Request failed (HTTP {status})',
      'login.badKey': 'The access key is wrong; check data/access-key.txt.',
      'login.unreachable': 'Cannot reach the console; make sure the program is running.',
      'login.connFailed': 'Connection failed; make sure DNS Monitor is running.',
      'logout.confirmTitle': 'Sign out of the console',
      'logout.confirmText': 'There are unsaved probe config changes; signing out will discard them.',
      'logout.confirmButton': 'Sign out',
      'refresh.updatedAt': 'Updated at {time}',
      'refresh.runtimeHint': 'Recent runtime hint: {message}',
      'refresh.failed': 'Couldn’t update data: {reason}',
      'refresh.connLost': 'connection lost; check that the program or Windows service is running',
      'refresh.probing': 'Probing {done}/{total}',
      'refresh.label2': 'Refresh',
      'refresh.pausedTitle': 'Concurrency is 0; probing is paused. Adjust config to refresh manually.',
      'refresh.batchTitle': 'Waiting for the in-flight probe round',
      'refresh.normalTitle': 'Probe all enabled DNS servers now and load fresh results',
      'probe.roundError': 'This round: {error}',
      'probe.doneRound': 'Refresh done: {count} DNS servers completed a new probe round.',
      'probe.running': 'Re-probing DNS: {done} / {total} done, {remaining} remaining.',
      'probe.pausedToast': 'Probing is paused; set concurrency > 0 and retry',
      'probe.batchChanged': 'The probe batch changed on the server; the list is up to date. Click refresh again.',
      'probe.progressUnreadable': 'Can’t read probe progress right now; the background task may still be running. Refresh state once the connection is back.',
      'probe.doneToast': 'All enabled DNS servers finished a new probe round.',
      'probe.nothingToDo': 'No enabled DNS servers to probe',
      'runtime.pausedTitle': 'Probing paused',
      'runtime.runningTitle': 'Scheduler running',
      'runtime.pausedDesc': 'Concurrency is set to 0; saved records remain visible.',
      'runtime.runningDesc': '{active} / {total} concurrent · base interval {interval} s',
      'filter.selected': '{count} selected',
      'filter.gradeA': 'A · Excellent',
      'filter.gradeB': 'B · Good',
      'filter.gradeC': 'C · Average',
      'filter.gradeD': 'D · Poor',
      'filter.gradeE': 'E · Suspicious',
      'filter.gradeF': 'F · Polluted',
      'filter.gradePending': 'Pending',
      'filter.gradeUnavailable': 'Unavailable',
      'server.statusStopped': 'Stopped',
      'server.statusAwait': 'Awaiting probe',
      'server.statusOk': 'OK',
      'server.statusDown': 'Failing',
      'server.viewHistory': 'View history of {name}',
      'server.trustedMark': 'Trusted',
      'server.lastProbe': 'Last probe: {time}',
      'server.edit': 'Edit',
      'server.delete': 'Delete',
      'server.editAria': 'Edit {name}',
      'server.deleteAria': 'Delete {name}',
      'server.emptyFilteredTitle': 'No servers match',
      'server.emptyFilteredText': 'Adjust the search or filters to keep exploring other servers.',
      'server.emptyAllTitle': 'Start with your first DNS',
      'server.emptyAllText': 'Add servers and trusted DNS and the observatory starts sampling automatically.',
      'server.count': 'Showing {shown} of {total} servers',
      'navigate.leaveTitle': 'Leave probe config',
      'navigate.leaveText': 'Changes are not saved yet; leaving will discard them.',
      'navigate.leaveButton': 'Discard changes',
      'config.loaded': 'Current config loaded',
      'config.dirty': 'Unsaved changes',
      'config.saved': 'Config saved',
      'config.restartToast': 'Config saved; the listen address takes effect after a restart.',
      'config.pausedToast': 'Config saved; all probing is paused.',
      'config.savedToast': 'Probe config saved',
      'config.number': 'Enter a number within {range}.',
      'config.value': 'Enter a value within {range}.',
      'config.integer': 'Enter an integer within {range}.',
      'config.decimal': 'Enter a value within {range}, with at most 1 decimal.',
      'config.coverage': 'Minimum coverage cannot exceed the rating window ({window} min).',
      'server.editTitle': 'Edit DNS server',
      'server.addTitle': 'Add DNS server',
      'server.updatedToast': 'Server config updated',
      'server.addedToast': 'DNS server added',
      'server.deleteTitle': 'Delete DNS server',
      'server.deleteText': 'Delete “{name}”?\nThe server and its historical data will be permanently removed. This cannot be undone.',
      'server.deleteButton': 'Delete server',
      'server.deletedToast': 'Server deleted',
      'server.domainAria': 'Probe domain',
      'server.typeAria': 'Record type',
      'server.removeDomainAria': 'Remove this probe domain',
      'server.minOneDomain': 'Keep at least one probe domain',
      'detail.loadingHistory': 'Loading status history…',
      'detail.loadingRecords': 'Loading raw records…',
      'detail.recent': 'Recent {time}',
      'detail.trustedTag': 'Trusted user DNS',
      'detail.enabledTag': 'Enabled',
      'detail.stoppedTag': 'Stopped',
      'detail.probeWaiting': 'Waiting for probe results…',
      'detail.probeNow': 'Probe now',
      'detail.currentRating': 'Current rating',
      'detail.metricsTitle': 'Availability',
      'detail.metricsSuccess': 'Query success rate',
      'detail.metricsP95': 'P95 latency',
      'detail.metricsSamples': 'Raw samples',
      'detail.metricsCoverage': 'Coverage',
      'detail.historyFailed': 'Failed to load status history; retry.',
      'detail.historyError': 'History load failed: {error}',
      'detail.resultsError': 'Raw records load failed: {error}',
      'detail.rcodeError': 'Error response',
      'detail.rcodeNone': 'No response',
      'detail.anomalyAria': 'Resolution anomaly marker',
      'detail.emptyResults': 'No probe records yet. Enable the server and wait for the next round, or click “Probe now”.',
      'detail.resultsCount': 'Showing {count} records · newest first',
      'detail.probeQueued': 'Queued for probing; waiting for a fresh result',
      'detail.probeWritten': 'New probe results written and shown',
      'detail.probePaused': 'Probing is paused or the server is disabled; check config and retry.',
      'detail.probeProgressFail': 'Can’t read new probe progress. The background task may still be running; update records once connected.',
      'detail.chartNote': 'Charts and metrics follow the selected range; blanks mean no sample. {samples} samples in range. Current resolution quality: {quality}; current rating window: last {window} min.',
      'detail.chartEmpty': 'No samples yet; data will appear here',
      'detail.chartAria': '{union} history, {count} sampled windows. See the summary metrics above.',
      'detail.chartSeriesAvail': 'Availability',
      'detail.chartSeriesSuccess': 'Success rate',
      'detail.chartSeriesP95': 'P95',
      'detail.chartSeriesAvg': 'Average',
      'detail.chartSamples': '{count} samples',
      'evid.noAnswer': 'No answers returned',
      'evid.ttl': ' · TTL {ttl}s',
      'evid.fresh': ' · fresh then',
      'evid.historical': ' · recent historical',
      'verdict.trustedReason': 'This server is a user-designated trusted DNS; its resolution quality is always marked as clean.',
      'verdict.legacyReason': 'Legacy automatic verdict; evidence retained but not used in current E/F ratings.',
      'verdict.noReason': 'There isn’t enough evidence for a verdict yet.',
      'verdict.manual': 'Manual',
      'verdict.auto': 'Automatic',
      'verdict.original': 'Original: ',
      'verdict.legacySuffix': ' (legacy rules)',
      'verdict.answersTitle': 'Answers from the probed server',
      'verdict.refTitle': 'Trusted references ({count})',
      'verdict.refOk': 'OK',
      'verdict.refFail': 'Failed',
      'verdict.rawRef': 'Reference raw output',
      'verdict.rawDoggo': 'View doggo raw output',
      'verdict.noRef': 'This record has no trusted reference. Enable a trusted user DNS in server management.',
      'verdict.trustedBlocked': 'A trusted user DNS cannot be marked as polluted.',
      'verdict.autoToast': 'Automatic verdict restored',
      'verdict.cleanToast': 'Domain marked as clean',
      'verdict.pollutedToast': 'Domain marked as polluted',
      'service.namesRunning': 'Running',
      'service.namesStopped': 'Stopped',
      'service.namesStarting': 'Starting',
      'service.namesStopping': 'Stopping',
      'service.namesPaused': 'Paused',
      'service.namesPending': 'Transitioning',
      'service.namesUnavailable': 'Unavailable now',
      'service.namesUnknown': 'Unknown state',
      'service.namesNotInstalled': 'Not installed',
      'service.busy': 'Performing a service management operation…',
      'service.transitioning': 'The service state is changing or unavailable; refresh and retry.',
      'service.runningDesc': 'Windows is running DNS Monitor in the background.',
      'service.installedDesc': 'The service is registered; start or remove it here.',
      'service.plainDesc': 'Currently running as a regular program; install it as a service if you like.',
      'service.installButton': 'Install Windows service',
      'service.stopButton': 'Stop service',
      'service.startButton': 'Start service',
      'service.restartButton': 'Restart service',
      'service.uninstallButton': 'Uninstall service',
      'service.refreshButton': '↻ Refresh state',
      'service.readError': 'Can’t read service state: {error}',
      'service.actionInstall': 'Install',
      'service.actionUninstall': 'Uninstall',
      'service.actionStart': 'Start',
      'service.actionStop': 'Stop',
      'service.actionRestart': 'Restart',
      'service.uninstallMsg': 'The Windows service registration will be removed. Config and monitoring data are kept; a running service may stop and the web connection will drop.',
      'service.manageMsg': 'The {action} request is about to be sent for the Windows service. The web connection may drop; {extra}',
      'service.restartExtra': 'log in again once the service is back.',
      'service.stopExtra': 'the UI is reachable again after the service or main program restarts.',
      'service.confirmPrefix': 'Confirm {action}',
      'service.feedbackBusy': '{action}ing the service; if a Windows admin prompt appears, approve it on this machine.',
      'service.feedbackDone': '{action} request submitted.',
      'service.feedbackToast': 'Service {action} request submitted',
      'service.connGone': 'Connection lost; the service may be stopping. Check the service state on this machine and log in again.',
      'service.opFailed': 'Service operation failed: {error}',
      'csv.exportBusy': 'Refreshing the selected range; wait for data to finish loading.',
      'csv.exportDone': 'Exported current results: {count} servers, keeping current filters and sorting',
      'csv.rawExported': 'Raw records CSV exported',
      'csv.exported': 'CSV exported',
      'csv.cancelled': 'Session expired; sign in again.',
      'csv.exportFailed': 'Export failed (HTTP {status})',
      'csv.fileDesc': 'CSV file',
      'csv.serversExported': 'DNS server inventory exported',
      'csv.templateDownloaded': 'Server CSV template downloaded; replace the example rows in it',
      'csv.hName': 'Name',
      'csv.hProvider': 'Provider',
      'csv.hAddress': 'Server address',
      'csv.hProtocol': 'Protocol',
      'csv.hStatus': 'Current status',
      'csv.hAvgLatency': 'Average latency (ms)',
      'csv.hP95Latency': 'P95 latency (ms)',
      'csv.hAvailability': 'Availability (%)',
      'csv.hSuccessRate': 'Query success rate (%)',
      'csv.hQuality': 'Current resolution quality',
      'csv.hGrade': 'Current grade',
      'csv.hSamples': 'Samples in range',
      'csv.hCoverage': 'Coverage (%)',
      'csv.hLastProbe': 'Last probe time (UTC)',
      'csv.hNextProbe': 'Next probe time (UTC)',
      'csv.hRange': 'Statistic range',
      'csv.hSnapshot': 'Data snapshot time (UTC)',
      'csv.hTrusted': 'User trusted',
      'csv.hEnabled': 'Enabled',
      'csv.hWindow': 'Rating window (min)',
      'csv.hFormalSamples': 'Formal rating samples',
      'csv.hCoveredMinutes': 'Covered rating minutes',
      'csv.hScore': 'Recent performance score',
      'csv.hGradeNote': 'Rating note',
      'csv.statusDisabled': 'Disabled',
      'csv.statusPending': 'Pending first probe',
      'csv.statusOk': 'Queries OK',
      'csv.statusError': 'Query error',
      'csv.yes': 'Yes',
      'csv.no': 'No',
      'csv.trustedClean': 'Trusted / clean',
      'detail.domainTitle': 'View resolution evidence and manual verdict',
      'import.busyToast': 'An import is already running, please wait',
      'import.awaitPreview': 'A preview starts automatically after you pick a file',
      'import.reading': 'Reading CSV file…',
      'import.tooLarge': 'The CSV file exceeds 900 KiB; split it and import separately.',
      'import.badUtf8': 'The file is not valid UTF-8; save it as UTF-8 CSV.',
      'import.empty': 'The CSV file is empty; add server records first.',
      'import.requestTooLarge': 'The CSV request exceeds 1 MiB; split it before importing.',
      'import.notChecked': 'The file failed validation',
      'import.validating': 'Validating servers and duplicates…',
      'import.tooMany': 'The CSV has more than 1000 records; split and import in parts.',
      'import.fixRows': 'Fix the invalid rows in the CSV and select the file again',
      'import.previewDone': 'Preview complete; confirm how to handle duplicates',
      'import.previewFailed': 'Preview failed; fix the file and retry',
      'import.summary': '{filename} · {rows} rows · {fresh} new · {conflicts} duplicates · {errors} invalid',
      'import.dupRow': 'duplicate of CSV data row {row}',
      'import.exists': 'exists: {name}',
      'import.dupServer': 'duplicate of an existing server',
      'import.canAdd': 'can be added',
      'import.fix': 'fix',
      'import.create': 'create server',
      'import.rowAria': 'Duplicate handling for data row {row}',
      'import.skipRow': 'Skip this row',
      'import.overwriteConfig': 'Overwrite config',
      'import.noProvider': 'No provider',
      'import.enabled': 'Enabled',
      'import.disabled': 'Stopped',
      'import.trustedTag': 'Trusted',
      'import.normalTag': 'Regular DNS',
      'import.invalidRows': 'There are {count} invalid rows; import is blocked. Fix the original CSV and select it again.',
      'import.requestWithDecisions': 'With decisions the request exceeds 1 MiB; split the CSV.',
      'import.writing': 'Writing server inventory…',
      'import.doneToast': 'Import finished: {created} created, {updated} updated, {skipped} skipped',
      'import.staleError': 'The server inventory changed after the preview; click “Preview again” to re-check duplicates.',
      'import.stalePreview': 'Preview is stale; preview again',
      'import.failed': 'Import failed; check the messages above and retry',
      'confirm.default': 'Confirm',
      'config.minOneDomain': 'Keep at least one probe domain',
    }
  };
  const t = (key, vars) => {
    if (currentLang !== 'en') return undefined;
    const template = I18N.en[key];
    if (template === undefined) return undefined;
    return String(template).replace(/\{([^}]+)\}/g, (match, name) => vars && Object.prototype.hasOwnProperty.call(vars, name) ? String(vars[name]) : match);
  };
  const unitLabel = raw => currentLang !== 'en' ? raw : ({ '秒': 's', '分钟': 'min', '小时': 'h', '个': 'servers', '次': 'samples', '台': 'servers' })[raw] || raw;
  const locale = () => currentLang === 'en' ? 'en-US' : 'zh-CN';
  const zhStatic = new Map();
  const applyStaticI18n = () => {
    const snapshot = el => { if (!zhStatic.has(el)) zhStatic.set(el, { html: el.innerHTML, placeholder: el.placeholder, title: el.title, aria: el.getAttribute('aria-label') }); };
    $$('[data-i18n]').forEach(snapshot);
    $$('[data-i18n-placeholder]').forEach(snapshot);
    $$('[data-i18n-title]').forEach(snapshot);
    $$('[data-i18n-aria-label]').forEach(snapshot);
    for (const [el, saved] of zhStatic) {
      if (currentLang === 'en') {
        if (el.dataset.i18n !== undefined && I18N.en[el.dataset.i18n] !== undefined) el.innerHTML = I18N.en[el.dataset.i18n];
        if (el.dataset.i18nPlaceholder !== undefined && I18N.en[el.dataset.i18nPlaceholder] !== undefined) el.placeholder = I18N.en[el.dataset.i18nPlaceholder];
        if (el.dataset.i18nTitle !== undefined && I18N.en[el.dataset.i18nTitle] !== undefined) el.title = I18N.en[el.dataset.i18nTitle];
        if (el.dataset.i18nAriaLabel !== undefined && I18N.en[el.dataset.i18nAriaLabel] !== undefined) el.setAttribute('aria-label', I18N.en[el.dataset.i18nAriaLabel]);
      } else {
        if (el.dataset.i18n !== undefined) el.innerHTML = saved.html;
        if (el.dataset.i18nPlaceholder !== undefined) el.placeholder = saved.placeholder;
        if (el.dataset.i18nTitle !== undefined) el.title = saved.title;
        if (el.dataset.i18nAriaLabel !== undefined) el.setAttribute('aria-label', saved.aria);
      }
    }
  };
  const renderLanguageToggle = () => {
    const label = currentLang === 'en' ? '中文' : 'English';
    $$('#lang-toggle, #login-lang-toggle').forEach(button => { button.textContent = label; });
  };
  const rerenderLanguageUI = () => {
    document.documentElement.lang = currentLang === 'en' ? 'en' : 'zh-CN';
    applyStaticI18n();
    document.title = currentLang === 'en' ? (I18N.en.docTitle || zhTitle) : zhTitle;
    renderLanguageToggle();
    if (!state.token || !$('#app')) return;
    if (titles[state.page]) $('#page-title').textContent = currentLang === 'en' ? I18N.en['page.' + state.page] : titles[state.page][0];
    renderRuntime();
    renderRefreshButton();
    if (state.data) {
      renderOverview();
      renderService();
      if (state.page === 'config' && !state.configDirty) populateConfig();
    }
    if ($('#detail-dialog')?.open) {
      renderDetailHeader();
      if (state.statusHistory.length) renderStatusHistory();
      renderResults();
      renderDetailCurrent();
      renderCharts();
    }
  };
  const setLanguage = localeChoice => {
    if (localeChoice !== 'en' && localeChoice !== 'zh') return;
    if (localeChoice === currentLang) return;
    currentLang = localeChoice;
    try { if (typeof localStorage !== 'undefined') localStorage.setItem(langKey, localeChoice); } catch { /* storage unavailable */ }
    rerenderLanguageUI();
  };
  const pageTitle = page => currentLang === 'en' ? (I18N.en['page.' + page] || titles[page][0]) : titles[page][0];
  const filterStorageKey = 'dns-monitor.filters.v1';
  const storedFilters = (() => { try { const raw = typeof localStorage !== 'undefined' ? localStorage.getItem(filterStorageKey) : null; return raw ? JSON.parse(raw) : {}; } catch { return {}; } })();
  const sessionKey = 'dns-monitor.session.v1';
  const storedSession = (() => { try { if (typeof sessionStorage === 'undefined') return null; const raw = sessionStorage.getItem(sessionKey); return raw ? JSON.parse(raw) : null; } catch { return null; } })();
  const state = { token: storedSession?.token || '', data: null, range: '24h', detailRange: '24h', page: ['overview', 'config', 'service', 'guide'].includes(storedSession?.page) ? storedSession.page : 'overview', sort: 'grade', descending: false, detailID: null, history: [], statusHistory: [], historyMetrics: null, results: [], result: null, refreshTimer: null, refreshing: false, configDirty: false, loadingResults: null, resultEnd: false, detailRequest: 0, resultsRequest: 0, serviceBusy: false, manualRefresh: null, manualTimer: null, detailProbe: null, detailProbeTimer: null, importing: null, filters: { protocol: Array.isArray(storedFilters.protocol) ? storedFilters.protocol.filter(value => typeof value === 'string') : [], pollution: Array.isArray(storedFilters.pollution) ? storedFilters.pollution.filter(value => ['matched', 'clean', 'suspicious', 'polluted', 'unknown'].includes(value)) : [], grade: Array.isArray(storedFilters.grade) ? storedFilters.grade.filter(value => typeof value === 'string') : [] } };
  const persistFilters = () => { try { if (typeof localStorage !== 'undefined') localStorage.setItem(filterStorageKey, JSON.stringify({ protocol: state.filters.protocol, pollution: state.filters.pollution, grade: state.filters.grade })); } catch { /* storage unavailable */ } };
  const persistSession = () => { try { if (typeof sessionStorage !== 'undefined') sessionStorage.setItem(sessionKey, JSON.stringify({ token: state.token, page: state.page })); } catch { /* storage unavailable */ } };
  const clearSession = () => { try { if (typeof sessionStorage !== 'undefined') sessionStorage.removeItem(sessionKey); } catch { /* storage unavailable */ } };
  const ranges = { '24h': 24 * 3600000, '7d': 7 * 86400000, '30d': 30 * 86400000 };
  const titles = { overview: ['监测总览', 'NETWORK OBSERVABILITY'], config: ['探测配置', 'MONITORING PREFERENCES'], service: ['Windows 服务', 'ALWAYS-ON MONITORING'], guide: ['使用指南', 'YOUR OBSERVATION HANDBOOK'] };
  const escape = value => String(value ?? '').replace(/[&<>"']/g, char => ({ '&': '&amp;', '<': '&lt;', '>': '&gt;', '"': '&quot;', "'": '&#39;' }[char]));
  const number = (value, digits = 1) => Number.isFinite(Number(value)) ? Number(value).toLocaleString(locale(), { maximumFractionDigits: digits, minimumFractionDigits: digits }) : '—';
  const integer = value => number(value, 0);
  const timestamp = (value, full = false) => value ? new Date(value).toLocaleString(locale(), { month: '2-digit', day: '2-digit', hour: '2-digit', minute: '2-digit', ...(full ? { second: '2-digit' } : {}), hour12: false }) : t('time.neverProbed') ?? '尚未探测';
  const hasSamples = metrics => (metrics?.samples || 0) > 0;
  const hasCoverage = metrics => (metrics?.coverage || 0) > 0;
  const hasLatency = metrics => hasSamples(metrics) && (metrics.success_rate > 0 || metrics.average_ms > 0 || metrics.p95_ms > 0);
  const percent = (value, metrics, availability = false) => (availability ? hasCoverage(metrics) : hasSamples(metrics)) ? `${number(value)}%` : '—';
  const currentServer = () => state.data?.servers.find(server => server.id === state.detailID);
  const trustedResult = result => !!result.trusted || !!state.data?.servers.find(server => server.id === result.server_id)?.trusted;
  const currentEvaluation = server => server?.current || {};
  const serverPollution = server => server.trusted ? 'clean' : currentEvaluation(server).pollution;
  const displayedGrade = (metrics, trusted = false) => trusted && ['E', 'F'].includes(metrics?.grade) ? 'pending' : metrics?.grade;
  const trustedBadge = `<span class="pollution-badge trusted" title="${escape(t('trust.title') ?? '用户指定的可信 DNS，豁免污染判定；性能评级仍需足够采样。')}">${escape(t('trust.badge') ?? '✓ 用户可信 / 无污染')}</span>`;
  const pollutionKind = value => ['matched', 'clean', 'suspicious', 'polluted'].includes(value) ? value : 'unknown';
  const pollutionLabel = value => t('state.' + pollutionKind(value)) ?? ({ matched: '参考一致', clean: '正常', suspicious: '可疑', polluted: '疑似污染', unknown: '待判定' })[pollutionKind(value)];
  const pollutionHints = {
    matched: () => t('hint.matched') ?? '可比较的返回 IP 均命中 TTL 内的新鲜可信参考。',
    clean: () => t('hint.clean') ?? '可比较的返回 IP 命中有效可信历史，或已由用户手动标记正常。',
    suspicious: () => t('hint.suspicious') ?? '有 IP 未命中有效可信参考，但未命中的 IP 均与参考地址的前两段匹配。',
    polluted: () => t('hint.polluted') ?? '有 IP 既未命中有效可信参考，前两段也不匹配；此为疑似判定。',
    unknown: () => t('hint.unknown') ?? '查询失败、没有可比较的 IPv4 答案，或没有有效可信参考，暂无法判定。'
  };
  const badge = (value, confirmed = false) => {
    const kind = pollutionKind(value);
    const hint = confirmed && kind === 'polluted' ? t('hint.confirmedPolluted') ?? '用户已手动确认污染，可在域名菜单中修改判定。' : pollutionHints[kind]();
    const symbol = ['polluted', 'suspicious'].includes(kind) ? '!' : ['matched', 'clean'].includes(kind) ? '✓' : '·';
    return `<span class="pollution-badge ${kind}" title="${escape(hint)}"><span aria-hidden="true">${symbol}</span>${confirmed && kind === 'polluted' ? t('state.confirmedPolluted') ?? '已确认污染' : pollutionLabel(kind)}</span>`;
  };
  const gradeHints = {
    A: () => t('grade.A') ?? '优秀：综合评分 ≥ 95，依据可用率、查询成功率和 P95 时延。',
    B: () => t('grade.B') ?? '良好：综合评分 ≥ 85 且 < 95，依据可用率、查询成功率和 P95 时延。',
    C: () => t('grade.C') ?? '一般：综合评分 ≥ 70 且 < 85，依据可用率、查询成功率和 P95 时延。',
    D: () => t('grade.D') ?? '较差：综合评分 < 70，依据可用率、查询成功率和 P95 时延。',
    E: () => t('grade.E') ?? '解析结果可疑，优先评 E；未命中的 IP 与可信参考的前两段匹配。',
    F: () => t('grade.F') ?? '存在疑似或人工确认污染，优先评 F。',
    unavailable: () => t('grade.unavailable') ?? '不可用：最新一轮正式探测全部查询失败（连接失败、超时或解析错误）；可信标记不豁免。恢复成功后重新评级。',
    pending: () => t('grade.pending') ?? '等待足够的正式采样与有效覆盖；评级门槛可在探测配置中调整。'
  };
  const gradeHint = (value, evaluation = {}) => {
    const description = gradeHints[value] ? gradeHints[value]() : gradeHints.pending();
    if (!evaluation.window_minutes) return description;
    const windowMin = integer(evaluation.window_minutes);
    const prefix = t('grade.windowPrefix', { minutes: windowMin }) ?? `最近 ${windowMin} 分钟。`;
    if (value !== 'pending' && gradeHints[value]) return prefix + description;
    const pendingReasons = {
      no_samples: () => t('grade.noSamples') ?? '评级窗口内没有正式采样，等待下一轮探测。',
      quality_unknown: () => t('grade.qualityUnknown') ?? '最新正式探测的解析质量待判定，可查看域名记录中的原因。',
      insufficient_samples: () => t('grade.insufficientSamples') ?? '正式采样次数尚未达到评级门槛。',
      insufficient_coverage: () => t('grade.insufficientCoverage') ?? '有效监测覆盖尚未达到评级门槛。'
    };
    const reason = pendingReasons[evaluation.pending_reason] ? pendingReasons[evaluation.pending_reason]() : description;
    const covered = Math.floor((evaluation.covered_minutes || 0) * 10) / 10;
    const summary = t('grade.pendingSummary', { reason, samples: integer(evaluation.samples || 0), minSamples: integer(evaluation.min_samples), coverage: number(covered), minCoverage: integer(evaluation.min_coverage_minutes) });
    return prefix + summary;
  };
  const gradeHTML = (value, evaluation) => value === 'unavailable' ? `<span class="grade grade-unavailable" title="${escape(gradeHint(value, evaluation))}">${t('state.unavailable') ?? '不可用'}</span>` : ['A', 'B', 'C', 'D', 'E', 'F'].includes(value) ? `<span class="grade grade-${value.toLowerCase()}" title="${escape(gradeHint(value, evaluation))}">${value}</span>` : `<span class="grade grade-pending" title="${escape(gradeHint('pending', evaluation))}">${t('state.pending') ?? '待评估'}</span>`;

  const statusLabel = (value, channel) => value === 'no_data' ? t('state.noData') ?? '无数据' : channel === 'quality' ? pollutionLabel(value) : (t('state.' + value) ?? ({ pending: '待评估', unavailable: '不可用' })[value]) || value;
  const statusValue = (point, channel) => {
    const value = channel === 'quality' ? point.pollution : point.grade;
    const allowed = channel === 'quality' ? ['matched', 'clean', 'unknown', 'suspicious', 'polluted'] : ['A', 'B', 'C', 'D', 'E', 'F', 'pending', 'unavailable'];
    return point.snapshots > 0 && allowed.includes(value) ? value : 'no_data';
  };
  function statusDescription(point, channel) {
    const name = channel === 'quality' ? t('state.quality') ?? '解析质量' : t('state.rating') ?? '评级';
    const heading = `${timestamp(point.timestamp)} – ${timestamp(point.end)} · ${name}`;
    if (!point.snapshots) return heading + (t('status.noSnapshots') ?? '：无数据（此时段没有正式探测快照）');
    const counts = channel === 'quality' ? point.quality_counts : point.grade_counts;
    const joinText = currentLang === 'en' ? ', ' : '、';
    const distribution = Object.entries(counts || {}).map(([key, count]) => t('status.distItem', { label: statusLabel(key, channel), count: integer(count) }) ?? `${statusLabel(key, channel)} ${integer(count)} 次`).join(joinText);
    let text = t('status.summary', { heading, label: statusLabel(statusValue(point, channel), channel), count: integer(point.snapshots), distribution }) ?? `${heading}：最差记录 ${statusLabel(statusValue(point, channel), channel)}。${integer(point.snapshots)} 次探测快照；${distribution}。`;
    if (point.latest) {
      const latest = point.latest.current;
      const latestText = t('status.latest', { time: timestamp(point.latest.timestamp, true), pollution: pollutionLabel(latest.pollution), grade: statusLabel(latest.grade, 'grade'), trustedSuffix: point.latest.trusted ? t('status.trustedSuffix') ?? '，当时为用户可信 DNS' : '' }) ?? `该时段末次快照（${timestamp(point.latest.timestamp, true)}）：解析质量 ${pollutionLabel(latest.pollution)}，评级 ${statusLabel(latest.grade, 'grade')}${point.latest.trusted ? '，当时为用户可信 DNS' : ''}。`;
      text += latestText + (t('status.latestDetail', { window: integer(latest.window_minutes), samples: integer(latest.samples), rate: percent(latest.success_rate, latest), latency: hasLatency(latest) ? number(latest.average_ms) + ' ms' : '—' }) ?? `最近 ${integer(latest.window_minutes)} 分钟采样 ${integer(latest.samples)} 次，成功率 ${percent(latest.success_rate, latest)}，平均时延 ${hasLatency(latest) ? number(latest.average_ms) + ' ms' : '—'}。`);
    }
    return text;
  }
  function statusStrip(points, channel, detail = false) {
    return `<span class="status-strip${detail ? ' expanded' : ''}">${points.map((point, index) => {
      const value = statusValue(point, channel);
      const description = escape(statusDescription(point, channel));
      return detail ? `<button type="button" class="status-block status-${value.toLowerCase()}" data-status-index="${index}" data-status-channel="${channel}" aria-label="${description}" title="${description}"></button>` : `<span class="status-block status-${value.toLowerCase()}" title="${description}"></span>`;
    }).join('')}</span>`;
  }
  function compactHistory(server, channel) {
    const points = server.status_history || [];
    const name = channel === 'quality' ? t('state.quality') ?? '解析质量' : t('state.rating') ?? '评级';
    if (!points.length) return `<span class="sub-status">${t('status.empty') ?? '历史暂无数据'}</span>`;
    const rangeText = t('status.historyCaption', { range: currentLang === 'en' ? ({ '24h': '24h', '7d': '7d', '30d': '30d' })[state.loadedRange || state.range] : ({ '24h': '24 小时', '7d': '7 天', '30d': '30 天' })[state.loadedRange || state.range] }) ?? `${({ '24h': '24 小时', '7d': '7 天', '30d': '30 天' })[state.loadedRange || state.range]}历史 ↗`;
    return `<button type="button" class="history-open" data-action="detail" data-id="${server.id}" aria-label="${escape(t('status.ariaHistory', { name: server.name }) ?? `查看 ${server.name} 的${name}历史`)}">${statusStrip(points, channel)}<span class="history-caption">${rangeText}</span></button>`;
  }
  function renderStatusHistory() {
    const target = $('#status-history');
    const points = state.statusHistory;
    if (!points.length) { target.innerHTML = `<p class="field-help">${t('status.statsEmpty') ?? '暂无状态历史。'}</p>`; return; }
    const legend = (values, channel) => values.map(value => `<span><i class="status-${value.toLowerCase()}"></i>${escape(statusLabel(value, channel))}</span>`).join('');
    const intervalText = currentLang === 'en' ? ({ '24h': '30 min', '7d': '3 h', '30d': '12 h' })[state.detailRange] : ({ '24h': '30 分钟', '7d': '3 小时', '30d': '12 小时' })[state.detailRange];
    const cellHelp = t('status.cellHelp', { interval: intervalText }) ?? `每格 ${({ '24h': '30 分钟', '7d': '3 小时', '30d': '12 小时' })[state.detailRange]}，显示该时段探测快照中的最差状态；悬停、聚焦或点击查看详情。`;
    target.innerHTML = `<div class="status-history-heading"><h4>${t('status.historyHeading') ?? '解析质量与评级历史'}</h4><span>${t('status.pastNow') ?? '过去 → 现在'}</span></div><p class="field-help">${cellHelp}</p><div class="status-track"><h5>${t('status.qualityTitle') ?? '解析质量'}</h5>${statusStrip(points, 'quality', true)}<div class="status-legend">${legend(['matched', 'clean', 'unknown', 'suspicious', 'polluted', 'no_data'], 'quality')}</div></div><div class="status-track"><h5>${t('status.ratingTitle') ?? '探测时评级'}</h5>${statusStrip(points, 'grade', true)}<div class="status-legend">${legend(['A', 'B', 'C', 'D', 'pending', 'E', 'F', 'unavailable', 'no_data'], 'grade')}</div></div><div class="status-axis"><span>${timestamp(points[0].timestamp)}</span><span>${timestamp(points.at(-1).end)}</span></div><p id="status-history-inspection" class="status-inspection" role="status">${t('status.inspect') ?? '请选择色块查看状态分布及当时的评级指标。'}</p><p class="field-help">${t('status.note') ?? '仅记录正式探测完成时的状态，不代表整段时间持续如此。评级使用当时的观察窗口、配置与可信设置；后续修改不会重写历史。升级前未记录或无探测的时段显示为无数据。'}</p>`;
  }

  function setError(selector, message) {
    const element = $(selector);
    element.textContent = message || '';
    element.hidden = !message;
  }

  function toast(message, error = false) {
    const element = document.createElement('div');
    element.className = `toast${error ? ' error' : ''}`;
    element.textContent = message;
    $('#toast-stack').append(element);
    setTimeout(() => element.remove(), error ? 6500 : 4000);
  }

  function closeDialogs() {
    $$('dialog[open]').forEach(dialog => dialog.close());
  }

  function resetSession(message = '') {
    state.token = '';
    clearSession();
    clearTimeout(state.refreshTimer);
    clearTimeout(state.manualTimer);
    clearTimeout(state.detailProbeTimer);
    state.manualRefresh = null;
    state.detailProbe = null;
    closeDialogs();
    $('#app').hidden = true;
    $('#login-screen').hidden = false;
    setError('#login-error', message);
    $('#access-key').value = '';
    $('#access-key').focus();
  }

  async function api(path, options = {}) {
    const headers = new Headers(options.headers || {});
    headers.set('X-DNSMonitor-Token', state.token);
    if (options.body !== undefined && typeof options.body !== 'string') {
      headers.set('Content-Type', 'application/json');
      options.body = JSON.stringify(options.body);
    }
    const response = await fetch(`/api${path}`, { ...options, headers, cache: 'no-store' });
    let body;
    try { body = await response.json(); } catch { body = null; }
    if (!response.ok) {
      if (response.status === 401) resetSession(t('session.expired') ?? '会话已失效，请重新输入访问密钥。');
      const error = new Error(body?.error || (t('api.failed', { status: response.status }) ?? `请求未完成（HTTP ${response.status}）`));
      error.status = response.status;
      throw error;
    }
    return body;
  }

  async function login(event) {
    event.preventDefault();
    const button = $('.login-submit');
    const accessKey = $('#access-key').value.trim();
    if (!accessKey) return;
    button.disabled = true;
    setError('#login-error', '');
    try {
      const bytes = new TextEncoder().encode(`admin:${accessKey}`);
      const authorization = `Basic ${btoa(Array.from(bytes, byte => String.fromCharCode(byte)).join(''))}`;
      const response = await fetch('/api/session', { headers: { Authorization: authorization }, cache: 'no-store' });
      let body;
      try { body = await response.json(); } catch { body = null; }
      if (!response.ok || !body?.token) throw new Error(response.status === 401 ? t('login.badKey') ?? '访问密钥不正确，请检查 data/access-key.txt。' : body?.error || (t('login.unreachable') ?? '无法连接观测台，请确认程序正在运行。'));
      state.token = body.token;
      persistSession();
      $('#access-key').value = '';
      $('#login-screen').hidden = true;
      $('#app').hidden = false;
      rerenderLanguageUI();
      await refreshState(true);
      scheduleRefresh();
    } catch (error) {
      setError('#login-error', error.message === 'Failed to fetch' ? t('login.connFailed') ?? '连接失败，请确认 DNS Monitor 正在运行。' : error.message);
    } finally {
      button.disabled = false;
    }
  }

  async function logout() {
    if (state.configDirty && !(await confirmAction(t('logout.confirmTitle') ?? '退出观测台', t('logout.confirmText') ?? '探测配置有未保存的更改，退出将放弃这些更改。', t('logout.confirmButton') ?? '退出'))) return;
    try { await api('/session', { method: 'DELETE' }); } catch { /* The local session is cleared even if the server is unreachable. */ }
    state.configDirty = false;
    state.data = null;
    resetSession();
  }

  function scheduleRefresh() {
    clearTimeout(state.refreshTimer);
    if (!state.token) return;
    state.refreshTimer = setTimeout(async () => {
      if (!document.hidden) await refreshState();
      scheduleRefresh();
    }, 15000);
  }

  async function refreshState(initial = false) {
    if (!state.token || state.refreshing) return null;
    state.refreshing = true;
    renderRefreshButton();
    try {
      const requestedRange = state.range;
      const response = await api(`/state?range=${encodeURIComponent(requestedRange)}`);
      state.loadedRange = requestedRange;
      const previous = currentServer()?.last_probe || 0;
      state.data = response;
      state.data.servers ||= [];
      renderOverview();
      renderRuntime();
      renderService();
      if (initial || (state.page === 'config' && !state.configDirty)) populateConfig();
      $('#restart-notice').hidden = !response.listen_restart_required;
      const updatedAt = new Date().toLocaleTimeString(locale(), { hour12: false });
      $('#last-refresh').textContent = t('refresh.updatedAt', { time: updatedAt }) ?? `更新于 ${updatedAt}`;
      setError('#global-error', response.runtime?.last_error ? `${t('refresh.runtimeHint', { message: response.runtime.last_error }) ?? '最近运行提示：' + response.runtime.last_error}` : '');
      if ($('#detail-dialog').open) {
        renderDetailHeader();
        if ((currentServer()?.last_probe || 0) > previous) await Promise.allSettled([loadHistory(), loadResults(true)]);
        else renderDetailCurrent();
      }
      return response;
    } catch (error) {
      if (state.token) setError('#global-error', `${t('refresh.failed', { reason: error.message === 'Failed to fetch' ? t('refresh.connLost') ?? '连接中断，请检查程序或 Windows 服务是否运行。' : error.message }) ?? `无法更新数据：${error.message === 'Failed to fetch' ? '连接中断，请检查程序或 Windows 服务是否运行。' : error.message}`}`);
      return null;
    } finally {
      state.refreshing = false;
      renderRefreshButton();
    }
  }

  function renderRefreshButton() {
    const button = $('#refresh-button');
    const batch = state.manualRefresh;
    $('#export-button').disabled = !state.data || state.loadedRange !== state.range || state.refreshing;
    const paused = !!state.data?.runtime?.paused || state.data?.config?.concurrency === 0;
    button.disabled = !!batch || paused;
    button.innerHTML = batch ? `<span class="spinner" aria-hidden="true"></span> ${t('refresh.probing', { done: integer(batch.completed || 0), total: integer(batch.total || 0) }) ?? '探测 ' + integer(batch.completed || 0) + '/' + integer(batch.total || 0)}` : `<span aria-hidden="true">↻</span> ${t('refresh.label2') ?? '刷新'}`;
    button.title = paused ? t('refresh.pausedTitle') ?? '并发数为 0，探测已暂停；调整配置后可手动刷新' : batch ? t('refresh.batchTitle') ?? '等待本轮实际探测完成' : t('refresh.normalTitle') ?? '立即探测所有已启用 DNS，并读取新结果';
  }

  function showProbeProgress(batch, done = false) {
    const element = $('#probe-progress');
    element.hidden = false;
    element.className = 'notice ' + (batch.error ? 'warning' : 'info') + ' probe-progress';
    element.textContent = batch.error ? `${t('probe.roundError', { error: batch.error }) ?? '本轮刷新：' + batch.error}` : done ? t('probe.doneRound', { count: integer(batch.completed || 0) }) ?? `刷新完成：${integer(batch.completed || 0)} 台 DNS 已完成新的探测。` : t('probe.running', { done: integer(batch.completed || 0), total: integer(batch.total || 0), remaining: integer(Math.max(0, Number(batch.total || 0) - Number(batch.completed || 0))) }) ?? `正在重新探测 DNS：${integer(batch.completed || 0)} / ${integer(batch.total || 0)} 已完成，${integer(Math.max(0, Number(batch.total || 0) - Number(batch.completed || 0)))} 台等待完成。`;
  }

  async function manualRefresh() {
    if (state.manualRefresh) return;
    if (state.data?.runtime?.paused || state.data?.config?.concurrency === 0) return toast(t('probe.pausedToast') ?? '探测已暂停，请将并发数设为大于 0 后重试', true);
    state.manualRefresh = { id: null, total: 0, completed: 0, pending: 0, starting: true, failures: 0 };
    renderRefreshButton();
    try {
      const batch = await api('/probes', { method: 'POST', body: {} });
      state.manualRefresh = { ...batch, failures: 0 };
      showProbeProgress(batch);
      await pollManualRefresh();
    } catch (error) {
      state.manualRefresh = null;
      $('#probe-progress').hidden = true;
      toast(error.message, true);
      renderRefreshButton();
    }
  }

  async function pollManualRefresh() {
    clearTimeout(state.manualTimer);
    const tracking = state.manualRefresh;
    if (!tracking || !state.token) return;
    const response = await refreshState();
    if (!state.token || state.manualRefresh !== tracking) return;
    const reported = response?.runtime?.refresh;
    if (reported && String(reported.id) === String(tracking.id)) {
      Object.assign(tracking, reported, { failures: 0 });
    } else if (response && reported?.id && String(reported.id) !== String(tracking.id)) {
      state.manualRefresh = null;
      toast(t('probe.batchChanged') ?? '服务器上的刷新批次已变化，当前列表已更新；可再次点击刷新。', true);
      renderRefreshButton();
      return;
    } else if (!response && !state.refreshing) {
      tracking.failures = (tracking.failures || 0) + 1;
      if (tracking.failures >= 5) {
        state.manualRefresh = null;
        showProbeProgress({ error: t('probe.progressUnreadable') ?? '暂时无法读取探测进度，后台任务可能仍在运行。恢复连接后再刷新状态。' });
        renderRefreshButton();
        return;
      }
    }
    const done = tracking.pending === false || Number(tracking.completed || 0) >= Number(tracking.total || 0);
    showProbeProgress(tracking, done);
    renderRefreshButton();
    if (done) {
      state.manualRefresh = null;
      if ($('#detail-dialog').open) await Promise.allSettled([loadHistory(), loadResults(true)]);
      renderRefreshButton();
      toast(tracking.error || (tracking.total ? t('probe.doneToast') ?? '所有已启用 DNS 的新一轮探测已完成' : t('probe.nothingToDo') ?? '没有需要探测的已启用 DNS'), !!tracking.error);
      return;
    }
    state.manualTimer = setTimeout(pollManualRefresh, 1000);
  }

  function renderRuntime() {
    const runtime = state.data?.runtime || {};
    const config = state.data?.config || {};
    const paused = runtime.paused || config.concurrency === 0;
    $('#runtime-title').textContent = paused ? t('runtime.pausedTitle') ?? '探测已暂停' : t('runtime.runningTitle') ?? '调度器运行中';
    $('#runtime-dot').className = `status-dot${paused ? ' paused' : runtime.last_error ? ' error' : ''}`;
    $('#runtime-description').textContent = paused ? t('runtime.pausedDesc') ?? '并发数设为 0，保存记录仍可查看。' : t('runtime.runningDesc', { active: runtime.active || 0, total: config.concurrency ?? '—', interval: config.interval_seconds || '—' }) ?? `${runtime.active || 0} / ${config.concurrency ?? '—'} 并发 · 基础周期 ${config.interval_seconds || '—'} 秒`;
    $('#version').textContent = state.data?.version ? `v${String(state.data.version).replace(/^v/, '')}` : 'DNS Monitor';
    $('#nav-server-count').textContent = (state.data?.servers || []).length;
  }

  const multiSelectSpecs = {
    protocol: { label: () => t('filter.allProtocols') ?? '全部协议', list: servers => [...new Set(servers.map(server => server.protocol).filter(Boolean))].sort().map(value => ({ value, label: value.toUpperCase() })), valueOf: server => server.protocol },
    pollution: { label: () => t('filter.allPollution') ?? '全部解析状态', list: () => [['matched', '参考一致'], ['clean', '正常'], ['suspicious', '可疑'], ['polluted', '疑似 / 确认污染'], ['unknown', '待判定']].map(([value, label]) => ({ value, label: t('state.' + value) ?? label })), valueOf: server => pollutionKind(serverPollution(server)) },
    grade: { label: () => t('filter.allGrades') ?? '全部评级', list: () => [['A', 'A · 优秀'], ['B', 'B · 良好'], ['C', 'C · 一般'], ['D', 'D · 较差'], ['E', 'E · 可疑'], ['F', 'F · 疑似 / 确认污染'], ['pending', '待评估'], ['unavailable', '不可用']].map(([value, label]) => ({ value, label: ({ A: t('filter.gradeA'), B: t('filter.gradeB'), C: t('filter.gradeC'), D: t('filter.gradeD'), E: t('filter.gradeE'), F: t('filter.gradeF'), pending: t('filter.gradePending'), unavailable: t('filter.gradeUnavailable') })[value] ?? label })), valueOf: server => { const value = displayedGrade(currentEvaluation(server), server.trusted); return ['A', 'B', 'C', 'D', 'E', 'F', 'unavailable'].includes(value) ? value : 'pending'; } }
  };
  let filterSignatures = {};

  function filterOptionList(key) {
    const servers = state.data?.servers || [];
    const list = multiSelectSpecs[key].list(servers);
    const counts = new Map(list.map(option => [option.value, 0]));
    servers.forEach(server => { const value = multiSelectSpecs[key].valueOf(server); const count = counts.get(value); if (count !== undefined) counts.set(value, count + 1); });
    return { list, counts };
  }

  function updateFilterSummary(key) {
    const summary = $(`[data-summary="${key}"]`);
    if (summary) summary.textContent = state.filters[key].length ? t('filter.selected', { count: state.filters[key].length }) ?? `已选 ${state.filters[key].length} 项` : multiSelectSpecs[key].label();
  }

  function renderFilterOptions(key) {
    const root = $(`#${key}-filter-root`);
    if (!root) return;
    const { list, counts } = filterOptionList(key);
    const available = list.map(option => option.value);
    state.filters[key] = state.filters[key].filter(value => available.includes(value));
    const signature = list.map(option => `${option.value}:${option.label}:${counts.get(option.value) || 0}`).join('|');
    const optionsElement = $(`[data-options="${key}"]`, root);
    if (!optionsElement) return;
    if (filterSignatures[key] !== signature) {
      optionsElement.innerHTML = list.map(option => `<label class="multi-option"><input type="checkbox" value="${escape(option.value)}"><span class="multi-option-text">${escape(option.label)}</span><span class="multi-option-count">${integer(counts.get(option.value) || 0)}</span></label>`).join('');
      filterSignatures[key] = signature;
    }
    $$('input[type=checkbox]', optionsElement).forEach(input => { input.checked = state.filters[key].includes(input.value); });
    const all = $(`[data-all="${key}"]`, root);
    if (all) all.checked = state.filters[key].length === 0;
    updateFilterSummary(key);
  }

  function renderAllFilters() { Object.keys(multiSelectSpecs).forEach(renderFilterOptions); }

  function closeFilterPopovers() {
    $$('.multi-select[data-filter]').forEach(root => {
      const key = root.dataset.filter;
      const popover = $(`[data-popover="${key}"]`, root);
      if (popover) popover.hidden = true;
      const trigger = $(`[data-trigger="${key}"]`, root);
      if (trigger) trigger.setAttribute('aria-expanded', 'false');
    });
  }

  function toggleFilterPopover(key) {
    const root = $(`#${key}-filter-root`);
    if (!root) return;
    const popover = $(`[data-popover="${key}"]`, root);
    if (!popover) return;
    const opening = popover.hidden;
    closeFilterPopovers();
    popover.hidden = !opening;
    const trigger = $(`[data-trigger="${key}"]`, root);
    if (trigger) trigger.setAttribute('aria-expanded', String(opening));
  }

  function renderOverview() {
    const servers = state.data.servers;
    const enabled = servers.filter(server => server.enabled);
    const online = enabled.filter(server => server.last_probe && server.last_success);
    const polluted = servers.filter(server => pollutionKind(serverPollution(server)) === 'polluted');
    const suspicious = servers.filter(server => pollutionKind(serverPollution(server)) === 'suspicious');
    const sampled = servers.filter(server => hasSamples(server.metrics));
    const responding = sampled.filter(server => hasLatency(server.metrics));
    const average = responding.length ? responding.reduce((sum, server) => sum + server.metrics.average_ms, 0) / responding.length : null;
    const samples = sampled.reduce((sum, server) => sum + server.metrics.samples, 0);
    const cards = [
      { title: t('ov.cardServers') ?? '监测服务器', value: integer(servers.length), unit: unitLabel('台'), caption: t('ov.cardServersCaption', { enabled: enabled.length, trusted: servers.filter(server => server.trusted).length }) ?? `${enabled.length} 台已启用 · ${servers.filter(server => server.trusted).length} 台可信 DNS`, icon: '◫', kind: '' },
      { title: t('ov.cardOnline') ?? '最近查询正常', value: integer(online.length), unit: unitLabel('台'), caption: t('ov.cardOnlineCaption', { failed: enabled.filter(server => server.last_probe && !server.last_success).length, awaiting: enabled.filter(server => !server.last_probe).length }) ?? `${enabled.filter(server => server.last_probe && !server.last_success).length} 台异常 · ${enabled.filter(server => !server.last_probe).length} 台待首次探测`, icon: '↗', kind: 'good' },
      { title: t('ov.cardAnomaly') ?? '解析异常', value: integer(polluted.length + suspicious.length), unit: unitLabel('台'), caption: t('ov.cardAnomalyCaption', { f: polluted.length, e: suspicious.length, unknown: servers.filter(server => pollutionKind(serverPollution(server)) === 'unknown').length }) ?? `${polluted.length} 台 F · ${suspicious.length} 台 E · ${servers.filter(server => pollutionKind(serverPollution(server)) === 'unknown').length} 台待判定`, icon: '!', kind: polluted.length ? 'bad' : suspicious.length ? 'warning' : '' },
      { title: t('ov.cardLatency') ?? '服务器平均时延', value: average === null ? '—' : number(average, 0), unit: 'ms', caption: t('ov.cardLatencyCaption', { responding: responding.length, samples: integer(samples) }) ?? `${responding.length} 台响应服务器均值 · ${integer(samples)} 次采样`, icon: '⌁', kind: '' }
    ];
    $('#summary-cards').innerHTML = cards.map(card => `<article class="summary-card ${card.kind}"><div class="summary-top"><span>${escape(card.title)}</span><span class="summary-icon" aria-hidden="true">${escape(card.icon)}</span></div><div class="summary-value">${card.value}<small>${card.unit}</small></div><p class="summary-caption">${escape(card.caption)}</p></article>`).join('');
    $('#server-total').textContent = servers.length;
    const ratingWindow = state.data.config?.rating_window_minutes ?? 60;
    $('#rating-explanation').textContent = t('ov.ratingExplanation', { window: ratingWindow }) ?? `时延、可用率和成功率按所选历史区间统计；当前解析质量取最新正式探测，当前评级使用最近 ${ratingWindow} 分钟。待判定时不延续旧的正常评级；点击域名可查看原因与人工复核。`;
    renderAllFilters();
    renderServerRows();
  }

  function humanDuration(seconds) {
    const days = Math.floor(seconds / 86400);
    const hours = Math.floor((seconds % 86400) / 3600);
    const minutes = Math.floor((seconds % 3600) / 60);
    if (currentLang === 'en') {
      if (days > 0) return `${days}d ${hours}h ${minutes}min`;
      if (hours > 0) return `${hours}h ${minutes}min`;
      return `${Math.max(1, minutes)} min`;
    }
    if (days > 0) return `${days}天${hours}小时${minutes}分钟`;
    if (hours > 0) return `${hours}小时${minutes}分钟`;
    return `${Math.max(1, minutes)} 分钟`;
  }

  function relativeProbe(server) {
    if (!server.enabled) return t('server.statusStopped') ?? '已停止调度';
    if (!server.last_probe) return t('server.statusAwait') ?? '等待首次采样';
    if (state.data.runtime?.paused) return currentLang === 'en' ? 'Scheduling paused' : '调度已暂停';
    const backingOff = server.failures >= 3 && state.data.config?.smart_backoff && state.data.config?.max_backoff_hours > 0;
    if (server.next_due > Date.now()) {
      const seconds = Math.ceil((server.next_due - Date.now()) / 1000);
      const text = currentLang === 'en' ? (seconds >= 60 ? humanDuration(seconds) : `in ${seconds} s`) : (seconds >= 60 ? `${humanDuration(seconds)}后` : `${seconds} 秒后`);
      return `${backingOff ? (currentLang === 'en' ? 'backoff · ' : '退避 · ') : ''}${text}`;
    }
    return currentLang === 'en' ? 'Waiting for next probe' : '等待下一次探测';
  }

  function rateCell(value, metrics, availability = false) {
    if (!(availability ? hasCoverage(metrics) : hasSamples(metrics))) return '<span class="muted">—</span>';
    const rate = Math.max(0, Math.min(100, Number(value) || 0));
    return `<div class="rate-cell"><span>${number(rate)}%</span><span class="mini-track" aria-hidden="true"><span class="${rate < 90 ? 'bad' : rate < 99 ? 'warning' : ''}" style="width:${rate}%"></span></span></div>`;
  }

  function visibleServers() {
    if (!state.data) return [];
    const search = $('#search-filter').value.trim().toLowerCase();
    const protocol = state.filters.protocol;
    const pollution = state.filters.pollution;
    const grade = state.filters.grade;
    const servers = state.data.servers.filter(server => {
      const metrics = server.metrics || {};
      const value = displayedGrade(currentEvaluation(server), server.trusted);
      const gradeValue = ['A', 'B', 'C', 'D', 'E', 'F', 'unavailable'].includes(value) ? value : 'pending';
      return (!search || [server.name, server.provider, server.address].some(value => String(value || '').toLowerCase().includes(search))) && (!protocol.length || protocol.includes(server.protocol)) && (!pollution.length || pollution.includes(pollutionKind(serverPollution(server)))) && (!grade.length || grade.includes(gradeValue));
    });
    servers.sort((left, right) => {
      // Failed servers stay last for every sort direction and metric.
      const leftUnavailable = currentEvaluation(left).grade === 'unavailable';
      const rightUnavailable = currentEvaluation(right).grade === 'unavailable';
      if (leftUnavailable !== rightUnavailable) return leftUnavailable ? 1 : -1;
      const lm = left.metrics || {};
      const rm = right.metrics || {};
      if (state.sort === 'grade') {
        const grades = ['A', 'B', 'C', 'D', 'E', 'F'];
        const leftRank = grades.indexOf(displayedGrade(currentEvaluation(left), left.trusted));
        const rightRank = grades.indexOf(displayedGrade(currentEvaluation(right), right.trusted));
        if (leftRank < 0 || rightRank < 0) return leftRank < 0 && rightRank < 0 ? left.id - right.id : leftRank < 0 ? 1 : -1;
        return (state.descending ? rightRank - leftRank : leftRank - rightRank) || left.id - right.id;
      }
      const comparable = state.sort === 'average_ms' ? hasLatency : state.sort === 'availability' ? hasCoverage : hasSamples;
      if (!comparable(lm) || !comparable(rm)) return comparable(lm) ? -1 : comparable(rm) ? 1 : left.id - right.id;
      const difference = (lm[state.sort] || 0) - (rm[state.sort] || 0);
      return (state.descending ? -difference : difference) || left.id - right.id;
    });
    return servers;
  }

  function renderServerRows() {
    if (!state.data) return;
    const servers = visibleServers();
    $('#server-rows').innerHTML = servers.map(server => {
      const metrics = server.metrics || {};
      const evaluation = currentEvaluation(server);
      const known = !!server.last_probe;
      const statusClass = !server.enabled || !known ? 'unknown' : server.last_success ? '' : 'offline';
      const status = !server.enabled ? t('server.statusStopped') ?? '已停用' : !known ? t('server.statusAwait') ?? '待探测' : server.last_success ? t('server.statusOk') ?? '查询正常' : t('server.statusDown') ?? '查询异常';
      const trustedMark = server.trusted ? `<span class="trusted-mark">${t('server.trustedMark') ?? '可信'}</span>` : '';
      const historyTitle = `${t('server.viewHistory', { name: server.name }) ?? `查看 ${server.name} 的历史记录`}`;
      const lastProbeTitle = `${t('server.lastProbe', { time: timestamp(server.last_probe, true) }) ?? `最后探测：${timestamp(server.last_probe, true)}`}`;
      const editText = t('server.edit') ?? '编辑';
      const deleteText = t('server.delete') ?? '删除';
      return `<tr data-server-id="${server.id}"><td><button class="server-name-button" data-action="detail" data-id="${server.id}" title="${escape(historyTitle)}"><span class="server-avatar" aria-hidden="true">${escape((server.provider || server.name || 'D').slice(0, 1).toUpperCase())}</span><span><span class="server-name"><span class="name-text">${escape(server.name)}</span>${trustedMark}</span><span class="server-secondary" title="${escape(server.address)}">${escape(server.provider || server.address)}</span></span></button></td><td title="${escape(lastProbeTitle)}"><span class="status-label ${statusClass}"><span class="status-dot"></span>${status}</span><div class="sub-status">${escape(relativeProbe(server))}</div></td><td><span class="protocol-badge">${escape((server.protocol || '—').toUpperCase())}</span></td><td><span class="latency-value">${hasLatency(metrics) ? number(metrics.average_ms, 0) : '—'}<small>ms</small></span></td><td>${rateCell(metrics.availability, metrics, true)}</td><td>${rateCell(metrics.success_rate, metrics)}</td><td>${server.trusted ? trustedBadge : badge(evaluation.pollution)}${compactHistory(server, 'quality')}</td><td>${gradeHTML(displayedGrade(evaluation, server.trusted), evaluation)}${compactHistory(server, 'grade')}</td><td class="actions-cell"><div class="row-actions"><button class="row-action" data-action="edit" data-id="${server.id}" aria-label="${escape(t('server.editAria', { name: server.name }) ?? `编辑 ${server.name}`)}">${editText}</button><button class="row-action delete" data-action="delete" data-id="${server.id}" aria-label="${escape(t('server.deleteAria', { name: server.name }) ?? `删除 ${server.name}`)}">${deleteText}</button></div></td></tr>`;
    }).join('');
    $('#servers-empty').hidden = servers.length > 0;
    if (!servers.length) {
      const hasServers = state.data.servers.length > 0;
      $('#servers-empty h3').textContent = hasServers ? t('server.emptyFilteredTitle') ?? '没有符合条件的服务器' : t('server.emptyAllTitle') ?? '从第一台 DNS 开始';
      $('#servers-empty p').textContent = hasServers ? t('server.emptyFilteredText') ?? '调整搜索内容或筛选条件，继续查看其他服务器。' : t('server.emptyAllText') ?? '添加服务器和可信 DNS，观测台会按配置自动开始采样。';
      $('#empty-add-button').hidden = hasServers;
    }
    $('#table-count').textContent = t('server.count', { shown: servers.length, total: state.data.servers.length }) ?? `显示 ${servers.length} / ${state.data.servers.length} 台服务器`;
    $$('.sort-button').forEach(button => {
      const active = button.dataset.sort === state.sort;
      button.classList.toggle('active', active);
      $('span', button).textContent = active ? (state.descending ? '↓' : '↑') : '↕';
      button.closest('th').setAttribute('aria-sort', active ? state.descending ? 'descending' : 'ascending' : 'none');
    });
  }

  async function navigate(page, boot = false) {
    if (!titles[page] || (state.page === page && !boot)) return;
    if (state.page === 'config' && state.configDirty) {
      if (!(await confirmAction(t('navigate.leaveTitle') ?? '离开探测配置', t('navigate.leaveText') ?? '当前更改尚未保存，离开将放弃这些更改。', t('navigate.leaveButton') ?? '放弃更改'))) return;
      state.configDirty = false;
    }
    state.page = page;
    if (!boot) persistSession();
    $$('.page').forEach(section => { section.hidden = section.id !== `page-${page}`; });
    $$('.nav-item').forEach(button => {
      const active = button.dataset.page === page;
      button.classList.toggle('active', active);
      if (active) button.setAttribute('aria-current', 'page'); else button.removeAttribute('aria-current');
    });
    $('#page-title').textContent = currentLang === 'en' ? (I18N.en['page.' + page] || titles[page][0]) : titles[page][0];
    $('#page-eyebrow').textContent = titles[page][1];
    $('#add-server-button').hidden = page !== 'overview';
    if (page === 'config') populateConfig();
    if (page === 'service') refreshService();
    window.scrollTo({ top: 0, behavior: 'instant' });
  }

  async function changeRange(range) {
    if (range === state.range || !ranges[range]) return;
    state.range = range;
    $$('[data-range]').forEach(button => {
      button.classList.toggle('active', button.dataset.range === range);
      button.setAttribute('aria-pressed', String(button.dataset.range === range));
    });
    // An in-flight refresh may still use the previous range, so refresh again once it finishes.
    while (state.refreshing) await new Promise(resolve => setTimeout(resolve, 50));
    await refreshState();
  }

  function openServer(server = null) {
    const form = $('#server-form');
    form.reset();
    form.elements.id.value = server?.id || '';
    ['name', 'provider', 'address', 'notes'].forEach(field => { form.elements[field].value = server?.[field] || ''; });
    form.elements.enabled.checked = server ? server.enabled : true;
    form.elements.trusted.checked = server ? server.trusted : false;
    $('#server-dialog-title').textContent = server ? t('server.editTitle') ?? '编辑 DNS 服务器' : t('server.addTitle') ?? '添加 DNS 服务器';
    setError('#server-error', '');
    $('#server-dialog').showModal();
  }

  async function saveServer(event) {
    event.preventDefault();
    const form = event.currentTarget;
    const id = Number(form.elements.id.value);
    const body = {};
    ['name', 'provider', 'address', 'notes'].forEach(field => { body[field] = form.elements[field].value.trim(); });
    body.enabled = form.elements.enabled.checked;
    body.trusted = form.elements.trusted.checked;
    if (id) body.id = id;
    const button = $('#save-server-button');
    button.disabled = true;
    setError('#server-error', '');
    try {
      await api(`/servers${id ? `/${id}` : ''}`, { method: id ? 'PUT' : 'POST', body });
      $('#server-dialog').close();
      toast(id ? t('server.updatedToast') ?? '服务器配置已更新' : t('server.addedToast') ?? 'DNS 服务器已添加');
      await refreshState();
      if (state.detailID === id && $('#detail-dialog').open) { renderDetailHeader(); renderResults(); await loadHistory(); }
    } catch (error) { setError('#server-error', error.message); }
    finally { button.disabled = false; }
  }

  async function deleteServer(id) {
    const server = state.data.servers.find(item => item.id === id);
    if (!server || !(await confirmAction(t('server.deleteTitle') ?? '删除 DNS 服务器', t('server.deleteText', { name: server.name }) ?? `确定删除“${server.name}”？\n该服务器及其关联历史数据将被删除，此操作无法撤销。`, t('server.deleteButton') ?? '删除服务器'))) return;
    try {
      await api(`/servers/${id}`, { method: 'DELETE' });
      if (state.detailID === id) $('#detail-dialog').close();
      toast(t('server.deletedToast') ?? '服务器已删除');
      await refreshState();
    } catch (error) { toast(error.message, true); }
  }

  function domainRow(domain = { name: '', type: 'A' }) {
    const row = document.createElement('div');
    row.className = 'domain-row';
    const types = ['A', 'CNAME', 'TXT', 'NS', 'MX', 'SOA', 'SRV', 'CAA', 'HTTPS', 'SVCB', 'PTR'];
    row.innerHTML = `<input type="text" class="domain-name" aria-label="${escape(t('server.domainAria') ?? '探测域名')}" placeholder="www.youtube.com" spellcheck="false" maxlength="253" required value="${escape(domain.name)}"><select class="domain-type-select" aria-label="${escape(t('server.typeAria') ?? '记录类型')}">${types.map(type => `<option value="${type}"${type === domain.type ? ' selected' : ''}>${type}</option>`).join('')}</select><button class="icon-button remove-domain" type="button" aria-label="${escape(t('server.removeDomainAria') ?? '删除这个探测域名')}">×</button>`;
    return row;
  }

  function validateConfigNumber(input, showEmpty = false) {
    input.setCustomValidity('');
    const range = `${input.min}–${input.max} ${unitLabel(input.dataset.unit)}`;
    const validity = input.validity;
    let message = '';
    if (validity.badInput) message = t('config.number', { range }) ?? `请输入 ${range} 范围内的数字。`;
    else if (input.value.trim() === '' || validity.rangeUnderflow || validity.rangeOverflow) message = t('config.value', { range }) ?? `请输入 ${range} 范围内的数值。`;
    else if (validity.stepMismatch) message = input.step === '1' ? t('config.integer', { range }) ?? `请输入 ${range} 范围内的整数。` : t('config.decimal', { range }) ?? `请输入 ${range} 范围内的数值，最多保留 1 位小数。`;
    if (!message && input.name === 'rating_min_coverage_minutes') {
      const window = $('#config-form').elements.rating_window_minutes;
      if (window.value !== '' && window.validity.valid && input.valueAsNumber > window.valueAsNumber) message = t('config.coverage', { window: window.value }) ?? `最小覆盖不能超过评级观察窗口（${window.value} 分钟）。`;
    }
    const visible = !!message && (showEmpty || input.value !== '' || validity.badInput);
    input.setCustomValidity(message);
    input.setAttribute('aria-invalid', String(visible));
    input.closest('.input-unit').classList.toggle('invalid', visible);
    const hint = $(`#${input.id}-error`);
    hint.textContent = visible ? message : '';
    hint.hidden = !visible;
    if (input.name === 'rating_window_minutes') validateConfigNumber($('#config-form').elements.rating_min_coverage_minutes);
    return !message;
  }

  function populateConfig() {
    if (!state.data?.config) return;
    const form = $('#config-form');
    const config = state.data.config;
    ['listen', 'interval_seconds', 'timeout_seconds', 'concurrency', 'max_backoff_hours', 'reference_ttl_seconds', 'reference_history_hours', 'rating_window_minutes', 'rating_min_samples', 'rating_min_coverage_minutes'].forEach(field => { form.elements[field].value = config[field] ?? ''; });
    $$('#config-form input[type=number]').forEach(input => validateConfigNumber(input));
    form.elements.smart_backoff.checked = config.smart_backoff;
    $('#domain-rows').replaceChildren(...(config.domains || []).map(domainRow));
    if (!config.domains?.length) $('#domain-rows').append(domainRow());
    state.configDirty = false;
    $('#config-save-state').textContent = t('config.loaded') ?? '当前配置已加载';
    $('#config-save-state').classList.remove('unsaved');
    setError('#config-error', '');
  }

  function markConfigDirty() {
    state.configDirty = true;
    $('#config-save-state').textContent = t('config.dirty') ?? '有尚未保存的更改';
    $('#config-save-state').classList.add('unsaved');
  }

  async function saveConfig(event) {
    event.preventDefault();
    const form = event.currentTarget;
    $$('#config-form input[type=number]').forEach(input => validateConfigNumber(input, true));
    if (!form.reportValidity()) return;
    const body = { listen: form.elements.listen.value.trim(), smart_backoff: form.elements.smart_backoff.checked, domains: $$('.domain-row').map(row => ({ name: $('.domain-name', row).value.trim(), type: $('.domain-type-select', row).value })) };
    ['interval_seconds', 'timeout_seconds', 'concurrency', 'max_backoff_hours', 'reference_ttl_seconds', 'reference_history_hours', 'rating_window_minutes', 'rating_min_samples', 'rating_min_coverage_minutes'].forEach(field => { body[field] = Number(form.elements[field].value); });
    const button = $('#save-config-button');
    button.disabled = true;
    setError('#config-error', '');
    try {
      const response = await api('/config', { method: 'PUT', body });
      state.data.config = response.config || body;
      state.configDirty = false;
      $('#config-save-state').textContent = t('config.saved') ?? '配置已保存';
      $('#config-save-state').classList.remove('unsaved');
      $('#restart-notice').hidden = !response.restart_required;
      const savedToast = response.restart_required ? t('config.restartToast') ?? '配置已保存，监听地址将在重启后生效' : body.concurrency === 0 ? t('config.pausedToast') ?? '配置已保存，全部探测已暂停' : t('config.savedToast') ?? '探测配置已保存';
      toast(savedToast);
      await refreshState();
    } catch (error) { setError('#config-error', error.message); }
    finally { button.disabled = false; }
  }

  async function openDetail(id) {
    state.detailID = id;
    state.historyMetrics = null;
    state.detailRange = state.range;
    state.results = [];
    state.history = [];
    state.statusHistory = [];
    state.resultEnd = false;
    renderDetailHeader();
    $('#detail-metrics').innerHTML = '';
    $('#status-history').innerHTML = `<p class="field-help">${t('detail.loadingHistory') ?? '正在读取状态历史…'}</p>`;
    $('#result-rows').innerHTML = `<tr><td colspan="5" class="muted" style="text-align:center;padding:28px">${t('detail.loadingRecords') ?? '正在读取原始记录…'}</td></tr>`;
    $('#results-count').textContent = '';
    $('#results-more').hidden = true;
    setError('#detail-error', '');
    $('#detail-dialog').showModal();
    renderCharts();
    await Promise.allSettled([loadHistory(), loadResults(true)]);
  }

  function renderDetailHeader() {
    const server = currentServer();
    if (!server) return;
    $('#detail-title').textContent = server.name;
    $('#detail-address').textContent = server.address;
    $('#detail-tags').innerHTML = `<span class="protocol-badge">${escape((server.protocol || '—').toUpperCase())}</span>${server.provider ? `<span class="tag">${escape(server.provider)}</span>` : ''}${server.trusted ? `<span class="tag">${t('detail.trustedTag') ?? '用户可信 DNS'}</span>` : ''}<span class="tag">${server.enabled ? t('detail.enabledTag') ?? '已启用' : t('detail.stoppedTag') ?? '已停用'}</span><span class="tag">${t('detail.recent', { time: timestamp(server.last_probe) }) ?? `最近 ${escape(timestamp(server.last_probe))}`}</span>`;
    $('#detail-probe').disabled = !server.enabled || !!state.data.runtime?.paused || state.data.config?.concurrency === 0 || state.detailProbe?.id === server.id;
    $('#detail-probe').textContent = state.detailProbe?.id === server.id ? t('detail.probeWaiting') ?? '等待探测结果…' : t('detail.probeNow') ?? '立即探测';
    $$('[data-detail-range]').forEach(button => {
      const active = button.dataset.detailRange === state.detailRange;
      button.classList.toggle('active', active);
      button.setAttribute('aria-pressed', String(active));
    });
  }

  function renderDetailCurrent(evaluation = currentEvaluation(currentServer())) {
    const target = $('#detail-current-evaluation');
    if (!target || !state.historyMetrics) return;
    target.innerHTML = `<span>${t('detail.currentRating') ?? '当前评级'}</span>${gradeHTML(displayedGrade(evaluation, currentServer()?.trusted), evaluation)}`;
    const samples = integer(state.historyMetrics.samples || 0);
    const quality = currentServer()?.trusted ? t('state.trustedGood') ?? '用户可信 / 无污染' : pollutionLabel(evaluation.pollution);
    const windowMin = integer(evaluation.window_minutes || state.data?.config?.rating_window_minutes || 60);
    $('#chart-note').textContent = t('detail.chartNote', { samples, quality, window: windowMin }) ?? `图表和性能指标按所选历史区间统计，空白表示未采样；区间内 ${samples} 次采样。当前解析质量：${quality}；当前评级窗口：最近 ${windowMin} 分钟。`;
  }

  async function loadHistory() {
    const request = ++state.detailRequest;
    const id = state.detailID;
    const range = state.detailRange;
    renderDetailHeader();
    setError('#detail-error', '');
    state.statusHistory = [];
    $('#status-history').innerHTML = `<p class="field-help">${t('detail.loadingHistory') ?? '正在读取状态历史…'}</p>`;
    try {
      const response = await api(`/servers/${id}/history?range=${range}`);
      if (request !== state.detailRequest || id !== state.detailID || range !== state.detailRange) return;
      state.history = (response.points || []).slice().sort((left, right) => left.timestamp - right.timestamp);
      state.statusHistory = response.status_history || [];
      renderStatusHistory();
      const metrics = response.metrics || {};
      state.historyMetrics = metrics;
      const evaluation = response.current || currentEvaluation(currentServer());
      $('#detail-metrics').innerHTML = `<div class="detail-metric"><span>${t('detail.metricsTitle') ?? '可用率'}</span><strong>${percent(metrics.availability, metrics, true)}</strong></div><div class="detail-metric"><span>${t('detail.metricsSuccess') ?? '查询成功率'}</span><strong>${percent(metrics.success_rate, metrics)}</strong></div><div class="detail-metric"><span>${t('detail.metricsP95') ?? 'P95 时延'}</span><strong>${hasLatency(metrics) ? number(metrics.p95_ms, 0) : '—'}<small>ms</small></strong></div><div class="detail-metric"><span>${t('detail.metricsSamples') ?? '原始采样'}</span><strong>${integer(metrics.samples || 0)}<small>${t('unit.samples') ?? '次'}</small></strong></div><div class="detail-metric"><span>${t('detail.metricsCoverage') ?? '时间覆盖率'}</span><strong>${hasSamples(metrics) ? number(metrics.coverage) + '%' : '—'}</strong></div><div class="detail-metric" id="detail-current-evaluation"></div>`;
      renderDetailCurrent(evaluation);
      renderCharts();
    } catch (error) { if (request === state.detailRequest) { $('#status-history').innerHTML = `<p class="field-help">${t('detail.historyFailed') ?? '状态历史读取失败，请重试。'}</p>`; setError('#detail-error', `${t('detail.historyError', { error: error.message }) ?? `历史读取失败：${error.message}`}`); } }
  }

  async function loadResults(reset = false) {
    const id = state.detailID;
    if (state.loadingResults === id) return;
    const request = ++state.resultsRequest;
    state.loadingResults = id;
    $('#results-more').disabled = true;
    $('#results-refresh').disabled = true;
    try {
      const last = state.results.at(-1);
      const before = !reset && last ? `&before=${last.timestamp}&before_id=${last.id}` : '';
      const response = await api(`/servers/${id}/results?limit=100${before}`);
      if (id !== state.detailID || request !== state.resultsRequest) return;
      const results = response.results || [];
      state.results = reset ? results : [...state.results, ...results];
      state.resultEnd = results.length < 100;
      renderResults();
    } catch (error) { if (id === state.detailID) setError('#detail-error', `${t('detail.resultsError', { error: error.message }) ?? `原始记录读取失败：${error.message}`}`); }
    finally {
      if (request === state.resultsRequest) {
        state.loadingResults = null;
        $('#results-more').disabled = false;
        $('#results-refresh').disabled = false;
      }
    }
  }

  function effectivePollution(result) {
    if (trustedResult(result)) return 'clean';
    if (result.override === 'clean' || result.override === 'polluted') return result.override;
    return result.effective_pollution ?? result.pollution;
  }

  function renderResults() {
    $('#result-rows').innerHTML = state.results.length ? state.results.map((result, index) => `<tr><td>${escape(timestamp(result.timestamp, true))}</td><td><button class="domain-button ${pollutionKind(effectivePollution(result))}" data-result-index="${index}" title="${escape(t('detail.domainTitle') ?? '查看解析证据与人工判定')}">${['polluted', 'suspicious'].includes(effectivePollution(result)) ? `<span aria-label="${escape(t('detail.anomalyAria') ?? '解析异常标记')}">⚑</span>` : ''}${escape(result.domain)}<span aria-hidden="true">⌄</span></button><span class="domain-type">${escape(result.type)}</span></td><td title="${escape(result.error || '')}"><span class="result-code${result.success ? '' : ' failed'}">${escape(result.rcode || (result.received ? t('detail.rcodeError') ?? '响应异常' : t('detail.rcodeNone') ?? '无响应'))}</span></td><td>${result.received ? number(result.latency_ms, 0) + ' ms' : '—'}</td><td>${trustedResult(result) ? trustedBadge : badge(effectivePollution(result), result.override === 'polluted')}${!trustedResult(result) && result.override && result.override !== 'auto' ? `<small class="muted" style="margin-left:5px;font-size:13px">${t('verdict.manual') ?? '人工'}</small>` : ''}</td></tr>`).join('') : `<tr><td colspan="5" class="muted" style="text-align:center;padding:35px">${t('detail.emptyResults') ?? '尚无探测记录。启用服务器并等待下一轮采样，或点击“立即探测”。'}</td></tr>`;
    $('#results-count').textContent = t('detail.resultsCount', { count: integer(state.results.length) }) ?? `已显示 ${integer(state.results.length)} 条记录 · 从新到旧`;
    $('#results-more').hidden = state.resultEnd || !state.results.length;
  }

  async function probeNow() {
    const server = currentServer();
    if (!server || state.detailProbe?.id === server.id) return;
    state.detailProbe = { id: server.id, baseline: server.last_probe || 0, failures: 0 };
    renderDetailHeader();
    try {
      await api(`/servers/${server.id}/probe`, { method: 'POST', body: {} });
      toast(t('detail.probeQueued') ?? '已加入探测队列，正在等待新的真实结果');
      pollDetailProbe();
    } catch (error) {
      state.detailProbe = null;
      toast(error.message, true);
      renderDetailHeader();
    }
  }

  async function pollDetailProbe() {
    clearTimeout(state.detailProbeTimer);
    const tracking = state.detailProbe;
    if (!tracking || !state.token) return;
    if (!$('#detail-dialog').open || tracking.id !== state.detailID) {
      state.detailProbe = null;
      return;
    }
    const response = await refreshState();
    if (state.detailProbe !== tracking) return;
    const server = state.data?.servers.find(item => item.id === tracking.id);
    if (server && server.last_probe > tracking.baseline) {
      state.detailProbe = null;
      await Promise.allSettled([loadHistory(), loadResults(true)]);
      renderDetailHeader();
      toast(t('detail.probeWritten') ?? '新的探测结果已写入并显示');
      return;
    }
    if (!server || !server.enabled || response?.runtime?.paused || response?.config?.concurrency === 0) {
      state.detailProbe = null;
      renderDetailHeader();
      setError('#detail-error', t('detail.probePaused') ?? '探测已暂停或服务器已停用，请检查配置后重试。');
      return;
    }
    if (!response && !state.refreshing) tracking.failures++; else if (response) tracking.failures = 0;
    if (tracking.failures >= 5) {
      state.detailProbe = null;
      renderDetailHeader();
      setError('#detail-error', t('detail.probeProgressFail') ?? '无法读取新探测进度。后台任务可能仍在运行，请恢复连接后更新记录。');
      return;
    }
    state.detailProbeTimer = setTimeout(pollDetailProbe, 1000);
  }

  function answerEvidence(answer, queryTime, reference = false) {
    if (!answer.records?.length) return answer.answers?.join('\n') || answer.error || (t('evid.noAnswer') ?? '没有返回答案');
    return answer.records.map(record => {
      const ttl = Number(record.ttl_seconds);
      const ttlText = Number.isFinite(ttl) && ttl >= 0 ? t('evid.ttl', { ttl: integer(ttl) }) ?? ` · TTL ${integer(ttl)} 秒` : '';
      const freshness = reference && record.expires_at && queryTime ? record.expires_at > queryTime ? t('evid.fresh') ?? ' · 当时新鲜' : t('evid.historical') ?? ' · 近期历史' : '';
      return `${record.value}${ttlText}${freshness}`;
    }).join('\n');
  }

  function showVerdict(index) {
    const result = state.results[index];
    if (!result) return;
    state.result = result;
    $('[data-verdict="polluted"]').hidden = trustedResult(result);
    $('[data-verdict="polluted"]').disabled = trustedResult(result);
    $('#verdict-domain').textContent = result.domain;
    $('#verdict-subtitle').textContent = `${result.type} · ${timestamp(result.timestamp, true)} · ${currentServer()?.name || ''}`;
    const references = result.references || [];
    const legacy = !trustedResult(result) && (result.policy_version || 0) < 2 && result.pollution === 'polluted' && effectivePollution(result) === 'unknown';
    const reason = trustedResult(result) ? t('verdict.trustedReason') ?? '此服务器由用户指定为可信 DNS，其解析质量固定标为无污染。' : legacy ? t('verdict.legacyReason') ?? '旧版自动判定，仅保留原始证据，不参与当前 E/F 评级。' : result.reason || (t('verdict.noReason') ?? '当前没有可用于判定的充分证据。');
    const evidenceBadge = trustedResult(result) ? trustedBadge : badge(effectivePollution(result), result.override === 'polluted') + `<span class="tag">${result.override && result.override !== 'auto' ? t('verdict.manual') ?? '人工判定' : t('verdict.auto') ?? '自动判定'}</span><span class="tag">${t('verdict.original') ?? '原始：'}${pollutionLabel(result.pollution)}${legacy ? t('verdict.legacySuffix') ?? '（旧规则）' : ''}</span>`;
    const referenceRows = references.map(reference => `<div class="reference-item"><div class="reference-heading"><span>${escape(reference.address)} · ${escape(reference.rcode || (reference.success ? t('verdict.refOk') ?? '成功' : t('verdict.refFail') ?? '失败'))}</span><span>${escape(timestamp(reference.timestamp, true))}</span></div><pre class="evidence-answers">${escape(answerEvidence(reference, result.compared_at || result.timestamp, true))}</pre>${reference.raw ? `<details class="raw-details"><summary>${t('verdict.rawRef') ?? '参考原始输出'}</summary><pre class="evidence-answers">${escape(reference.raw)}</pre></details>` : ''}</div>`).join('');
    $('#verdict-evidence').innerHTML = `<div class="evidence-summary">${evidenceBadge}<span class="tag">${escape(result.rcode || (t('detail.rcodeNone') ?? '无响应'))}</span>${result.received ? `<span class="tag">${number(result.latency_ms)} ms</span>` : ''}</div><p class="evidence-reason">${escape(reason)}${result.error ? `<br>${t('evid.queryError') ?? '查询错误：'}${escape(result.error)}` : ''}</p><section class="evidence-section"><h4>${t('verdict.answersTitle') ?? '被测服务器的答案'}</h4><pre class="evidence-answers">${escape(answerEvidence(result, result.timestamp))}</pre></section><section class="evidence-section"><h4>${t('verdict.refTitle', { count: references.length }) ?? `可信参考 <span class="muted">${references.length} 条</span>`}</h4>${references.length ? referenceRows : `<p class="field-help">${t('verdict.noRef') ?? '本条记录没有可信参考。可在服务器管理中启用用户可信 DNS。'}</p>`}</section>${result.raw ? `<details class="raw-details"><summary>${t('verdict.rawDoggo') ?? '查看 doggo 原始输出'}</summary><pre class="evidence-answers">${escape(result.raw)}</pre></details>` : ''}`;
    $('#verdict-note').value = '';
    setError('#verdict-error', '');
    $('#verdict-dialog').showModal();
  }

  async function saveVerdict(verdict) {
    const result = state.result;
    if (!result) return;
    if (verdict === 'polluted' && trustedResult(result)) return toast(t('verdict.trustedBlocked') ?? '用户可信 DNS 不能标记为污染', true);
    const buttons = $$('[data-verdict]');
    buttons.forEach(button => { button.disabled = true; });
    setError('#verdict-error', '');
    try {
      await api('/overrides', { method: 'PUT', body: { server_id: result.server_id, domain: result.domain, type: result.type, verdict, note: $('#verdict-note').value.trim() } });
      $('#verdict-dialog').close();
      toast(verdict === 'auto' ? t('verdict.autoToast') ?? '已恢复自动判定' : verdict === 'clean' ? t('verdict.cleanToast') ?? '已将该域名判定为正常' : t('verdict.pollutedToast') ?? '已将该域名标记为污染');
      await Promise.allSettled([refreshState(), loadHistory(), loadResults(true)]);
    } catch (error) { setError('#verdict-error', error.message); }
    finally { buttons.forEach(button => { button.disabled = button.dataset.verdict === 'polluted' && trustedResult(result); }); }
  }

  const chartState = new Map();

  function renderCharts() {
    if (!$('#detail-dialog').open) return;
    drawChart($('#rate-chart'), [{ key: 'availability', label: t('detail.chartSeriesAvail') ?? '可用率', color: '#14877d' }, { key: 'success_rate', label: t('detail.chartSeriesSuccess') ?? '成功率', color: '#589ad9' }], true);
    drawChart($('#latency-chart'), [{ key: 'p95_ms', label: t('detail.chartSeriesP95') ?? 'P95', color: '#9a85c7' }, { key: 'average_ms', label: t('detail.chartSeriesAvg') ?? '平均', color: '#14877d' }], false);
  }

  function drawChart(canvas, series, percentage) {
    const rect = canvas.getBoundingClientRect();
    const width = Math.max(200, rect.width);
    const height = Math.max(140, rect.height);
    const scale = Math.min(window.devicePixelRatio || 1, 2);
    canvas.width = Math.round(width * scale);
    canvas.height = Math.round(height * scale);
    const context = canvas.getContext('2d');
    context.scale(scale, scale);
    const padding = { left: 49, right: 22, top: 18, bottom: 35 };
    const plot = { x: padding.left, y: padding.top, width: width - padding.left - padding.right, height: height - padding.top - padding.bottom };
    const points = state.history;
    const now = state.data?.now || Date.now();
    const end = Math.max(now, points.at(-1)?.timestamp || 0);
    const start = end - ranges[state.detailRange];
    let maximum = percentage ? 100 : points.reduce((max, point) => hasSamples(point) ? Math.max(max, ...series.map(item => Number(point[item.key]) || 0)) : max, 0);
    if (!percentage) {
      maximum = Math.max(10, maximum * 1.15);
      const step = 10 ** Math.floor(Math.log10(maximum));
      maximum = Math.ceil(maximum / step) * step;
    }
    context.font = '12px "Segoe UI", "Microsoft YaHei", sans-serif';
    context.textBaseline = 'middle';
    context.strokeStyle = '#edf1f3';
    context.lineWidth = 1;
    for (let i = 0; i <= 4; i++) {
      const y = plot.y + plot.height * i / 4;
      context.beginPath(); context.moveTo(plot.x, y); context.lineTo(plot.x + plot.width, y); context.stroke();
      context.fillStyle = '#5e6c75'; context.textAlign = 'right';
      const value = maximum * (4 - i) / 4;
      context.fillText(`${value >= 1000 ? number(value / 1000, 1) + 'k' : number(value, value < 10 ? 1 : 0)}${percentage ? '%' : ''}`, plot.x - 8, y);
    }
    context.textAlign = 'center';
    for (let i = 0; i <= 4; i++) {
      const time = start + (end - start) * i / 4;
      const label = state.detailRange === '24h' ? new Date(time).toLocaleTimeString(locale(), { hour: '2-digit', minute: '2-digit', hour12: false }) : new Date(time).toLocaleDateString(locale(), { month: '2-digit', day: '2-digit' });
      context.fillText(label, plot.x + plot.width * i / 4, plot.y + plot.height + 19);
    }
    const valid = percentage ? point => hasCoverage(point) || hasSamples(point) : hasLatency;
    const usable = points.filter(valid);
    if (!usable.length) {
      context.fillStyle = '#637781';
      context.font = '13px "Segoe UI", "Microsoft YaHei", sans-serif';
      context.fillText(t('detail.chartEmpty') ?? '尚无采样，数据到来后将在这里呈现', plot.x + plot.width / 2, plot.y + plot.height / 2);
    } else {
      const interval = points.length > 1 ? Math.max(1, points[1].timestamp - points[0].timestamp) : ranges[state.detailRange] / 96;
      for (const item of series) {
        const seriesValid = item.key === 'availability' ? hasCoverage : percentage ? hasSamples : hasLatency;
        context.strokeStyle = item.color;
        context.lineWidth = 1.8;
        context.lineJoin = 'round';
        let started = false;
        let previousTime = 0;
        context.beginPath();
        for (const point of points) {
          if (!seriesValid(point) || point.timestamp < start || !Number.isFinite(Number(point[item.key]))) { started = false; continue; }
          const x = plot.x + (point.timestamp - start) / (end - start) * plot.width;
          const y = plot.y + plot.height - Math.max(0, Math.min(maximum, Number(point[item.key]))) / maximum * plot.height;
          if (started && point.timestamp - previousTime <= interval * 1.6) context.lineTo(x, y); else context.moveTo(x, y);
          started = true;
          previousTime = point.timestamp;
        }
        context.stroke();
        context.fillStyle = item.color;
        if (usable.length < 70) for (const point of usable) {
          if (!seriesValid(point) || point.timestamp < start) continue;
          const x = plot.x + (point.timestamp - start) / (end - start) * plot.width;
          const y = plot.y + plot.height - Math.max(0, Math.min(maximum, Number(point[item.key]) || 0)) / maximum * plot.height;
          context.beginPath(); context.arc(x, y, 2.1, 0, Math.PI * 2); context.fill();
        }
      }
    }
    chartState.set(canvas, { plot, start, end, series, percentage, points: usable });
    canvas.setAttribute('aria-label', t('detail.chartAria', { union: percentage ? t('detail.chartRateTitle') ?? '可用率与查询成功率' : t('detail.chartLatencyTitle') ?? '响应时延', count: usable.length }) ?? `${percentage ? '可用率与查询成功率' : '响应时延'}历史图，${usable.length} 个有样本的时间段。具体汇总指标见上方。`);
  }

  function chartHover(event) {
    const canvas = event.currentTarget;
    const data = chartState.get(canvas);
    const tooltip = $('.chart-tooltip', canvas.parentElement);
    if (!data?.points.length) { tooltip.hidden = true; return; }
    const rect = canvas.getBoundingClientRect();
    const x = event.clientX - rect.left;
    if (x < data.plot.x || x > data.plot.x + data.plot.width) { tooltip.hidden = true; return; }
    const time = data.start + (x - data.plot.x) / data.plot.width * (data.end - data.start);
    const closest = data.points.reduce((best, point) => Math.abs(point.timestamp - time) < Math.abs(best.timestamp - time) ? point : best);
    tooltip.innerHTML = `<strong>${escape(timestamp(closest.timestamp))}</strong>${data.series.map(item => `<span>${item.label}：${(item.key === 'availability' ? hasCoverage(closest) : data.percentage ? hasSamples(closest) : hasLatency(closest)) ? number(closest[item.key]) + (data.percentage ? '%' : ' ms') : '—'}</span>`).join('')}<span>${t('detail.chartSamples', { count: integer(closest.samples) }) ?? `${integer(closest.samples)} 次采样`}</span>`;
    tooltip.hidden = false;
    tooltip.style.left = `${Math.max(0, Math.min(x + 10, rect.width - tooltip.offsetWidth - 5))}px`;
  }

  function renderService() {
    const service = state.data?.service || {};
    const serviceState = String(service.state || '').toLowerCase();
    const running = serviceState === 'running';
    const installed = !!service.installed;
    const canManage = service.can_manage !== false && !state.serviceBusy && !['starting', 'stopping', 'pending', 'unavailable'].includes(serviceState);
    const names = { running: t('service.namesRunning') ?? '正在运行', stopped: t('service.namesStopped') ?? '已停止', start_pending: t('service.namesStarting') ?? '启动中', stop_pending: t('service.namesStopping') ?? '停止中', paused: t('service.namesPaused') ?? '已暂停', starting: t('service.namesStarting') ?? '启动中', stopping: t('service.namesStopping') ?? '停止中', pending: t('service.namesPending') ?? '状态转换中', unavailable: t('service.namesUnavailable') ?? '暂时无法读取', unknown: t('service.namesUnknown') ?? '状态未知', not_installed: t('service.namesNotInstalled') ?? '尚未安装', absent: t('service.namesNotInstalled') ?? '尚未安装' };
    $('#service-state').textContent = names[serviceState] || (installed ? (service.state || (t('service.namesUnknown') ?? '状态未知')) : (t('service.namesNotInstalled') ?? '尚未安装'));
    $('#service-description').textContent = service.message || (state.serviceBusy ? t('service.busy') ?? '正在执行服务管理操作，请稍候。' : !canManage ? t('service.transitioning') ?? '服务状态正在变化或暂时不可用，请刷新后重试。' : running ? t('service.runningDesc') ?? 'Windows 正在后台运行 DNS Monitor。' : installed ? t('service.installedDesc') ?? '服务已注册，可在此启动或移除。' : t('service.plainDesc') ?? '当前以普通程序方式运行，可按需安装为服务。');
    const buttons = !installed ? [{ action: 'install', text: t('service.installButton') ?? '安装 Windows 服务', primary: true }] : [{ action: running ? 'stop' : 'start', text: running ? t('service.stopButton') ?? '停止服务' : t('service.startButton') ?? '启动服务', primary: !running }, ...(running ? [{ action: 'restart', text: t('service.restartButton') ?? '重启服务' }] : []), { action: 'uninstall', text: t('service.uninstallButton') ?? '卸载服务', danger: true }];
    $('#service-buttons').innerHTML = buttons.map(button => `<button class="button ${button.primary ? 'primary' : button.danger ? 'danger' : 'secondary'}" data-service-action="${button.action}"${canManage ? '' : ' disabled'}>${button.text}</button>`).join('') + `<button class="button subtle" data-service-action="refresh">${t('service.refreshButton') ?? '↻ 刷新状态'}</button>`;
  }

  async function refreshService() {
    if (!state.data) return;
    try { state.data.service = await api('/service'); renderService(); }
    catch (error) { $('#service-feedback').textContent = t('service.readError', { error: error.message }) ?? `无法读取服务状态：${error.message}`; }
  }

  async function serviceAction(action) {
    if (action === 'refresh') return refreshService();
    if (state.serviceBusy) return;
    const names = { install: t('service.actionInstall') ?? '安装', uninstall: t('service.actionUninstall') ?? '卸载', start: t('service.actionStart') ?? '启动', stop: t('service.actionStop') ?? '停止', restart: t('service.actionRestart') ?? '重启' };
    if (['stop', 'restart', 'uninstall'].includes(action)) {
      const message = action === 'uninstall' ? t('service.uninstallMsg') ?? '将移除 Windows 服务注册。配置与监测数据保留；正在运行的服务可能停止，网页连接会中断。' : t('service.manageMsg', { action: names[action], extra: action === 'restart' ? t('service.restartExtra') ?? '服务恢复后请重新登录。' : t('service.stopExtra') ?? '再次启动服务或主程序后才能继续访问。' }) ?? `即将${names[action]} Windows 服务。网页连接可能中断，${action === 'restart' ? '服务恢复后请重新登录。' : '再次启动服务或主程序后才能继续访问。'}`;
      if (!(await confirmAction(`${names[action]} Windows 服务`, message, t('service.confirmPrefix', { action: names[action] }) ?? `确认${names[action]}`))) return;
    }
    state.serviceBusy = true;
    $('[data-service-action]').forEach(button => { button.disabled = true; });
    $('#service-feedback').textContent = t('service.feedbackBusy', { action: names[action] }) ?? `正在${names[action]}服务；如果出现 Windows 管理员授权窗口，请在本机完成授权。`;
    try {
      const response = await api(`/service/${action}`, { method: 'POST', body: {} });
      $('#service-feedback').textContent = response.message || (t('service.feedbackDone', { action: names[action] }) ?? `已提交${names[action]}请求。`);
      toast(response.message || (t('service.feedbackToast', { action: names[action] }) ?? `服务${names[action]}请求已提交`));
      if (!['stop', 'restart', 'uninstall'].includes(action)) setTimeout(refreshService, 1500);
    } catch (error) {
      const message = ['stop', 'restart', 'uninstall'].includes(action) && error.message === 'Failed to fetch' ? t('service.connGone') ?? '连接已中断，可能是服务正在停止。请在本机检查服务状态，恢复后重新登录。' : t('service.opFailed', { error: error.message }) ?? `服务操作失败：${error.message}`;
      $('#service-feedback').textContent = message;
      toast(message, true);
    } finally { state.serviceBusy = false; renderService(); }
  }

  function summaryCSVCell(value) {
    let text = String(value ?? '');
    if (/^[=+\-@\t\r\n]/.test(text) || /^[\s\uFEFF]*[=+\-@]/.test(text)) text = "'" + text;
    return '"' + text.replace(/"/g, '""') + '"';
  }

  function summaryCSV() {
    const servers = visibleServers();
    const range = state.loadedRange || state.range;
    const snapshot = new Date(state.data?.now || Date.now()).toISOString();
    const headers = [t('csv.hName') ?? '名称', t('csv.hProvider') ?? '供应商', t('csv.hAddress') ?? '服务器地址', t('csv.hProtocol') ?? '协议', t('csv.hStatus') ?? '当前状态', t('csv.hAvgLatency') ?? '平均时延(ms)', t('csv.hP95Latency') ?? 'P95时延(ms)', t('csv.hAvailability') ?? '可用率(%)', t('csv.hSuccessRate') ?? '查询成功率(%)', t('csv.hQuality') ?? '当前解析质量', t('csv.hGrade') ?? '当前评级', t('csv.hSamples') ?? '历史区间样本数', t('csv.hCoverage') ?? '覆盖率(%)', t('csv.hLastProbe') ?? '最近探测时间(UTC)', t('csv.hNextProbe') ?? '下次探测时间(UTC)', t('csv.hRange') ?? '统计区间', t('csv.hSnapshot') ?? '数据快照时间(UTC)', t('csv.hTrusted') ?? '用户可信', t('csv.hEnabled') ?? '已启用', t('csv.hWindow') ?? '评级窗口(分钟)', t('csv.hFormalSamples') ?? '评级正式采样数', t('csv.hCoveredMinutes') ?? '评级有效覆盖(分钟)', t('csv.hScore') ?? '近期性能评分', t('csv.hGradeNote') ?? '评级说明'];
    const rows = servers.map(server => {
      const metrics = server.metrics || {};
      const status = !server.enabled ? t('csv.statusDisabled') ?? '已停用' : !server.last_probe ? t('csv.statusPending') ?? '待探测' : server.last_success ? t('csv.statusOk') ?? '查询正常' : t('csv.statusError') ?? '查询异常';
      const evaluation = currentEvaluation(server);
      const grade = displayedGrade(evaluation, server.trusted);
      return [server.name, server.provider, server.address, server.protocol, status, hasLatency(metrics) ? metrics.average_ms : '', hasLatency(metrics) ? metrics.p95_ms : '', hasCoverage(metrics) ? metrics.availability : '', hasSamples(metrics) ? metrics.success_rate : '', server.trusted ? t('csv.trustedClean') ?? '用户可信 / 无污染' : pollutionLabel(evaluation.pollution), grade === 'unavailable' ? t('state.unavailable') ?? '不可用' : ['A', 'B', 'C', 'D', 'E', 'F'].includes(grade) ? grade : t('state.pending') ?? '待评估', metrics.samples || 0, metrics.coverage || 0, server.last_probe ? new Date(server.last_probe).toISOString() : '', server.next_due ? new Date(server.next_due).toISOString() : '', range, snapshot, server.trusted ? t('csv.yes') ?? '是' : t('csv.no') ?? '否', server.enabled ? t('csv.yes') ?? '是' : t('csv.no') ?? '否', evaluation.window_minutes ?? '', evaluation.samples ?? '', evaluation.covered_minutes ?? '', evaluation.score ?? '', gradeHint(grade, evaluation)];
    });
    return { text: '\uFEFF' + [headers, ...rows].map(row => row.map(summaryCSVCell).join(',')).join('\r\n') + '\r\n', count: servers.length, range };
  }

  function exportSummary() {
    if (!state.data || state.loadedRange !== state.range || state.refreshing) return toast(t('csv.exportBusy') ?? '正在更新所选区间，请等待数据加载后导出', true);
    const result = summaryCSV();
    const blob = new Blob([result.text], { type: 'text/csv;charset=utf-8' });
    const url = URL.createObjectURL(blob);
    const link = document.createElement('a');
    link.href = url;
    link.download = 'dns-monitor-current-' + result.range + '-' + new Date().toISOString().slice(0, 10) + '.csv';
    document.body.append(link);
    link.click();
    link.remove();
    setTimeout(() => URL.revokeObjectURL(url), 30000);
    toast(t('csv.exportDone', { count: result.count }) ?? '已导出当前结果：' + result.count + ' 台服务器，保留当前筛选与排序');
  }

  async function exportCSV(serverID = null) {
    const range = serverID ? state.detailRange : state.range;
    return downloadCSV('/export?range=' + range + (serverID ? '&server_id=' + serverID : ''), 'dns-monitor-results-' + (serverID || 'all') + '-' + range + '-' + new Date().toISOString().slice(0, 10) + '.csv', serverID ? $('#detail-export') : $('#export-all-results-button'), t('csv.rawExported') ?? '原始记录 CSV 已导出');
  }

  async function downloadCSV(path, filename, button, successMessage = '') {
    if (button) button.disabled = true;
    try {
      const file = typeof window.showSaveFilePicker === 'function' ? await window.showSaveFilePicker({ suggestedName: filename, types: [{ description: t('csv.fileDesc') ?? 'CSV 文件', accept: { 'text/csv': ['.csv'] } }] }) : null;
      const response = await fetch('/api' + path, { headers: { 'X-DNSMonitor-Token': state.token }, cache: 'no-store' });
      if (!response.ok) {
        if (response.status === 401) resetSession(t('csv.cancelled') ?? '会话已失效，请重新登录。');
        let body;
        try { body = await response.json(); } catch { body = null; }
        throw new Error(body?.error || (t('csv.exportFailed', { status: response.status }) ?? '导出失败（HTTP ' + response.status + '）'));
      }
      if (file && response.body) {
        const output = await file.createWritable();
        await response.body.pipeTo(output);
      } else {
        const blob = await response.blob();
        const url = URL.createObjectURL(blob);
        const link = document.createElement('a');
        link.href = url;
        link.download = filename;
        document.body.append(link);
        link.click();
        link.remove();
        setTimeout(() => URL.revokeObjectURL(url), 30000);
      }
      toast(successMessage || (t('csv.exported') ?? 'CSV 已导出'));
    } catch (error) { if (error.name !== 'AbortError') toast(error.message, true); }
    finally { if (button) button.disabled = false; }
  }

  function importConflict(row) {
    return !!row.duplicate || !!row.existing || Number(row.duplicate_of_row || 0) > 0;
  }

  function openImport() {
    if (state.importing?.busy) return toast(t('import.busyToast') ?? '导入操作正在执行，请稍候', true);
    state.importing = { csv: '', filename: '', snapshot: '', rows: [], errors: 0, decisions: new Map(), busy: false };
    $('#import-file').value = '';
    $('#import-preview').hidden = true;
    $('#import-preview-button').disabled = true;
    $('#import-commit-button').disabled = true;
    $('#import-status').textContent = t('import.awaitPreview') ?? '选择文件后将自动预览';
    setError('#import-error', '');
    $('#import-dialog').showModal();
  }

  function importBusy(busy, message = '') {
    if (!state.importing) return;
    state.importing.busy = busy;
    $('#import-file').disabled = busy;
    $('#import-preview-button').disabled = busy || !state.importing.csv;
    $('#import-commit-button').disabled = busy || !state.importing.snapshot || state.importing.errors > 0 || !state.importing.rows.length;
    $$('#import-rows select').forEach(select => { select.disabled = busy; });
    $('#import-overwrite-all').disabled = busy;
    $('#import-skip-all').disabled = busy;
    syncImportBulk();
    if (message) $('#import-status').textContent = message;
  }

  async function readImportFile() {
    const file = $('#import-file').files?.[0];
    const session = state.importing;
    if (!file || !session || session.busy) return;
    session.csv = '';
    session.snapshot = '';
    session.rows = [];
    session.filename = file.name;
    $('#import-preview').hidden = true;
    setError('#import-error', '');
    importBusy(true, t('import.reading') ?? '正在读取 CSV 文件…');
    try {
      if (file.size > 900 * 1024) throw new Error(t('import.tooLarge') ?? 'CSV 文件超过 900 KiB，请拆分后分别导入。');
      const bytes = await file.arrayBuffer();
      if (state.importing !== session) return;
      let csv;
      try { csv = new TextDecoder('utf-8', { fatal: true }).decode(bytes).replace(/^\uFEFF/, ''); }
      catch { throw new Error(t('import.badUtf8') ?? '文件不是有效的 UTF-8 编码，请以 UTF-8 CSV 格式重新保存。'); }
      if (!csv.trim()) throw new Error(t('import.empty') ?? 'CSV 文件为空，请先填写服务器记录。');
      if (new TextEncoder().encode(JSON.stringify({ csv })).length >= 1024 * 1024) throw new Error(t('import.requestTooLarge') ?? 'CSV 的请求内容超过 1 MiB，请拆分后再导入。');
      session.csv = csv;
    } catch (error) {
      setError('#import-error', error.message);
      $('#import-status').textContent = t('import.notChecked') ?? '文件尚未通过检查';
    } finally { if (state.importing === session) importBusy(false); }
    if (session.csv && state.importing === session) await previewImport();
  }

  async function previewImport() {
    const session = state.importing;
    if (!session?.csv || session.busy) return;
    session.snapshot = '';
    setError('#import-error', '');
    importBusy(true, t('import.validating') ?? '正在校验服务器与重复项…');
    try {
      const preview = await api('/servers/import/preview', { method: 'POST', body: { csv: session.csv } });
      if (state.importing !== session) return;
      session.snapshot = preview.snapshot || '';
      session.rows = preview.rows || [];
      session.errors = Math.max(Number(preview.errors || 0), session.rows.filter(row => row.error).length);
      session.decisions = new Map(session.rows.filter(importConflict).map(row => [Number(row.row), 'skip']));
      if (session.rows.length > 1000) throw new Error(t('import.tooMany') ?? 'CSV 超过 1000 条记录，请拆分后导入。');
      renderImport();
      $('#import-status').textContent = session.errors ? t('import.fixRows') ?? '修正 CSV 中的无效行后，请重新选择文件' : t('import.previewDone') ?? '预览完成，请确认重复项的处理方式';
    } catch (error) {
      session.snapshot = '';
      setError('#import-error', error.message);
      $('#import-status').textContent = t('import.previewFailed') ?? '预览失败，可修正文件后重试';
    } finally { if (state.importing === session) importBusy(false); }
  }

  function renderImport() {
    const session = state.importing;
    if (!session) return;
    const conflicts = session.rows.filter(importConflict);
    const fresh = session.rows.filter(row => !row.error && !importConflict(row)).length;
    $('#import-preview').hidden = false;
    $('#import-summary').textContent = t('import.summary', { filename: session.filename, rows: session.rows.length, fresh, conflicts: conflicts.length, errors: session.errors }) ?? session.filename + ' · ' + session.rows.length + ' 行 · ' + fresh + ' 条新记录 · ' + conflicts.length + ' 条重复 · ' + session.errors + ' 条无效';
    $('#import-rows').innerHTML = session.rows.map(row => {
      const server = row.server || {};
      const conflict = importConflict(row);
      const existing = row.existing;
      const detail = row.duplicate_of_row ? t('import.dupRow', { row: row.duplicate_of_row }) ?? '与 CSV 数据行 ' + row.duplicate_of_row + ' 重复' : existing ? t('import.exists', { name: existing.name || existing.address || '' }) ?? '已存在：' + (existing.name || existing.address || '') : t('import.dupServer') ?? '与已有服务器重复';
      const status = row.error ? '<span class="import-row-error">' + escape(row.error) + '</span>' : conflict ? '<span class="import-duplicate">' + escape(detail) + '</span>' : '<span class="import-new">' + (t('import.canAdd') ?? '可新增') + '</span>';
      const action = row.error ? '<span class="muted">' + (t('import.fix') ?? '请修正') + '</span>' : conflict ? '<select data-import-row="' + Number(row.row) + '" aria-label="' + (t('import.rowAria', { row: Number(row.row) }) ?? '数据行 ' + Number(row.row) + ' 的重复项处理') + '"><option value="skip"' + (session.decisions.get(Number(row.row)) === 'skip' ? ' selected' : '') + '>' + (t('import.skipRow') ?? '跳过这行') + '</option><option value="overwrite"' + (session.decisions.get(Number(row.row)) === 'overwrite' ? ' selected' : '') + '>' + (t('import.overwriteConfig') ?? '覆盖配置') + '</option></select>' : '<span class="import-new">' + (t('import.create') ?? '创建服务器') + '</span>';
      return '<tr><td>' + Number(row.row) + '</td><td><strong>' + escape(server.name || '—') + '</strong><span class="server-secondary">' + escape(server.provider || (t('import.noProvider') ?? '未填写供应商')) + '</span><span class="import-flags"><span class="tag">' + (server.enabled ? t('import.enabled') ?? '已启用' : t('import.disabled') ?? '已停用') + '</span><span class="tag">' + (server.trusted ? t('import.trustedTag') ?? '用户可信' : t('import.normalTag') ?? '普通 DNS') + '</span></span></td><td>' + escape(server.address || '—') + '</td><td>' + status + '</td><td>' + action + '</td></tr>';
    }).join('');
    syncImportBulk();
    if (session.errors) setError('#import-error', t('import.invalidRows', { count: session.errors }) ?? '存在 ' + session.errors + ' 条无效记录，当前不能导入。请修正原 CSV 文件并重新选择。');
  }

  function syncImportBulk() {
    const actions = [...(state.importing?.decisions.values() || [])];
    $('#import-overwrite-all').checked = actions.length > 0 && actions.every(action => action === 'overwrite');
    $('#import-skip-all').checked = actions.length > 0 && actions.every(action => action === 'skip');
    $('#import-overwrite-all').disabled = !actions.length || !!state.importing?.busy;
    $('#import-skip-all').disabled = !actions.length || !!state.importing?.busy;
  }

  function bulkImport(action) {
    const session = state.importing;
    if (!session || session.busy) return;
    session.decisions.forEach((_, row) => session.decisions.set(row, action));
    $$('#import-rows select').forEach(select => { select.value = action; });
    syncImportBulk();
  }

  async function commitImport() {
    const session = state.importing;
    if (!session?.snapshot || session.busy || session.errors || !session.rows.length) return;
    const body = { csv: session.csv, snapshot: session.snapshot, decisions: [...session.decisions].map(([row, action]) => ({ row, action })) };
    if (new TextEncoder().encode(JSON.stringify(body)).length >= 1024 * 1024) return setError('#import-error', t('import.requestWithDecisions') ?? '包含处理选项后的请求超过 1 MiB，请拆分 CSV 后导入。');
    setError('#import-error', '');
    importBusy(true, t('import.writing') ?? '正在写入服务器清单…');
    try {
      const result = await api('/servers/import', { method: 'POST', body });
      $('#import-dialog').close();
      toast(t('import.doneToast', { created: integer(result.created || 0), updated: integer(result.updated || 0), skipped: integer(result.skipped || 0) }) ?? '导入完成：新增 ' + integer(result.created || 0) + '，覆盖 ' + integer(result.updated || 0) + '，跳过 ' + integer(result.skipped || 0));
      await refreshState();
    } catch (error) {
      if (error.status === 409) {
        session.snapshot = '';
        setError('#import-error', t('import.staleError') ?? '服务器清单在预览后发生变化，请点击“重新预览”，重新核对重复项后再导入。');
        $('#import-status').textContent = t('import.stalePreview') ?? '预览已过期，需要重新预览';
      } else {
        setError('#import-error', error.message);
        $('#import-status').textContent = t('import.failed') ?? '导入失败，请检查提示后重试';
      }
    } finally { if (state.importing === session) importBusy(false); }
  }

  function confirmAction(title, description, confirmText = '') {
    return new Promise(resolve => {
      const dialog = $('#confirm-dialog');
      $('#confirm-title').textContent = title;
      $('#confirm-description').textContent = description;
      $('#confirm-ok').textContent = confirmText || (t('confirm.default') ?? '确认');
      dialog.returnValue = '';
      const onClose = () => { dialog.removeEventListener('close', onClose); resolve(dialog.returnValue === 'confirm'); };
      dialog.addEventListener('close', onClose);
      dialog.showModal();
      $('#confirm-cancel').focus();
    });
  }

  $('#login-form').addEventListener('submit', login);
  $('#logout-button').addEventListener('click', logout);
  $('#refresh-button').addEventListener('click', manualRefresh);
  $$('.nav-item').forEach(button => button.addEventListener('click', () => navigate(button.dataset.page)));
  $('.sidebar>.brand').addEventListener('click', event => { event.preventDefault(); navigate('overview'); });
  $$('[data-open-guide]').forEach(button => button.addEventListener('click', () => navigate('guide')));
  $$('[data-range]').forEach(button => button.addEventListener('click', () => changeRange(button.dataset.range)));
  $$('.sort-button').forEach(button => button.addEventListener('click', () => { state.descending = state.sort === button.dataset.sort ? !state.descending : !['average_ms', 'grade'].includes(button.dataset.sort); state.sort = button.dataset.sort; renderServerRows(); }));
  $('#search-filter').addEventListener('input', renderServerRows);
  $$('[data-trigger]').forEach(button => button.addEventListener('click', event => { event.stopPropagation(); toggleFilterPopover(button.dataset.trigger); }));
  $$('.multi-select [data-popover]').forEach(popover => popover.addEventListener('change', event => {
    const checkbox = event.target.closest('input[type=checkbox]');
    if (!checkbox) return;
    const key = popover.dataset.popover;
    if (checkbox.dataset.all !== undefined) state.filters[key] = [];
    else {
      const current = state.filters[key];
      const index = current.indexOf(checkbox.value);
      if (checkbox.checked && index < 0) current.push(checkbox.value);
      if (!checkbox.checked && index >= 0) current.splice(index, 1);
    }
    persistFilters();
    renderFilterOptions(key);
    renderServerRows();
  }));
  document.addEventListener('click', event => { if (!event.target.closest('.multi-select')) closeFilterPopovers(); });
  document.addEventListener('keydown', event => { if (event.key === 'Escape') closeFilterPopovers(); });
  $('#add-server-button').addEventListener('click', () => openServer());
  $('#empty-add-button').addEventListener('click', () => openServer());
  $('#server-form').addEventListener('submit', saveServer);
  $('#server-rows').addEventListener('click', event => {
    const button = event.target.closest('[data-action]');
    if (!button) return;
    const id = Number(button.dataset.id);
    if (button.dataset.action === 'detail') openDetail(id);
    if (button.dataset.action === 'edit') openServer(state.data.servers.find(server => server.id === id));
    if (button.dataset.action === 'delete') deleteServer(id);
  });
  $$('[data-close]').forEach(button => button.addEventListener('click', () => $(`#${button.dataset.close}`).close()));
  $('#confirm-cancel').addEventListener('click', () => $('#confirm-dialog').close('cancel'));
  $('#confirm-ok').addEventListener('click', () => $('#confirm-dialog').close('confirm'));
  $('#config-form').addEventListener('input', event => {
    markConfigDirty();
    if (event.target.matches('input[type=number]')) validateConfigNumber(event.target);
  });
  $('#config-form').addEventListener('change', event => {
    markConfigDirty();
    if (event.target.matches('input[type=number]')) validateConfigNumber(event.target, true);
  });
  $('#config-form').addEventListener('focusout', event => {
    if (event.target.matches('input[type=number]')) validateConfigNumber(event.target, true);
  });
  $('#config-form').addEventListener('invalid', event => {
    if (event.target.matches('input[type=number]')) validateConfigNumber(event.target, true);
  }, true);
  $('#config-form').addEventListener('submit', saveConfig);
  $('#add-domain-button').addEventListener('click', () => { const row = domainRow(); $('#domain-rows').append(row); $('.domain-name', row).focus(); markConfigDirty(); });
  $('#domain-rows').addEventListener('click', event => {
    const button = event.target.closest('.remove-domain');
    if (!button) return;
    if ($$('.domain-row').length <= 1) return toast(t('config.minOneDomain') ?? '至少需要保留一个探测域名', true);
    button.closest('.domain-row').remove();
    markConfigDirty();
  });
  ['mouseover', 'focusin', 'click'].forEach(eventName => $('#status-history').addEventListener(eventName, event => {
    const block = event.target.closest('[data-status-index]');
    if (!block) return;
    const point = state.statusHistory[Number(block.dataset.statusIndex)];
    if (point) $('#status-history-inspection').textContent = statusDescription(point, block.dataset.statusChannel);
  }));
  $$('[data-detail-range]').forEach(button => button.addEventListener('click', () => { if (button.dataset.detailRange === state.detailRange) return; state.detailRange = button.dataset.detailRange; loadHistory(); }));
  $('#detail-probe').addEventListener('click', probeNow);
  $('#detail-edit').addEventListener('click', () => openServer(currentServer()));
  $('#results-refresh').addEventListener('click', () => { setError('#detail-error', ''); Promise.allSettled([loadHistory(), loadResults(true)]); });
  $('#results-more').addEventListener('click', () => loadResults());
  $('#result-rows').addEventListener('click', event => { const button = event.target.closest('[data-result-index]'); if (button) showVerdict(Number(button.dataset.resultIndex)); });
  $$('[data-verdict]').forEach(button => button.addEventListener('click', () => saveVerdict(button.dataset.verdict)));
  $('#service-buttons').addEventListener('click', event => { const button = event.target.closest('[data-service-action]'); if (button && !button.disabled) serviceAction(button.dataset.serviceAction); });
  $('#import-servers-button').addEventListener('click', openImport);
  $('#import-file').addEventListener('change', readImportFile);
  $('#import-preview-button').addEventListener('click', previewImport);
  $('#import-commit-button').addEventListener('click', commitImport);
  $('#import-overwrite-all').addEventListener('change', () => bulkImport('overwrite'));
  $('#import-skip-all').addEventListener('change', () => bulkImport('skip'));
  $('#import-rows').addEventListener('change', event => { const select = event.target.closest('[data-import-row]'); if (!select || !state.importing || state.importing.busy) return; state.importing.decisions.set(Number(select.dataset.importRow), select.value); syncImportBulk(); });
  $('#export-servers-button').addEventListener('click', () => downloadCSV('/servers/export', 'dns-monitor-servers.csv', $('#export-servers-button'), t('csv.serversExported') ?? 'DNS 服务器清单已导出'));
  ['server-template-button', 'import-template-button'].forEach(id => $('#' + id).addEventListener('click', () => downloadCSV('/servers/template', 'dns-monitor-servers-template.csv', $('#' + id), t('csv.templateDownloaded') ?? '服务器 CSV 模板已下载，请替换其中的示例行')));
  $('#export-button').addEventListener('click', exportSummary);
  $('#export-all-results-button').addEventListener('click', () => exportCSV());
  $('#detail-export').addEventListener('click', () => exportCSV(state.detailID));
  ['rate-chart', 'latency-chart'].forEach(id => { const canvas = $(`#${id}`); canvas.addEventListener('pointermove', chartHover); canvas.addEventListener('pointerleave', () => { $('.chart-tooltip', canvas.parentElement).hidden = true; }); });
  let resizeTimer;
  window.addEventListener('resize', () => { clearTimeout(resizeTimer); resizeTimer = setTimeout(renderCharts, 100); });
  document.addEventListener('visibilitychange', () => { if (!document.hidden && state.token) refreshState(); });
  window.addEventListener('beforeunload', event => { if (state.configDirty) { event.preventDefault(); event.returnValue = ''; } });
  $$('#lang-toggle, #login-lang-toggle').forEach(button => button.addEventListener('click', () => setLanguage(currentLang === 'en' ? 'zh' : 'en')));
  if (currentLang === 'en') applyStaticI18n();
  document.documentElement.lang = currentLang === 'en' ? 'en' : 'zh-CN';
  document.title = currentLang === 'en' ? (I18N.en.docTitle || zhTitle) : zhTitle;
  renderLanguageToggle();
  if (storedSession?.token) {
    state.token = storedSession.token;
    persistSession();
    $('#login-screen').hidden = true;
    $('#app').hidden = false;
    rerenderLanguageUI();
    if (state.page !== 'overview') navigate(state.page, true);
    refreshState(true);
    scheduleRefresh();
  }
})();
