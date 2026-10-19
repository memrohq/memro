"""Clients learn when to use Memro from the server itself."""
import os

os.environ["MCP_TRANSPORT"] = "sse"

import anyio

import memro_mcp.server as srv


def test_initialize_carries_instructions():
    opts = srv.server.create_initialization_options()
    assert opts.instructions == srv.INSTRUCTIONS
    for must in ("recall", "remember", "secrets"):
        assert must in srv.INSTRUCTIONS


def test_a_client_receives_the_instructions():
    from mcp.client.session import ClientSession

    # The in-memory transport runs the real initialize handshake.
    async def handshake():
        from mcp.shared.memory import create_client_server_memory_streams
        async with create_client_server_memory_streams() as (client_streams, server_streams):
            async with anyio.create_task_group() as tg:
                tg.start_soon(lambda: srv.server.run(server_streams[0], server_streams[1],
                                                     srv.server.create_initialization_options(), raise_exceptions=True))
                async with ClientSession(client_streams[0], client_streams[1]) as session:
                    result = await session.initialize()
                tg.cancel_scope.cancel()
                return result

    result = anyio.run(handshake)
    assert result.instructions == srv.INSTRUCTIONS
    assert result.serverInfo.name == "memro-memory"
