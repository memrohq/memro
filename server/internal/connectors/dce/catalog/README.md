# Connector catalog

One YAML manifest per token connector, embedded in the server binary
([ADR 0014](../../../../../docs/decisions/0014-declarative-connector-engine.md)).
The file name is the connector's name. `TestCatalogLoads` and
`TestCatalogConnectorsAgainstFakeAPIs` check every manifest; add contract
tests (`catalog_contract_test.go`) for anything unusual about a service.

Write a manifest from the service's API documentation, not from memory:
the auth header, pagination, response paths and an incremental filter if the
service has one.

```yaml
apiVersion: memro.dev/v1
kind: Connector
metadata:
  name: zendesk                     # = file name
  version: "1"
  displayName: Zendesk
  category: communication           # engineering | knowledge | work | communication
  description: Support tickets with their status, priority, tags and description.
  docsURL: https://…                # where the user creates a token (https)
spec:
  type: rest
  baseURL: https://{{config.subdomain}}.zendesk.com/api/v2   # https only
  fields:                           # the connect form
    - {name: subdomain, label: Subdomain, pattern: "[A-Za-z0-9][A-Za-z0-9-]{0,62}"}  # host fields need a pattern
    - {name: email, label: Agent email}
    - {name: token, label: API token, secret: true, help: "Where to create it."}
  auth: {type: basic, secretRef: token, username: "{{config.email}}/token", password: "{{config.token}}"}
  # auth types: bearer (prefix), apiKey (header, prefix), basic (username/password templates), query (header = parameter name)
  headers: {X-Api-Version: "2"}     # optional, on every request
  verify: {path: /users/me.json, accountId: user.id, accountName: user.name}   # required: proves the token works
  sources:                          # optional: things the user chooses (projects, boards); without it, one source "all"
    operation: {name: projects, method: GET, path: /projects, response: {itemsPath: projects}}
    id: id
    name: name
    where: {archived: ["false"]}    # optional filter
  operations:                       # run in order each pass; {{source.id}} is the chosen source
    - name: tickets
      method: GET                   # or POST with body: '{"cursor":{{cursor|json}}}'
      path: /tickets.json
      queryParams: {sort: -updated_at, updated_since: "{{checkpoint}}"}   # a parameter with an empty value is left out
      pagination: {type: cursor, cursorParam: "page[after]", cursorPath: meta.after_cursor}
      # cursor | nextURL (cursorPath) | linkHeader | page (pageParam, zeroBased) | offset (offsetParam) | none
      # limit + limitParam send the page size; pageParam/offsetParam "-" put {{page}}/{{offset}} in the body only
      response: {itemsPath: tickets}  # "$" when the response is the array
  mapping:                          # paths or templates over one item
    id: id
    title: "#{{id}} {{subject}}"
    content: |
      Status: {{status}}
      Tags: {{tags[]}}
      {{#each comments}}- {{author.name}}: {{body|text}}
      {{/each}}
    memoryType: episodic            # semantic (default) | episodic
    eventDate: updated_at
    url: html_url
  sync:
    mode: incremental
    checkpoint: updated_at          # highest value seen becomes {{checkpoint}} for the next pass
    newestFirst: true               # stop paging at the checkpoint (less overlap)
    overlap: 24h                    # default 1h
  policies: {timeout: 30s, maxRetries: 3}   # timeout per request
```

Filters: `|text` (HTML to text), `|date` (YYYY-MM-DD), `|unix`, `|or:default`,
`|kv` (an object as sorted `key: value` lines), `|list` (one `- ` line per
element), `|json` (in bodies: a JSON value, `null` when empty). Lines that
end up as a bare label (`Assignee:`) are dropped.
