import {test} from 'node:test';
import assert from 'node:assert/strict';
import fs from 'node:fs';
import os from 'node:os';
import path from 'node:path';
import {fileURLToPath,pathToFileURL} from 'node:url';
import ts from 'typescript';
const root=fileURLToPath(new URL('.',import.meta.url));
test('practice mock materializes once, validates input, persists attempts and replays request IDs',async(t)=>{
 const tmp=fs.mkdtempSync(path.join(os.tmpdir(),'worklingo-practice-'));t.after(()=>fs.rmSync(tmp,{recursive:true,force:true}));
 const time=new Date().toISOString();let snapshot=JSON.stringify({users:[{id:'u1',username:'learner'},{id:'u2'}],sessions:[{id:'s1',userId:'u1',status:'COMPLETED'}],messages:[{id:'m1',sessionId:'s1',channelId:'c1',content:'I has finished.',authorType:'USER',createdAt:time}],evaluations:[{id:'e1',sessionId:'s1',status:'DONE',scoreStatus:'AVAILABLE',messageReviewStatus:'AVAILABLE',taskReviewStatus:'AVAILABLE',messageReviews:[{messageId:'m1',reviewStatus:'AVAILABLE',content:'I has finished.',issues:[{original:'has',corrected:'have',category:'GRAMMAR',explanation:'Chủ ngữ I dùng have.'}]}],taskReviews:[]}]});
 global.window={localStorage:{getItem:()=>snapshot,setItem:(_k,v)=>snapshot=v}};t.after(()=>delete global.window);
 async function load(n){for(const name of ['lib/personas.ts','lib/focus.ts','lib/mock/server.ts']){const out=path.join(tmp,String(n),name.replace(/\.ts$/,'.js'));fs.mkdirSync(path.dirname(out),{recursive:true});fs.writeFileSync(out,ts.transpileModule(fs.readFileSync(path.join(root,'src',name),'utf8'),{compilerOptions:{module:ts.ModuleKind.CommonJS,target:ts.ScriptTarget.ES2022}}).outputText)}return (await import(pathToFileURL(path.join(tmp,String(n),'lib/mock/server.js')).href)).mockTransport}
 let transport=await load(1);const req=(method,path,body,token='mock.u1')=>transport({method,path,body,token});
 for(const body of [null,[],true,4,'bad'])assert.equal((await req('POST','/sessions',body)).status,400);
 assert.equal((await req('GET','/sessions/s1/practice',undefined,'mock.u2')).status,404);
 const queue=await req('GET','/sessions/s1/practice');assert.equal(queue.status,200);assert.equal(queue.body.items.length,1);const id=queue.body.items[0].id;assert.equal(queue.body.items[0].correction,'I have finished.');assert.deepEqual(queue.body.items[0].attempts,[]);
 assert.equal((await req('GET','/sessions/s1/practice')).body.items[0].id,id);
 const rid=()=>crypto.randomUUID();for(const body of [null,{}, {requestId:rid(),answers:[]},{requestId:rid(),answers:[{exerciseId:'unknown',answer:'x'}]},{requestId:rid(),answers:[{exerciseId:id,answer:' '}]},{requestId:rid(),answers:[{exerciseId:id,answer:'x'.repeat(4001)}]},{requestId:rid(),answers:[{exerciseId:id,answer:'x'},{exerciseId:id,answer:'x'}]}])assert.equal((await req('POST','/sessions/s1/practice/attempts',body)).status,400);
 const body={requestId:rid(),answers:[{exerciseId:id,answer:"I've finished."}]};const responses=await Promise.all([req('POST','/sessions/s1/practice/attempts',body),req('POST','/sessions/s1/practice/attempts',body)]);assert.deepEqual(responses.map(r=>r.status).sort(),[200,409]);const result=responses.find(r=>r.status===200).body;assert.equal(result.attempts[0].verdict,'ACCEPTED');assert.match(result.attempts[0].feedback,/mô phỏng/);assert.deepEqual((await req('POST','/sessions/s1/practice/attempts',body)).body,result);
 assert.equal((await req('POST','/sessions/s1/practice/attempts',{...body,answers:[{exerciseId:id,answer:'Different'}]})).status,409);
 transport=await load(2);const restored=await req('GET','/sessions/s1/practice');assert.equal(restored.body.items[0].id,id);assert.equal(restored.body.items[0].attempts.length,1);assert.equal(restored.body.items[0].attempts[0].answer,"I've finished.");
 // Interrupted fixture claims recover as unavailable, retaining answers on reload.
 const stored=JSON.parse(snapshot);stored.practiceBatches.push({requestId:rid(),sessionId:'s1',userId:'u1',status:'PENDING',answers:[{exerciseId:id,answer:'Retained after interruption'}],attempts:[]});snapshot=JSON.stringify(stored);transport=await load(3);const recovered=await req('GET','/sessions/s1/practice');assert.equal(recovered.body.items[0].attempts.at(-1).verdict,'UNAVAILABLE');assert.equal(recovered.body.items[0].attempts.at(-1).answer,'Retained after interruption');
});

test('practice drafts retain answers and reuse request IDs after ambiguous network failures',async(t)=>{
 const tmp=fs.mkdtempSync(path.join(os.tmpdir(),'worklingo-drafts-'));t.after(()=>fs.rmSync(tmp,{recursive:true,force:true}));
 const out=path.join(tmp,'practice.js');fs.writeFileSync(out,ts.transpileModule(fs.readFileSync(path.join(root,'src/lib/practice.ts'),'utf8'),{compilerOptions:{module:ts.ModuleKind.CommonJS,target:ts.ScriptTarget.ES2022}}).outputText);
 const {submissionFor,restoreDraft}=await import(pathToFileURL(out).href);
 const answers=[{exerciseId:'one',answer:'My draft'}];const pending={requestId:'already-sent',answers};
 const restored=restoreDraft(JSON.stringify({answers:{one:'My draft'},selected:['one'],pending}));assert.equal(restored.answers.one,'My draft');assert.deepEqual(submissionFor(answers,restored.pending),pending);
 const edited=submissionFor([{exerciseId:'one',answer:'Edited'}],restored.pending);assert.notEqual(edited.requestId,'already-sent');
 const retry=submissionFor(answers,null);assert.notEqual(retry.requestId,'already-sent');assert.deepEqual(restoreDraft('broken'),{answers:{},selected:[],pending:null});
});
