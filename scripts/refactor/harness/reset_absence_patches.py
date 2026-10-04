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
    return replace_once(
        source,
        "            if exc.result.get(\"error\", {}).get(\"upstream_status\") == 404:\n"
        "                return None\n",
        "            if exc.result.get(\"error\", {}).get(\"upstream_status\") == 404:\n"
        "                if self.confirm_absence is None or self.confirm_absence(kind, luid) is not True:\n"
        "                    raise ResetError(\"Independent exact GET 404 is required before confirming absence.\") from exc\n"
        "                return None\n",
    )


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
    return replace_once(
        source,
        "    def manager(self):\n"
        "        return reset_site.ResetSite(self.manifest_path, self.cli)\n",
        "    def manager(self):\n"
        "        # The stateful fake supplies an independent exact-identity observation.\n"
        "        return reset_site.ResetSite(\n"
        "            self.manifest_path, self.cli,\n"
        "            confirm_absence=lambda kind, luid: (kind, luid) not in self.cli.remote)\n",
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
