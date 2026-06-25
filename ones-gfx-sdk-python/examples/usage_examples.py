"""
End-to-end usage examples for the ONES Spectrum-X SDK.

This file is intended to be read top-to-bottom by partners as a tour of
the SDK's surface. It covers:

    1. Constructing the client with JWT auth.
    2. Reading fabrics and tenants.
    3. Running a full tenant lifecycle in a single mode (sync/async-poll/async-webhook).
    4. Error handling.

How to run
----------
1. Edit the CONFIG section below with your ONES base URL, JWT tokens,
   and a fabric name that exists in your environment.

2. Choose which mode to run by passing ``--mode`` (sync, async-poll,
    async-webhook). The default is sync.

3. Run the script. Any of these work:

   a) Without installing — directly against the source tree:

          python examples/usage_examples.py       # from repo root
          python usage_examples.py                # from inside examples/
          python -m examples.usage_examples       # module style, from repo root

      The bootstrap block below adds the repo root to sys.path, so the
      ``ones_gfx`` package is importable without ``pip install``.

   b) After installing the SDK (editable or wheel):

          pip install -e .          # from repo root, once
          python examples/usage_examples.py
"""

from __future__ import annotations

# ---------------------------------------------------------------------------
# Bootstrap — make the ``ones_gfx`` package importable when this
# file is run directly from a source checkout, without ``pip install``.
#
# This locates the repo root (the directory two levels above this file:
# usage_example.py -> examples/ -> repo root) and prepends it to
# sys.path, so ``import ones_gfx`` resolves to the local source.
#
# The block is a no-op when the SDK is already installed, because the
# installed package takes precedence over a path-based import only when
# the path entry comes later in sys.path. We use ``insert(0, ...)`` so
# the local source wins during development; remove this block in
# production scripts that should use the installed package.
# ---------------------------------------------------------------------------
import os
import sys

_REPO_ROOT = os.path.abspath(os.path.join(os.path.dirname(__file__), os.pardir))
if _REPO_ROOT not in sys.path:
    sys.path.insert(0, _REPO_ROOT)
# ---------------------------------------------------------------------------

import argparse
import logging
import time
import warnings

from pathlib import Path

import requests

from urllib3.exceptions import InsecureRequestWarning

try:
	from dotenv import load_dotenv
except ImportError:
	def load_dotenv(path=None, verbose=False):
		"""Fallback if python-dotenv is not installed."""
		if path and os.path.exists(path):
			with open(path) as f:
				for line in f:
					line = line.strip()
					if line and not line.startswith("#"):
						if "=" in line:
							key, value = line.split("=", 1)
							if os.environ.get(key) is None:
								os.environ[key] = value

from ones_gfx import (
    AuthenticationError,
    ConflictError,
    JWTAuth,
    NotFoundError,
    ONESClient,
    ONESError,
    Operation,
    OperationMode,
    UNLIMITED_GPUS,
)

# ---------------------------------------------------------------------------
# CONFIG — loaded from .env file or environment variables.
# See ../../.env.example for available configuration options.
# ---------------------------------------------------------------------------

def load_config():
	"""Load configuration from .env file and environment variables."""
	# Try to load .env file from repo root
	env_file = Path("../.env")
	if not env_file.exists():
		env_file = Path("../../.env")
	
	if env_file.exists():
		load_dotenv(env_file, verbose=False)
	
	# Helper function to get env vars with defaults
	def get_env(key, default=None):
		value = os.environ.get(key, default)
		if value is None:
			raise ValueError(f"{key} environment variable not set (see ../../.env.example)")
		return value
	
	def get_bool_env(key, default=False):
		value = os.environ.get(key, str(default)).lower()
		return value in ("true", "1", "yes")
	
	def get_int_env(key, default=1200):
		try:
			return int(os.environ.get(key, default))
		except ValueError:
			return default
	
	try:
		config = {
			"BASE_URL": get_env("BASE_URL"),
			"REFRESH_URL": get_env("REFRESH_URL"),
			"ACCESS_TOKEN": get_env("ACCESS_TOKEN"),
			"REFRESH_TOKEN": get_env("REFRESH_TOKEN"),
			"LOGIN_USERNAME": get_env("LOGIN_USERNAME"),
			"LOGIN_PASSWORD": get_env("LOGIN_PASSWORD"),
			"FABRIC_NAME": get_env("FABRIC_NAME"),
			"WEBHOOK_URL": os.environ.get("WEBHOOK_URL", "http://your_webhook_endpoint:5000/test/webhook-receiver"),
			"VERIFY_TLS": get_bool_env("VERIFY_TLS", False),
			"DEFAULT_TIMEOUT_S": get_int_env("DEFAULT_TIMEOUT_S", 1200),
		}
		return config
	except ValueError as e:
		raise RuntimeError(f"Configuration error: {e}")

