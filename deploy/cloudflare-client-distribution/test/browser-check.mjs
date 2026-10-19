// Optional, dependency-free, real Chromium smoke test. Uses only synthetic data
// and a loopback server; external artifact requests are fulfilled by CDP.
import assert from 'node:assert/strict';
import {createServer} from 'node:http';
import {spawn} from 'node:child_process';
import {readFile,writeFile,mkdir,rm} from 'node:fs/promises';
import path from 'node:path';
import {fileURLToPath} from 'node:url';
import {fixture} from './fixtures.mjs';
import {prepareRelease} from '../tools/release.mjs';
import worker from '../src/worker.mjs';
const f = await fixture(), {current} = await prepareRelease(f.configFile,f.stage);
const site = fileURLToPath(new URL('../site/',import.meta.url));
const errors = [],requests = []; let failRelease = false,corruptCatalog = false,browser,server,ws;
const delay = ms=>new Promise(resolve=>setTimeout(resolve,ms));
const env = {
  RELEASES:{async get(){if(failRelease)return null;const text=JSON.stringify(current);return {size:text.length,text:async()=>text};}},
  ASSETS:{async fetch(request){
    let pathname = decodeURIComponent(new URL(request.url).pathname); if(pathname.endsWith('/'))pathname+='index.html';
    const file=path.resolve(site,'.'+pathname);if(!file.startsWith(site))return new Response(null,{status:404});
    try {return new Response(await readFile(file),{headers:{'content-type':file.endsWith('.html')?'text/html; charset=utf-8':/\.(mjs|js)$/.test(file)?'text/javascript; charset=utf-8':file.endsWith('.css')?'text/css':'application/octet-stream'}});}catch{return new Response(null,{status:404});}
  }},
};
try {
  server=createServer(async(req,res)=>{
    try {const r=await worker.fetch(new Request(`http://127.0.0.1${req.url}`,{method:req.method}),env);res.writeHead(r.status,Object.fromEntries(r.headers));res.end(Buffer.from(await r.arrayBuffer()));}catch(e){res.writeHead(500);res.end(String(e));}
  });
  await new Promise(resolve=>server.listen(0,'127.0.0.1',resolve)); const origin=`http://127.0.0.1:${server.address().port}`;
  const userData=path.join(f.root,'browser');await mkdir(userData);
  browser=spawn(process.env.CHROMIUM || 'chromium',['--headless','--disable-background-networking','--disable-component-update','--disable-sync','--no-first-run','--disable-default-apps','--no-pings','--metrics-recording-only','--no-sandbox','--disable-gpu','--disable-dev-shm-usage',`--user-data-dir=${userData}`,'--remote-debugging-port=0','about:blank'],{stdio:['ignore','ignore','pipe']});
  let stderr='';browser.stderr.on('data',chunk=>stderr+=chunk);
  let devtools;for(let i=0;i<150;i++){try{devtools=(await readFile(path.join(userData,'DevToolsActivePort'),'utf8')).split('\n');break;}catch{await delay(100);}}
  assert.ok(devtools,`Chromium failed: ${stderr.slice(-1000)}`);
  const targets=await (await fetch(`http://127.0.0.1:${devtools[0]}/json/list`)).json();
  ws=new WebSocket(targets.find(t=>t.type==='page').webSocketDebuggerUrl);await new Promise((resolve,reject)=>{ws.onopen=resolve;ws.onerror=reject;});
  const pending=new Map(),listeners=new Map();let id=0;
  const send=(method,params={})=>new Promise((resolve,reject)=>{const call=++id;pending.set(call,{resolve,reject});ws.send(JSON.stringify({id:call,method,params}));});
  ws.onmessage=async event=>{
    const message=JSON.parse(event.data);
    if(message.id){const p=pending.get(message.id);pending.delete(message.id);if(message.error)p?.reject(new Error(JSON.stringify(message.error)));else p?.resolve(message.result);}
    else if(message.method==='Runtime.exceptionThrown')errors.push(message.params.exceptionDetails.text+': '+(message.params.exceptionDetails.exception?.description||''));
    else if(message.method==='Network.requestWillBeSent')requests.push(message.params.request.url);
    else if(message.method==='Fetch.requestPaused'){
      const {requestId,request}=message.params;
      if(request.url.startsWith(origin+'/')) { await send('Fetch.continueRequest',{requestId}); return; }
      const bytes=f.blobs.get(request.url);
      const body=corruptCatalog && request.url.includes('catalog-')?Buffer.from('corrupt'):bytes;
      await send('Fetch.fulfillRequest',{requestId,responseCode:bytes?200:404,responseHeaders:[{name:'Content-Type',value:'application/json'},{name:'Access-Control-Allow-Origin',value:'*'},{name:'Cache-Control',value:'no-store'}],body:(body||Buffer.from('not found')).toString('base64')});
    }
  };
  await send('Runtime.enable');await send('Page.enable');await send('Network.enable');
  await send('Fetch.enable',{patterns:[{urlPattern:'*'}]});
  const evaluate=async(expression)=>{const r=await send('Runtime.evaluate',{expression,awaitPromise:true,returnByValue:true,userGesture:true});if(r.exceptionDetails)throw new Error(r.exceptionDetails.exception?.description||r.exceptionDetails.text);return r.result.value;};
  const until=async(expression)=>{for(let i=0;i<100;i++){if(await evaluate(expression))return;await delay(100);}throw new Error('Timed out: '+expression+'\n'+await evaluate('document.body.innerText'));};
  const navigate=async(url)=>{await send('Page.navigate',{url});await delay(150);};
  await navigate(origin+'/download/');await until('document.querySelectorAll(".download-link").length===3');
  assert.equal(await evaluate('document.getElementById("dispatch").textContent'),current.defaultDispatchUrl);
  assert.equal(await evaluate('document.querySelector(".download-link").href'),current.dispatch.update.clients.android.artifactUrl);
  await evaluate('document.getElementById("retry").click();document.getElementById("retry").click();');await until('document.querySelectorAll(".download-link").length===3');
  failRelease=true;await evaluate('document.getElementById("retry").click()');await until('document.getElementById("status").textContent.includes("读取失败")');assert.equal(await evaluate('document.querySelectorAll(".download-link").length'),0);failRelease=false;
  await navigate(origin+'/fantasy-db/');await until('document.getElementById("result-count").textContent==="2"');
  assert.equal(await evaluate('document.querySelectorAll("#account-panel").length'),0);
  assert.ok(await evaluate('document.getElementById("item-grid").innerText.includes("物攻 0–10")'));
  assert.ok(!(await evaluate('document.getElementById("item-grid").innerText.includes("undefined")')));
  await evaluate('document.getElementById("search").value="测试剑";document.getElementById("search").dispatchEvent(new Event("input",{bubbles:true}));');await until('document.getElementById("result-count").textContent==="1"');
  for(let i=0;i<2;i++) {await evaluate('document.querySelector("[data-detail]").click()');await until('document.getElementById("detail-dialog").open');await evaluate('document.querySelector("#detail-dialog [data-close]").click()');await until('!document.getElementById("detail-dialog").open');}
  await evaluate('document.querySelector("[data-section=gm]").click()');await until('!document.getElementById("gm-tools").hidden');
  assert.ok((await evaluate('document.getElementById("command-preview").textContent')).startsWith('/'));
  await evaluate('document.querySelector("[data-section=items]").click()');await until('!document.getElementById("catalog").hidden');
  await evaluate('history.back()');await until('!document.getElementById("gm-tools").hidden');
  await evaluate('history.forward()');await until('!document.getElementById("catalog").hidden');
  // A failed initial load must recover when Retry is used, without stale cards.
  corruptCatalog=true;await navigate(origin+'/fantasy-db/');await until('!document.getElementById("error-state").hidden');
  assert.equal(await evaluate('document.querySelectorAll(".item-card").length'),0);corruptCatalog=false;
  await evaluate('document.getElementById("retry").click()');await until('document.getElementById("result-count").textContent==="2"');
  const screenshot=process.env.BROWSER_SCREENSHOT;
  if(screenshot){const shot=await send('Page.captureScreenshot',{format:'png',captureBeyondViewport:false});await writeFile(screenshot,Buffer.from(shot.data,'base64'));}
  assert.deepEqual(errors,[]);assert.ok(!requests.some(url=>url.includes('/api/')),'public UI must never send API/GM requests');
  console.log('Chromium PASS: download/dispatch, repeated retry, unavailable release, catalog search/details, GM text, Back/Forward, corrupt-snapshot recovery; no API requests or runtime errors.');
} finally {
  ws?.close();browser?.kill('SIGTERM');await new Promise(resolve=>server?.close(resolve));await delay(200);
  await rm(f.root,{recursive:true,force:true,maxRetries:5,retryDelay:100});
}
