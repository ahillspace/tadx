"""Offline contract checks for four prepared-copy site-consent cases."""

import importlib.util
import itertools
import json
import os
from pathlib import Path
import sys
import tempfile
import unittest
from unittest.mock import patch

ROOT = Path(__file__).resolve().parents[1]
suite_input = os.environ.get('TADX_G9_MUTATION_TEST_SUITE')
if not suite_input:
    raise RuntimeError('Set TADX_G9_MUTATION_TEST_SUITE to the locked prepared source or current source suite')
SUITE = Path(suite_input).resolve(strict=True)
for required in ('suite/exercises/P-mutation-set.json', 'fixtures/profiles/P-mutation-set.json',
                 'integration/audit_report.py'):
    if not (SUITE / required).is_file():
        raise RuntimeError('Mutation test suite input lacks required locked source: ' + required)
sys.path.insert(0, str(SUITE))


def load(name, path):
    spec = importlib.util.spec_from_file_location(name, path)
    module = importlib.util.module_from_spec(spec)
    spec.loader.exec_module(module)
    return module


patches = load('mutation_patches_tested', ROOT / 'mutation_patches.py')
candidate = load('session_profiles_tested', ROOT / 'source/integration/session_profiles.py')
bridge = load('docker_local_bridge_tested', ROOT / 'source/integration/docker_local_bridge.py')


PROFILE_TRANSITION = {
    'original': {
        'starting_state_recipe': [
            'verify the precise requested fields against the independent pre-task role manifest and baseline',
            'preserve target identity unless creation/clone explicitly requires a new identity',
            'all unrequested fields, unrelated objects and sentinel paths remain unchanged',
        ],
        'independent_observation_contract': 'local_configuration',
        'required_raw_fields': [
            'profiles', 'default_environment', 'default_workspace', 'saved_enabled',
            'effective', 'source', 'cache_max_concurrency',
        ],
        'implementation_binding': (
            'bridge.prepare + bridge.observe; shared readers and assertions are implemented; '
            'deployment-specific raw-field/asset/PTY/fault bindings must be supplied and qualified as documented.'
        ),
    },
    'prepared': {
        'starting_state_recipe': [
            'create disposable network-none CLI config with one canonical selected-site consent',
            'record contradictory legacy global and process values as ignored inputs',
            'freeze exact site identity, initial consent, and sentinel baseline',
        ],
        'independent_observation_contract': 'exact_site_mutation_consent',
        'required_raw_fields': [
            'site_mutations.server_url', 'site_mutations.site_content_url',
            'site_mutations.enabled', 'native.argv', 'native.stdout',
            'native.exit_status', 'protected_files.sha256',
        ],
        'implementation_binding': 'session_profiles.prepare + observe; session_broker.runtimeEvidence',
    },
}
SITE_INTENT = {'server_url': candidate.SERVER, 'site_content_url': candidate.SITE}
SITE_ASSERTIONS = {'policy-server-url', 'policy-site-content-url'}
SITE_REFERENCES = {'/fixture/expect/server_url', '/fixture/expect/site_content_url'}
SITE_FIELDS = {'/after/result/server_url', '/after/result/site_content_url'}


