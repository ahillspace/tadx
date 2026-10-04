"""Exact locked-broker patch for one memory-only credential channel."""


def patch_broker(source, replace_once):
    if "credentialBroker" in source:
        raise ValueError("Credential broker was already installed")
    source = replace_once(
        source,
        "function startBroker() {",
        "function startBroker() {\n"
        "  if(process.env.BENCH_CREDENTIAL_CHANNEL==='1') credentialBroker.start();",
    )
    source = replace_once(
        source,
        "    if (fs.existsSync('/cli-state/credentials.json')) "
        "Object.assign(env,JSON.parse(fs.readFileSync('/cli-state/credentials.json','utf8')));",
        "    Object.assign(env,credentialBroker.environment());",
    )
    return "const credentialBroker = require('./credential_broker.cjs');\n" + source


def patch_bridge(source, replace_once):
    """Include the private broker module after existing exact image transforms."""
    return replace_once(
        source,
        "'g9_windows_installer_broker.cjs')",
        "'g9_windows_installer_broker.cjs','credential_broker.cjs')",
    )
