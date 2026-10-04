import copy
from pathlib import Path
import tempfile
import unittest
from unittest.mock import patch

from bench.core import Blocked, load
from integration import session_profiles as adapter

ROOT = Path(__file__).resolve().parents[1]


class SessionProfilesTests(unittest.TestCase):
    def test_exact_six_contracts_and_no_login_or_fake_keystore(self):
        for name in ('P-auth-status','V-auth-status-offline','P-mutation-status','V-mutation-env-precedence','V-mutation-no-setting-authorization','P-mutation-set'):
            exercise=load(ROOT/'suite/exercises'/(name+'.json'))
            self.assertTrue(adapter.supports(exercise),name)
            exercise['evaluator']['expected_outcomes'][0]['expected']='changed'
            self.assertFalse(adapter.supports(exercise))
        self.assertFalse(adapter.supports(load(ROOT/'suite/exercises/P-auth-login.json')))

    def test_setting_changes_require_exact_disposable_permission(self):
        for constraints in ({}, {'mutation_setting_changes_authorized': True, 'mutation_setting_change_scope': 'host'}):
            with self.assertRaises(Blocked): adapter.authorized({'run_constraints': constraints})
        adapter.authorized({'run_constraints': {'mutation_setting_changes_authorized': True,
                                             'mutation_setting_change_scope': 'disposable_test_containers'}})

    def test_exact_site_policy_ignores_legacy_override(self):
        self.assertEqual(adapter.policy_facts(False),{'saved_enabled':False,'effective':False,
            'source':'saved_site_setting','server_url':adapter.SERVER,'site_content_url':adapter.SITE})
        self.assertTrue(adapter.policy_facts(True)['effective'])
        with self.assertRaises(Blocked): adapter.policy_facts('true')
        with self.assertRaises(ValueError): adapter.selected_consent({'site_mutations':[]})

    def test_native_auth_never_accepts_ready_or_unknown_source_for_empty_fixture(self):
        data={'status':'incomplete','environment':adapter.ENVIRONMENT,'server_url':'https://example.test',
              'site_content_url':'session-fixture','credential_source':'none','pat_name_present':False,
              'pat_secret_present':False,'stored_credential_reference_present':False}
        self.assertEqual(adapter.auth_facts(data),{'ready':False,'source':'none','network_requests':0})
        data['credential_source']='os_credential_store'
        with self.assertRaises(ValueError): adapter.auth_facts(data)

    def test_prompt_ownership_requires_exact_reviewed_hash(self):
        exercise=load(ROOT/'suite/exercises/P-auth-status.json')
        result=adapter.task_instructions(exercise,{})
        self.assertEqual(result['status'],'separated');self.assertNotIn('deliverables/result.json',result['exercise']['agent']['prompt'])
        self.assertIn('deliverables/result.json',exercise['agent']['prompt'])
        exercise['agent']['prompt']+=' Save another mandatory report.'
        with self.assertRaises(Blocked):adapter.task_instructions(exercise,{})

    def test_auth_decoder_accepts_explicit_readiness_but_not_ambiguous_or_failed_data(self):
        self.assertEqual(adapter._auth_document('{"status":"incomplete"}')['status'],'incomplete')
        for text in ('{"status":"incomplete","status":"ready"}','{"status":"incomplete","error":{"id":"failure"}}'):
            with self.assertRaises(ValueError):adapter._auth_document(text)


if __name__=='__main__': unittest.main()