def definition_state(case, exercise, profile):
    if (case not in patches.CASES or exercise.get('id') != case or profile.get('id') != case
            or exercise.get('evaluator', {}).get('fixture_profile') != profile.get('base_profile')):
        raise RuntimeError('Mutation source has an unexpected exercise/profile identity')
    original = {'saved_enabled': True} if case.startswith('P-') else {}
    prepared = {**original, **SITE_INTENT}
    exercise_intent = exercise['evaluator'].get('intent')
    profile_intent = profile.get('intent')
    if exercise_intent == original and profile_intent == original:
        state = 'original'
    elif exercise_intent == prepared and profile_intent == prepared:
        state = 'prepared'
    else:
        raise RuntimeError('Mutation exercise/profile pair mixes original and prepared intent')

    if case.startswith('P-'):
        exercise_ids = [row.get('id') for row in exercise['evaluator'].get('expected_outcomes', [])]
        profile_ids = [row.get('id') for row in profile.get('expected_outcome_assertions', [])]
        oracle = exercise['evaluator'].get('oracle_requirements', [])
        bindings = profile.get('expected_value_bindings_required_before_launch', [])
        normalized = profile.get('normalized_assertion_fields', [])
        marker_lists = [(exercise_ids, SITE_ASSERTIONS), (profile_ids, SITE_ASSERTIONS),
                        (oracle, SITE_REFERENCES), (bindings, SITE_REFERENCES),
                        (normalized, SITE_FIELDS)]
        for values, markers in marker_lists:
            if state == 'prepared' and any(values.count(marker) != 1 for marker in markers):
                raise RuntimeError('Prepared mutation assertion or binding marker is missing or duplicated')
            if state == 'original' and any(marker in values for marker in markers):
                raise RuntimeError('Original mutation source contains prepared assertion or binding markers')

    actual_transition = {key: profile.get(key) for key in PROFILE_TRANSITION[state]}
    if actual_transition != PROFILE_TRANSITION[state]:
        raise RuntimeError('Mutation profile has incomplete original or prepared observation metadata')
    assertions = exercise['evaluator'].get('expected_outcomes', [])
    if (profile.get('expected_outcome_assertions') != assertions
            or profile.get('normalized_assertion_fields') != [row.get('actual') for row in assertions]
            or profile.get('expected_value_bindings_required_before_launch')
               != exercise['evaluator'].get('oracle_requirements')):
        raise RuntimeError('Mutation profile assertions or bindings differ from the exercise')
    return state


def qualified_cases(exercises, profiles):
    if set(exercises) != set(profiles):
        raise RuntimeError('Mutation source exercise/profile selection differs')
    selected = set(exercises)
    if not {'P-mutation-set', 'P-mutation-status'} <= selected or not selected <= set(patches.CASES):
        raise RuntimeError('Mutation source has an invalid selected case set')
    states = {definition_state(case, exercises[case], profiles[case]) for case in selected}
    if len(states) != 1:
        raise RuntimeError('Mutation source mixes prepared and original definitions')
    if states == {'prepared'}:
        return {case: (exercises[case], profiles[case]) for case in patches.CASES if case in selected}
    return patches.definitions(exercises, profiles)


def source_cases():
    selected = [case for case in patches.CASES
                if (SUITE / 'suite/exercises' / (case + '.json')).is_file()
                or (SUITE / 'fixtures/profiles' / (case + '.json')).is_file()]
    exercises = {case: json.loads((SUITE / 'suite/exercises' / (case + '.json')).read_text())
                 for case in selected}
    profiles = {case: json.loads((SUITE / 'fixtures/profiles' / (case + '.json')).read_text())
                for case in selected}
    return qualified_cases(exercises, profiles)