# Load configuration
try:
	_CONFIG = load_config()
	BASE_URL = _CONFIG["BASE_URL"]
	REFRESH_URL = _CONFIG["REFRESH_URL"]
	ACCESS_TOKEN = _CONFIG["ACCESS_TOKEN"]
	REFRESH_TOKEN = _CONFIG["REFRESH_TOKEN"]
	LOGIN_USERNAME = _CONFIG["LOGIN_USERNAME"]
	LOGIN_PASSWORD = _CONFIG["LOGIN_PASSWORD"]
	FABRIC_NAME = _CONFIG["FABRIC_NAME"]
	WEBHOOK_URL = _CONFIG["WEBHOOK_URL"]
	VERIFY_TLS = _CONFIG["VERIFY_TLS"]
	DEFAULT_TIMEOUT_S = _CONFIG["DEFAULT_TIMEOUT_S"]
except RuntimeError as e:
	print(f"Error: {e}")
	sys.exit(1)

# A couple of sample server hostnames you expect to be available in the
# fabric. The example will try to allocate then deallocate these.
SAMPLE_SERVERS = ["hgx-su00-h00"]

# Webhook receiver URL for the async-webhook example. The SDK does not
# implement the receiver — point this at an HTTP endpoint you control.
WEBHOOK_URL = "http://10.4.5.87:5000/test/webhook-receiver"

# Disable TLS verification only against dev/lab deployments with self-
# signed certs. In production, leave this as True or supply a CA path.
VERIFY_TLS: bool | str = False

# Sync operations can take several minutes (allocate/deallocate up to ~15 min).
DEFAULT_TIMEOUT_S = 1200

# Silence noisy TLS warnings when intentionally disabling verification.
if VERIFY_TLS is False:
    warnings.filterwarnings("ignore", category=InsecureRequestWarning)


# ---------------------------------------------------------------------------
# Auth callback (optional) — persist rotated tokens so they survive a
# process restart. Replace the print() with whatever your secret store
# requires.
# ---------------------------------------------------------------------------

def on_token_refresh(access: str, refresh: str, expires_in: int) -> None:
    print(f"[auth] tokens rotated; new access expires in {expires_in}s")
    # Example:
    #   keyring.set_password("ones", "access_token", access)
    #   keyring.set_password("ones", "refresh_token", refresh)


def build_client() -> ONESClient:
    """Construct an ONESClient with JWT auth."""
    auth = JWTAuth(
        access_token=ACCESS_TOKEN,
        refresh_token=REFRESH_TOKEN,
        refresh_url=REFRESH_URL,
        on_token_refresh=on_token_refresh,
        verify_tls=VERIFY_TLS,
    )
    return ONESClient(
        base_url=BASE_URL,
        auth=auth,
        verify_tls=VERIFY_TLS,
        timeout=DEFAULT_TIMEOUT_S,
    )


# ---------------------------------------------------------------------------
# Scenario 1: list fabrics and tenants.
# ---------------------------------------------------------------------------

def scenario_read_only(client: ONESClient) -> None:
    print("\n--- Scenario: read-only ---")

    fabrics = client.fabrics.list()
    print(f"Found {len(fabrics)} fabric(s):")
    for fab in fabrics:
        print(
            f"  - {fab.fabric_name} (SUs={fab.num_of_sus}/{fab.max_num_of_sus}, "
            f"default storage={fab.default_storage_name})"
        )

    tenants = client.tenants.list(FABRIC_NAME)
    print(f"\nTenants in {FABRIC_NAME}: {len(tenants)}")
    for t in tenants:
        gpu_summary = "unlimited" if t.is_unlimited_gpus else f"{t.gpus_allocated}/{t.max_gpus_allowed}"
        print(f"  - {t.name} | gpus={gpu_summary} | status={t.config_status.name}")

    available = client.tenants.available_servers(FABRIC_NAME)
    print(f"\nAvailable servers in {FABRIC_NAME}: {available}")


