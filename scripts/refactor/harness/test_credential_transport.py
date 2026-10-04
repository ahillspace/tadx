"""Fake-secret transport checks for the maintained copied bridge."""

import ast
import json
from pathlib import Path
import re
import shutil
import subprocess
import unittest
from unittest.mock import patch

from credential_patch import patch_broker, patch_bridge


HERE = Path(__file__).resolve().parent
BRIDGE = HERE / "source/integration/docker_local_bridge.py"
BROKER = HERE / "source/integration/credential_broker.cjs"
SENTINEL = "SENTINEL-NOT-A-REAL-PAT"


def bridge_functions(*names):
    tree = ast.parse(BRIDGE.read_text(encoding="utf-8"))
    selected = [node for node in tree.body if isinstance(node, ast.FunctionDef)
                and node.name in names]
    if {node.name for node in selected} != set(names):
        raise AssertionError("Maintained bridge function is missing")
    namespace = {"Path": Path, "json": json, "re": re, "subprocess": subprocess}
    exec(compile(ast.Module(body=selected, type_ignores=[]), str(BRIDGE), "exec"), namespace)
    return namespace


class CredentialTransportTests(unittest.TestCase):
    @unittest.skipUnless(shutil.which("node"), "Node is unavailable")
    def test_embedded_client_requires_ready_before_close(self):
        tree = ast.parse(BRIDGE.read_text(encoding="utf-8"))
        function = next(node for node in tree.body if isinstance(node, ast.FunctionDef)
                        and node.name == "_initialize_broker_credentials")
        script = next(ast.literal_eval(node.value) for node in ast.walk(function)
                      if isinstance(node, ast.Assign)
                      and any(isinstance(target, ast.Name) and target.id == "script"
                              for target in node.targets))
        driver = r"""
const vm=require('node:vm'),EventEmitter=require('node:events');
const socket=new EventEmitter();let timeout;
socket.setTimeout=(_ms,callback)=>{timeout=callback;};socket.end=()=>{};
socket.destroy=()=>socket.emit('close');
const sandbox={require:name=>name==='net'?{createConnection:()=>socket}:
  {readFileSync:()=>Buffer.from('{}')},
  process:{argv:['node','-'],exit:code=>{process.exitCode=code;}},Buffer};
vm.runInNewContext(process.argv[1],sandbox);
setImmediate(()=>{
  socket.emit('connect');
  const mode=process.argv[2];
  if(mode==='ready'){socket.emit('data',Buffer.from('ready'));socket.emit('end');socket.emit('close');}
  else if(mode==='wrong'){socket.emit('data',Buffer.from('denied'));socket.emit('end');socket.emit('close');}
  else if(mode==='timeout'){timeout();}
  else if(mode==='oversize'){socket.emit('data',Buffer.alloc(17));}
  else socket.emit('close');
});
"""
        for mode in ("close", "ready", "wrong", "timeout", "oversize"):
            result = subprocess.run(["node", "-e", driver, script, mode], capture_output=True,
                                    text=True, timeout=10, check=False)
            with self.subTest(mode=mode):
                self.assertEqual(result.stderr, "")
                self.assertEqual(result.returncode == 0, mode == "ready")

    @unittest.skipUnless(shutil.which("node"), "Node is unavailable")
    def test_embedded_client_rejects_socket_close_without_ready(self):
        tree = ast.parse(BRIDGE.read_text(encoding="utf-8"))
        function = next(node for node in tree.body if isinstance(node, ast.FunctionDef)
                        and node.name == "_initialize_broker_credentials")
        script = next(ast.literal_eval(node.value) for node in ast.walk(function)
                      if isinstance(node, ast.Assign)
                      and any(isinstance(target, ast.Name) and target.id == "script"
                              for target in node.targets))
        driver = r"""
const cp=require('node:child_process'),net=require('node:net'),os=require('node:os'),path=require('node:path');
const location=process.platform==='win32'?'\\\\.\\pipe\\tadx-credential-close-'+process.pid:
  path.join(os.tmpdir(),'tadx-credential-close-'+process.pid+'.sock');
let peer,accepted=false;
const server=net.createServer({allowHalfOpen:true},socket=>{peer=socket;accepted=true;socket.pause();});
server.listen(location,()=>{
  const client=process.argv[1].replace("'/tmp/tadx-private/credential-init.sock'",JSON.stringify(location))
    .replace('socket.setTimeout(5000,','socket.setTimeout(100,');
  const child=cp.spawn(process.execPath,['-e',client],{stdio:['pipe','pipe','pipe']});
  let stderr='';child.stderr.on('data',chunk=>{stderr+=chunk.toString('utf8');});
  child.stdin.end('{}');
  child.on('close',(code,signal)=>{if(peer)peer.destroy();server.close();
    process.stdout.write(JSON.stringify({code,signal,accepted,stderr}));});
});
"""
        result = subprocess.run(["node", "-e", driver, script], capture_output=True,
                                text=True, timeout=10, check=False)
        self.assertEqual(result.returncode, 0, result.stderr)
        observed = json.loads(result.stdout)
        self.assertTrue(observed["accepted"], observed)
        self.assertEqual(observed["stderr"], "")
        self.assertNotEqual(observed["code"], 0)

    def test_binding_is_bounded_and_never_saved_or_passed_in_argv(self):
        functions = bridge_functions("_remote_credentials", "_credential_exec")
        request = {"private_case_dir": "/private-test-case"}
        original = {"exercise": {"id": "P-auth-check"}}
        binding = {"agent": {"pat_name": "fixture-name", "pat_secret": SENTINEL},
                   "destination": {"agent": {"pat_name": "other-name",
                                              "pat_secret": "OTHER-SENTINEL-NOT-A-PAT"}}}

        class FakeProfile:
            def load_binding(self, value):
                self_value = value
                assert self_value is original
                return binding

        functions.update(load=lambda path: original, profile=lambda value: FakeProfile(),
                         Blocked=ValueError, os=__import__("os"))
        state = {"additional_credential_bindings": [{"binding_key": "destination",
                  "pat_name": "TADX_BENCH_DEST_PAT_NAME",
                  "pat_secret": "TADX_BENCH_DEST_PAT_SECRET"}]}
        values, payload = functions["_remote_credentials"](request, state)
        self.assertEqual(values["TADX_BENCH_AGENT_PAT_SECRET"], SENTINEL)
        self.assertEqual(values["TADX_BENCH_DEST_PAT_SECRET"], "OTHER-SENTINEL-NOT-A-PAT")
        seen = []

        def fake_run(argv, **kwargs):
            seen.append((argv, kwargs))
            return subprocess.CompletedProcess(argv, 0, b"ready", b"")

        with patch.object(subprocess, "run", fake_run):
            result = functions["_credential_exec"]("fixture-container", "process.exit(0)",
                                                    payload, "fixture-argument")
        self.assertEqual(result.returncode, 0)
        argv, kwargs = seen.pop()
        self.assertEqual(argv[:7], ["docker", "exec", "-i", "--user", "0:0",
                                    "fixture-container", "node"])
        self.assertNotIn(SENTINEL, " ".join(argv))
        self.assertEqual(kwargs["input"], payload)
        self.assertNotIn("credentials.json", BRIDGE.read_text(encoding="utf-8").split(
            "def _remote_credentials", 1)[1].split("def _credential_exec", 1)[0])

    def test_broker_initialization_is_stdin_only_and_checks_persistent_volume(self):
        functions = bridge_functions("_initialize_broker_credentials")
        calls = []

        def fake_docker(*args, **kwargs):
            calls.append((args, kwargs))
            return subprocess.CompletedProcess(args, 0, "", "")

        payload = json.dumps({"TADX_BENCH_AGENT_PAT_NAME": "fixture-name",
                              "TADX_BENCH_AGENT_PAT_SECRET": SENTINEL}).encode()
        sent = []

        def fake_exec(name, script, data):
            sent.append((name, script, data))
            return subprocess.CompletedProcess([], 0, b"", b"")

        functions.update(docker=fake_docker, _credential_exec=fake_exec,
                         READY_POLL_ATTEMPTS=1, READY_POLL_DELAY_S=0,
                         Blocked=ValueError)
        functions["_initialize_broker_credentials"]("fixture-container", payload)
        self.assertEqual(len(sent), 1)
        self.assertEqual(sent[0][2], payload)
        self.assertIn("fs.readFileSync(0)", sent[0][1])
        self.assertNotIn(SENTINEL, sent[0][1])
        self.assertIn(("exec", "--user", "0:0", "fixture-container", "test", "!", "-e",
                       "/cli-state/credentials.json"), [item[0] for item in calls])

    def test_preflight_and_store_observer_use_stdin_without_secret_artifacts(self):
        source = BRIDGE.read_text(encoding="utf-8")
        self.assertNotIn("fs.writeFileSync('/cli-state/credentials.json'", source)
        self.assertNotIn("fs.readFileSync('/cli-state/credentials.json'", source)
        self.assertIn("_credential_exec(name,probe_js,payload", source)
        self.assertIn("_credential_exec(name,seed,payload", source)
        self.assertIn("_credential_exec(name,script,payload", source)
        self.assertIn("_redact_credentials(probe.stdout,credentials)", source)
        self.assertIn("_redact_credentials(probe.stderr,credentials)", source)
        self.assertIn("'BENCH_CREDENTIAL_CHANNEL=1'", source)
        self.assertTrue(BROKER.is_file())
        functions = bridge_functions("_redact_credentials")
        output = functions["_redact_credentials"](
            ("output " + SENTINEL + " fixture-name").encode(),
            {"TADX_BENCH_AGENT_PAT_NAME": "fixture-name",
             "TADX_BENCH_AGENT_PAT_SECRET": SENTINEL})
        self.assertEqual(output, "output [REDACTED] [REDACTED]")

    def test_exact_broker_patch_replaces_volume_read(self):
        original = ("const fs = require('node:fs');\n"
                    "function startBroker() {\n"
                    "    if (fs.existsSync('/cli-state/credentials.json')) "
                    "Object.assign(env,JSON.parse(fs.readFileSync('/cli-state/credentials.json','utf8')));\n")

        def replace_once(source, before, after):
            self.assertEqual(source.count(before), 1)
            return source.replace(before, after, 1)

        patched = patch_broker(original, replace_once)
        self.assertIn("credentialBroker.start()", patched)
        self.assertIn("credentialBroker.environment()", patched)
        self.assertNotIn("fs.readFileSync('/cli-state/credentials.json'", patched)
        with self.assertRaises(ValueError):
            patch_broker(patched, replace_once)
        image = patch_bridge("broker_files=('g9_windows_installer_broker.cjs')", replace_once)
        self.assertIn("'g9_windows_installer_broker.cjs','credential_broker.cjs'", image)


if __name__ == "__main__":
    unittest.main()
