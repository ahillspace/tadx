"""Qualify one completed Codex turn against its same-thread runtime context."""

from __future__ import annotations

import json
import re


class EffectiveRuntimeError(ValueError):
    """A safe, credential-free runtime qualification failure."""


# This is executed only in the disposable worker whose CODEX_HOME is tmpfs.
# It reads the rollout locally and exports only fixed, bounded fields.
ROLLOUT = r"""
const fs=require('fs'), path=require('path');
const base='/home/worker/.codex/sessions', thread=process.argv[1];
const out={thread_matched:false,context_count:0,context_consistent:true,provider:null,model:null,effort:null};
const safe=s=>typeof s==='string'&&/^[A-Za-z0-9_.:-]{1,128}$/.test(s);
let matched=0, files=0;
function walk(dir,depth){
  if(depth>8||!fs.existsSync(dir))return;
  if(fs.lstatSync(dir).isSymbolicLink())throw Error('session root link');
  for(const entry of fs.readdirSync(dir,{withFileTypes:true})){
    const name=path.join(dir,entry.name);
    if(entry.isSymbolicLink())continue;
    if(entry.isDirectory()){walk(name,depth+1);continue;}
    if(!entry.isFile()||!name.endsWith('.jsonl'))continue;
    if(++files>10000)throw Error('rollout count');
    const stat=fs.statSync(name);
    if(stat.size>16*1024*1024)throw Error('rollout size');
    const rows=fs.readFileSync(name,'utf8').split('\n').filter(Boolean).map(x=>JSON.parse(x));
    const identities=rows.filter(x=>x.type==='session_meta'&&x.payload&&x.payload.id===thread);
    if(!identities.length)continue;
    if(++matched!==1||identities.length!==1)throw Error('ambiguous thread');
    const provider=identities[0].payload.model_provider;
    if(safe(provider))out.provider=provider;
    for(const row of rows){
      if(row.type!=='turn_context'||!row.payload)continue;
      out.context_count++;
      const value=row.payload;
      const effort=typeof value.effort==='string'?value.effort:
        value.collaboration_mode&&value.collaboration_mode.reasoning_effort;
      const model=safe(value.model)?value.model:null;
      const selectedEffort=safe(effort)?effort:null;
      if(out.context_count>1&&(model!==out.model||selectedEffort!==out.effort))out.context_consistent=false;
      out.model=model;
      out.effort=selectedEffort;
    }
  }
}
try{walk(base,0);out.thread_matched=matched===1;process.stdout.write(JSON.stringify(out));}
catch(_){process.exit(2);}
"""


def thread_from_events(events):
    if not isinstance(events, list):
        raise EffectiveRuntimeError("Codex task events are unavailable")
    threads = [row.get("thread_id") for row in events
               if isinstance(row, dict) and row.get("type") == "thread.started"]
    if (len(threads) != 1 or not isinstance(threads[0], str)
            or not re.fullmatch(r"[A-Za-z0-9-]{1,128}", threads[0])):
        raise EffectiveRuntimeError("Codex task has no unique thread identity")
    return threads[0]


def parse_observation(raw):
    try:
        item = json.loads(raw)
    except (TypeError, ValueError):
        raise EffectiveRuntimeError("Same-thread runtime context is unavailable") from None
    if (not isinstance(item, dict) or set(item) !=
            {"thread_matched", "context_count", "context_consistent", "provider", "model", "effort"}
            or item["thread_matched"] is not True
            or item["context_consistent"] is not True
            or type(item["context_count"]) is not int or item["context_count"] < 1):
        raise EffectiveRuntimeError("Same-thread runtime context is incomplete")
    for key in ("provider", "model", "effort"):
        if not isinstance(item[key], str) or not re.fullmatch(r"[A-Za-z0-9_.:-]{1,128}", item[key]):
            raise EffectiveRuntimeError("Same-thread runtime configuration is invalid")
    return item


def qualify(task, observation, *, model, effort, previous_contexts=0, expected_thread=None):
    """Return only allowlisted proof for one successful, completed task turn."""
    events = task.get("events") if isinstance(task, dict) else None
    thread = thread_from_events(events)
    if expected_thread is not None and thread != expected_thread:
        raise EffectiveRuntimeError("Codex continuation used another thread")
    if (task.get("exit_status") != 0 or task.get("timed_out") is not False
            or sum(isinstance(row, dict) and row.get("type") == "turn.completed" for row in events) != 1
            or any(isinstance(row, dict) and row.get("type") in ("turn.failed", "error") for row in events)):
        raise EffectiveRuntimeError("Codex task did not complete one successful turn")
    item = parse_observation(observation)
    if item["context_count"] <= previous_contexts:
        raise EffectiveRuntimeError("Codex turn has no fresh runtime context")
    if item["provider"] != "openai" or item["model"] != model or item["effort"] != effort:
        raise EffectiveRuntimeError("Effective Codex runtime differs from the requested configuration")
    return {"thread_matched": True, "turn_completed": True, "provider": item["provider"],
            "model": item["model"], "effort": item["effort"],
            "context_count": item["context_count"]}