def scenario_login(username: str, password: str) -> None:
    print("\n--- Scenario: login ---")
    payload = {"username": username, "password": password}
    url = f"{BASE_URL.rstrip('/')}/login"
    response = requests.post(
        url,
        json=payload,
        timeout=DEFAULT_TIMEOUT_S,
        verify=VERIFY_TLS,
    )
    response.raise_for_status()
    print(response.json())


# ---------------------------------------------------------------------------
# Scenario 2: tenant lifecycle in a single mode.
# ---------------------------------------------------------------------------

def _poll_operation(
    client: ONESClient,
    operation_id: str,
    *,
    label: str,
    max_attempts: int = 360,
    poll_interval_s: int = 5,
) -> bool:
    print(f"  -> polling {label} operation: {operation_id}")
    for attempt in range(1, max_attempts + 1):
        current = client.operations.get(operation_id)
        if current.status in {"RUNNING", "PENDING"}:
            print(".", end="", flush=True)
        else:
            print(f"\n  [poll {attempt}] status={current.status}")
        if current.is_done:
            print("")
            if current.is_success:
                return True
            print(f"  -> FAILED: {current.error_message}")
            return False
        time.sleep(poll_interval_s)
    print("  -> still not done after polling window; giving up")
    return False


def _poll_webhook_status(
    client: ONESClient,
    operation_id: str,
    *,
    label: str,
    max_attempts: int = 360,
    poll_interval_s: int = 5,
) -> bool:
    """Poll the webhook delivery status endpoint until SUCCESS or failure."""
    path = f"operations/{operation_id}/webhook-status"
    print(f"  -> polling {label} webhook delivery: {operation_id}")
    for attempt in range(1, max_attempts + 1):
        result = client._transport.get(path)
        status = result.get("deliveryStatus", "PENDING") if isinstance(result, dict) else "PENDING"
        if status == "SUCCESS":
            print(f"\n  [poll {attempt}] webhook deliveryStatus={status}")
            return True
        if status in {"FAILED", "EXHAUSTED"}:
            print(f"\n  [poll {attempt}] webhook deliveryStatus={status}")
            print(f"  -> FAILED: webhook delivery did not succeed")
            return False
        print(".", end="", flush=True)
        time.sleep(poll_interval_s)
    print("\n  -> still not delivered after polling window; giving up")
    return False


def _tenant_name_for_mode(mode: OperationMode) -> str:
    if mode is OperationMode.SYNCHRONOUS:
        return "sdk_sync"
    if mode is OperationMode.ASYNC_POLL:
        return "sdk_async"
    return "sdk_hook"


def _mode_label(mode: OperationMode) -> str:
    if mode is OperationMode.SYNCHRONOUS:
        return "sync"
    if mode is OperationMode.ASYNC_POLL:
        return "async-poll"
    return "async-webhook"


def _default_vpc_name(tenant_name: str) -> str:
    return f"{tenant_name}-{FABRIC_NAME}-north-south"


def _default_peer_vpc_name() -> str:
    return f"{FABRIC_NAME}-Storage-VPC"


