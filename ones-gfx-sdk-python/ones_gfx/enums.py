from enum import Enum


class OperationMode(str, Enum):
    SYNCHRONOUS = "sync"
    ASYNC_POLL = "async_poll"
    ASYNC_WEBHOOK = "async_webhook"


class OperationStatus(str, Enum):
    PENDING = "PENDING"
    RUNNING = "RUNNING"
    SUCCESS = "SUCCESS"
    FAILURE = "FAILURE"

    @classmethod
    def is_terminal(cls, status: str) -> bool:
        return status in (cls.SUCCESS.value, cls.FAILURE.value)


class ConfigStatus(int, Enum):
    FAIL = 0
    PASS = 1
    NOT_STARTED = 2
    IN_PROGRESS = 3

    @classmethod
    def from_api(cls, value):
        try:
            return cls(int(value))
        except (ValueError, TypeError):
            return cls.NOT_STARTED


UNLIMITED_GPUS = -1
