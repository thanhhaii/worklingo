import { test } from 'node:test';
import assert from 'node:assert/strict';
import fs from 'node:fs';
import os from 'node:os';
import path from 'node:path';
import { fileURLToPath, pathToFileURL } from 'node:url';
import ts from 'typescript';
const testDir = fileURLToPath(new URL('.', import.meta.url));

test('mock retry preserves available components, rejects running work, and normalizes legacy coverage', async (t) => {
  const tmp = fs.mkdtempSync(path.join(os.tmpdir(), 'worklingo-evaluation-'));
  t.after(() => fs.rmSync(tmp, { recursive: true, force: true }));
  for (const name of ['lib/personas.ts', 'lib/mock/server.ts']) {
    const target = path.join(tmp, name.replace(/\.ts$/, '.js'));
    fs.mkdirSync(path.dirname(target), { recursive: true });
    fs.writeFileSync(target, ts.transpileModule(fs.readFileSync(path.join(testDir, 'src', name), 'utf8'), {
      compilerOptions: { module: ts.ModuleKind.CommonJS, target: ts.ScriptTarget.ES2022 },
    }).outputText);
  }
  const available = { messageId: 'm1', reviewStatus: 'AVAILABLE', channelLabel: '#general', content: 'Keep this', createdAt: new Date().toISOString(), issues: [] };
  const task = { taskId: 't1', reviewStatus: 'AVAILABLE', title: 'Keep task', status: 'OPEN', personaId: 'alex', talkingPoints: [] };
  const fixture = {
    users: [{ id: 'u1', username: 'learner', email: 'learner@test.local' }, { id: 'u2', username: 'other' }],
    sessions: [{ id: 's1', userId: 'u1', status: 'COMPLETED', startedAt: new Date().toISOString(), expiresAt: new Date().toISOString(), warnings: 0 }],
    evaluations: [{ id: 'e1', sessionId: 's1', status: 'PARTIAL', scoreStatus: 'AVAILABLE', messageReviewStatus: 'UNAVAILABLE', taskReviewStatus: 'AVAILABLE', grammar: 91, tone: 92, vocabulary: 93, overall: 92, summary: 'Keep summary', messageReviews: [available], taskReviews: [task] }],
    messages: [{ id: 'm1', sessionId: 's1', channelId: 'c1', authorType: 'USER', content: 'Keep this', createdAt: new Date().toISOString() }, { id: 'm2', sessionId: 's1', channelId: 'c1', authorType: 'USER', content: 'Review this', createdAt: new Date().toISOString() }],
    channels: [{ id: 'c1', sessionId: 's1', kind: 'CHANNEL', name: 'general' }],
    tasks: [{ id: 't1', sessionId: 's1', title: 'Keep task', personaId: 'alex', status: 'OPEN' }],
  };
  fixture.sessions.push({ ...fixture.sessions[0], id: 'legacy' }, { ...fixture.sessions[0], id: 'active', status: 'ACTIVE' }, { ...fixture.sessions[0], id: 'missing' });
  fixture.evaluations.push({ id: 'old', sessionId: 'legacy', status: 'DONE', grammar: 80, messageReviews: [{ messageId: 'old', issues: [] }], taskReviews: [{ taskId: 'old', talkingPoints: [] }] }, { id: 'active-eval', sessionId: 'active', status: 'FAILED' });
  global.window = { localStorage: { getItem: () => JSON.stringify(fixture), setItem() {} } };
  t.after(() => delete global.window);
  const { mockTransport } = await import(pathToFileURL(path.join(tmp, 'lib/mock/server.js')).href);
  const req = { method: 'POST', path: '/sessions/s1/evaluation/retry', token: 'mock.u1' };
  assert.equal((await mockTransport({ ...req, token: null })).status, 401);
  assert.equal((await mockTransport({ ...req, token: 'mock.u2' })).status, 404);
  assert.equal((await mockTransport({ ...req, path: '/sessions/active/evaluation/retry' })).status, 409);
  assert.equal((await mockTransport({ ...req, path: '/sessions/missing/evaluation/retry' })).status, 409);
  const legacy = await mockTransport({ ...req, method: 'GET', path: '/sessions/legacy/evaluation' });
  assert.equal(legacy.body.status, 'PARTIAL');
  assert.equal(legacy.body.scoreStatus, 'AVAILABLE');
  assert.equal(legacy.body.messageReviews[0].reviewStatus, 'UNAVAILABLE');
  assert.equal(legacy.body.taskReviews[0].reviewStatus, 'UNAVAILABLE');
  const responses = await Promise.all([mockTransport(req), mockTransport(req)]);
  assert.deepEqual(responses.map(r => r.status).sort(), [202, 409]);
  await new Promise(resolve => setTimeout(resolve, 4200));
  const final = await mockTransport({ ...req, method: 'GET', path: '/sessions/s1/evaluation' });
  assert.equal(final.body.status, 'DONE');
  assert.equal(final.body.grammar, 91);
  assert.equal(final.body.summary, 'Keep summary');
  assert.deepEqual(final.body.messageReviews[0], available);
  assert.deepEqual(final.body.taskReviews[0], task);
  assert.equal(final.body.messageReviews[1].reviewStatus, 'AVAILABLE');
  assert.equal((await mockTransport(req)).status, 409);
});