def scenario_tenant_lifecycle(client: ONESClient, mode: OperationMode) -> None:
    label = _mode_label(mode)

    print(f"\n--- Scenario: tenant lifecycle ({label}) ---")

    tenant_name = _tenant_name_for_mode(mode)

    print(f"Creating tenant {tenant_name!r} ({label})...")
    create_result = client.tenants.create(
        fabric_name=FABRIC_NAME,
        name=tenant_name,
        description=f"Created by SDK example ({label} mode)",
        max_gpus_allowed=8,
        mode=mode,
        webhook_url=WEBHOOK_URL if mode is OperationMode.ASYNC_WEBHOOK else None,
        webhook_events=["tenant.create"] if mode is OperationMode.ASYNC_WEBHOOK else None,
    )

    if isinstance(create_result, Operation):
        print(f"  -> operation id: {create_result.id}")
        if mode is OperationMode.ASYNC_WEBHOOK:
            print(f"  -> webhook registered: {create_result.webhook_registered}")
        poll_fn = _poll_webhook_status if mode is OperationMode.ASYNC_WEBHOOK else _poll_operation
        if not poll_fn(client, create_result.id, label="create"):
            return
    else:
        print(f"  -> id={create_result.id}, vlan={create_result.vlan_id}, vni={create_result.vni_id}")

    if SAMPLE_SERVERS:
        print(f"Allocating GPUs {SAMPLE_SERVERS} to {tenant_name} ({label})...")
        allocate_result = client.tenants.allocate_gpus(
            fabric_name=FABRIC_NAME,
            name=tenant_name,
            servers=SAMPLE_SERVERS,
            timeout=500,
            mode=mode,
            webhook_url=WEBHOOK_URL if mode is OperationMode.ASYNC_WEBHOOK else None,
            webhook_events=["tenant.allocate"] if mode is OperationMode.ASYNC_WEBHOOK else None,
        )
        if isinstance(allocate_result, Operation):
            print(f"  -> operation id: {allocate_result.id}")
            poll_fn = _poll_webhook_status if mode is OperationMode.ASYNC_WEBHOOK else _poll_operation
            if not poll_fn(client, allocate_result.id, label="allocate"):
                return
        else:
            print("  -> allocate done")

        refreshed = client.tenants.get(FABRIC_NAME, tenant_name)
        print(f"  Tenant now has servers: {refreshed.alloted_servers}")

        # Peering after allocate so tenant VPC / NS path is live with GPUs attached.
        peering_name = f"{tenant_name}-storage-route-leak"
        vpc_name = _default_vpc_name(tenant_name)
        peer_vpc_name = _default_peer_vpc_name()
        print(
            f"Creating VPC peering {peering_name!r} between {vpc_name!r} and {peer_vpc_name!r} (sync-only)..."
        )
        peering_result = client.peering.create(
            fabric_name=FABRIC_NAME,
            name=peering_name,
            vpc_name=vpc_name,
            peer_vpc_name=peer_vpc_name,
        )
        print(f"  -> response: {peering_result}")

        print(f"Deallocating GPUs {SAMPLE_SERVERS} ({label})...")
        deallocate_result = client.tenants.deallocate_gpus(
            fabric_name=FABRIC_NAME,
            name=tenant_name,
            servers=SAMPLE_SERVERS,
            timeout=500,
            mode=mode,
            webhook_url=WEBHOOK_URL if mode is OperationMode.ASYNC_WEBHOOK else None,
            webhook_events=["tenant.deallocate"] if mode is OperationMode.ASYNC_WEBHOOK else None,
        )
        if isinstance(deallocate_result, Operation):
            print(f"  -> operation id: {deallocate_result.id}")
            poll_fn = _poll_webhook_status if mode is OperationMode.ASYNC_WEBHOOK else _poll_operation
            if not poll_fn(client, deallocate_result.id, label="deallocate"):
                return
        else:
            print("  -> deallocate done")

        # --- Partial GPU allocation (fine-grained suid addressing) ---
        print(f"Modifying GPU allocations on {tenant_name} ({label})...")
        # This is more granular than allocate_gpus: targets specific GPU IDs
        # on a specific server rather than allocating all GPUs on a server.
        gpu_alloc_result = client.fabrics.modify_gpu_allocations(
            fabric_name=FABRIC_NAME,
            tenant_name=tenant_name,
            operation="ADD",
            suid={
                "0": {
                    "hgx-su00-h00": {"gpus": ["G0", "G1", "G2", "G3"]},
                }
            },
        )
        print(f"  -> status: {gpu_alloc_result.get('status')}")
        if gpu_alloc_result.get("operationId"):
            print(f"  -> operation id: {gpu_alloc_result['operationId']} (async)")
        if gpu_alloc_result.get("message"):
            print(f"  -> message: {gpu_alloc_result['message']}")

    print(f"Deleting tenant {tenant_name!r} ({label})...")
    delete_result = client.tenants.delete(
        fabric_name=FABRIC_NAME,
        name=tenant_name,
        mode=mode,
        webhook_url=WEBHOOK_URL if mode is OperationMode.ASYNC_WEBHOOK else None,
        webhook_events=["tenant.delete"] if mode is OperationMode.ASYNC_WEBHOOK else None,
    )
    if isinstance(delete_result, Operation):
        print(f"  -> operation id: {delete_result.id}")
        poll_fn = _poll_webhook_status if mode is OperationMode.ASYNC_WEBHOOK else _poll_operation
        poll_fn(client, delete_result.id, label="delete")
    else:
        print("  -> delete done")


