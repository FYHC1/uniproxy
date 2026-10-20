import test from 'node:test';
import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import ts from 'typescript';

// The helpers have no React/runtime dependencies. Compile this TSX module in
// memory so the normal Node unit runner needs no loader or generated files.
const source = readFileSync(new URL('../CoreInstallModel.tsx', import.meta.url), 'utf8');
const compiled = ts.transpileModule(source, { compilerOptions: { target: ts.ScriptTarget.ES2022, module: ts.ModuleKind.ESNext } });
const { isCoreInstallation, shouldPromptForCore, coreStageLabel, coreDownloadPercent, coreBytes } = await import('data:text/javascript;base64,' + Buffer.from(compiled.outputText).toString('base64'));
const base = { state:'idle', stage:'', version:'1.19.32', os:'windows', arch:'amd64', path:'C:/fixture/mihomo.exe', installed:false, supported:true, canInstall:true, reason:'', bytesDownloaded:0, totalBytes:0, error:'', startedAt:'', finishedAt:'' };

test('core installation contract validates required fields without inventing defaults', () => {
  assert(isCoreInstallation(base));
  for (const field of Object.keys(base)) {
    const missing = { ...base }; delete missing[field];
    assert.equal(isCoreInstallation(missing), false, field + ' is required');
  }
  for (const patch of [{state:'ready'}, {state:['idle']}, {canInstall:'true'}, {bytesDownloaded:-1}, {bytesDownloaded:Infinity}, {totalBytes:NaN}, {stage:null}]) assert.equal(isCoreInstallation({...base,...patch}),false);
  assert.equal(isCoreInstallation(null),false);
  assert.equal(isCoreInstallation({...base,stage:'future-server-stage'}),true,'stage is an open string contract');
});

test('automatic missing-core prompt only follows a new missing non-active destination', () => {
  const runtime = {available:false};
  assert.equal(shouldPromptForCore(base,runtime,new Set()),true);
  assert.equal(shouldPromptForCore(base,runtime,new Set([base.path])),false);
  assert.equal(shouldPromptForCore({...base,path:'another-path'},runtime,new Set([base.path])),true);
  assert.equal(shouldPromptForCore({...base,state:'installing'},runtime,new Set()),false);
  assert.equal(shouldPromptForCore({...base,installed:true},runtime,new Set()),false,'existing incompatible files are not missing');
  assert.equal(shouldPromptForCore(base,{available:true},new Set()),false);
  assert.equal(shouldPromptForCore({...base,path:''},runtime,new Set()),true,'missing PATH override still explains why installation is disabled');
});

test('download percentage uses only received bytes and never implies verification completion', () => {
  const active = {...base,state:'installing',stage:'downloading',bytesDownloaded:1024,totalBytes:4096};
  assert.equal(coreDownloadPercent(active),25);
  assert.equal(coreDownloadPercent({...active,totalBytes:0}),null);
  assert.equal(coreDownloadPercent({...active,bytesDownloaded:5000}),100);
  for (const stage of ['metadata','verifying','extracting','installing','inspecting','complete','']) assert.equal(coreDownloadPercent({...active,stage}),null,stage);
  for (const state of ['idle','succeeded','failed']) assert.equal(coreDownloadPercent({...active,state}),null,state);
});

test('stage and byte labels distinguish missing, active, failed and installed states', () => {
  assert.equal(coreStageLabel(base),'尚未安装');
  assert.equal(coreStageLabel({...base,installed:true}),'已检测到文件');
  assert.equal(coreStageLabel({...base,state:'failed'}),'安装失败');
  assert.equal(coreStageLabel({...base,state:'succeeded'}),'安装完成');
  assert.equal(coreStageLabel({...base,state:'installing',stage:'verifying'}),'校验 SHA256');
  assert.equal(coreStageLabel({...base,state:'installing',stage:'unexpected'}),'准备安装');
  assert.equal(coreBytes(0),'0 B');assert.equal(coreBytes(1024),'1.0 KiB');assert.equal(coreBytes(1048576),'1.0 MiB');
});