class MutationSiteFixtureTests(unittest.TestCase):
    def test_four_ids_prompts_profiles_and_assertions(self):
        pairs = source_cases()
        self.assertTrue({'P-mutation-set', 'P-mutation-status'} <= set(pairs))
        self.assertTrue(set(pairs) <= set(patches.CASES))
        selected = {name for name in patches.CASES if name.startswith('P-')}
        self.assertEqual(set(qualified_cases(
            {name: json.loads((SUITE / 'suite/exercises' / (name + '.json')).read_text()) for name in selected},
            {name: json.loads((SUITE / 'fixtures/profiles' / (name + '.json')).read_text()) for name in selected})), selected)
        self.assertEqual(qualified_cases({name: pair[0] for name, pair in pairs.items()},
                                         {name: pair[1] for name, pair in pairs.items()}), pairs)
        for case, (exercise, profile) in pairs.items():
            self.assertEqual(exercise['id'], case)
            self.assertEqual(profile['id'], case)
            self.assertEqual(candidate.contract(exercise)['profile'], profile['base_profile'])
            self.assertEqual(candidate.task_instructions(exercise, {})['exercise']['agent']['prompt'], patches.PROMPTS[case])
            self.assertEqual(exercise['evaluator']['intent']['server_url'], candidate.SERVER)
            self.assertEqual(exercise['evaluator']['intent']['site_content_url'], candidate.SITE)
            if case.startswith('P-'):
                ids = {row['id'] for row in exercise['evaluator']['expected_outcomes']}
                self.assertTrue({'policy-effective', 'policy-source', 'policy-server-url',
                                 'policy-site-content-url'} <= ids)

    def test_two_p_pairs_exhaustive_original_prepared_truth_table(self):
        names = ('P-mutation-set', 'P-mutation-status')
        originals_e = {name: json.loads((SUITE / 'suite/exercises' / (name + '.json')).read_text())
                       for name in names}
        originals_p = {name: json.loads((SUITE / 'fixtures/profiles' / (name + '.json')).read_text())
                       for name in names}
        if all(definition_state(name, originals_e[name], originals_p[name]) == 'prepared' for name in names):
            self.skipTest('Truth table requires explicit original-source input; prepared-source acceptance runs separately')
        prepared = patches.definitions(originals_e, originals_p)
        prepared_e = {name: prepared[name][0] for name in names}
        prepared_p = {name: prepared[name][1] for name in names}
        accepted = 0
        rejected = 0
        for states in itertools.product((False, True), repeat=4):
            exercise_set = {name: (prepared_e if states[index] else originals_e)[name]
                            for index, name in enumerate(names)}
            profile_set = {name: (prepared_p if states[index + 2] else originals_p)[name]
                           for index, name in enumerate(names)}
            if states in ((False,) * 4, (True,) * 4):
                self.assertEqual(set(qualified_cases(exercise_set, profile_set)), set(names))
                accepted += 1
            else:
                with self.assertRaises(RuntimeError):
                    qualified_cases(exercise_set, profile_set)
                rejected += 1
        self.assertEqual((accepted, rejected), (2, 14))

    def test_prepared_profile_markers_are_required(self):
        pairs = source_cases()
        exercises = {name: pair[0] for name, pair in pairs.items()}
        profiles = {name: json.loads(json.dumps(pair[1])) for name, pair in pairs.items()}
        profiles['P-mutation-set']['expected_value_bindings_required_before_launch'].remove(
            '/fixture/expect/server_url')
        with self.assertRaisesRegex(RuntimeError, 'marker'):
            qualified_cases(exercises, profiles)

    def test_each_prepared_profile_transition_field_rejects_original_value(self):
        pairs = source_cases()
        exercises = {name: pair[0] for name, pair in pairs.items()}
        for field in PROFILE_TRANSITION['prepared']:
            with self.subTest(field=field):
                profiles = {name: json.loads(json.dumps(pair[1])) for name, pair in pairs.items()}
                profiles['P-mutation-set'][field] = PROFILE_TRANSITION['original'][field]
                with self.assertRaisesRegex(RuntimeError, 'observation metadata'):
                    qualified_cases(exercises, profiles)
        for field in ('expected_outcome_assertions', 'normalized_assertion_fields'):
            with self.subTest(field=field):
                profiles = {name: json.loads(json.dumps(pair[1])) for name, pair in pairs.items()}
                profiles['P-mutation-set'][field] = []
                with self.assertRaises(RuntimeError):
                    qualified_cases(exercises, profiles)

    def test_site_consent_ignores_legacy_values(self):
        self.assertEqual(candidate.policy_facts(False)['effective'], False)
        self.assertEqual(candidate.policy_facts(True)['source'], 'saved_site_setting')
        config = {'site_mutations': [candidate.site_setting(False), candidate.unrelated_setting()],
                  'mutations_enabled': True}
        self.assertFalse(candidate.selected_consent(config))
        for corrupted in ([], [candidate.site_setting(False)],
                          [candidate.site_setting(False), {**candidate.unrelated_setting(), 'enabled': False}],
                          [{**candidate.site_setting(False), 'site_content_url': 'other'}, candidate.unrelated_setting()]):
            with self.assertRaises(ValueError):
                candidate.selected_consent({'site_mutations': corrupted})

    def test_native_full_status_and_set_require_exact_identity_and_persistence(self):
        site = {'environment': candidate.ENVIRONMENT, 'server_url': candidate.SERVER,
                'site_content_url': candidate.SITE, 'enabled': False, 'source': 'saved_site_setting'}
        self.assertFalse(candidate.mutation_facts({'sites': [site]}, 'mutation.status')['effective'])
        receipt = {**site, 'saved_enabled': False, 'persisted': True,
                   'scope': 'site', 'source_setting': 'site_mutations'}
        self.assertFalse(candidate.mutation_facts(receipt, 'mutation.set')['saved_enabled'])
        for altered in ({**site, 'site_content_url': 'other'}, {**site, 'source': 'process_environment'}):
            with self.assertRaises(ValueError):
                candidate.mutation_facts({'sites': [altered]}, 'mutation.status')
        with self.assertRaises(ValueError):
            candidate.mutation_facts({**receipt, 'persisted': False}, 'mutation.set')

    def test_bridge_uses_legacy_value_only_as_ignored_adversarial_input(self):
        seed = {'environments': {candidate.ENVIRONMENT: {'url': candidate.SERVER,
                'site_content_url': candidate.SITE}, candidate.OTHER_ENVIRONMENT: {
                'url': candidate.SERVER, 'site_content_url': candidate.OTHER_SITE}},
                'site_mutations': [candidate.site_setting(False), candidate.unrelated_setting()],
                'mutations_enabled': True}
        state = {'broker_guard': {'mode': 'session', 'environment': candidate.ENVIRONMENT,
                 'server_url': candidate.SERVER, 'site_content_url': candidate.SITE,
                 'unrelated_environment': candidate.OTHER_ENVIRONMENT,
                 'unrelated_site_content_url': candidate.OTHER_SITE,
                 'initial_site_consent': False, 'process_policy': '1'},
                 'mutation_policy': None, 'config_seed': seed}
        self.assertEqual(bridge._policy_args({}, state), ['--env', 'TADX_ENABLE_MUTATIONS=1'])
        evidence = bridge._policy_evidence({}, state, {'Id': 'synthetic',
            'Config': {'Env': ['TADX_ENABLE_MUTATIONS=1']}})
        self.assertIs(evidence['effective'], False)
        self.assertEqual(evidence['source'], 'saved_site_setting')
        state['mutation_policy'] = 'enabled'
        with self.assertRaises(Exception):
            bridge._policy_args({}, state)

    def test_prepare_exact_disposable_config_and_variant_reversal(self):
        pairs = source_cases()
        with tempfile.TemporaryDirectory() as temporary:
            root = Path(temporary)
            registry = [{'id': action, 'command_path': words,
                         'remote_mutation': action == 'project.create'}
                        for action, words in candidate.PATHS.items()]
            (root / 'registry.json').write_text(json.dumps(registry))
            for case, (exercise, _) in pairs.items():
                private = root / case
                private.mkdir()
                work = private / 'agent-public'
                work.mkdir()
                req = {'exercise': exercise, 'private_case_dir': str(private), 'case_id': case,
                       'run_constraints': {'mutation_setting_changes_authorized': True,
                         'mutation_setting_change_scope': 'disposable_test_containers'}}
                prepared = candidate.prepare(req, {'root': str(root)}, work)
                guard = prepared['broker_guard']
                seed = prepared['config_seed']
                self.assertEqual(seed['environments'][candidate.ENVIRONMENT]['url'], candidate.SERVER)
                self.assertEqual(seed['environments'][candidate.ENVIRONMENT]['site_content_url'], candidate.SITE)
                self.assertEqual(len(seed['site_mutations']), 2)
                self.assertEqual(seed['site_mutations'][1], candidate.unrelated_setting())
                self.assertEqual(seed['environments'][candidate.OTHER_ENVIRONMENT]['site_content_url'], candidate.OTHER_SITE)
                self.assertEqual(guard['initial_site_consent'], candidate.selected_consent(seed))
                self.assertIsNone(prepared.get('mutation_policy'))
                self.assertEqual(candidate.selected_consent(prepared['config_seed']),
                    case in ('P-mutation-status', 'V-mutation-env-precedence'))
                baseline = json.loads((private / candidate.BASELINE).read_text())
                self.assertEqual(candidate.selected_consent(baseline['desired_config']),
                    case in ('P-mutation-set', 'P-mutation-status'))
                self.assertEqual(baseline['desired_config']['site_mutations'][1], candidate.unrelated_setting())

    def test_observer_requires_native_full_exact_site_and_confirmed_config(self):
        exercise = source_cases()['P-mutation-set'][0]
        with tempfile.TemporaryDirectory() as temporary:
            root = Path(temporary)
            registry = [{'id': action, 'command_path': words,
                         'remote_mutation': action == 'project.create'}
                        for action, words in candidate.PATHS.items()]
            (root / 'registry.json').write_text(json.dumps(registry))
            private = root / 'private'
            private.mkdir()
            work = private / 'agent-public'
            work.mkdir()
            req = {'exercise': exercise, 'private_case_dir': str(private), 'case_id': exercise['id'],
                   'run_constraints': {'mutation_setting_changes_authorized': True,
                     'mutation_setting_change_scope': 'disposable_test_containers'}}
            prepared = candidate.prepare(req, {'root': str(root)}, work)
            baseline = json.loads((private / candidate.BASELINE).read_text())
            candidate.save(private / 'delivery-evidence.json', {'cli_session_runtime': {
                'status': 'verified', 'credential_material_present': False, 'network_none': True,
                'process_policy': prepared['broker_guard']['process_policy'],
                'server_url': candidate.SERVER, 'site_content_url': candidate.SITE,
                'initial_site_consent': False}})
            result = {'environment': candidate.ENVIRONMENT, 'server_url': candidate.SERVER,
                      'site_content_url': candidate.SITE, 'enabled': True,
                      'saved_enabled': True, 'persisted': True, 'scope': 'site',
                      'source': 'saved_site_setting', 'source_setting': 'site_mutations'}
            command = {'capability': 'mutation.set', 'id': 'synthetic-command', 'executed': True,
                       'evidence': 'synthetic-command.json', 'exit_status': 0,
                       'argv': ['tadx', 'mutation', 'set', '--environment', candidate.ENVIRONMENT,
                                '--enabled=true', '--json', '--full'],
                       'flags': {'environment': candidate.ENVIRONMENT, 'enabled': True,
                                 'json': True, 'full': True}, 'stdout': json.dumps(result)}
            with patch.object(candidate, '_audit'):
                after = candidate.observe(req, {}, baseline['desired_config'], work, [command])
                self.assertTrue(after['native_command_verified'])
                self.assertTrue(after['result']['effective'])
                self.assertEqual(after['unowned_changes'], [])
                command['stdout'] = json.dumps({**result, 'site_content_url': 'other'})
                after = candidate.observe(req, {}, baseline['desired_config'], work, [command])
                self.assertFalse(after['native_command_verified'])
                self.assertTrue(after['result']['effective'])
                command['stdout'] = json.dumps(result)
                for changed in ('drop-unrelated-setting', 'alter-unrelated-setting', 'drop-unrelated-environment'):
                    config = json.loads(json.dumps(baseline['desired_config']))
                    if changed == 'drop-unrelated-setting':
                        config['site_mutations'].pop()
                    elif changed == 'alter-unrelated-setting':
                        config['site_mutations'][1]['enabled'] = False
                    else:
                        del config['environments'][candidate.OTHER_ENVIRONMENT]
                    after = candidate.observe(req, {}, config, work, [command])
                    self.assertIn('config:unrequested-or-missing-change', after['unowned_changes'])

    def test_refusal_oracle_does_not_fabricate_a_request_count(self):
        pairs = source_cases()
        if 'V-mutation-no-setting-authorization' not in pairs:
            self.skipTest('Optional V refusal case is not selected in this source input')
        exercise = pairs['V-mutation-no-setting-authorization'][0]
        with tempfile.TemporaryDirectory() as temporary:
            root = Path(temporary)
            (root / 'registry.json').write_text(json.dumps([
                {'id': action, 'command_path': words, 'remote_mutation': action == 'project.create'}
                for action, words in candidate.PATHS.items()]))
            private = root / 'private'
            private.mkdir()
            work = private / 'agent-public'
            work.mkdir()
            req = {'exercise': exercise, 'private_case_dir': str(private), 'case_id': exercise['id'],
                   'run_constraints': {'mutation_setting_changes_authorized': True,
                     'mutation_setting_change_scope': 'disposable_test_containers'}}
            prepared = candidate.prepare(req, {'root': str(root)}, work)
            self.assertNotIn('action_requests', prepared['fixture']['expect']['result'])
            baseline = json.loads((private / candidate.BASELINE).read_text())
            candidate.save(private / 'delivery-evidence.json', {'cli_session_runtime': {
                'status': 'verified', 'credential_material_present': False, 'network_none': True,
                'process_policy': '1', 'server_url': candidate.SERVER,
                'site_content_url': candidate.SITE, 'initial_site_consent': False}})
            with patch.object(candidate, '_audit'):
                after = candidate.observe(req, {}, baseline['config'], work, [])
            self.assertNotIn('action_requests', after['result'])
            self.assertFalse(after['result']['refused'])
            self.assertFalse(after['native_command_verified'])


if __name__ == '__main__':
    unittest.main()
