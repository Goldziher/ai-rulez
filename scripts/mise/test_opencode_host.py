"""Optional, model-free registration checks in the official OpenCode v2 host."""

import argparse
import base64
import json
import os
import secrets
import shutil
import socket
import subprocess
import sys
import tarfile
import tempfile
import threading
import time
import urllib.error
import urllib.parse
import urllib.request
from pathlib import Path
from typing import TextIO

HOST = 'npm:@opencode/cli[allow_builds=["@opencode/cli"]]@2.2.5'
COMMAND = "host-command"
PLUGIN = "host-probe"


def checked(arguments: list[str], root: Path, env: dict[str, str]) -> str:
    result = subprocess.run(arguments, cwd=root, env=env, capture_output=True, text=True, timeout=180)
    if result.returncode:
        raise RuntimeError(f"{arguments[0]} exited {result.returncode}\n{result.stdout}\n{result.stderr}")
    return result.stdout


def isolated_environment(root: Path) -> dict[str, str]:
    root.mkdir(parents=True, exist_ok=True)
    env = {key: os.environ[key] for key in ["PATH", "SystemRoot", "COMSPEC"] if key in os.environ}
    for kind in ["CONFIG", "DATA", "CACHE", "STATE"]:
        directory = root / kind.lower()
        directory.mkdir(exist_ok=True)
        env[f"XDG_{kind}_HOME"] = str(directory)
    for key in ["OPENCODE_TEST_HOME", "OPENCODE_CONFIG_DIR", "TMPDIR", "NPM_CONFIG_CACHE"]:
        directory = root / key.lower()
        directory.mkdir(exist_ok=True)
        env[key] = str(directory)
    for key in ["USERCONFIG", "GLOBALCONFIG"]:
        path = root / f"{key.lower()}.npmrc"
        path.write_text("")
        env[f"NPM_CONFIG_{key}"] = str(path)
    env["NPM_CONFIG_REGISTRY"] = "https://registry.npmjs.org"
    for key in [
        "OPENCODE_CONFIG_PROJECT_DISABLE",
        "OPENCODE_DISABLE_CLAUDE_CODE",
        "OPENCODE_FILEWATCHER_DISABLE",
        "OPENCODE_DISABLE_FFF",
        "OPENCODE_DISABLE_MODELS_FETCH",
        "OPENCODE_DISABLE_AUTOUPDATE",
        "OPENCODE_DISABLE_FREE_MODELS",
        "OPENCODE_DISABLE_EXTERNAL_SKILLS",
        "OPENCODE_DISABLE_LSP_DOWNLOAD",
    ]:
        env[key] = "1"
    return env


def check_system_isolation() -> None:
    if sys.platform == "darwin":
        import pwd

        username = pwd.getpwuid(os.getuid()).pw_name
        paths = [Path("/Library/Application Support/opencode")]
        paths += [Path("/Library/Managed Preferences") / name / "ai.opencode.managed.plist" for name in ["", username]]
    elif sys.platform.startswith("linux"):
        paths = [Path("/etc/opencode")]
    else:
        raise RuntimeError("System configuration isolation has not been qualified on this platform")
    present = [str(path) for path in paths if path.exists()]
    if present:
        raise RuntimeError(f"Refusing to read outside host configuration; system paths exist: {present}")


def create_fixture(root: Path, setting: str, required_import: bool) -> Path:
    config = root / ".ai-rulez/config.toml"
    config.parent.mkdir(parents=True)
    config.write_text(f'''version = "5.0"
name = "{PLUGIN}"
builtins = false
presets = []
[plugin]
name = "{PLUGIN}"
description = "Model-free packed host registration fixture"
version = "0.1.0"
runtimes = ["opencode"]
provenance_headers = false
{setting}
''')
    content = {
        f"commands/{COMMAND}.md": "---\ndescription: Host command probe\n---\nHOST_COMMAND_SENTINEL $ARGUMENTS\n",
        "agents/host-agent.md": "---\ndescription: Host instruction probe\n---\nHOST_AGENT_INSTRUCTION_SENTINEL\n",
        "skills/host-skill/SKILL.md": "---\nname: host-skill\ndescription: Host skill probe\n---\nHOST_SKILL_SENTINEL\n",
    }
    if required_import:
        content["opencode/index.js"] = (
            'import { Plugin } from "@opencode/plugin";\nexport default Plugin.define({id:"host-probe",setup(){}});\n'
        )
    for relative, text in content.items():
        path = config.parent / relative
        path.parent.mkdir(parents=True, exist_ok=True)
        path.write_text(text)
    return config