def scenario_gpu_allocations(
    client: ONESClient,
    tenant_name: str,
    operation: str,
    server_index: str,
    hostname: str,
    gpu_ids: list[str],
) -> None:
    """Call POST /fabrics/{fabric}/tenants/{tenant}/gpuAllocations directly."""
    print("\n--- Scenario: gpu-allocations ---")
    print(f"  fabric:   {FABRIC_NAME}")
    print(f"  tenant:   {tenant_name}")
    print(f"  op:       {operation}")
    print(f"  suid[{server_index}][{hostname}].gpus: {gpu_ids}")

    try:
        result = client.fabrics.modify_gpu_allocations(
            fabric_name=FABRIC_NAME,
            tenant_name=tenant_name,
            operation=operation,
            suid={
                server_index: {
                    hostname: {"gpus": gpu_ids},
                }
            },
        )
        print(f"  -> status: {result.get('status')}")
        if result.get("operationId"):
            print(f"  -> operation id: {result['operationId']}")
        if result.get("message"):
            print(f"  -> message: {result['message']}")
    except ONESError as e:
        _report_sdk_error(e)


def _parse_servers(raw: str | None) -> list[str]:
    if not raw:
        return []
    return [item.strip() for item in raw.split(",") if item.strip()]


def _parse_bool_flag(raw: str | bool) -> bool:
    if isinstance(raw, bool):
        return raw
    val = str(raw).strip().lower()
    if val in {"1", "true", "t", "yes", "y", "on"}:
        return True
    if val in {"0", "false", "f", "no", "n", "off"}:
        return False
    raise argparse.ArgumentTypeError(f"invalid boolean value: {raw!r}")


def _report_sdk_error(err: ONESError) -> None:
    status_code = getattr(err, "status_code", None)
    status = f" (status={status_code})" if status_code is not None else ""
    print(f"[SDK error] {err}{status}")


