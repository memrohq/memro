# 0030. Organisation Owners Can Read Their Users' Memories
Date: 2026-11-14 · Status: Accepted · Changes 0026

## Context
ADR 0026 let an organisation's members see usage per end user but never end
users' memories: content was reachable only through the customer's own app
with its server key. In practice the customer, e.g. Vaultnel, needs to see
what its users' assistants learned, for support, debugging and checking
quality. So far that has meant asking us, or writing their own tooling.

## Decision
- **Owners and admins can read an end user's memories and graph**, read-only:
  - `GET /tenant/{id}/end-users/{agent_id}/memories?limit=` (newest 100 at
    most)
  - `GET /tenant/{id}/end-users/{agent_id}/graph?limit=`
  - In the dashboard: Organization → Usage per user → click a user.
- **Billing and viewer members still see usage only.**
- **Every view is logged** in the organisation's own audit log as
  `end_user.memories.view` or `end_user.graph.view`, naming the user and the
  viewer. The customer can show its users who looked, and Memro staff see it
  on the organisation's detail page.
- Only the organisation's own end users can be opened; anyone else gets
  `404`. Deleted users' memories are gone, so there is nothing to show.
- Nothing can be changed or deleted from this view. Users' data is still
  written only through the customer's app, and erased with `DELETE /end-user`.

## Consequences
- **Customers must tell their users.** Vaultnel's privacy policy has to say
  that Vaultnel's team can read members' Memro memories, and why.
- Memro staff still have no content view of their own (ADR 0029). Support
  questions about content go through the customer.
