package connectors

import (
	"context"
	"encoding/json"
	"regexp"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/memrohq/memro/server/internal/domain"
)

var (
	// Matches ticket keys like ENG-123, JIRA-456, CORE-99
	reTicketKey = regexp.MustCompile(`\b([A-Z][A-Z0-9]+-\d+)\b`)

	// Matches GitHub issue/PR shorthands: #42 or acme/repo#42
	reGitHubRef = regexp.MustCompile(`(?:\b([a-zA-Z0-9_.-]+/[a-zA-Z0-9_.-]+))?#(\d+)\b`)

	// Matches GitHub URLs: https://github.com/org/repo/issues/42 or pull/42
	reGitHubURL = regexp.MustCompile(`https?://github\.com/([a-zA-Z0-9_.-]+/[a-zA-Z0-9_.-]+)/(?:issues|pull)/(\d+)`)

	// Matches Linear URLs: https://linear.app/org/issue/ENG-123
	reLinearURL = regexp.MustCompile(`https?://linear\.app/[a-zA-Z0-9_.-]+/issue/([A-Z][A-Z0-9]+-\d+)`)

	// Matches Sentry references: Sentry #1234 or Sentry Issue 1234
	reSentryRef = regexp.MustCompile(`(?i)\bSentry(?:\s+Issue)?\s+#?(\d+)\b`)
)

// Reference represents an identified cross-source entity.
type Reference struct {
	Kind string // "ticket", "github", "sentry"
	Key  string // canonical identifier in lowercase
}

// ExtractReferences scans text for issue trackers, PRs, and cross-source keys.
func ExtractReferences(content string) []Reference {
	seen := map[string]bool{}
	var refs []Reference

	add := func(kind, key string) {
		k := strings.ToLower(strings.TrimSpace(key))
		if k == "" || seen[kind+":"+k] {
			return
		}
		seen[kind+":"+k] = true
		refs = append(refs, Reference{Kind: kind, Key: k})
	}

	// 1. Ticket keys (e.g. ENG-123)
	for _, m := range reTicketKey.FindAllStringSubmatch(content, -1) {
		add("ticket", m[1])
	}

	// 2. Linear URLs
	for _, m := range reLinearURL.FindAllStringSubmatch(content, -1) {
		add("ticket", m[1])
	}

	// 3. GitHub URLs (org/repo#42)
	for _, m := range reGitHubURL.FindAllStringSubmatch(content, -1) {
		add("github", m[1]+"#"+m[2])
	}

	// 4. Sentry references (processed before shorthand #number to prevent false github match)
	sentryNums := map[string]bool{}
	for _, m := range reSentryRef.FindAllStringSubmatch(content, -1) {
		add("sentry", m[1])
		sentryNums[m[1]] = true
	}

	// 5. GitHub issue shorthand (#42 or org/repo#42)
	for _, m := range reGitHubRef.FindAllStringSubmatch(content, -1) {
		if m[1] != "" {
			add("github", m[1]+"#"+m[2])
		} else if !sentryNums[m[2]] {
			add("github", "#"+m[2])
		}
	}

	return refs
}

// ReferenceTags returns clean entity tags representing cross-source links.
func ReferenceTags(content string) []string {
	refs := ExtractReferences(content)
	tags := make([]string, 0, len(refs))
	for _, r := range refs {
		tags = append(tags, r.Kind+":"+r.Key)
	}
	return tags
}

// linkCrossReferences inspects newly stored memories for links to existing memories
// from other providers (e.g., a Slack message mentioning a GitHub issue or Linear ticket).
func (m *Manager) linkCrossReferences(ctx context.Context, tx pgx.Tx, c *conn, created []domain.Memory) error {
	for _, mem := range created {
		refs := ExtractReferences(mem.Content)
		if len(refs) == 0 {
			continue
		}

		for _, ref := range refs {
			pattern := "%" + ref.Key + "%"
			// Look for existing memories for this agent from a different connection or item
			rows, err := tx.Query(ctx, `
				SELECT id, connection_id
				FROM memories
				WHERE agent_id = $1
				  AND id != $2
				  AND (
				    content ILIKE $3
				    OR source_item ILIKE $3
				    OR entity_tags @> $4
				  )
				  AND tenant_id = $5
				LIMIT 5`,
				mem.AgentID, mem.ID, pattern, []string{ref.Key}, mem.TenantID)
			if err != nil {
				continue
			}

			var targetIDs []uuid.UUID
			for rows.Next() {
				var tid uuid.UUID
				var cid *uuid.UUID
				if err := rows.Scan(&tid, &cid); err == nil {
					// Avoid linking chunks of the exact same item
					if cid == nil || *cid != c.ID {
						targetIDs = append(targetIDs, tid)
					}
				}
			}
			rows.Close()

			for _, targetID := range targetIDs {
				metaJSON, _ := json.Marshal(map[string]any{
					"cross_reference": ref.Kind + ":" + ref.Key,
					"source_provider": c.Provider,
				})
				// Create relationship: new memory extends or derives from related context
				_, _ = tx.Exec(ctx, `
					INSERT INTO memory_relations (id, tenant_id, source_memory_id, target_memory_id, relation_type, confidence, metadata, created_by)
					VALUES ($1, $2, $3, $4, 'derives', 0.9, $5, $6)
					ON CONFLICT DO NOTHING`,
					uuid.New(), mem.TenantID, mem.ID, targetID, metaJSON, c.AgentID)

				// Denormalize relationship type on memory
				_, _ = tx.Exec(ctx, `
					UPDATE memories
					SET relationship_types = ARRAY(
						SELECT DISTINCT UNNEST(array_append(relationship_types, 'cross_source'))
					)
					WHERE id IN ($1, $2) AND agent_id = $3 AND tenant_id = $4`, mem.ID, targetID, mem.AgentID, mem.TenantID)
			}
		}
	}
	return nil
}
