import asyncio
import sys
from mcp.server import Server
from mcp.server.stdio import stdio_server
from mcp.types import TextContent

server = Server("test-minimal")

@server.list_tools()
async def list_tools():
    return []

@server.call_tool()
async def call_tool(name, arguments):
    return [TextContent(type="text", text="ok")]

async def main():
    async with stdio_server() as (read_stream, write_stream):
        await server.run(
            read_stream,
            write_stream,
            server.create_initialization_options()
        )

if __name__ == "__main__":
    asyncio.run(main())
