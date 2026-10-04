"""A CLI 404 cannot settle a reset without a fresh independent exact GET."""

import io
import unittest
from unittest.mock import patch
import urllib.error
import urllib.request

from integration import operator_reset
from tools import reset_site


class ResetAbsenceTests(unittest.TestCase):
    def setUp(self):
        self.remote_present = True
        self.calls = []
        self.events = []

        class CLI:
            def run(cli, command):
                self.calls.append(list(command))
                if command[:3] == ['content', 'workbook', 'inspect']:
                    raise reset_site.CLIError(command, {'error': {'upstream_status': 404}})
                raise AssertionError('A deletion must not run after a CLI 404')

        self.manager = reset_site.ResetSite.__new__(reset_site.ResetSite)
        self.manager.cli = CLI()
        self.manager.env = 'test-env'
        self.manager.event = lambda event, **fields: self.events.append((event, fields))
        self.manager.confirm_absence = lambda kind, luid: not self.remote_present

    def test_cli_404_with_present_exact_resource_blocks_delete(self):
        with self.assertRaisesRegex(reset_site.ResetError, 'Independent exact GET 404'):
            self.manager.delete_exact('workbook', 'workbook-original', {'phase': 'pending'})
        self.assertEqual(self.events, [])
        self.assertEqual([call[:3] for call in self.calls], [['content', 'workbook', 'inspect']])

    def test_cli_404_and_independent_404_confirms_absence(self):
        self.remote_present = False
        holder = {'phase': 'pending'}
        self.manager.delete_exact('workbook', 'workbook-original', holder)
        self.assertEqual(holder['phase'], 'deleted')
        self.assertEqual(self.events[0][0], 'delete_confirmed')
        self.assertTrue(self.events[0][1]['already_absent'])
        self.assertEqual([call[:3] for call in self.calls], [['content', 'workbook', 'inspect']])


class ObserverAbsenceTests(unittest.TestCase):
    def setUp(self):
        self.site = {'environment': 'test-env', 'url': 'https://tableau.example.test',
                     'content_url': 'test-site', 'id': 'site-1'}
        self.deployment = {'sites': {'A': self.site}, 'targets': {'A': {}},
                           'execution': {'mode': 'serial', 'concurrency': 1}}
        self.binding = {'environment': 'test-env', 'server_url': 'https://tableau.example.test/',
                        'site_content_url': 'test-site', 'site_luid': 'site-1'}
        self.opens = 0
        self.status = 404
        self.detail = None
        self.path_override = None
        self.signed_out = 0

    def check(self):
        class Reader:
            def __init__(reader, owner, record):
                reader.owner, reader.record = owner, record

            def path(reader, suffix):
                return '/api/3.29/sites/site-1/' + suffix

            def detail_if_present(reader, kind, luid):
                reader.record({'method': 'GET',
                               'path': reader.owner.path_override or reader.path(kind + 's/' + luid),
                               'status': reader.owner.status})
                if reader.owner.status == 403:
                    raise RuntimeError('observer denied')
                return reader.owner.detail

            def signout(reader):
                reader.owner.signed_out += 1

        def opened(binding, record, role):
            self.assertEqual(role, 'observer')
            self.opens += 1
            return Reader(self, record), {'site_id': 'site-1'}

        with patch('integration.operator_binding.load_operator_binding', return_value=self.binding), \
                patch('integration.content_profiles._open', side_effect=opened):
            return operator_reset._confirmed_absence(self.deployment, 'A', 'workbook', 'exact-id')

    def test_exact_404_requires_fresh_get_each_time(self):
        self.assertIs(self.check(), True)
        self.assertIs(self.check(), True)
        self.assertEqual((self.opens, self.signed_out), (2, 2))

    def test_present_detail_and_wrong_route_do_not_confirm_absence(self):
        self.status, self.detail = 200, {'id': 'exact-id'}
        with self.assertRaisesRegex(operator_reset.OperatorResetError, 'did not confirm'):
            self.check()
        self.status, self.detail = 404, None
        self.path_override = '/api/3.29/sites/site-1/workbooks/other-id'
        with self.assertRaisesRegex(operator_reset.OperatorResetError, 'did not confirm'):
            self.check()
        self.assertEqual((self.opens, self.signed_out), (2, 2))

    def test_denied_observer_and_wrong_site_fail_closed(self):
        self.status = 403
        with self.assertRaisesRegex(RuntimeError, 'observer denied'):
            self.check()
        self.binding['site_luid'] = 'other-site'
        with self.assertRaisesRegex(operator_reset.OperatorResetError, 'differs'):
            self.check()

    def test_missing_observer_and_authentication_failure_fail_closed(self):
        with patch('integration.operator_binding.load_operator_binding',
                   side_effect=operator_reset.OperatorResetError('observer unavailable')):
            with self.assertRaisesRegex(operator_reset.OperatorResetError, 'observer unavailable'):
                operator_reset._confirmed_absence(self.deployment, 'A', 'workbook', 'exact-id')
        with patch('integration.operator_binding.load_operator_binding', return_value=self.binding), \
                patch('integration.content_profiles._open', side_effect=RuntimeError('observer sign-in failed')):
            with self.assertRaisesRegex(RuntimeError, 'observer sign-in failed'):
                operator_reset._confirmed_absence(self.deployment, 'A', 'workbook', 'exact-id')

    def test_real_reader_get_404_confirms_only_exact_scope(self):
        self.binding.update(api_version='3.29', observer={
            'pat_name': 'offline-observer', 'pat_secret': 'offline-secret'})
        calls = []

        class Response:
            status = 200
            headers = {}

            def __init__(response, body):
                response.body = body

            def __enter__(response):
                return response

            def __exit__(response, *_):
                return False

            def read(response, *_):
                return response.body

        class Opener:
            def open(opener, request, timeout):
                calls.append((request.get_method(), request.full_url))
                if request.full_url.endswith('/auth/signin'):
                    return Response(b'<tsResponse><credentials token="offline-token">'
                                    b'<site id="site-1"/><user id="user-1"/>'
                                    b'</credentials></tsResponse>')
                if request.full_url.endswith('/auth/signout'):
                    return Response(b'<tsResponse/>')
                if request.full_url.endswith('/sites/site-1/workbooks/exact-id'):
                    raise urllib.error.HTTPError(request.full_url, 404, 'not found', {},
                                                 io.BytesIO(b'<tsResponse/>'))
                raise AssertionError('Unexpected remote path')

        with patch('integration.operator_binding.load_operator_binding', return_value=self.binding), \
                patch.object(urllib.request, 'build_opener', return_value=Opener()):
            self.assertIs(operator_reset._confirmed_absence(
                self.deployment, 'A', 'workbook', 'exact-id'), True)
        self.assertEqual(calls, [
            ('POST', 'https://tableau.example.test/api/3.29/auth/signin'),
            ('GET', 'https://tableau.example.test/api/3.29/sites/site-1/workbooks/exact-id'),
            ('POST', 'https://tableau.example.test/api/3.29/auth/signout'),
        ])


if __name__ == '__main__':
    unittest.main()
