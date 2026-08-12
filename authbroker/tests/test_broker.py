from __future__ import annotations

import contextlib
import errno
import io
import json
import logging
import os
import shutil
import stat
import tempfile
import threading
import unittest
from pathlib import Path
from types import SimpleNamespace
from unittest import mock

from cryptography.hazmat.primitives.ciphers.aead import AESGCM

import authbroker.vault as vault_module
from authbroker.broker import BrokerError, BrokerState, Dispatcher
from authbroker.daemon import (
    _UnixServer,
    _owned_by_service as daemon_owned_by_service,
    _protect_socket,
)
from authbroker.protocol import (
    MAX_FRAME_BYTES,
    ProtocolError,
    call_unix_socket,
    decode_document,
)
from authbroker.vault import (
    VaultError,
    VaultStore,
    _owned_by_service as vault_owned_by_service,
    empty_payload,
    new_record,
)


CONTEXT_A = "018bcfe5-687b-7000-8000-000000000001"
CONTEXT_B = "018bcfe5-687b-7000-8000-000000000002"
PROJECT_A = "018bcfe5-687b-7000-8000-000000000101"
PROJECT_B = "018bcfe5-687b-7000-8000-000000000102"
KEY = bytes(range(32))
OTHER_KEY = bytes(reversed(range(32)))
CANARY = b"tobari-super-secret-canary"


def github_bindings() -> list[dict[str, object]]:
    common = {
        "provider_id": "github",
        "target": {"scheme": "https", "host": "api.github.com", "port": 443},
        "destination": {
            "header": "authorization",
            "format": "preserve_scheme",
            "secret_field": "primary_secret",
        },
        "secret_headers": ["authorization"],
    }
    return [
        {**common, "source": {"header": "authorization", "format": "bearer"}},
        {**common, "source": {"header": "authorization", "format": "token"}},
    ]


def write_document(path: Path, document: dict[str, object]) -> None:
    path.write_text(json.dumps(document, separators=(",", ":")), encoding="utf-8")
    path.chmod(0o600)