def scenario_tenant_action(
    client: ONESClient,
    mode: OperationMode,
    action: str,
    tenant_name: str | None,
    servers: list[str] | None,
    shared_server: bool,
    peering_name: str | None,
    vpc_name: str | None,
    peer_vpc_name: str | None,
) -> None:
    label = _mode_label(mode)
    tenant_name = tenant_name or _tenant_name_for_mode(mode)
    servers = servers or SAMPLE_SERVERS

    if action == "read-only":
        scenario_read_only(client)
        return

    print(f"\n--- Scenario: tenant {action} ({label}) ---")

    try:
        if action == "create":
            print(f"Creating tenant {tenant_name!r} ({label})...")
            create_result = client.tenants.create(
                fabric_name=FABRIC_NAME,
                name=tenant_name,
                description=f"Created by SDK example ({label} mode)",
                max_gpus_allowed=8,
                mode=mode,
                webhook_url=WEBHOOK_URL if mode is OperationMode.ASYNC_WEBHOOK else None,
                webhook_events=["tenant.create"] if mode is OperationMode.ASYNC_WEBHOOK else None,
            )

            if isinstance(create_result, Operation):
                print(f"  -> operation id: {create_result.id}")
                if mode is OperationMode.ASYNC_WEBHOOK:
                    print(f"  -> webhook registered: {create_result.webhook_registered}")
                poll_fn = _poll_webhook_status if mode is OperationMode.ASYNC_WEBHOOK else _poll_operation
                poll_fn(client, create_result.id, label="create")
            else:
                print(f"  -> id={create_result.id}, vlan={create_result.vlan_id}, vni={create_result.vni_id}")
            return

        if action == "allocate":
            if not servers:
                raise ValueError("No servers provided for allocate. Use --servers or update SAMPLE_SERVERS.")
            server_specs = [{"serverName": host, "shared": shared_server} for host in servers]
            print(f"Allocating GPUs {servers} to {tenant_name} ({label}, shared={shared_server})...")
            allocate_result = client.tenants.allocate_gpus(
                fabric_name=FABRIC_NAME,
                name=tenant_name,
                servers=server_specs,
                timeout=500,
                mode=mode,
                webhook_url=WEBHOOK_URL if mode is OperationMode.ASYNC_WEBHOOK else None,
                webhook_events=["tenant.allocate"] if mode is OperationMode.ASYNC_WEBHOOK else None,
            )
            if isinstance(allocate_result, Operation):
                print(f"  -> operation id: {allocate_result.id}")
                poll_fn = _poll_webhook_status if mode is OperationMode.ASYNC_WEBHOOK else _poll_operation
                poll_fn(client, allocate_result.id, label="allocate")
            else:
                print("  -> allocate done")
            return

        if action == "deallocate":
            if not servers:
                raise ValueError("No servers provided for deallocate. Use --servers or update SAMPLE_SERVERS.")
            server_specs = [{"serverName": host, "shared": shared_server} for host in servers]
            print(f"Deallocating GPUs {servers} ({label}, shared={shared_server})...")
            deallocate_result = client.tenants.deallocate_gpus(
                fabric_name=FABRIC_NAME,
                name=tenant_name,
                servers=server_specs,
                timeout=500,
                mode=mode,
                webhook_url=WEBHOOK_URL if mode is OperationMode.ASYNC_WEBHOOK else None,
                webhook_events=["tenant.deallocate"] if mode is OperationMode.ASYNC_WEBHOOK else None,
            )
            if isinstance(deallocate_result, Operation):
                print(f"  -> operation id: {deallocate_result.id}")
                poll_fn = _poll_webhook_status if mode is OperationMode.ASYNC_WEBHOOK else _poll_operation
                poll_fn(client, deallocate_result.id, label="deallocate")
            else:
                print("  -> deallocate done")
            return

        if action == "delete":
            print(f"Deleting tenant {tenant_name!r} ({label})...")
            delete_result = client.tenants.delete(
                fabric_name=FABRIC_NAME,
                name=tenant_name,
                mode=mode,
                webhook_url=WEBHOOK_URL if mode is OperationMode.ASYNC_WEBHOOK else None,
                webhook_events=["tenant.delete"] if mode is OperationMode.ASYNC_WEBHOOK else None,
            )
            if isinstance(delete_result, Operation):
                print(f"  -> operation id: {delete_result.id}")
                poll_fn = _poll_webhook_status if mode is OperationMode.ASYNC_WEBHOOK else _poll_operation
                poll_fn(client, delete_result.id, label="delete")
            else:
                print("  -> delete done")
            return

        if action == "vpcpeering":
            if mode is not OperationMode.SYNCHRONOUS:
                print("VPC peering is supported only in sync mode.")
                return
            vpc_name = vpc_name or _default_vpc_name(tenant_name)
            peer_vpc_name = peer_vpc_name or _default_peer_vpc_name()
            peering_name = peering_name or f"{tenant_name}-storage-route-leak"
            print(
                f"Creating VPC peering {peering_name!r} between {vpc_name!r} and {peer_vpc_name!r}..."
            )
            result = client.peering.create(
                fabric_name=FABRIC_NAME,
                name=peering_name,
                vpc_name=vpc_name,
                peer_vpc_name=peer_vpc_name,
            )
            print(f"  -> response: {result}")
            return
    except ONESError as e:
        _report_sdk_error(e)
        return


# ---------------------------------------------------------------------------
# Scenario 5: error handling.
# ---------------------------------------------------------------------------

def scenario_error_handling(client: ONESClient) -> None:
    print("\n--- Scenario: error handling ---")

    # Invalid input — caught client-side before any HTTP call.
    try:
        client.tenants.create(
            fabric_name=FABRIC_NAME,
            name="bad_quota_demo",
            description="will not be sent",
            max_gpus_allowed=0,  # disallowed: must be -1 or >= 1
        )
    except ValueError as e:
        print(f"[client-side validation] {e}")

    # Server-side 404 — looking up a tenant that doesn't exist.
    try:
        client.tenants.get(FABRIC_NAME, "this_tenant_does_not_exist_xyz")
    except NotFoundError as e:
        print(f"[NotFoundError] {e} (status={e.status_code})")
    except ONESError as e:
        # Catch-all if the server returns a different shape than expected.
        print(f"[ONESError] {e}")


# ---------------------------------------------------------------------------
# Entry point.
# ---------------------------------------------------------------------------

