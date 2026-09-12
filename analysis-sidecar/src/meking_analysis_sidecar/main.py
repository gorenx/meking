"""Start a loopback-only Uvicorn server and emit exactly one ready record."""

from __future__ import annotations

import asyncio
import os
import socket
import sys

import uvicorn

from meking_analysis_sidecar.app import CAPABILITIES, create_app
from meking_analysis_sidecar.contract import ReadyMessage, TOKEN_ENVIRONMENT_NAME


async def serve() -> int:
    """Bind an ephemeral loopback port before Uvicorn starts accepting requests."""

    token = os.environ.get(TOKEN_ENVIRONMENT_NAME, "")
    if not token.strip():
        print("analysis bearer token is required", file=sys.stderr, flush=True)
        return 2
    listener = socket.socket(socket.AF_INET, socket.SOCK_STREAM)
    listener.setsockopt(socket.SOL_SOCKET, socket.SO_REUSEADDR, 1)
    listener.bind(("127.0.0.1", 0))
    listener.listen(2048)
    listener.setblocking(False)
    port = int(listener.getsockname()[1])

    config = uvicorn.Config(
        create_app(token),
        host="127.0.0.1",
        port=port,
        access_log=False,
        log_config=None,
        lifespan="on",
    )
    server = uvicorn.Server(config)
    server_task = asyncio.create_task(server.serve(sockets=[listener]))
    while not server.started:
        if server_task.done():
            return 2
        await asyncio.sleep(0.005)
    ready = ReadyMessage(
        port=port,
        pid=os.getpid(),
        capabilities=list(CAPABILITIES),
    )
    print(ready.model_dump_json(), flush=True)
    await server_task
    return 0


def main() -> None:
    """Run until the Go parent sends an interrupt or termination signal."""

    try:
        exit_code = asyncio.run(serve())
    except KeyboardInterrupt:
        exit_code = 0
    raise SystemExit(exit_code)


if __name__ == "__main__":
    main()
