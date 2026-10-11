package usagestats

// chart_endpoints_removed_test.go locks the backend contract after the dashboard
// charts were removed: the trend endpoint is gone and the remaining payloads no
// longer carry chart-only series fields, while the surfaces the cards and both
// tables depend on stay intact.

import (
	"encoding/json"
	"net/http"
	"net/url"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func openChartRegressionStore(t *testing.T) *Store {
	t.Helper()
	dir := t.TempDir()
	store, err := Open(Config{
		DataPath:        filepath.Join(dir, "usage-stats.db"),
		RetentionDays:   365,
		FlushInterval:   100 * time.Millisecond,
		FlushMaxRecords: 10,
		SyncOnRecord:    true,
	})
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { store.Close() })

	now := time.Now().UTC().Truncate(time.Second)
	first := NewUsage("workbuddy", "workbuddy-provider", "gpt-4o", "gpt-4o", "https://www.codebuddy.cn", "oauth", "uid-1", now.Add(-2*time.Minute), 1_500_000_000, false, 200, NewCounters(1000, 500, 0, 200, 150, 50, 0))
	if err := store.Record(first); err != nil {
		t.Fatalf("Record first: %v", err)
	}
	second := NewUsage("workbuddy", "qoder-ai-provider", "deepseek-v4.1-flash", "deepseek-v4.1-flash", "https://www.codebuddy.cn", "oauth", "uid-2", now.Add(-time.Minute), 400_000_000, false, 200, NewCounters(200, 100, 0, 0, 0, 0, 0))
	if err := store.Record(second); err != nil {
		t.Fatalf("Record second: %v", err)
	}
	return store
}

// TestTrendsRouteIsRemoved asserts /stats/trends is no longer a served route.
func TestTrendsRouteIsRemoved(t *testing.T) {
	store := openChartRegressionStore(t)

	res := store.HandleQuery(http.MethodGet, "/stats/trends", url.Values{"range": []string{"24h"}}, nil, nil)
	if res.Status != http.StatusNotFound {
		t.Fatalf("/stats/trends status=%d want 404 body=%s", res.Status, res.Body)
	}
	if !strings.Contains(string(res.Body), "route not found") {
		t.Fatalf("/stats/trends body=%s want route-not-found error", res.Body)
	}
}

// TestInitialStatsDropsChartSeries asserts /stats/initial keeps the card and
// filter surfaces but stops shipping the hourly series and bucket width.
func TestInitialStatsDropsChartSeries(t *testing.T) {
	store := openChartRegressionStore(t)

	res := store.HandleQuery(http.MethodGet, "/stats/initial", url.Values{"range": []string{"24h"}}, nil, nil)
	if res.Status != http.StatusOK {
		t.Fatalf("/stats/initial status=%d body=%s", res.Status, res.Body)
	}
	body := string(res.Body)
	for _, forbidden := range []string{`"series"`, `"bucket_seconds"`, `"model_series"`} {
		if strings.Contains(body, forbidden) {
			t.Fatalf("/stats/initial still ships chart-only field %s: %s", forbidden, body)
		}
	}

	var initial InitialStatsResponse
	if err := json.Unmarshal(res.Body, &initial); err != nil {
		t.Fatalf("/stats/initial decode: %v", err)
	}
	if initial.Summary.Requests != 2 {
		t.Errorf("Summary.Requests=%d want 2", initial.Summary.Requests)
	}
	if len(initial.Models) != 2 {
		t.Errorf("Models len=%d want 2", len(initial.Models))
	}
	if len(initial.Sources) == 0 {
		t.Error("Sources must stay available for the filter dropdown")
	}
	if initial.LastUsed.IsZero() {
		t.Error("LastUsed must stay available for the status line")
	}
}

// TestCostsDropsSeriesForSameSummary asserts /costs keeps summary and per-model
// rows (cards, coverage line and the cost column) without the series payload.
func TestCostsDropsSeriesForSameSummary(t *testing.T) {
	store := openChartRegressionStore(t)

	res := store.HandleQuery(http.MethodGet, "/costs", url.Values{"range": []string{"24h"}}, nil, nil)
	if res.Status != http.StatusOK {
		t.Fatalf("/costs status=%d body=%s", res.Status, res.Body)
	}
	body := string(res.Body)
	if strings.Contains(body, `"series"`) {
		t.Fatalf("/costs still ships chart-only series: %s", body)
	}

	var costs CostResponse
	if err := json.Unmarshal(res.Body, &costs); err != nil {
		t.Fatalf("/costs decode: %v", err)
	}
	if costs.Summary.Requests != 2 {
		t.Errorf("costs Summary.Requests=%d want 2", costs.Summary.Requests)
	}
	if len(costs.Models) == 0 {
		t.Error("costs Models must stay populated for the cost column")
	}
}

// TestGroupsAndRequestsStayAvailable asserts the two kept tables are unaffected.
func TestGroupsAndRequestsStayAvailable(t *testing.T) {
	store := openChartRegressionStore(t)

	res := store.HandleQuery(http.MethodGet, "/stats/groups", url.Values{"range": []string{"24h"}}, nil, nil)
	if res.Status != http.StatusOK {
		t.Fatalf("/stats/groups status=%d body=%s", res.Status, res.Body)
	}
	var groups GroupStatsPage
	if err := json.Unmarshal(res.Body, &groups); err != nil {
		t.Fatalf("/stats/groups decode: %v", err)
	}
	if groups.Total != 2 {
		t.Errorf("/stats/groups Total=%d want 2", groups.Total)
	}

	res = store.HandleQuery(http.MethodGet, "/requests", url.Values{"limit": []string{"10"}}, nil, nil)
	if res.Status != http.StatusOK {
		t.Fatalf("/requests status=%d body=%s", res.Status, res.Body)
	}
	var page RequestPage
	if err := json.Unmarshal(res.Body, &page); err != nil {
		t.Fatalf("/requests decode: %v", err)
	}
	if page.Total != 2 {
		t.Errorf("/requests Total=%d want 2", page.Total)
	}
	if len(page.Items) > 0 && page.Items[0].Provider == "" {
		t.Errorf("/requests provider column must stay populated: %+v", page.Items[0].Dimensions)
	}
}
