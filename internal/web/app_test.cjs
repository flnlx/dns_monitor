const { test } = require('node:test');
const assert = require('node:assert/strict');
const fs = require('node:fs');
const vm = require('node:vm');
const path = require('node:path');
const source = fs.readFileSync(path.join(__dirname, 'assets/app.js'), 'utf8');
// Exercise the actual view helpers without starting network requests or timers.
function view() {
  const filters = {};
  const context = vm.createContext({ document: { querySelector: selector => ({ value: filters[selector] || '' }) } });
  const helpers = source.slice(source.indexOf("  const $ ="), source.indexOf('  function setError'));
  const sorting = source.slice(source.indexOf('  function visibleServers()'), source.indexOf('  function renderServerRows()'));
  return { filters, ...vm.runInContext(helpers + sorting + '\n({state, visibleServers, gradeHTML, displayedGrade, statusStrip, statusDescription, compactHistory})', context) };
}
test('unavailable is last for every sort key and direction, including trusted servers', () => {
  const ui = view();
  ui.state.data = { servers: ['unavailable', 'F', 'A', 'pending', 'D'].map((grade, id) => ({
    id, trusted: grade === 'unavailable', current: { grade },
    metrics: { samples: 10, coverage: 100, average_ms: 1, success_rate: 100, availability: 100 }
  })) };
  for (const sort of ['grade', 'average_ms', 'success_rate', 'availability']) {
    for (const descending of [false, true]) {
      Object.assign(ui.state, { sort, descending });
      assert.equal(ui.visibleServers().at(-1).current.grade, 'unavailable');
    }
  }
  Object.assign(ui.state, { sort: 'grade', descending: false });
  assert.equal(ui.visibleServers().map(s => s.current.grade).join(','), 'A,D,F,pending,unavailable');
  ui.filters['#grade-filter'] = 'unavailable';
  assert.equal(ui.visibleServers().length, 1);
  assert.equal(ui.visibleServers()[0].id, 0);
  assert.equal(ui.displayedGrade({grade: 'unavailable'}, true), 'unavailable');
  assert.match(ui.gradeHTML('unavailable', {}), /grade-unavailable/);
  assert.match(ui.gradeHTML('unavailable', {}), /不可用/);
});

test('history keeps past F for currently trusted servers and distinguishes gaps from pending', () => {
  const ui = view();
  const empty = {timestamp:1,end:1000,snapshots:0,grade:'no_data',pollution:'no_data'};
  const past = {timestamp:1000,end:2000,snapshots:2,grade:'F',pollution:'polluted',grade_counts:{F:1,pending:1},quality_counts:{polluted:1,unknown:1},latest:{timestamp:1500,trusted:false,current:{grade:'pending',pollution:'unknown',window_minutes:60,samples:1,success_rate:100,average_ms:10}}};
  const server = {id:1,name:'<unsafe>',trusted:true,status_history:[empty,past]};
  const strip = ui.compactHistory(server,'grade');
  assert.match(strip,/status-f/);
  assert.match(strip,/status-no_data/);
  assert.match(strip,/&lt;unsafe&gt;/);
  const description = ui.statusDescription(past,'grade');
  assert.match(description,/最差记录 F/);
  assert.match(description,/F 1 次、待评估 1 次/);
  assert.match(description,/末次快照/);
  assert.match(ui.statusDescription(empty,'quality'),/无数据/);
  assert.match(ui.statusStrip([past],'quality',true),/aria-label=/);
  assert.match(ui.statusStrip([past],'grade',true),/data-status-channel="grade"/);
});