def main() -> None:
    # Turn on debug logging if you want to see the SDK's internal HTTP
    # activity (token refreshes, retry-on-401, etc.).
    # logging.basicConfig(level=logging.DEBUG)
    logging.basicConfig(level=logging.INFO)

    parser = argparse.ArgumentParser(description="ONES Spectrum-X SDK examples")
    parser.add_argument(
        "--mode",
        default="sync",
        choices=["sync", "async-poll", "async-webhook"],
        help="Tenant lifecycle mode to run (default: sync)",
    )
    parser.add_argument(
        "--action",
        default="lifecycle",
        choices=[
            "lifecycle",
            "read-only",
            "login",
            "create",
            "allocate",
            "deallocate",
            "delete",
            "vpcpeering",
            "gpu-allocations",
        ],
        help="Action to run (default: lifecycle)",
    )
    parser.add_argument(
        "--tenant-name",
        default=None,
        help="Override tenant name for create/delete/allocate/deallocate",
    )
    parser.add_argument(
        "--username",
        default=None,
        help="Username for login action (default: LOGIN_USERNAME)",
    )
    parser.add_argument(
        "--password",
        default=None,
        help="Password for login action (default: LOGIN_PASSWORD)",
    )
    parser.add_argument(
        "--servers",
        default=None,
        help="Comma-separated server list for allocate/deallocate",
    )
    parser.add_argument(
        "--shared",
        nargs="?",
        const=True,
        default=False,
        type=_parse_bool_flag,
        help="Set shared on allocate/deallocate server specs (e.g. --shared=true)",
    )
    parser.add_argument(
        "--peering-name",
        default=None,
        help="Peering name for vpcpeering (default: <tenant>-storage-route-leak)",
    )
    parser.add_argument(
        "--vpc-name",
        default=None,
        help="Tenant VPC name for vpcpeering (default: <tenant>-<fabric>-north-south)",
    )
    parser.add_argument(
        "--peer-vpc-name",
        default=None,
        help="Peer VPC name for vpcpeering (default: <fabric>-Storage-VPC)",
    )
    parser.add_argument(
        "--gpu-operation",
        default="ADD",
        choices=["ADD", "DELETE"],
        help="GPU allocation operation: ADD or DELETE (used with --action gpu-allocations)",
    )
    parser.add_argument(
        "--gpu-hostname",
        default=None,
        help="Compute node hostname for gpu-allocations (e.g. hgx-su00-h00)",
    )
    parser.add_argument(
        "--gpu-ids",
        default="G0,G1,G2,G3",
        help="Comma-separated GPU IDs for gpu-allocations (e.g. G0,G1,G2,G3)",
    )
    parser.add_argument(
        "--gpu-server-index",
        default="0",
        help="Server index key in suid map (default: 0)",
    )
    args = parser.parse_args()

    mode_map = {
        "sync": OperationMode.SYNCHRONOUS,
        "async-poll": OperationMode.ASYNC_POLL,
        "async-webhook": OperationMode.ASYNC_WEBHOOK,
    }

    # The ``with`` block ensures the underlying HTTP session is closed
    # cleanly even if a scenario raises.
    try:
        if args.action == "login":
            username = args.username or LOGIN_USERNAME
            password = args.password or LOGIN_PASSWORD
            scenario_login(username, password)
            return

        with build_client() as client:
            if args.action == "lifecycle":
                scenario_read_only(client)
                scenario_tenant_lifecycle(client, mode_map[args.mode])
                scenario_error_handling(client)
            elif args.action == "gpu-allocations":
                if not args.tenant_name:
                    parser.error("--tenant-name is required for gpu-allocations")
                hostname = args.gpu_hostname or (SAMPLE_SERVERS[0] if SAMPLE_SERVERS else None)
                if not hostname:
                    parser.error("--gpu-hostname is required (or set SAMPLE_SERVERS in config)")
                scenario_gpu_allocations(
                    client,
                    args.tenant_name,
                    args.gpu_operation,
                    args.gpu_server_index,
                    hostname,
                    _parse_servers(args.gpu_ids),
                )
            else:
                scenario_tenant_action(
                    client,
                    mode_map[args.mode],
                    args.action,
                    args.tenant_name,
                    _parse_servers(args.servers),
                    args.shared,
                    args.peering_name,
                    args.vpc_name,
                    args.peer_vpc_name,
                )
    except AuthenticationError as e:
        print(f"\nAuth failed: {e}")
        print("Update ACCESS_TOKEN and REFRESH_TOKEN at the top of this file.")
    except ONESError as e:
        _report_sdk_error(e)


if __name__ == "__main__":
    main()
