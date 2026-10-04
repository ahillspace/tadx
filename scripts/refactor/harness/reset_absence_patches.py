"""Bind copied reset absence decisions to an independent exact REST read."""


def patch_reset_site(source, replace_once):
    source = replace_once(
        source,
        "    def __init__(self, manifest_path, cli):\n"
        "        self.path = Path(manifest_path).resolve()\n",
        "    def __init__(self, manifest_path, cli, confirm_absence=None):\n"
        "        self.confirm_absence = confirm_absence\n"
        "        self.path = Path(manifest_path).resolve()\n",
    )
    source = replace_once(
        source,
        "            if exc.result.get(\"error\", {}).get(\"upstream_status\") == 404:\n"
        "                return None\n",
        "            if exc.result.get(\"error\", {}).get(\"upstream_status\") == 404:\n"
        "                if self.confirm_absence is None or self.confirm_absence(kind, luid) is not True:\n"
        "                    raise ResetError(\"Independent exact GET 404 is required before confirming absence.\") from exc\n"
        "                return None\n",
    )
    return replace_once(source, """        retry_count = holder.get("unknown_retry_count", 0)
        last_uncertain = next((event for event in reversed(self.manifest.get("journal", []))
                               if event.get("event") == "publish_uncertain"
                               and event.get("generation") == self.manifest.get("generation")
                               and event.get("kind") == kind), None)
        not_attempted = ((last_uncertain or {}).get("evidence") or {}).get("id") == "mutation.disabled" \\
            and ((last_uncertain or {}).get("evidence") or {}).get("outcome") == "not_attempted"
        if len(candidates) == 0 and (retry_count < 1 or not_attempted):
            # A transport or container failure can leave a publish outcome
            # unknown even though a complete remote inventory proves that no
            # resource with the exact baseline name exists.  Permit one
            # durable retry only after the deletion boundary is confirmed and
            # the absence is independently observed twice.  A later unknown
            # outcome remains quarantined, and an acknowledged identity is
            # never replayed.
            import time
            time.sleep(2)
            second = self.matching(kind, resource["state"]["name"], resource["state"]["project_luid"])
            second = [item for item in second if item.get("luid") not in old_ids]
            deleted = any(event.get("event") == "delete_confirmed"
                          and event.get("kind") == kind
                          and event.get("id") == holder.get("old_id")
                          and event.get("generation") == self.manifest.get("generation")
                          for event in self.manifest.get("journal", []))
            if len(second) == 1 and second[0].get("luid"):
                self.confirm_publish(kind, second[0]["luid"], holder, reconciled=True)
                return
            if not second and deleted:
                holder["unknown_retry_count"] = 1
                holder["phase"] = "deleted"
                self.event("publish_retry_authorized", kind=kind,
                           reason="complete exact-name inventory absent after uncertain publish")
                self.publish(kind, holder)
                return
""", "")


def patch_operator_reset(source, replace_once):
    source = replace_once(
        source,
        "import json\nfrom pathlib import Path\n",
        "import json\nfrom pathlib import Path\nimport urllib.parse\n",
    )
    source = replace_once(
        source,
        "OPERATIONS = ('capture', 'plan', 'reset', 'verify')\n\n\n",
        "OPERATIONS = ('capture', 'plan', 'reset', 'verify')\n\n\n"
        "def _confirmed_absence_with_binding(deployment, site, kind, luid, binding):\n"
        "    \"\"\"Confirm a CLI 404 with one fresh, authenticated exact resource GET.\"\"\"\n"
        "    if kind not in reset_site.KINDS or not isinstance(luid, str) or not luid:\n"
        "        raise OperatorResetError('Independent absence needs an exact recorded resource.')\n"
        "    # Import after module initialization: content profiles use this reset owner.\n"
        "    from integration import content_profiles\n"
        "    selected, _ = _site(deployment, site)\n"
        "    if (binding['environment'] != selected['environment']\n"
        "            or binding['server_url'].rstrip('/') != selected['url'].rstrip('/')\n"
        "            or binding['site_content_url'] != selected['content_url']\n"
        "            or binding['site_luid'] != selected.get('id')):\n"
        "        raise OperatorResetError('Independent observer target differs from the selected reset site.')\n"
        "    observed = []\n"
        "    def record(event):\n"
        "        observed.append((event.get('method'), event.get('path'), event.get('status')))\n"
        "    reader, _ = content_profiles._open(binding, record, role='observer')\n"
        "    try:\n"
        "        route = reader.path(kind + 's/' + urllib.parse.quote(luid, safe=''))\n"
        "        detail = reader.detail_if_present(kind, luid)\n"
        "    finally:\n"
        "        reader.signout()\n"
        "    if detail is not None or observed.count(('GET', route, 404)) != 1:\n"
        "        raise OperatorResetError('Independent exact GET did not confirm resource absence.')\n"
        "    return True\n\n\n"
        "def _confirmed_absence(deployment, site, kind, luid):\n"
        "    from integration.operator_binding import load_operator_binding\n"
        "    request = {'deployment': {'operator_settings': deployment, 'operator_site': site}}\n"
        "    binding = load_operator_binding(request)\n"
        "    return _confirmed_absence_with_binding(deployment, site, kind, luid, binding)\n\n\n",
    )
    source = replace_once(
        source,
        "            manager = reset_site.ResetSite(manifest_path, cli)\n"
        "            _scope(manager, deployment, site)\n"
        "            candidates = _save_candidates(deployment, site, base, deployment_path, settings_path, manager)\n",
        "            manager = reset_site.ResetSite(\n"
        "                manifest_path, cli,\n"
        "                confirm_absence=lambda kind, luid: _confirmed_absence(deployment, site, kind, luid))\n"
        "            _scope(manager, deployment, site)\n"
        "            candidates = _save_candidates(deployment, site, base, deployment_path, settings_path, manager)\n",
    )
    return source


