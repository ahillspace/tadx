"""Synthetic checks for strict job routing in the blocked copied broker."""

import unittest
from pathlib import Path
import subprocess

from job_patches import patch_job_broker, patch_job_image


class JobPatchesTests(unittest.TestCase):
    def test_exact_job_guard_precedes_disposable_native_check(self):
        source = ("const projectBroker = require('./project_broker.cjs');\n"
                  "function allowed(args, state) {\n"
                  "  const parsed = parseArgs(args);\n"
                  "  if(state.guard?.execution_mode==='disposable_native')return false;\n"
                  "  if (Object.hasOwn(flags,'config')) return false;\n"
                  "}\n"
                  "  if(state.guardPresent)try{state.guard=JSON.parse(fs.readFileSync('/cli-state/task-policy.json','utf8'));}catch{}\n  return state;\n"
                  "    let result, executed=allowed(args,policyState);\n")
        patched = patch_job_broker(source)
        self.assertLess(patched.index("g9JobBroker.allowed("),
                        patched.index("execution_mode==='disposable_native'"))
        self.assertIn("const g9JobBroker = require('./g9_job_broker.cjs');", patched)
        self.assertIn("g9-job-cancel-attempted", patched)

    def test_unpatched_native_bypass_and_changed_anchors_fail_closed(self):
        source = ("const projectBroker = require('./project_broker.cjs');\n"
                  "if(state.guard?.execution_mode==='disposable_native'&&state.baselinePresent)return true;")
        with self.assertRaises(ValueError):
            patch_job_broker(source)
        with self.assertRaises(ValueError):
            patch_job_image("broker_files=('local_broker.cjs',)")

    def test_image_input_is_explicitly_included(self):
        source = "broker_files=(*broker_files,'fault_broker.cjs','credential_pty.py','g9_project_read_broker.cjs')"
        self.assertIn("'g9_job_broker.cjs'", patch_job_image(source))

    def test_disposable_native_cannot_bypass_exact_job_or_unknown_target(self):
        source = patch_job_broker(
            "const projectBroker = require('./project_broker.cjs');\n"
            "function parseArgs(args) { return args; }\n"
            "function classify(args) { return args.capability; }\n"
            "function allowed(args,state) {\n"
            "  const parsed=parseArgs(args),flags=parsed.flags;\n"
            "  if(state.guard?.execution_mode==='disposable_native')return false;\n"
            "  if (Object.hasOwn(flags,'config')) return false;\n"
            "  return false;\n"
            "}\nmodule.exports={allowed};\n"
            "function diskState(){\n  if(state.guardPresent)try{state.guard=JSON.parse(fs.readFileSync('/cli-state/task-policy.json','utf8'));}catch{}\n  return state;\n}\n"
            "function start(){\n    let result, executed=allowed(args,policyState);\n}\n")
        helper = Path(__file__).resolve().parent / "source/integration/job_broker.cjs"
        script = r"""
const fs=require('node:fs'),vm=require('node:vm');
const source=fs.readFileSync(0,'utf8');
const module={exports:{}};
vm.runInNewContext(source,{module,require:name=>name.includes('g9_job_broker')?
  require(process.argv[1]):{}});
const job='11111111-1111-4111-8111-111111111111';
const guard={family:'job',action:'job.cancel',job_id:job,
  site_luid:'22222222-2222-4222-8222-222222222222',environment:'fixture',
  site:'fixture-site',run_id:'run-1',case_id:'P-job-cancel',
  ownership_sha256:'a'.repeat(64),baseline_sha256:'a'.repeat(64),
  execution_mode:'disposable_native'};
const state={guard,guardPresent:true,baselinePresent:true,baselineUnchanged:true,
  jobOwned:true,jobFresh:true,cancelAttempted:false,registry:[]};
const parsed={words:['job','cancel'],errors:[],capability:'job.cancel',flags:{
  environment:'fixture',site:'fixture-site',id:job,json:true,full:true}};
if(!module.exports.allowed(parsed,state))process.exit(1);
if(module.exports.allowed({...parsed,flags:{...parsed.flags,id:guard.site_luid}},state))process.exit(2);
if(module.exports.allowed({...parsed,capability:'project.delete'},state))process.exit(3);
if(module.exports.allowed(parsed,{...state,guard:{...guard,family:'other'}}))process.exit(4);
"""
        result = subprocess.run(["node", "-e", script, str(helper)], input=source,
                                text=True, capture_output=True, timeout=10, check=False)
        self.assertEqual(result.returncode, 0, result.stderr)


if __name__ == "__main__":
    unittest.main()
