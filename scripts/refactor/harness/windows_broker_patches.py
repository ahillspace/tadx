"""Anchored, exact-source broker edits for the hosted Windows installer route."""


def once(source, before, after):
    if source.count(before) != 1:
        raise ValueError("Windows broker source anchor changed or is ambiguous")
    return source.replace(before, after)


def broker(source):
    source = once(source, "const sharedBroker = require('./shared_broker.cjs');",
                  "const sharedBroker = require('./shared_broker.cjs');\n"
                  "const windowsInstallerBroker = require('./g9_windows_installer_broker.cjs');")
    source = once(source, "function startBroker() {",
        "function hostedWindowsResult(args,guard,id) {\n"
        "  let bound;try{bound=windowsInstallerBroker.request(args,guard,id);}catch{return {status:125,stdout:'',stderr:'Windows installer request was refused.\\n'};}\n"
        "  try{fs.writeFileSync('/audit/windows-request.json',JSON.stringify(bound),{flag:'wx',mode:0o600});}\n"
        "  catch{return {status:125,stdout:'',stderr:'Windows installer request was already present.\\n'};}\n"
        "  const deadline=Date.now()+35*60*1000,sleeper=new Int32Array(new SharedArrayBuffer(4));\n"
        "  while(Date.now()<deadline&&!fs.existsSync('/audit/windows-response.json'))Atomics.wait(sleeper,0,0,500);\n"
        "  if(!fs.existsSync('/audit/windows-response.json'))return {status:124,stdout:'',stderr:'Hosted Windows installer did not complete within its bound.\\n'};\n"
        "  let reply;try{reply=JSON.parse(fs.readFileSync('/audit/windows-response.json','utf8'));}catch{}\n"
        "  if(!windowsInstallerBroker.validResponse(bound,reply))return {status:125,stdout:'',stderr:'Hosted Windows installer response failed identity checks.\\n'};\n"
        "  return {status:reply.exit_status,stdout:reply.stdout,stderr:reply.stderr,hosted_run_id:reply.run_id,hosted_command_sha256:bound.command_sha256};\n"
        "}\nfunction startBroker() {")
    source = once(source,
        "    const installer=body.helper==='installer'&&policyState.guard?.mode==='shared-installer';\n"
        "    if(body.helper&&!installer){res.writeHead(400);res.end();return;}\n"
        "    if(installer)executed=crypto.createHash('sha256').update(fs.readFileSync('/work/installer-assets/install.sh')).digest('hex')===policyState.guard.installer_sha256;",
        "    const installer=body.helper==='installer'&&policyState.guard?.mode==='shared-installer';\n"
        "    const windowsInstaller=body.helper==='installer'&&policyState.guard?.mode==='shared-windows-installer';\n"
        "    if(body.helper&&!installer&&!windowsInstaller){res.writeHead(400);res.end();return;}\n"
        "    if(installer)executed=crypto.createHash('sha256').update(fs.readFileSync('/work/installer-assets/install.sh')).digest('hex')===policyState.guard.installer_sha256;\n"
        "    if(windowsInstaller)executed=policyState.networkNone===true&&policyState.baselinePresent===true\n"
        "      &&policyState.baselineUnchanged===true&&!fs.existsSync('/cli-state/credentials.json')\n"
        "      &&!fs.existsSync('/audit/windows-request.json')\n"
        "      &&windowsInstallerBroker.allowed(args,policyState.guard);")
    source = once(source,
        "      argv:[installer?'tadx-bench-installer':'tadx',...args],capability:installer?null:classify(args),flags:flagsOf(args),",
        "      argv:[installer||windowsInstaller?'tadx-bench-installer':'tadx',...args],capability:installer||windowsInstaller?null:classify(args),flags:flagsOf(args),")
    source = once(source,
        "      cwd,binary_sha256:executedBinarySHA,started_at,executed,batch_input:batchInput,helper:installer?'installer':undefined})+'\\n');\n"
        "    if(executed&&installer){",
        "      cwd,binary_sha256:executedBinarySHA,started_at,executed,batch_input:batchInput,helper:installer||windowsInstaller?'installer':undefined})+'\\n');\n"
        "    if(executed&&windowsInstaller)result=hostedWindowsResult(args,policyState.guard,id);\n"
        "    else if(executed&&installer){")
    source = once(source,
        "    if(installer){row.helper='installer';row.argv=['tadx-bench-installer',...args];row.capability=null;row.installer_sha256=policyState.guard.installer_sha256;}",
        "    if(installer||windowsInstaller){row.helper='installer';row.argv=['tadx-bench-installer',...args];row.capability=null;row.installer_sha256=policyState.guard.installer_sha256;}\n"
        "    if(windowsInstaller){row.hosted_run_id=result.hosted_run_id;row.hosted_command_sha256=result.hosted_command_sha256;}")
    return source


def client(source):
    return once(source,
        "    request.on('error',reject);request.setTimeout(310000,()=>request.destroy(Error('CLI broker response timed out')));",
        "    request.on('error',reject);request.setTimeout(process.argv[1]?.endsWith('tadx-bench-installer')?2160000:310000,()=>request.destroy(Error('CLI broker response timed out')));")
