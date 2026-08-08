"""Purpose-limited GitHub CLI acquisition with ephemeral configuration."""

from __future__ import annotations

import os
import json
import re
import shutil
import signal
import subprocess
import tempfile
from collections.abc import Callable
from pathlib import Path
from typing import Any

from .protocol import MAX_SECRET_BYTES

GITHUB_HOST = "github.com"
GH_COMMAND = "/usr/local/bin/gh"
# Compose owns this as a private tmpfs; no gh configuration reaches the image
# layer or persistent Context vault mount.
LOGIN_TMP_ROOT = "/run/tobari-auth/login"


class GitHubLoginError(Exception):
    def __init__(self, code: str):
        super().__init__(code)
        self.code = code


Runner = Callable[..., Any]
ACCOUNT_LABEL_PATTERN = re.compile(
    r"^[A-Za-z0-9](?:[A-Za-z0-9-]{0,62}[A-Za-z0-9])?$"
)


def _account_label(payload: bytes) -> str:
    if not payload or len(payload) > 64 * 1024:
        raise GitHubLoginError("account_capture_failed")
    try:
        def pairs(values: list[tuple[str, Any]]) -> dict[str, Any]:
            result: dict[str, Any] = {}
            for key, value in values:
                if key in result:
                    raise GitHubLoginError("account_capture_failed")
                result[key] = value
            return result

        document = json.loads(payload.decode("utf-8"), object_pairs_hook=pairs)
    except GitHubLoginError:
        raise
    except (UnicodeDecodeError, json.JSONDecodeError):
        raise GitHubLoginError("account_capture_failed") from None
    if not isinstance(document, dict) or set(document) != {"hosts"}:
        raise GitHubLoginError("account_capture_failed")
    hosts = document.get("hosts")
    entries = hosts.get(GITHUB_HOST) if isinstance(hosts, dict) else None
    if not isinstance(entries, list) or not entries or len(entries) > 16:
        raise GitHubLoginError("account_capture_failed")
    active = [
        entry
        for entry in entries
        if isinstance(entry, dict)
        and entry.get("active") is True
        and entry.get("state") == "success"
    ]
    if len(active) != 1:
        raise GitHubLoginError("account_capture_failed")
    login = active[0].get("login")
    if (
        not isinstance(login, str)
        or len(login) > 64
        or not ACCOUNT_LABEL_PATTERN.fullmatch(login)
    ):
        raise GitHubLoginError("account_capture_failed")
    return login


def _safe_environment(config_directory: str) -> dict[str, str]:
    environment = dict(os.environ)
    for name in (
        "GH_TOKEN",
        "GITHUB_TOKEN",
        "GH_ENTERPRISE_TOKEN",
        "GITHUB_ENTERPRISE_TOKEN",
        "GH_HOST",
        "GH_REPO",
        "GH_PROMPT_DISABLED",
    ):
        environment.pop(name, None)
    environment["GH_CONFIG_DIR"] = config_directory
    return environment


def acquire_github_credential(
    runner: Runner = subprocess.run,
    gh_command: str = GH_COMMAND,
    temporary_root: str = LOGIN_TMP_ROOT,
) -> tuple[bytes, str]:
    """Run interactive web login, then capture token and bounded account label."""

    root = Path(temporary_root)
    acquired: tuple[bytes, str] | None = None
    pending_error: GitHubLoginError | None = None
    try:
        root.mkdir(mode=0o700, parents=True, exist_ok=True)
        os.chmod(root, 0o700)
        config_directory = tempfile.mkdtemp(prefix="gh-", dir=root)
        os.chmod(config_directory, 0o700)
    except OSError:
        raise GitHubLoginError("login_setup_failed") from None

    environment = _safe_environment(config_directory)
    try:
        try:
            login = runner(
                [
                    gh_command,
                    "auth",
                    "login",
                    "--hostname",
                    GITHUB_HOST,
                    "--git-protocol",
                    "https",
                    "--web",
                ],
                env=environment,
                check=False,
            )
        except (OSError, subprocess.SubprocessError):
            raise GitHubLoginError("login_failed") from None
        if not isinstance(login.returncode, int):
            raise GitHubLoginError("login_failed")
        if login.returncode in {130, -signal.SIGINT}:
            raise GitHubLoginError("login_cancelled")
        if login.returncode != 0:
            raise GitHubLoginError("login_failed")

        try:
            status_result = runner(
                [
                    gh_command,
                    "auth",
                    "status",
                    "--active",
                    "--hostname",
                    GITHUB_HOST,
                    "--json",
                    "hosts",
                ],
                env=environment,
                check=False,
                stdout=subprocess.PIPE,
                stderr=subprocess.DEVNULL,
            )
        except (OSError, subprocess.SubprocessError):
            raise GitHubLoginError("account_capture_failed") from None
        if not isinstance(status_result.returncode, int) or status_result.returncode != 0:
            raise GitHubLoginError("account_capture_failed")
        status_payload = status_result.stdout
        if not isinstance(status_payload, bytes):
            raise GitHubLoginError("account_capture_failed")
        account_label = _account_label(status_payload)

        try:
            token_result = runner(
                [gh_command, "auth", "token", "--hostname", GITHUB_HOST],
                env=environment,
                check=False,
                stdout=subprocess.PIPE,
                stderr=subprocess.DEVNULL,
            )
        except (OSError, subprocess.SubprocessError):
            raise GitHubLoginError("token_capture_failed") from None
        if not isinstance(token_result.returncode, int) or token_result.returncode != 0:
            raise GitHubLoginError("token_capture_failed")
        token = token_result.stdout
        if not isinstance(token, bytes):
            raise GitHubLoginError("token_capture_failed")
        # The newline is gh's output framing, not part of the opaque credential.
        if token.endswith(b"\r\n"):
            token = token[:-2]
        elif token.endswith(b"\n"):
            token = token[:-1]
        if not token or len(token) > MAX_SECRET_BYTES or b"\n" in token or b"\r" in token:
            raise GitHubLoginError("token_capture_failed")
        acquired = (token, account_label)
    except GitHubLoginError as error:
        pending_error = error
    finally:
        try:
            shutil.rmtree(config_directory)
        except OSError:
            pending_error = GitHubLoginError("login_cleanup_failed")
            acquired = None
    if pending_error is not None:
        raise pending_error
    if acquired is None:
        raise GitHubLoginError("token_capture_failed")
    return acquired