def packed_fixture(ai: Path, root: Path, setting: str, required_import: bool) -> tuple[Path, dict]:
    config = create_fixture(root, setting, required_import)
    env = isolated_environment(root / "producer-environment")
    checked([str(ai), "--config", str(config), "generate", "--plugin", "--no-local"], root, env)
    result = json.loads(checked(["npm", "pack", "--ignore-scripts", "--json"], root, env))
    if len(result) != 1:
        raise AssertionError(f"Expected one packed package, got {len(result)}")
    archive = root / result[0]["filename"]
    destination = root / "unpacked"
    with tarfile.open(archive) as package:
        for member in package.getmembers():
            path = Path(member.name)
            if path.is_absolute() or ".." in path.parts or member.issym() or member.islnk():
                raise AssertionError(f"Unsafe package member {member.name}")
        package.extractall(destination, filter="data")
    packed = destination / "package"
    manifest = json.loads((packed / "package.json").read_text())
    if required_import:
        copied = packed / ".opencode/plugins/host-probe.js"
        if "@opencode/plugin" not in copied.read_text():
            raise AssertionError("Authored required import was not preserved")
    return packed, manifest


def get_data(base: str, endpoint: str, project: Path, password: str) -> object:
    query = urllib.parse.urlencode({"location[directory]": str(project)})
    token = base64.b64encode(f"opencode:{password}".encode()).decode()
    request = urllib.request.Request(f"{base}/api/{endpoint}?{query}", headers={"Authorization": f"Basic {token}"})
    opener = urllib.request.build_opener(urllib.request.ProxyHandler({}))
    with opener.open(request, timeout=8) as response:
        return json.load(response)["data"]


def await_host(base: str, project: Path, process: subprocess.Popen, password: str) -> object:
    deadline = time.monotonic() + 40
    while time.monotonic() < deadline:
        if process.poll() is not None:
            raise RuntimeError(f"Host exited before activation: {process.returncode}")
        try:
            return get_data(base, "agent", project, password)
        except (urllib.error.URLError, TimeoutError):
            time.sleep(0.25)
    raise TimeoutError("Host registration did not become observable within 40 seconds")


def copy_redacted_output(process: subprocess.Popen[str], log: TextIO, password: str) -> None:
    token = base64.b64encode(f"opencode:{password}".encode()).decode()
    assert process.stdout is not None
    for line in process.stdout:
        log.write(line.replace(password, "[redacted]").replace(token, "[redacted]"))
        log.flush()


def observe_host(host: Path, packed: Path, root: Path) -> dict:
    env = isolated_environment(root / "environment")
    password = secrets.token_urlsafe(32)
    env["OPENCODE_PASSWORD"] = password
    env["OPENCODE_CONFIG_CONTENT"] = json.dumps({"plugins": ["-opencode.provider.*", str(packed)], "snapshot": False})
    project = root / "project"
    project.mkdir()
    with socket.socket() as listener:
        listener.bind(("127.0.0.1", 0))
        port = listener.getsockname()[1]
    with (root / "host.log").open("w") as log:
        process = subprocess.Popen(
            [str(host), "serve", "--hostname", "127.0.0.1", "--port", str(port)],
            cwd=project,
            env=env,
            stdout=subprocess.PIPE,
            stderr=subprocess.STDOUT,
            text=True,
        )
        reader = threading.Thread(target=copy_redacted_output, args=(process, log, password), daemon=True)
        reader.start()
        try:
            base = f"http://127.0.0.1:{port}"
            data = {"agent": await_host(base, project, process, password)}
            for endpoint in ["plugin", "command", "skill"]:
                data[endpoint] = get_data(base, endpoint, project, password)
            (root / "observations.json").write_text(json.dumps(data, indent=2) + "\n")
            return data
        finally:
            process.terminate()
            try:
                process.wait(timeout=10)
            except subprocess.TimeoutExpired:
                process.kill()
                process.wait(timeout=5)
            reader.join(timeout=5)
            if reader.is_alive():
                raise RuntimeError("Host output reader did not terminate")


