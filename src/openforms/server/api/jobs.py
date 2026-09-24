"""``/jobs`` (admin only): list and retry background jobs."""

from __future__ import annotations

from fastapi import APIRouter, Depends
from starlette.requests import Request
from starlette.responses import Response

from ..exceptions import JobNotFound
from ..services.jobs import STATUS_DONE, STATUS_FAILED, STATUS_PENDING, STATUS_RUNNING, Job, Queue
from .context import get_ctx
from .dependencies import require_admin
from .errors import bad_request
from .serialize import ts

router = APIRouter(prefix="/jobs", dependencies=[Depends(require_admin)])

VALID_STATUS = {"", STATUS_PENDING, STATUS_RUNNING, STATUS_DONE, STATUS_FAILED}


def queue(request: Request) -> Queue:
    return get_ctx(request).services["queue"]


def job_json(j: Job) -> dict:
    return {
        "id": j.id,
        "kind": j.kind,
        "status": j.status,
        "attempts": j.attempts,
        "maxAttempts": j.max_attempts,
        "runAt": ts(j.run_at),
        "lastError": j.last_error,
        "payload": j.payload,
        "createdAt": ts(j.created_at),
        "updatedAt": ts(j.updated_at),
    }


@router.get("")
async def list_jobs(request: Request) -> dict:
    status = request.query_params.get("status", "")
    if status not in VALID_STATUS:
        raise bad_request("status must be one of pending, running, done, failed")
    limit = 100
    if (raw := request.query_params.get("limit", "")) != "":
        try:
            limit = int(raw)
        except ValueError:
            limit = 0
        if not 1 <= limit <= 500:
            raise bad_request("limit must be between 1 and 500")
    jobs = await queue(request).list(get_ctx(request).org_id, status, limit)
    return {"items": [job_json(j) for j in jobs]}


@router.post("/{job_id}/retry")
async def retry(job_id: str, request: Request) -> Response:
    try:
        jid = int(job_id)
    except ValueError:
        raise JobNotFound() from None
    await queue(request).retry(get_ctx(request).org_id, jid)
    return Response(status_code=204)