def patch_content_profiles(source, replace_once):
    return replace_once(
        source,
        "        manager = reset_site.ResetSite(manifest_path, None)\n"
        "        operator_reset._scope(manager, baseline['deployment'], baseline['site_key'])\n"
        "        manager.cli = _reset_cli(root, binding, manager)\n",
        "        manager = reset_site.ResetSite(\n"
        "            manifest_path, None,\n"
        "            confirm_absence=lambda observed_kind, observed_id: operator_reset._confirmed_absence_with_binding(\n"
        "                baseline['deployment'], baseline['site_key'], observed_kind, observed_id, binding))\n"
        "        operator_reset._scope(manager, baseline['deployment'], baseline['site_key'])\n"
        "        manager.cli = _reset_cli(root, binding, manager)\n",
    )


def patch_reset_site_tests(source, replace_once):
    source = replace_once(
        source,
        "    def manager(self):\n"
        "        return reset_site.ResetSite(self.manifest_path, self.cli)\n",
        "    def manager(self):\n"
        "        # The stateful fake supplies an independent exact-identity observation.\n"
        "        return reset_site.ResetSite(\n"
        "            self.manifest_path, self.cli,\n"
        "            confirm_absence=lambda kind, luid: (kind, luid) not in self.cli.remote)\n",
    )
    return replace_once(
        source,
        "    def test_publish_partial_error_preserves_confirmed_identity(self):\n",
        "    def test_policy_refusal_after_publish_intent_does_not_recurse_or_retry(self):\n"
        "        original = self.cli.run\n"
        "        attempts = []\n"
        "        def denied(command):\n"
        "            if command[:3] == ['content', 'datasource', 'publish']:\n"
        "                attempts.append(list(command))\n"
        "                if len(attempts) == 1:\n"
        "                    self.cli.calls.append(list(command))\n"
        "                    raise reset_site.CLIError(command,\n"
        "                        {'error': {'id': 'mutation.disabled', 'outcome': 'not_attempted'}})\n"
        "            return original(command)\n"
        "        self.cli.run = denied\n"
        "        with self.assertRaisesRegex(reset_site.ResetError, 'uniquely reconciled'):\n"
        "            self.manager().reset()\n"
        "        self.assertEqual(len(attempts), 1)\n"
        "        self.assertFalse(any(event['event'] == 'publish_retry_authorized'\n"
        "                             for event in self.manager().manifest['journal']))\n\n"
        "    def test_publish_partial_error_preserves_confirmed_identity(self):\n",
    )


def patch_operator_reset_tests(source, replace_once):
    return replace_once(
        source,
        "        self.cli = FAKES.FakeTADX(self.root / 'workspace')\n",
        "        self.cli = FAKES.FakeTADX(self.root / 'workspace')\n"
        "        observer = patch.object(operator_reset, '_confirmed_absence',\n"
        "            side_effect=lambda deployment, site, kind, luid: (kind, luid) not in self.cli.remote)\n"
        "        observer.start()\n"
        "        self.addCleanup(observer.stop)\n",
    )


def patch_cleanup_config_tests(source, replace_once):
    return replace_once(
        source,
        "        self.cli = FAKES.FakeTADX(self.root / 'workspace')\n",
        "        self.cli = FAKES.FakeTADX(self.root / 'workspace')\n"
        "        observer = patch.object(operator_reset, '_confirmed_absence',\n"
        "            side_effect=lambda deployment, site, kind, luid: (kind, luid) not in self.cli.remote)\n"
        "        observer.start()\n"
        "        self.addCleanup(observer.stop)\n",
    )


def patch_content_operator_tests(source, replace_once):
    source = replace_once(
        source,
        "        self.enterContext(patch.object(content, '_reset_cli', return_value=self.cli))\n",
        "        self.enterContext(patch.object(content, '_reset_cli', return_value=self.cli))\n"
        "        self.absence_checks = []\n"
        "        def confirmed(deployment, site, kind, luid, binding=None):\n"
        "            self.absence_checks.append((kind, luid))\n"
        "            return (kind, luid) not in self.cli.remote\n"
        "        self.enterContext(patch.object(content.operator_reset, '_confirmed_absence',\n"
        "            side_effect=confirmed))\n"
        "        self.enterContext(patch.object(content.operator_reset, '_confirmed_absence_with_binding',\n"
        "            side_effect=confirmed))\n",
    )
    return replace_once(
        source,
        "        self.assertNotIn(('datasource', new_id), self.cli.remote)\n"
        "        ledger = json.loads((Path(req['private_case_dir']) / 'content-owned-ledger.json').read_text())\n",
        "        self.assertNotIn(('datasource', new_id), self.cli.remote)\n"
        "        self.assertIn(('datasource', new_id), self.absence_checks)\n"
        "        ledger = json.loads((Path(req['private_case_dir']) / 'content-owned-ledger.json').read_text())\n",
    )
