import { test } from 'node:test';
import assert from 'node:assert/strict';
import fs from 'node:fs';
import os from 'node:os';
import path from 'node:path';
import { fileURLToPath, pathToFileURL } from 'node:url';
import ts from 'typescript';
const testDir = fileURLToPath(new URL('.', import.meta.url));

test('focused mock persists task progress and schedule across reload without deadlines', async (t) => {
 const tmp = fs.mkdtempSync(path.join(os.tmpdir(),'worklingo-focus-'));
 t.after(()=>fs.rmSync(tmp,{recursive:true,force:true}));
 let snapshot=JSON.stringify({users:[{id:'u1',username:'learner'}]});
 const timers=new Map();let timerId=0;let clock=Date.now();const NativeDate=Date;
 global.Date=class extends NativeDate {constructor(...args){super(...(args.length?args:[clock]));}static now(){return clock}};
 const nativeSet=global.setTimeout,nativeClear=global.clearTimeout;
 global.setTimeout=(cb,ms=0)=>{const id=++timerId;if(ms<250)queueMicrotask(cb);else timers.set(id,{cb,at:clock+ms});return id};
 global.clearTimeout=(id)=>timers.delete(id);
 global.window={localStorage:{getItem:()=>snapshot,setItem:(_k,v)=>{snapshot=v}}};
 t.after(()=>{global.Date=NativeDate;global.setTimeout=nativeSet;global.clearTimeout=nativeClear;delete global.window});
 async function load(run){const dir=path.join(tmp,String(run));for(const name of ['lib/personas.ts','lib/focus.ts','lib/mock/server.ts']){const out=path.join(dir,name.replace(/\.ts$/,'.js'));fs.mkdirSync(path.dirname(out),{recursive:true});fs.writeFileSync(out,ts.transpileModule(fs.readFileSync(path.join(testDir,'src',name),'utf8'),{compilerOptions:{module:ts.ModuleKind.CommonJS,target:ts.ScriptTarget.ES2022}}).outputText)}return import(pathToFileURL(path.join(dir,'lib/mock/server.js')).href)}
 let {mockTransport:api,MockSocket}=await load(1);const req=(method,path,body)=>api({method,path,body,token:'mock.u1'});
 for(const body of [{durationMinutes:null},{learningGoal:null},{durationMinutes:5},{learningGoal:'GRAMMAR'}])assert.equal((await req('POST','/sessions',body)).status,400);
 const created=await req('POST','/sessions',{durationMinutes:10,learningGoal:'INCIDENT_UPDATE'});assert.equal(created.status,201);const sid=created.body.id;assert.equal(created.body.learningGoal,'INCIDENT_UPDATE');assert.equal(created.body.durationMinutes,10);assert.match(created.body.workContext.notes.join(' '),/11:15/);
 async function advance(ms){clock+=ms;for(let n=0;n<30;n++){const due=[...timers.entries()].filter(([,v])=>v.at<=clock).sort((a,b)=>a[1].at-b[1].at);if(!due.length)break;for(const [id,v]of due){timers.delete(id);v.cb();await Promise.resolve();await Promise.resolve()}}await new Promise(r=>queueMicrotask(r))}
 await advance(1500);let details=await req('GET',`/sessions/${sid}`);assert.equal(details.body.tasks.length,1);assert.equal(details.body.deadlines.length,0);
 const first=details.body.tasks[0];await advance(1);await req('POST',`/sessions/${sid}/channels/${first.channelId}/messages`,{parentId:first.messageId,content:'@sarah @david Checkout has 502 errors.'});await advance(1000);details=await req('GET',`/sessions/${sid}`);assert.equal(details.body.deadlines.length,0);assert.equal(details.body.tasks[0].status,'ACKNOWLEDGED');
 timers.clear();({mockTransport:api,MockSocket}=await load(2));const socket=new MockSocket('ws://mock?token=mock.u1');await Promise.resolve();socket.send(JSON.stringify({type:'subscribe',data:{sessionId:sid}}));await advance(300000);details=await req('GET',`/sessions/${sid}`);assert.equal(details.body.tasks.length,2);assert.equal(new Set(details.body.tasks.map(t=>t.title)).size,2);assert.equal(details.body.deadlines.length,0);
 timers.clear();({mockTransport:api,MockSocket}=await load(3));const resumed=new MockSocket('ws://mock?token=mock.u1');await Promise.resolve();resumed.send(JSON.stringify({type:'subscribe',data:{sessionId:sid}}));await advance(200000);details=await req('GET',`/sessions/${sid}`);assert.equal(details.body.tasks.length,2);assert.equal(details.body.session.learningGoal,'INCIDENT_UPDATE');assert.equal(details.body.warnings.length,0);
});
