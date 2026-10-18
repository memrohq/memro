#!/usr/bin/env node

const { EventSource } = require("eventsource");
const axios = require("axios");
const { spawn } = require("child_process");

// This is a minimal SSE-to-stdio bridge
// Usage: node bridge.js <SSE_URL>

const sseUrl = process.argv[2];
if (!sseUrl) {
  console.error("Usage: node bridge.js <SSE_URL>");
  process.exit(1);
}

const es = new EventSource(sseUrl);

es.onmessage = (event) => {
  // Pass through from SSE to stdout
  process.stdout.write(event.data + "\n");
};

es.onerror = (err) => {
  console.error("SSE Error:", err);
};

process.stdin.on("data", async (data) => {
  // Pass through from stdin to SSE messages endpoint
  // We need to parse the message to find the endpoint or use a standard /messages/
  const baseUrl = sseUrl.substring(0, sseUrl.lastIndexOf("/"));
  const messagesUrl = baseUrl + "/messages/";
  
  try {
    await axios.post(messagesUrl, data.toString(), {
      headers: { "Content-Type": "application/json" }
    });
  } catch (err) {
    console.error("Failed to post message:", err.message);
  }
});
