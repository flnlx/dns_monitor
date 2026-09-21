// Optional UI regression: node internal/web/history_browser_test.cjs
// Uses local Playwright/Chrome and deterministic API fixtures; no live DNS is queried.
const fs = require('fs');
const path = require('path');
const assert = require('node:assert/strict');
const {chromium} = require(path.join(__dirname, '../../.tools/browser/node_modules/playwright-core'));
const root = path.resolve(__dirname, '../..');
const assets = path.join(root, 'internal/web/assets');
fs.mkdirSync(path.join(root, '.test-output'), {recursive:true});
const now = Date.now();
const spans = {'24h':86400000,'7d':7*86400000,'30d':30*86400000};
const counts = {'24h':48,'7d':56,'30d':60};
const metrics = {samples:12,coverage:94,availability:99,success_rate:97,average_ms:18,p95_ms:30,grade:'A',pollution:'matched',score:96};
const current = {...metrics,window_minutes:60,min_samples:3,min_coverage_minutes:5,covered_minutes:58,quality_at:now};
function buckets(range, empty=false) {
 const n=counts[range], step=spans[range]/n;
 return Array.from({length:n},(_,i)=>{
  const quality=empty||i<3?'no_data':i===15?'polluted':i===16?'unknown':i===17?'suspicious':i%8===0?'clean':'matched';
  const grade=quality==='no_data'?'no_data':i===15?'F':i===16?'unavailable':i===17?'E':i===18?'pending':i===19?'D':i===20?'C':i===21?'B':'A';
  const p={timestamp:now-spans[range]+i*step,end:now-spans[range]+(i+1)*step,pollution:quality,grade,snapshots:quality==='no_data'?0:6};
  if(p.snapshots)Object.assign(p,{quality_counts:{[quality]:1,matched:5},grade_counts:{[grade]:1,A:5},latest:{timestamp:p.end-1000,trusted:false,current}});
  return p;
 });
}
let failHistory=false, slowSeven=false;
(async()=>{
 const browser=await chromium.launch({executablePath:'C:/Program Files/Google/Chrome/Application/chrome.exe',headless:true});
const page=await browser.newPage({viewport:{width:1600,height:1100}});
  await page.addInitScript(() => { try { localStorage.setItem('dns-monitor.lang.v1', 'zh'); } catch {} });
  const errors=[]; page.on('pageerror',e=>errors.push(e.message));
 await page.route('http://dns-history.test/**',async route=>{
  const url=new URL(route.request().url()), range=url.searchParams.get('range')||'24h';
  if(url.pathname.startsWith('/api')){
   let body={};
   if(url.pathname==='/api/session')body={token:'fixture-token'};
   else if(url.pathname==='/api/state')body={now,version:'1.3.0',servers:[1,2].map(id=>({id,name:id===1?'Cloudflare · 历史演示':'新服务器 · 尚无历史',provider:'DNS Monitor',address:'1.1.1.'+id,protocol:'UDP',trusted:false,enabled:true,last_probe:now,last_success:true,current,metrics,status_history:buckets(range,id===2).map(({latest,...rest})=>rest)})),config:{interval_seconds:300,concurrency:0,timeout_seconds:3,max_backoff_hours:1,reference_ttl_seconds:300,reference_history_hours:1,rating_window_minutes:60,rating_min_samples:3,rating_min_coverage_minutes:5,domains:[{name:'example.com',type:'A'}]},runtime:{paused:true},service:{installed:false}};
   else if(url.pathname.endsWith('/history')){
    if(slowSeven && range==='7d')await new Promise(r=>setTimeout(r,350));
    if(failHistory)return route.fulfill({status:500,contentType:'application/json',body:JSON.stringify({error:'fixture history failure'})});
    body={metrics,current,points:[{timestamp:now-1000,...metrics}],status_history:buckets(range,url.pathname.includes('/2/'))};
   }else if(url.pathname.endsWith('/results'))body={results:[]};
   return route.fulfill({contentType:'application/json',body:JSON.stringify(body)});
  }
  const file=url.pathname==='/'?'index.html':url.pathname.slice(1);
  if(!['index.html','style.css','app.js'].includes(file))return route.fulfill({status:404,body:''});
  await route.fulfill({contentType:file.endsWith('.js')?'application/javascript':file.endsWith('.css')?'text/css':'text/html',body:fs.readFileSync(path.join(assets,file))});
 });
 try {
  await page.goto('http://dns-history.test/');
  await page.locator('#access-key').fill('fixture');await page.locator('.login-submit').click();
  await page.waitForSelector('.history-open');
  assert.equal(await page.locator('.history-open').count(),4);
  assert.equal(await page.locator('tr[data-server-id="1"] .history-open .status-block').count(),96);
  await page.screenshot({path:path.join(root,'.test-output/status-history-overview.png'),fullPage:true});
  await page.locator('tr[data-server-id="1"] .history-open').last().click();
  await page.waitForSelector('#status-history .status-block');
  assert.equal(await page.locator('#status-history .status-block').count(),96);
  await page.locator('[data-status-channel="grade"][data-status-index="16"]').hover();
  assert.match(await page.locator('#status-history-inspection').innerText(),/不可用/);
  await page.locator('[data-status-channel="quality"][data-status-index="15"]').focus();
  assert.match(await page.locator('#status-history-inspection').innerText(),/疑似污染/);
  await page.locator('[data-status-channel="quality"][data-status-index="0"]').click();
  assert.match(await page.locator('#status-history-inspection').innerText(),/无数据/);
  await page.locator('[data-status-channel="grade"][data-status-index="16"]').click();
  await page.screenshot({path:path.join(root,'.test-output/status-history-detail.png'),fullPage:true});
  slowSeven=true;
  await page.locator('[data-detail-range="7d"]').click();
  await page.locator('[data-detail-range="30d"]').click();
  await page.waitForFunction(()=>document.querySelectorAll('#status-history .status-block').length===120);
  await page.waitForTimeout(450);
  assert.equal(await page.locator('#status-history .status-block').count(),120);
  assert.match(await page.locator('#status-history').innerText(),/12 小时/);
  await page.setViewportSize({width:390,height:844});
  await page.locator('#status-history').scrollIntoViewIfNeeded();
  const overflow=await page.locator('#status-history').evaluate(e=>e.scrollWidth>e.clientWidth);
  assert.equal(overflow,false,'history overflows mobile panel');
  await page.screenshot({path:path.join(root,'.test-output/status-history-mobile.png'),fullPage:true});
  failHistory=true;
  await page.locator('[data-detail-range="24h"]').click();
  await page.waitForFunction(()=>document.querySelector('#status-history').textContent.includes('读取失败'));
  assert.equal(await page.locator('#status-history .status-block').count(),0);
  failHistory=false;
  await page.locator('#results-refresh').click();
  await page.waitForSelector('#status-history .status-block');
  await page.locator('[data-close="detail-dialog"]').click();
  await page.setViewportSize({width:1600,height:1100});
  for(const range of ['7d','30d','24h']){
   await page.locator(`[data-range="${range}"]`).click();
   await page.waitForFunction(n=>document.querySelectorAll('tr[data-server-id="1"] .history-open .status-block').length===n,counts[range]*2);
  }
  await page.locator('tr[data-server-id="2"] .history-open').first().click();
  await page.waitForSelector('#status-history .status-block');
  assert.equal(await page.locator('#status-history .status-no_data.status-block').count(),96);
  assert.deepEqual(errors,[]);
  console.log('PASS: overview/detail strips, hover/focus/touch, 24h/7d/30d, stale response, error/retry, empty history, mobile overflow; no JS errors.');
 } finally { await browser.close(); }
})().catch(e=>{console.error(e);process.exitCode=1;});
