# DCE Connector Catalog

Place declarative connector manifests (`.yaml` or `.json`) in this directory.

Each manifest declares a REST integration adhering to `apiVersion: memro.dev/v1` and `kind: Connector`.

### Manifest Schema Template

```yaml
apiVersion: memro.dev/v1
kind: Connector

metadata:
  name: tool-name               # Lowercase identifier (e.g. asana, datadog, airtable)
  version: 1.0.0
  description: Sync items from Tool Name

spec:
  type: rest
  baseURL: https://api.example.com

  auth:
    type: bearer                # bearer, apiKey, basic, none
    secretRef: TOOL_API_TOKEN   # Reference name for user-provided token
    # header: X-API-Key         # (Optional) For apiKey auth
    # prefix: "Bearer "         # (Optional) For custom authorization headers

  operations:
    - name: list_items
      method: GET
      path: /v1/items
      # queryParams:
      #   limit: "50"
      pagination:
        type: cursor            # cursor, page, offset, none
        cursorParam: after      # Query param passed to API
        cursorPath: next_cursor # JSONPath to cursor in response body
      response:
        itemsPath: data         # JSONPath to list of items

  mapping:
    id: $.id                    # JSONPath to unique item ID
    title: $.name               # Item title / summary
    content: $.description      # Item body / content text
    eventDate: $.created_at     # RFC3339 or ISO timestamp
    authorName: $.creator.name  # (Optional) Author / actor name
    url: $.permalink            # (Optional) Deep link back to external tool

  sync:
    mode: incremental           # incremental or full
    checkpoint: updated_at      # Field tracked for checkpointing

  policies:
    timeout: 15s
    maxRetries: 3
    rateLimitPerMin: 60
```