def assert_registration(data: dict, command_count: int) -> dict:
    plugins = [row for row in data["plugin"] if row.get("id") == PLUGIN]
    commands = [row for row in data["command"] if row.get("name") == COMMAND]
    skills = [row for row in data["skill"] if row.get("id") == f"{PLUGIN}/host-skill"]
    agents = [row for row in data["agent"] if row.get("id") == "host-agent"]
    if len(plugins) != 1 or plugins[0].get("state", {}).get("status") != "active":
        raise AssertionError(f"Expected exactly one active packed plugin: {plugins}")
    if len(skills) != 1 or "HOST_SKILL_SENTINEL" not in skills[0].get("content", ""):
        raise AssertionError(f"Expected exactly one bundled skill with its real content: {skills}")
    if len(agents) != 1 or "HOST_AGENT_INSTRUCTION_SENTINEL" not in agents[0].get("system", ""):
        raise AssertionError(f"Expected exactly one bundled agent instruction: {agents}")
    if len(commands) != command_count:
        raise AssertionError(f"missing_registration:command expected={command_count} observed={len(commands)}")
    return {"active_plugins": len(plugins), "commands": len(commands), "skills": len(skills), "agents": len(agents)}


def run(root: Path) -> dict:
    check_system_isolation()
    repository = Path(__file__).resolve().parents[2]
    ai = repository / "bin/ai-rulez"
    if not ai.is_file():
        raise RuntimeError("Build the static CLI first with mise run build")
    installation = Path(checked(["mise", "where", HOST], repository, dict(os.environ)).strip())
    host = installation / "node_modules/.bin/opencode2"
    version = checked([str(host), "--version"], root, isolated_environment(root / "version-environment")).strip()
    if version != "opencode v2.2.5":
        raise AssertionError(f"Expected official host2.2.5, got {version!r}")
    packages = {}
    for name, setting, required in [
        ("sdk-free", "opencode_dependency_enabled = false", False),
        ("default", "", False),
        ("required", "opencode_dependency_enabled = true", True),
    ]:
        packed, manifest = packed_fixture(ai, root / name, setting, required)
        has_sdk = "@opencode/plugin" in manifest.get("dependencies", {})
        if has_sdk != (name != "sdk-free"):
            raise AssertionError(f"Wrong automatic SDK boundary for {name}: {manifest}")
        packages[name] = {"packed": str(packed), "dependencies": manifest.get("dependencies", {})}
    positive_root = root / "positive"
    positive_root.mkdir()
    positive = assert_registration(observe_host(host, Path(packages["sdk-free"]["packed"]), positive_root), 1)
    negative_root = root / "negative"
    negative_root.mkdir()
    altered = negative_root / "altered-package"
    shutil.copytree(packages["sdk-free"]["packed"], altered)
    (altered / ".opencode/commands/host-command.md").unlink()
    observation = observe_host(host, altered, negative_root)
    try:
        assert_registration(observation, 1)
    except AssertionError as error:
        if str(error) != "missing_registration:command expected=1 observed=0":
            raise
    else:
        raise AssertionError("Missing bundled command did not fail the registration oracle")
    negative = assert_registration(observation, 0)
    return {
        "host_version": version,
        "packed_variants": packages,
        "positive": positive,
        "negative": negative,
        "negative_control_detected": 1,
    }


def main() -> None:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--artifacts", type=Path)
    args = parser.parse_args()
    with tempfile.TemporaryDirectory(prefix="ai-rulez-opencode-host-") as temporary:
        root = args.artifacts.resolve() if args.artifacts else Path(temporary)
        root.mkdir(parents=True, exist_ok=True)
        result = run(root)
        (root / "summary.json").write_text(json.dumps(result, indent=2) + "\n")
        sys.stdout.write(json.dumps(result, indent=2) + "\n")


if __name__ == "__main__":
    main()
