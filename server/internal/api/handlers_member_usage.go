package api

import (
	"math"
	"net/http"
	"time"

	"github.com/google/uuid"

	"github.com/memrohq/memro/server/extension"
	"github.com/memrohq/memro/server/internal/store"
)

// Model list prices in US dollars per million tokens, to estimate what each
// end user costs Memro (ADR 0026).
// shortcut: hard-coded list prices (checked 2026-11); update them when a
// provider changes prices or Memro adds a model.
var embedPricePerM = map[string]float64{
	"text-embedding-3-small": 0.02,
	"text-embedding-3-large": 0.13,
	"voyage-3":               0.06,
	"voyage-3-lite":          0.02,
}

const visionInPricePerM, visionOutPricePerM = 2.50, 10.00 // gpt-4o

// usageCost estimates the model cost of one month of metrics.
func usageCost(m map[string]int64, embedModel string) float64 {
	embed := float64(m[extension.MetricStoredTokens]+m[extension.MetricQueryTokens]) * embedPricePerM[embedModel]
	vision := float64(m[extension.MetricVisionInputTokens])*visionInPricePerM + float64(m[extension.MetricVisionOutputTokens])*visionOutPricePerM
	return math.Round(embed+vision) / 1e6 // prices are per million tokens: dollars, to the micro-dollar
}

type memberUsageRow struct {
	store.MemberUsage
	CostUSD float64 `json:"cost_usd"`
}

// GET /tenant/{id}/usage/members?month=2026-11 — each end user's usage and
// estimated model cost for a month (default: this one). For the
// organisation's members, and for the tenant's own key (a customer's billing
// job). Usage only: no memory content.
func (s *Server) memberUsage(w http.ResponseWriter, r *http.Request) {
	a := agentFrom(r)
	tid, err := uuid.Parse(r.PathValue("id"))
	if !(err == nil && a.EndUser != "" && a.TenantID == tid) { // not this tenant's key
		var ok bool
		if tid, _, ok = s.tenantAccess(w, r, store.Roles...); !ok {
			return
		}
	}
	month := time.Now().UTC()
	if v := r.URL.Query().Get("month"); v != "" {
		if month, err = time.Parse("2006-01", v); err != nil {
			httpError(w, http.StatusBadRequest, "month must look like 2026-11")
			return
		}
	}
	month = time.Date(month.Year(), month.Month(), 1, 0, 0, 0, 0, time.UTC)
	usage, err := s.store.TenantMemberUsage(r.Context(), tid, month)
	if err != nil {
		httpError(w, http.StatusInternalServerError, "Database error")
		return
	}
	model := s.embed.Model()
	rows := make([]memberUsageRow, len(usage))
	totals := map[string]int64{}
	var cost float64
	for i, u := range usage {
		rows[i] = memberUsageRow{u, usageCost(u.Metrics, model)}
		cost += rows[i].CostUSD
		for k, v := range u.Metrics {
			totals[k] += v
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"tenant_id":       tid,
		"month":           month.Format("2006-01"),
		"embedding_model": model,
		"active_members":  len(rows), // anyone with usage this month
		"members":         rows,
		"totals":          totals,
		"cost_usd":        math.Round(cost*1e6) / 1e6,
	})
}
