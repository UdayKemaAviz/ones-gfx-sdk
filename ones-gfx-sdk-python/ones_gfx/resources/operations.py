from __future__ import annotations

from ..models import Operation
from ..transport import Transport


class OperationsResource:
    def __init__(self, transport: Transport):
        self._transport = transport

    def get(self, operation_id: str) -> Operation:
        if not operation_id or not isinstance(operation_id, str):
            raise ValueError("operation_id must be a non-empty string.")
        body = self._transport.get(f"operations/{operation_id}")
        return Operation.from_polled(body)
