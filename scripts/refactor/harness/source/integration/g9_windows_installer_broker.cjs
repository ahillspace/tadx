// One exact model-selected installer helper command, without shell delegation.
'use strict';
const crypto=require('node:crypto');

const CASES=new Set(['fresh','idempotent','no-completion','completion-opt-in',
  'failed-download-preserves-binary','uninstall','no-modify-path']);
const HEX=/^[0-9a-f]{64}$/;
const GIT_SHA=/^[0-9a-f]{40}$/;
const VERSION=/^[0-9]+\.[0-9]+\.[0-9]+(?:[-+][0-9A-Za-z.-]+)?$/;

function expected(caseName,version){
  if(!CASES.has(caseName)||typeof version!=='string'||!VERSION.test(version))return null;
  if(caseName==='uninstall')return ['uninstall'];
  const actual=caseName==='failed-download-preserves-binary'?version+'-missing':version;
  const args=['install','--version',actual];
  if(caseName==='no-completion')args.push('--no-completion');
  if(caseName==='no-modify-path')args.push('--no-modify-path');
  return args;
}

function allowed(args,guard){
  if(!guard||guard.mode!=='shared-windows-installer'||!Array.isArray(args))return false;
  if(!GIT_SHA.test(guard.source_sha||'')||!HEX.test(guard.binary_sha256||'')
      ||!HEX.test(guard.installer_sha256||'')||!GIT_SHA.test(guard.setup_commit||'')
      ||!HEX.test(guard.baseline_sha256||'')||!Number.isSafeInteger(guard.hosted_run_id)
      ||guard.hosted_run_id<1||!/^V-installer-windows-[a-z-]+$/.test(guard.exercise_id||''))return false;
  const caseName=guard.exercise_id.slice('V-installer-windows-'.length);
  if(caseName!==guard.case||guard.credential_input!==false||guard.runtime_network!=='none')return false;
  const wanted=expected(caseName,guard.fixture_version);
  return wanted!==null&&args.length===wanted.length&&args.every((value,index)=>value===wanted[index]);
}

function request(args,guard,id){
  if(!allowed(args,guard)||!/^cmd-[0-9]+$/.test(id))throw Error('Unbound Windows installer request');
  const body={protocol:'tadx-windows-installer-broker/1',id,case:guard.case,
    argv:args,source_sha:guard.source_sha,binary_sha256:guard.binary_sha256,
    installer_sha256:guard.installer_sha256,fixture_version:guard.fixture_version,
    setup_commit:guard.setup_commit,baseline_sha256:guard.baseline_sha256,
    run_id:guard.hosted_run_id};
  body.command_sha256=crypto.createHash('sha256').update(JSON.stringify(body)).digest('hex');
  return body;
}

function validResponse(body,response){
  return response&&response.protocol==='tadx-windows-installer-response/1'
    &&response.id===body.id&&response.command_sha256===body.command_sha256
    &&response.setup_commit===body.setup_commit&&response.baseline_sha256===body.baseline_sha256
    &&['verified','not_verified'].includes(response.status)
    &&response.run_id===body.run_id
    &&Number.isInteger(response.exit_status)&&response.exit_status>=0&&response.exit_status<=125
    &&typeof response.stdout==='string'&&Buffer.byteLength(response.stdout)<=8192
    &&typeof response.stderr==='string'&&Buffer.byteLength(response.stderr)<=8192
    &&(response.status==='verified'
      ?(body.case==='failed-download-preserves-binary'
        ?response.exit_status>0&&response.exit_status<124:response.exit_status===0)
      :response.exit_status===125);
}

module.exports={allowed,request,validResponse,expected};
