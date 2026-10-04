"""Only the seven native hosted installer tasks get a bounded queue allowance."""

import copy

VARIANTS = frozenset({"fresh", "idempotent", "no-completion", "completion-opt-in",
                      "failed-download-preserves-binary", "uninstall", "no-modify-path"})
IDS = frozenset("V-installer-windows-" + item for item in VARIANTS)
ORIGINAL_WALL_SECONDS = 600
HOSTED_WALL_SECONDS = 2400


def extend_budget(exercise):
    identity = exercise.get("id") if isinstance(exercise, dict) else None
    if identity not in IDS:
        raise ValueError("Windows installer budget scope is not one of the seven declared cases")
    original = exercise.get("evaluator", {}).get("budgets", {}).get("wall_seconds")
    if type(original) is not int or original != ORIGINAL_WALL_SECONDS:
        raise ValueError("Installer exercise budget changed from the source-locked premise")
    prepared = copy.deepcopy(exercise)
    prepared["evaluator"]["budgets"]["wall_seconds"] = HOSTED_WALL_SECONDS
    return prepared
