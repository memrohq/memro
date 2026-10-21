# Contributing a Connector to Memro

Memro connects to external developer tools, productivity platforms, and knowledge bases to give AI agents persistent memory. 

You can add support for any REST API service in **under 15 minutes** using Memro's **Declarative Connector Engine (DCE)** without writing Go code!

---

## How DCE Works

Every declarative connector is defined as a single YAML manifest stored in:
```
server/internal/connectors/dce/catalog/<service_name>.yaml
```

The manifest defines:
1. **Metadata**: Display name, icon category, description, and link to docs.
2. **Auth & Verification**: How API tokens are sent (`bearer`, `apiKey`, or `basic`) and an endpoint to verify credentials.
3. **Data Ingestion**: API endpoint, pagination rules, and incremental checkpointing.
4. **Memory Mapping**: How API response fields are extracted into agent memories.

---

## Step-by-Step Guide

### 1. Pick a Service or Check Open Issues
Check our [Good First Issues](https://github.com/memrohq/memro/issues?q=is%3Aissue+is%3Aopen+label%3Aconnector) for requested connectors, and check `server/internal/connectors/dce/catalog/` so you don't duplicate one that already exists. Native OAuth connectors (Slack, GitHub, Linear, Sentry, Notion, Jira, GitLab) live in Go and are not YAML.

### 2. Create the YAML Manifest
Create `server/internal/connectors/dce/catalog/<your-service>.yaml`. `metadata.name` must match the file name (e.g. `example_service.yaml` → `name: example_service`).

Here is an example:

```yaml
apiVersion: memro.dev/v1
kind: Connector
metadata:
  name: example_service
  version: "1"
  displayName: Example Service
  category: work # engineering | knowledge | work | communication
  description: Sync notes, tasks, or documents into persistent agent memory.
  docsURL: https://example.com/api-docs/tokens
spec:
  type: rest
  baseURL: https://api.example.com/v1
  fields:
    - name: token
      label: API Token
      secret: true
      help: "Account Settings → API Tokens → Generate New Token."
  auth:
    type: bearer
    secretRef: token
  verify:
    path: /me
    accountId: id
    accountName: username
  operations:
    - name: items
      method: GET
      path: /items
      queryParams:
        since: "{{checkpoint|or:2000-01-01T00:00:00Z}}"
      pagination:
        type: nextURL # or 'page', 'offset', 'cursor'
        cursorPath: next_page_url
      response:
        itemsPath: data
  mapping:
    id: id
    title: "{{title}}"
    content: |
      Item: {{title}}
      Description: {{body}}
      Tags: {{tags[]}}
    eventDate: updated_at
    url: web_url
  sync:
    mode: incremental
    checkpoint: updated_at
  policies:
    timeout: 30s
    maxRetries: 3
```

### 3. Run the Catalog Test
Memro automatically validates all manifests against structure and execution constraints:

```bash
cd server
go test -v ./internal/connectors/dce/... -run TestCatalog
```

To exercise your manifest against a fake API, add a case to `TestCatalogConnectorsAgainstFakeAPIs` in `catalog_contract_test.go`.

```bash
go test ./internal/connectors/dce/...
```

The test runner will verify:
- Required metadata fields and help strings exist.
- Auth configurations and URL templates parse correctly.
- Mappings and pagination rules resolve valid shapes.

### 4. Submit Your Pull Request
1. Fork the repo and push your branch: `git checkout -b connector/<service-name>`.
2. Commit your new YAML file: `git commit -m "feat(connectors): add <service-name> declarative connector"`.
3. Open a Pull Request on GitHub.
4. Sign the CLA via the automated PR bot.

We aim to review connector PRs within a few days.
