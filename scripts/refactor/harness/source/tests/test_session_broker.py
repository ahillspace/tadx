import json
from pathlib import Path
import shutil
import subprocess
import unittest
from integration import session_profiles as profiles

ROOT=Path(__file__).resolve().parents[1]
NODE=shutil.which('node')
SCRIPT="const fs=require('fs'),h=require('./integration/session_broker.cjs'),v=JSON.parse(fs.readFileSync(0,'utf8'));process.stdout.write(JSON.stringify(h.allowed(v.parsed,v.cap,v.state)));"


@unittest.skipUnless(NODE,'Node is required for session guard tests')
class SessionBrokerTests(unittest.TestCase):
    def state(self,name):
        actions=profiles.VARIANTS.get(name,[name])
        return {'guardPresent':True,'baselinePresent':True,'baselineUnchanged':True,'networkNone':True,
            'mutationDisabled':False,
            'registry':[{'id':key,'command_path':path,'remote_mutation':key=='project.create'} for key,path in profiles.PATHS.items()],
            'guard':{'mode':'session','profile':name,'environment':profiles.ENVIRONMENT,
                     'server_url':profiles.SERVER,'site_content_url':profiles.SITE,
                     'unrelated_environment':profiles.OTHER_ENVIRONMENT,
                     'unrelated_site_content_url':profiles.OTHER_SITE,
                     'initial_site_consent':None if name in ('auth.status','auth-status-offline') else name in ('mutation.status','mutation-env-precedence'),
                     'process_policy':'1',
                     'commands':profiles._rules({'profile':name,'actions':actions})}}

    def check(self,name,cap,flags,words=None,state=None):
        body={'state':state or self.state(name),'cap':cap,'parsed':{'words':words or profiles.PATHS[cap],'flags':flags,'errors':[]}}
        result=subprocess.run([NODE,'-e',SCRIPT],input=json.dumps(body),text=True,capture_output=True,cwd=ROOT)
        self.assertEqual(result.returncode,0,result.stderr)
        return json.loads(result.stdout)

    def test_set_is_allowed_only_for_exact_authorized_profile_and_value(self):
        flags={'environment':profiles.ENVIRONMENT,'json':True,'full':True}
        self.assertTrue(self.check('mutation.set','mutation.set',{**flags,'enabled':True}))
        self.assertFalse(self.check('mutation.set','mutation.set',{**flags,'enabled':False}))
        self.assertFalse(self.check('mutation.status','mutation.set',{**flags,'enabled':True}))
        self.assertTrue(self.check('mutation-env-precedence','mutation.set',{**flags,'enabled':False}))
        state=self.state('mutation-env-precedence');state['baselineUnchanged']=False
        self.assertTrue(self.check('mutation-env-precedence','mutation.status',flags,state=state))

    def test_no_permission_case_allows_only_disabled_offline_exact_project(self):
        flags={'environment':profiles.ENVIRONMENT,'name':profiles.PROJECT}
        self.assertTrue(self.check('mutation-no-setting-authorization','project.create',flags))
        for key in ('networkNone','baselinePresent'):
            state=self.state('mutation-no-setting-authorization');state[key]=False
            self.assertFalse(self.check('mutation-no-setting-authorization','project.create',flags,state=state))
        state=self.state('mutation-no-setting-authorization');state['guard']['initial_site_consent']=True
        self.assertFalse(self.check('mutation-no-setting-authorization','project.create',flags,state=state))
        self.assertFalse(self.check('mutation-no-setting-authorization','project.create',{**flags,'preview':True}))
        self.assertFalse(self.check('mutation-no-setting-authorization','project.create',{**flags,'name':'other'}))
        self.assertFalse(self.check('mutation-no-setting-authorization','mutation.set',{'enabled':True}))

    def test_auth_status_cannot_turn_into_login_or_remote_check_or_secret_reference(self):
        self.assertTrue(self.check('auth.status','auth.status',{}))
        for flags in ({'environment':'other'},{'check':True},{'config':'/tmp/config'},{'pat-secret-env':'SECRET'}):
            self.assertFalse(self.check('auth.status','auth.status',flags))
        self.assertFalse(self.check('auth.status','auth.status',{},words=['auth','login']))

    def test_runtime_evidence_refuses_material_and_never_returns_values(self):
        script=r'''
const fs=require('fs'),h=require('./integration/session_broker.cjs'),v=JSON.parse(fs.readFileSync(0,'utf8'));
fs.readFileSync=()=>JSON.stringify(v.config);fs.existsSync=()=>v.filePresent===true;
process.env.TADX_ENABLE_MUTATIONS='1';
delete process.env.BENCH_SESSION_MISSING_NAME;delete process.env.BENCH_SESSION_MISSING_SECRET;
if(v.material)process.env.BENCH_SESSION_MISSING_SECRET='unit-test-sensitive-value';
try{process.stdout.write(JSON.stringify(h.runtimeEvidence(v.state)));}
catch(e){process.stdout.write(JSON.stringify({error:String(e)}));}
'''
        config={'default_environment':profiles.ENVIRONMENT,'environments':{
            profiles.ENVIRONMENT:{'url':profiles.SERVER,'site_content_url':profiles.SITE,
                'auth':{'type':'pat','pat_name_env':profiles.PREFIX+'_NAME','pat_secret_env':profiles.PREFIX+'_SECRET'}},
            profiles.OTHER_ENVIRONMENT:{'url':profiles.SERVER,'site_content_url':profiles.OTHER_SITE,
                'auth':{'type':'pat','pat_name_env':profiles.OTHER_PREFIX+'_NAME',
                        'pat_secret_env':profiles.OTHER_PREFIX+'_SECRET'}}}}
        for options,valid in (({},True),({'material':True},False),({'filePresent':True},False)):
            result=subprocess.run([NODE,'-e',script],input=json.dumps({'state':self.state('auth.status'),'config':config,**options}),
                                  text=True,capture_output=True,cwd=ROOT)
            self.assertEqual(result.returncode,0,result.stderr);self.assertNotIn('unit-test-sensitive-value',result.stdout)
            self.assertEqual(json.loads(result.stdout).get('status')=='verified',valid)


if __name__=='__main__':unittest.main()