class VaultTests(unittest.TestCase):
    def setUp(self) -> None:
        self.temporary = tempfile.TemporaryDirectory()
        self.root = Path(self.temporary.name) / "contexts"
        self.store = VaultStore(self.root)

    def tearDown(self) -> None:
        self.temporary.cleanup()

    def payload(self, secret: bytes = CANARY) -> dict[str, object]:
        payload = empty_payload()
        payload["providers"]["github"] = new_record(secret)
        return payload

    def test_virtualized_root_owner_is_accepted_only_for_non_root_service(self) -> None:
        info = SimpleNamespace(st_uid=0)
        for helper, module in (
            (vault_owned_by_service, "authbroker.vault.os.geteuid"),
            (daemon_owned_by_service, "authbroker.daemon.os.geteuid"),
        ):
            with self.subTest(helper=helper.__module__):
                with mock.patch(module, return_value=1000):
                    self.assertTrue(helper(info))
                with mock.patch(module, return_value=1001):
                    self.assertTrue(helper(SimpleNamespace(st_uid=1001)))
                    self.assertFalse(helper(SimpleNamespace(st_uid=1002)))

    def test_vault_round_trip_is_context_bound_and_owner_only(self) -> None:
        payload = self.payload()
        self.store.save(CONTEXT_A, KEY, payload)
        vault = self.root / CONTEXT_A / "vault.enc"
        self.assertEqual(vault.stat().st_mode & 0o777, 0o600)
        self.assertEqual((self.root / CONTEXT_A).stat().st_mode & 0o777, 0o700)
        self.assertEqual(self.store.load(CONTEXT_A, KEY), payload)
        self.assertNotIn(CANARY, vault.read_bytes())

    def test_tamper_is_rejected(self) -> None:
        self.store.save(CONTEXT_A, KEY, self.payload())
        path = self.root / CONTEXT_A / "vault.enc"
        envelope = json.loads(path.read_text(encoding="utf-8"))
        ciphertext = envelope["ciphertext"]
        envelope["ciphertext"] = ("A" if ciphertext[0] != "A" else "B") + ciphertext[1:]
        write_document(path, envelope)
        with self.assertRaisesRegex(VaultError, "vault_integrity_failed"):
            self.store.load(CONTEXT_A, KEY)

    def test_wrong_key_is_rejected(self) -> None:
        self.store.save(CONTEXT_A, KEY, self.payload())
        with self.assertRaisesRegex(VaultError, "vault_integrity_failed"):
            self.store.load(CONTEXT_A, OTHER_KEY)

    def test_moved_vault_is_rejected_by_context_binding(self) -> None:
        self.store.save(CONTEXT_A, KEY, self.payload())
        destination = self.root / CONTEXT_B
        destination.mkdir(mode=0o700)
        shutil.copyfile(self.root / CONTEXT_A / "vault.enc", destination / "vault.enc")
        (destination / "vault.enc").chmod(0o600)
        with self.assertRaisesRegex(VaultError, "vault_invalid"):
            self.store.load(CONTEXT_B, KEY)

    def test_truncated_vault_is_rejected(self) -> None:
        self.store.save(CONTEXT_A, KEY, self.payload())
        path = self.root / CONTEXT_A / "vault.enc"
        value = path.read_bytes()
        path.write_bytes(value[: len(value) // 2])
        path.chmod(0o600)
        with self.assertRaisesRegex(VaultError, "vault_invalid"):
            self.store.load(CONTEXT_A, KEY)

    def test_envelope_version_is_rejected(self) -> None:
        self.store.save(CONTEXT_A, KEY, self.payload())
        path = self.root / CONTEXT_A / "vault.enc"
        envelope = json.loads(path.read_text(encoding="utf-8"))
        envelope["schema_version"] = 2
        write_document(path, envelope)
        with self.assertRaisesRegex(VaultError, "vault_version_unsupported"):
            self.store.load(CONTEXT_A, KEY)

    def test_encrypted_payload_version_is_rejected(self) -> None:
        self.store.save(CONTEXT_A, KEY, self.payload())
        path = self.root / CONTEXT_A / "vault.enc"
        envelope = json.loads(path.read_text(encoding="utf-8"))
        nonce = vault_module._b64decode(envelope["nonce"])
        plaintext = AESGCM(KEY).decrypt(
            nonce,
            vault_module._b64decode(envelope["ciphertext"]),
            vault_module._associated_data(CONTEXT_A),
        )
        payload = json.loads(plaintext)
        payload["schema_version"] = 3
        new_nonce = os.urandom(12)
        envelope["nonce"] = vault_module._b64encode(new_nonce)
        envelope["ciphertext"] = vault_module._b64encode(
            AESGCM(KEY).encrypt(
                new_nonce,
                json.dumps(payload, separators=(",", ":"), sort_keys=True).encode(),
                vault_module._associated_data(CONTEXT_A),
            )
        )
        write_document(path, envelope)
        with self.assertRaisesRegex(VaultError, "vault_version_unsupported"):
            self.store.load(CONTEXT_A, KEY)

    def test_atomic_failure_preserves_prior_valid_vault(self) -> None:
        original = self.payload(b"first-value")
        replacement = self.payload(b"second-value")
        self.store.save(CONTEXT_A, KEY, original)
        with mock.patch("authbroker.vault.os.replace", side_effect=OSError("synthetic")):
            with self.assertRaisesRegex(VaultError, "vault_write_failed"):
                self.store.save(CONTEXT_A, KEY, replacement)
        self.assertEqual(self.store.load(CONTEXT_A, KEY), original)

    def test_symlink_nonregular_and_unsafe_mode_are_rejected(self) -> None:
        self.store.save(CONTEXT_A, KEY, self.payload())
        path = self.root / CONTEXT_A / "vault.enc"
        path.chmod(0o644)
        with self.assertRaisesRegex(VaultError, "vault_path_invalid"):
            self.store.load(CONTEXT_A, KEY)
        path.chmod(0o600)
        target = Path(self.temporary.name) / "copy.enc"
        path.replace(target)
        path.symlink_to(target)
        with self.assertRaisesRegex(VaultError, "vault_path_invalid"):
            self.store.load(CONTEXT_A, KEY)


class BrokerStateTests(unittest.TestCase):
    def setUp(self) -> None:
        self.temporary = tempfile.TemporaryDirectory()
        self.store = VaultStore(Path(self.temporary.name) / "contexts")
        self.state = BrokerState(self.store)

    def tearDown(self) -> None:
        self.temporary.cleanup()

    def unlock_import_issue(self, project: str = PROJECT_A) -> dict[str, object]:
        self.state.unlock(KEY)
        self.state.import_secret(CONTEXT_A, "github", CANARY)
        return self.state.issue_handle(CONTEXT_A, project, "github", github_bindings())

    def test_starts_locked_and_mutations_fail_locked(self) -> None:
        self.assertTrue(self.state.locked)
        status = self.state.status(CONTEXT_A, "github")
        self.assertEqual(status["state"], "locked")
        with self.assertRaisesRegex(BrokerError, "locked"):
            self.state.import_secret(CONTEXT_A, "github", CANARY)

    def test_retired_dynamic_providers_and_runtime_operations_are_closed(self) -> None:
        self.state.unlock(KEY)
        dispatcher = Dispatcher(self.state, "control")
        for provider in ("aws", "datadog", "openai", "anthropic", "chatwork"):
            with self.subTest(provider=provider):
                with self.assertRaisesRegex(BrokerError, "invalid_provider"):
                    dispatcher.dispatch(
                        {
                            "schema_version": 1,
                            "op": "login",
                            "context_id": CONTEXT_A,
                            "provider": provider,
                            "secret_length": len(CANARY),
                            "account_label": "retired",
                        },
                        CANARY,
                    )
        runtime = Dispatcher(self.state, "runtime")
        for operation in ("introspect_signing", "sign_sigv4"):
            with self.subTest(operation=operation):
                with self.assertRaisesRegex(BrokerError, "unknown_operation"):
                    runtime.dispatch({"schema_version": 1, "op": operation}, b"")
        for operation in ("companion_prepare", "companion_status"):
            with self.subTest(operation=operation):
                with self.assertRaisesRegex(BrokerError, "unknown_operation"):
                    dispatcher.dispatch({"schema_version": 1, "op": operation}, b"")

    def test_import_omits_account_but_login_preserves_verified_label(self) -> None:
        self.state.unlock(KEY)
        imported = self.state.import_secret(CONTEXT_A, "github", CANARY)
        self.assertNotIn("account_label", imported)
        self.assertNotIn("account_label", self.state.status(CONTEXT_A, "github"))
        logged_in = Dispatcher(self.state, "control").dispatch(
            {
                "schema_version": 1,
                "op": "login",
                "context_id": CONTEXT_A,
                "provider": "github",
                "secret_length": len(CANARY),
                "account_label": "octo-user",
            },
            CANARY,
        )
        self.assertEqual(logged_in["account_label"], "octo-user")
        self.assertEqual(self.state.status(CONTEXT_A, "github")["account_label"], "octo-user")

    def test_issue_is_idempotent_in_process_and_after_restart(self) -> None:
        first = self.unlock_import_issue()
        second = self.state.issue_handle(
            CONTEXT_A, PROJECT_A, "github", list(reversed(github_bindings()))
        )
        self.assertEqual(first["handle"], second["handle"])

        restarted = BrokerState(self.store)
        restarted.unlock(KEY)
        third = restarted.issue_handle(CONTEXT_A, PROJECT_A, "github", github_bindings())
        self.assertEqual(first["handle"], third["handle"])
        vault_bytes = (self.store.root / CONTEXT_A / "vault.enc").read_bytes()
        self.assertNotIn(first["handle"].encode(), vault_bytes)
        self.assertTrue(
            all(isinstance(digest, bytes) and len(digest) == 32 for digest in restarted._handles)
        )
        self.assertNotIn(first["handle"], repr(restarted._handles))

    def test_introspection_rehydrates_hash_index_after_restart(self) -> None:
        issued = self.unlock_import_issue()
        restarted = BrokerState(self.store)
        restarted.unlock(KEY)
        response = restarted.introspect(
            issued["handle"],
            CONTEXT_A,
            PROJECT_A,
            "github",
            {"scheme": "https", "host": "api.github.com", "port": 443},
            "authorization",
            "token",
        )
        self.assertEqual(response["provider"], "github")
        self.assertNotIn("secret", response)
        self.assertNotIn(CANARY.decode(), json.dumps(response))

    def test_handles_are_random_and_isolated_by_project(self) -> None:
        first = self.unlock_import_issue(PROJECT_A)
        second = self.state.issue_handle(CONTEXT_A, PROJECT_B, "github", github_bindings())
        self.assertNotEqual(first["handle"], second["handle"])
        with self.assertRaisesRegex(BrokerError, "handle_(not_found|binding_mismatch)"):
            self.state.introspect(
                first["handle"],
                CONTEXT_A,
                PROJECT_B,
                "github",
                {"scheme": "https", "host": "api.github.com", "port": 443},
                "authorization",
                "token",
            )

    def test_replace_rotates_record_revision_and_revokes_handles(self) -> None:
        first = self.unlock_import_issue()
        original_revision = first["revision"]
        replacement = self.state.import_secret(CONTEXT_A, "github", b"replacement")
        self.assertNotEqual(original_revision, replacement["revision"])
        with self.assertRaisesRegex(BrokerError, "handle_not_found"):
            self.state.introspect(
                first["handle"],
                CONTEXT_A,
                PROJECT_A,
                "github",
                {"scheme": "https", "host": "api.github.com", "port": 443},
                "authorization",
                "token",
            )

    def test_binding_status_checks_live_persisted_handle_without_revealing_it(self) -> None:
        self.state.unlock(KEY)
        imported = self.state.import_secret(CONTEXT_A, "github", CANARY)
        missing = self.state.binding_status(
            CONTEXT_A,
            PROJECT_A,
            "github",
            imported["revision"],
            github_bindings(),
        )
        self.assertEqual(missing["state"], "missing")

        issued = self.state.issue_handle(
            CONTEXT_A, PROJECT_A, "github", github_bindings()
        )
        ready = Dispatcher(self.state, "control").dispatch(
            {
                "schema_version": 1,
                "op": "binding_status",
                "context_id": CONTEXT_A,
                "project_id": PROJECT_A,
                "provider": "github",
                "revision": issued["revision"],
                "bindings": github_bindings(),
            },
            b"",
        )
        self.assertEqual(ready["state"], "ready")
        self.assertEqual(ready["provider"], "github")
        self.assertEqual(ready["revision"], issued["revision"])
        self.assertNotIn("handle", ready)
        self.assertNotIn(issued["handle"], json.dumps(ready))
        self.assertNotIn(CANARY.decode(), json.dumps(ready))

        wrong_bindings = github_bindings()
        wrong_bindings[0]["target"]["host"] = "uploads.github.com"
        stale = self.state.binding_status(
            CONTEXT_A,
            PROJECT_A,
            "github",
            issued["revision"],
            wrong_bindings,
        )
        self.assertEqual(stale["state"], "stale")

        replacement = self.state.import_secret(CONTEXT_A, "github", b"replacement")
        self.assertNotEqual(replacement["revision"], issued["revision"])
        stale_revision = self.state.binding_status(
            CONTEXT_A,
            PROJECT_A,
            "github",
            issued["revision"],
            github_bindings(),
        )
        self.assertEqual(stale_revision["state"], "stale")

    def test_logout_atomically_removes_record_and_handles(self) -> None:
        first = self.unlock_import_issue()
        response = self.state.logout(CONTEXT_A, "github")
        self.assertTrue(response["changed"])
        self.assertEqual(self.state.status(CONTEXT_A, "github")["state"], "not_configured")
        with self.assertRaisesRegex(BrokerError, "handle_not_found"):
            self.state.introspect(
                first["handle"],
                CONTEXT_A,
                PROJECT_A,
                "github",
                {"scheme": "https", "host": "api.github.com", "port": 443},
                "authorization",
                "token",
            )

    def test_introspect_and_resolve_double_validate_every_binding(self) -> None:
        issued = self.unlock_import_issue()
        target = {"scheme": "https", "host": "api.github.com", "port": 443}
        metadata = self.state.introspect(
            issued["handle"],
            CONTEXT_A,
            PROJECT_A,
            "github",
            target,
            "authorization",
            "bearer",
        )
        resolved = self.state.resolve(
            issued["handle"],
            CONTEXT_A,
            PROJECT_A,
            "github",
            metadata["revision"],
            target,
            "authorization",
            "bearer",
        )
        self.assertEqual(resolved["secret"]["field"], "primary_secret")
        self.assertEqual(resolved["source"]["format"], "bearer")

        mismatches = [
            ({"scheme": "https", "host": "github.com", "port": 443}, "authorization", "bearer"),
            (target, "x-api-key", "bearer"),
            (target, "authorization", "raw"),
        ]
        for wrong_target, wrong_header, wrong_format in mismatches:
            with self.subTest(wrong_target=wrong_target, wrong_format=wrong_format):
                with self.assertRaisesRegex(BrokerError, "handle_binding_mismatch"):
                    self.state.resolve(
                        issued["handle"],
                        CONTEXT_A,
                        PROJECT_A,
                        "github",
                        metadata["revision"],
                        wrong_target,
                        wrong_header,
                        wrong_format,
                    )

    def test_http_and_l7_fields_are_rejected(self) -> None:
        self.state.unlock(KEY)
        self.state.import_secret(CONTEXT_A, "github", CANARY)
        invalid = github_bindings()[0]
        invalid["target"] = {"scheme": "http", "host": "api.github.com", "port": 80}
        with self.assertRaisesRegex(BrokerError, "invalid_binding"):
            self.state.issue_handle(CONTEXT_A, PROJECT_A, "github", [invalid])

        for forbidden in ("cookie", "set-cookie"):
            invalid = github_bindings()[0]
            invalid["source"]["header"] = forbidden
            invalid["destination"]["header"] = forbidden
            invalid["secret_headers"] = [forbidden]
            with self.subTest(forbidden=forbidden):
                with self.assertRaisesRegex(BrokerError, "invalid_binding"):
                    self.state.issue_handle(CONTEXT_A, PROJECT_A, "github", [invalid])
        invalid = github_bindings()[0]
        invalid["target"] = {
            "scheme": "https",
            "host": "api.github.com",
            "port": 443,
            "method": "GET",
        }
        with self.assertRaisesRegex(BrokerError, "invalid_binding"):
            self.state.issue_handle(CONTEXT_A, PROJECT_A, "github", [invalid])

    def test_operations_do_not_log_canary(self) -> None:
        stream = io.StringIO()
        handler = logging.StreamHandler(stream)
        root = logging.getLogger()
        root.addHandler(handler)
        try:
            with contextlib.redirect_stderr(stream):
                self.unlock_import_issue()
        finally:
            root.removeHandler(handler)
        self.assertNotIn(CANARY.decode(), stream.getvalue())


class ProtocolTests(unittest.TestCase):
    def test_duplicate_keys_and_oversize_are_rejected(self) -> None:
        with self.assertRaisesRegex(ProtocolError, "invalid_json"):
            decode_document(b'{"schema_version":1,"schema_version":1}')
        with self.assertRaisesRegex(ProtocolError, "invalid_frame"):
            decode_document(b"x" * (MAX_FRAME_BYTES + 1))

    def test_virtualized_socket_chmod_fallback_is_exact(self) -> None:
        info = SimpleNamespace(st_uid=1000, st_mode=stat.S_IFSOCK | 0o755)
        with (
            mock.patch("authbroker.daemon.os.geteuid", return_value=1000),
            mock.patch(
                "authbroker.daemon.os.chmod",
                side_effect=OSError(errno.EINVAL, "synthetic"),
            ),
            mock.patch("authbroker.daemon.os.lstat", return_value=info),
        ):
            _protect_socket("/synthetic/broker.sock", True)
        with (
            mock.patch(
                "authbroker.daemon.os.chmod",
                side_effect=OSError(errno.EPERM, "synthetic"),
            ),
            self.assertRaisesRegex(RuntimeError, "could not be protected"),
        ):
            _protect_socket("/synthetic/broker.sock", True)

    def test_private_socket_end_to_end_starts_locked_and_accepts_raw_key(self) -> None:
        with tempfile.TemporaryDirectory() as temporary:
            root = Path(temporary)
            socket_dir = root / "control"
            socket_dir.mkdir(mode=0o700)
            socket_path = str(socket_dir / "broker.sock")
            state = BrokerState(VaultStore(root / "contexts"))
            server = _UnixServer(socket_path, Dispatcher(state, "control"))
            thread = threading.Thread(target=server.serve_forever)
            thread.start()
            try:
                health = call_unix_socket(
                    socket_path, {"schema_version": 1, "op": "health"}
                )
                self.assertEqual(health["state"], "locked")
                unlocked = call_unix_socket(
                    socket_path,
                    {"schema_version": 1, "op": "unlock", "key_length": 32},
                    KEY,
                )
                self.assertEqual(unlocked["state"], "unlocked")
                self.assertTrue(os.stat(socket_path).st_mode & 0o777 == 0o600)
            finally:
                server.shutdown()
                server.server_close()
                thread.join()



if __name__ == "__main__":
    unittest.main()
