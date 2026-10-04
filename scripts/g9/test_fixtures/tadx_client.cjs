#!/usr/bin/env node
const fs=require('node:fs'),http=require('node:http');
const DIRECTORY='/run/tadx-broker',SOCKET=DIRECTORY+'/invoke.sock';

function endpoint(){
  if(fs.existsSync(DIRECTORY)){
    const directory=fs.lstatSync(DIRECTORY),socket=fs.lstatSync(SOCKET);
    if(!directory.isDirectory()||directory.isSymbolicLink()||directory.uid!==0||(directory.mode&0o022)!==0
       ||!socket.isSocket()||socket.uid!==0||socket.isSymbolicLink())throw Error('CLI socket ownership is not trusted');
    return {socketPath:SOCKET};
  }
  return {hostname:'127.0.0.1',port:8765};
}
function invoke(argv){
  return new Promise((resolve,reject)=>{
    let destination;try{destination=endpoint();}catch(error){reject(error);return;}
    const request=http.request({...destination,path:'/invoke',method:'POST',headers:{'content-type':'application/json'}},response=>{
      if(response.statusCode!==200){response.resume();reject(Error('CLI broker rejected malformed invocation'));return;}
      let bytes=0,chunks=[];
      response.on('data',chunk=>{bytes+=chunk.length;if(bytes>33554432){request.destroy(Error('CLI response exceeds its bound'));return;}chunks.push(chunk);});
      response.on('error',reject);
      response.on('end',()=>{try{
        const result=JSON.parse(Buffer.concat(chunks).toString('utf8'));
        if(typeof result.stdout!=='string'||typeof result.stderr!=='string'||!Number.isInteger(result.exit_status)
           ||result.exit_status<0||result.exit_status>255)throw Error('CLI broker response is invalid');
        resolve(result);
      }catch(error){reject(error);}});
    });
    request.on('error',reject);request.setTimeout(310000,()=>request.destroy(Error('CLI broker response timed out')));
    request.end(JSON.stringify({argv,cwd:process.cwd(),helper:process.argv[1]?.endsWith('tadx-bench-installer')?'installer':undefined}));
  });
}
if(require.main===module)invoke(process.argv.slice(2))
  .then(result=>{process.stdout.write(result.stdout);process.stderr.write(result.stderr);process.exitCode=result.exit_status;})
  .catch(error=>{process.stderr.write(String(error)+'\n');process.exitCode=125;});
module.exports={invoke,endpoint};
