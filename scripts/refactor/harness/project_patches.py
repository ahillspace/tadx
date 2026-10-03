"""Exact anchors applied only after the external project source hash matches."""

PROJECT_REPLACEMENTS = [
    ("    from integration.validation_profiles import _gate_authorized\n    _gate_authorized(req)",
     "    from integration.g9_consent import project_scope\n    project_scope(req, binding)"),
    ("        _one(original, shared_settings(req)['targets'][source_site(req)[0]].get('project'))\n", ""),
    ("        save(path, checkpoint)\n        suffix =", 
     "        from integration.g9_consent import project_scope\n"
     "        checkpoint['g9_scope'] = project_scope(req, binding)\n"
     "        save(path, checkpoint)\n        suffix ="),
    ("    checkpoint=load(path)\n", "    checkpoint=load(path)\n"
     "    from integration.g9_consent import verify_cleanup_scope\n"
     "    verify_cleanup_scope(req, binding, checkpoint)\n"),
    ("    previous = checkpoint['steps'].get(key)\n", 
     "    from integration.g9_consent import admit_project_write\n"
     "    admit_project_write(checkpoint, action, key, identity, fields)\n"
     "    previous = checkpoint['steps'].get(key)\n"),
    ("        if step['phase'] == 'confirmed': continue\n", 
     "        if step['phase'] == 'confirmed': continue\n"
     "        from integration.g9_consent import admit_project_write\n"
     "        if 'rule' in step:\n"
     "            if step.get('id') not in checkpoint['owned']: raise Blocked('Unowned permission restoration')\n"
     "        else:\n"
     "            admit_project_write(checkpoint, step['action'], key, step.get('id'), step.get('fields'))\n"),
    ("    except UnknownWrite: acknowledged = None\n", 
     "    except UnknownWrite: acknowledged = None\n"
     "    if action == 'create':\n"
     "        from integration.g9_consent import require_creation_acknowledgment\n"
     "        require_creation_acknowledgment(acknowledged)\n"),
    ("        if step['action'] == 'create':\n            fields = step['fields']", 
     "        if step['action'] == 'create':\n"
     "            from integration.g9_consent import require_creation_acknowledgment\n"
     "            acknowledged_id = require_creation_acknowledgment(step.get('result'))\n"
     "            if step.get('id') != acknowledged_id: raise Blocked('Creation acknowledgment differs from its journal identity')\n"
     "            fields = step['fields']"),
    ("                checkpoint['owned'].append(matches[0]['id']);save(path,checkpoint)",
     "                raise Blocked('Task creation lacks exact acknowledged ownership; quarantine for reconciliation')"),
    ("            target=baseline['target']; current=", 
     "            target=baseline['target']\n"
     "            if target['id'] not in checkpoint['owned']: raise Blocked('Cleanup target is not run-owned')\n"
     "            current="),
    ("    covered, refs, issues, preview = set(), [], [], None", 
     "    covered, refs, issues, preview = set(), [], [], None\n    created_ids = []"),
    ("            covered.update(set(fields) & required if fields else {'action'}); refs +=", 
     "            if action == 'create' and not spec['preview']: created_ids.append(row['luid'])\n"
     "            covered.update(set(fields) & required if fields else {'action'}); refs +="),
    ("'evidence': sorted(set(refs)), 'issues': issues, 'preview': preview}",
     "'evidence': sorted(set(refs)), 'issues': issues, 'preview': preview, 'created_ids': created_ids}"),
    ("            row=matches[0] if len(matches)==1 else {}; owned=checkpoint['owned']+[r['id'] for r in matches]", 
     "            from integration.g9_consent import record_task_creation\n"
     "            record_task_creation(checkpoint, evidence, matches)\n"
     "            save(root/JOURNAL, checkpoint)\n"
     "            row=matches[0] if len(matches)==1 else {}; owned=checkpoint['owned']"),
]


def patch_project(source, replace_once):
    for before, after in PROJECT_REPLACEMENTS:
        source = replace_once(source, before, after)
    return source
