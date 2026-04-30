"""
Main client entry point for the ONES Spectrum-X SDK.
"""

from __future__ import annotations

from .auth import AuthBase
from .resources.fabrics import FabricsResource
from .resources.operations import OperationsResource
from .resources.peering import PeeringResource
from .resources.tenants import TenantsResource
from .transport import Transport


class ONESClient:
    def __init__(
        self,
        base_url: str,
        auth: AuthBase,
        verify_tls: bool | str = True,
        timeout: int = 30,
    ):
        self._transport = Transport(
            base_url=base_url,
            auth=auth,
            verify_tls=verify_tls,
            timeout=timeout,
        )
        self.fabrics = FabricsResource(self._transport)
        self.tenants = TenantsResource(self._transport)
        self.operations = OperationsResource(self._transport)
        self.peering = PeeringResource(self._transport)

    def close(self) -> None:
        self._transport.close()

    def __enter__(self) -> "ONESClient":
        return self

    def __exit__(self, exc_type, exc_val, exc_tb) -> None:
        self.close()
