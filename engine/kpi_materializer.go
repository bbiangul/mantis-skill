package engine

import (
	"context"
	"database/sql"
	"fmt"
	"math"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/bbiangul/mantis-skill/engine/models"
	"github.com/bbiangul/mantis-skill/skill"
)

// ---------------------------------------------------------------------------
// KPI model types — lightweight replacements for connect-ai/internal/models
// ---------------------------------------------------------------------------

// I18nLabel holds internationalised label strings.
type I18nLabel struct {
	En string `json:"en"`
	Pt string `json:"pt,omitempty"`
	Es string `json:"es,omitempty"`
}

// KpiTrendData holds trend information for a specific comparison period.
type KpiTrendData struct {
	Value      interface{} `json:"value"`
	Trend      string      `json:"trend"`
	TrendValue string      `json:"trendValue"`
	Status     string      `json:"status"`
}

// KpiItem represents a single KPI metric.
type KpiItem struct {
	ID          string                  `json:"id"`
	Label       I18nLabel               `json:"label"`
	Value       interface{}             `json:"value"`
	ValueType   string                  `json:"valueType"`
	Trend       string                  `json:"trend"`
	TrendValue  string                  `json:"trendValue"`
	Comparison  string                  `json:"comparison"`
	Status      string                  `json:"status"`
	Icon        string                  `json:"icon,omitempty"`
	Description I18nLabel               `json:"description,omitempty"`
	Trends      map[string]KpiTrendData `json:"trends,omitempty"`
}

// KpiCategory groups KPI items.
type KpiCategory struct {
	ID    string    `json:"id"`
	Label I18nLabel `json:"label"`
	Kpis  []KpiItem `json:"kpis"`
}

// KpiSummary highlights a single featured KPI.
type KpiSummary struct {
	Label     I18nLabel   `json:"label"`
	Value     interface{} `json:"value"`
	ValueType string      `json:"valueType"`
}

// KpiDashboardPayload is the full KPI dashboard update payload.
type KpiDashboardPayload struct {
	AgentName     string        `json:"agentName"`
	LastUpdatedAt string        `json:"lastUpdatedAt"`
	Categories    []KpiCategory `json:"categories"`
	Summary       *KpiSummary   `json:"summary,omitempty"`
}

// KpiDashboardMessage wraps the payload for delivery.
type KpiDashboardMessage struct {
	RequestType string              `json:"requestType"`
	CreatedAt   time.Time           `json:"createdAt"`
	Payload     KpiDashboardPayload `json:"payload"`
}

// KpiWriteFn delivers a dashboard update to the host application.
type KpiWriteFn func(ctx context.Context, msg KpiDashboardMessage) error

// KpiDBProvider gives the materializer access to a *sql.DB for kpi_events queries.
type KpiDBProvider interface {
	GetDB() (*sql.DB, error)
}

// ---------------------------------------------------------------------------
// KPIMaterializer
// ---------------------------------------------------------------------------

// KPIMaterializer periodically computes KPI values from kpi_events and sends
// dashboard updates via the write function.
type KPIMaterializer struct {
	toolEngine models.IToolEngine
	writeFn    KpiWriteFn
	dbProvider KpiDBProvider
	agentName  string

	mu             sync.Mutex
	cancel         context.CancelFunc
	stopped        bool
	lastEventMaxID int64
}

// NewKPIMaterializer creates a new KPIMaterializer.
// Returns nil if any required dependency is nil.
func NewKPIMaterializer(
	toolEngine models.IToolEngine,
	writeFn KpiWriteFn,
	dbProvider KpiDBProvider,
	agentName string,
) *KPIMaterializer {
	if toolEngine == nil || writeFn == nil || dbProvider == nil {
		return nil
	}
	return &KPIMaterializer{
		toolEngine: toolEngine,
		writeFn:    writeFn,
		dbProvider: dbProvider,
		agentName:  agentName,
	}
}

// Start begins the background materialization loop.
func (m *KPIMaterializer) Start(ctx context.Context) {
	ctx, cancel := context.WithCancel(ctx)
	m.mu.Lock()
	m.cancel = cancel
	m.stopped = false
	m.mu.Unlock()

	go m.run(ctx)
}

// Stop cancels the background loop.
func (m *KPIMaterializer) Stop() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.stopped = true
	if m.cancel != nil {
		m.cancel()
	}
}

func (m *KPIMaterializer) run(ctx context.Context) {
	// Initial delay to let tools load
	select {
	case <-time.After(30 * time.Second):
	case <-ctx.Done():
		return
	}

	m.materializeAll(ctx)

	interval := m.computeMinRefreshInterval()
	if interval < 1*time.Minute {
		interval = 1 * time.Minute
	}

	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			m.materializeAll(ctx)
		case <-ctx.Done():
			return
		}
	}
}

func (m *KPIMaterializer) computeMinRefreshInterval() time.Duration {
	allKPIs := m.collectAllKPIs()
	minDur := 10 * time.Minute

	for _, kpi := range allKPIs {
		d := kpi.GetRefreshDuration()
		if d > 0 && d < minDur {
			minDur = d
		}
	}
	return minDur
}

func (m *KPIMaterializer) collectAllKPIs() []skill.KPIDefinition {
	tools := m.toolEngine.GetAllTools()
	var all []skill.KPIDefinition
	for _, t := range tools {
		all = append(all, t.Kpis...)
	}
	sort.Slice(all, func(i, j int) bool {
		if all[i].Category != all[j].Category {
			return all[i].Category < all[j].Category
		}
		return all[i].Order < all[j].Order
	})
	return all
}

func (m *KPIMaterializer) materializeAll(ctx context.Context) {
	allKPIs := m.collectAllKPIs()
	if len(allKPIs) == 0 {
		return
	}

	db, err := m.dbProvider.GetDB()
	if err != nil {
		if logger != nil {
			logger.Errorf("[KPIMaterializer] Failed to get database: %v", err)
		}
		return
	}

	if !kpiTableExists(db, "kpi_events") {
		return
	}

	migrateKpiEventsSchema(db)

	var currentMaxID int64
	if err := db.QueryRow("SELECT COALESCE(MAX(id), 0) FROM kpi_events").Scan(&currentMaxID); err == nil {
		if currentMaxID > 0 && currentMaxID == m.lastEventMaxID {
			return
		}
	}

	now := time.Now().UTC()

	type catInfo struct {
		id    string
		label I18nLabel
		kpis  []KpiItem
	}
	categoriesMap := make(map[string]*catInfo)
	var categoryOrder []string

	var summary *KpiSummary

	allComparisons := []string{"previous_day", "previous_week", "previous_month"}

	for _, kpi := range allKPIs {
		defaultComparison := kpi.GetComparison()

		trends := make(map[string]KpiTrendData, len(allComparisons))
		var defaultCurrent float64
		var defaultTrend, defaultTrendValue, defaultStatus string

		for _, comp := range allComparisons {
			kpiCopy := kpi
			kpiCopy.Comparison = comp

			current, err := computeKPIValue(db, kpiCopy, now, false)
			if err != nil {
				continue
			}

			previous, err := computeKPIValue(db, kpiCopy, now, true)
			if err != nil {
				previous = 0
			}

			trend, trendValue, status := computeTrend(current, previous, kpi.GetPositiveDirection())

			trends[comp] = KpiTrendData{
				Value:      formatKPIValue(current, kpi.ValueType),
				Trend:      trend,
				TrendValue: trendValue,
				Status:     status,
			}

			if comp == defaultComparison {
				defaultCurrent = current
				defaultTrend = trend
				defaultTrendValue = trendValue
				defaultStatus = status
			}
		}

		if defaultTrend == "" {
			current, err := computeKPIValue(db, kpi, now, false)
			if err != nil {
				continue
			}
			previous, err := computeKPIValue(db, kpi, now, true)
			if err != nil {
				previous = 0
			}
			defaultCurrent = current
			defaultTrend, defaultTrendValue, defaultStatus = computeTrend(current, previous, kpi.GetPositiveDirection())
		}

		item := KpiItem{
			ID: kpi.ID,
			Label: I18nLabel{
				En: kpi.Label.En,
				Pt: kpi.Label.Pt,
				Es: kpi.Label.Es,
			},
			Value:      formatKPIValue(defaultCurrent, kpi.ValueType),
			ValueType:  kpi.ValueType,
			Trend:      defaultTrend,
			TrendValue: defaultTrendValue,
			Comparison: defaultComparison,
			Status:     defaultStatus,
			Icon:       kpi.Icon,
			Description: I18nLabel{
				En: kpi.Description.En,
				Pt: kpi.Description.Pt,
				Es: kpi.Description.Es,
			},
			Trends: trends,
		}

		cat, exists := categoriesMap[kpi.Category]
		if !exists {
			cat = &catInfo{
				id: kpi.Category,
				label: I18nLabel{
					En: kpi.CategoryLabel.En,
					Pt: kpi.CategoryLabel.Pt,
					Es: kpi.CategoryLabel.Es,
				},
			}
			if cat.label.En == "" {
				cat.label.En = kpi.Category
			}
			categoriesMap[kpi.Category] = cat
			categoryOrder = append(categoryOrder, kpi.Category)
		}
		cat.kpis = append(cat.kpis, item)

		if kpi.Featured && summary == nil {
			summary = &KpiSummary{
				Label: I18nLabel{
					En: kpi.Label.En,
					Pt: kpi.Label.Pt,
					Es: kpi.Label.Es,
				},
				Value:     formatKPIValue(defaultCurrent, kpi.ValueType),
				ValueType: kpi.ValueType,
			}
		}
	}

	categories := make([]KpiCategory, 0, len(categoryOrder))
	for _, catID := range categoryOrder {
		cat := categoriesMap[catID]
		categories = append(categories, KpiCategory{
			ID:    cat.id,
			Label: cat.label,
			Kpis:  cat.kpis,
		})
	}

	payload := KpiDashboardPayload{
		AgentName:     m.agentName,
		LastUpdatedAt: now.UTC().Format(time.RFC3339),
		Categories:    categories,
		Summary:       summary,
	}

	msg := KpiDashboardMessage{
		RequestType: "kpi_dashboard_update",
		CreatedAt:   now,
		Payload:     payload,
	}

	if err := m.writeFn(ctx, msg); err != nil {
		if logger != nil {
			logger.Errorf("[KPIMaterializer] Failed to send KPI dashboard update: %v", err)
		}
	} else {
		m.lastEventMaxID = currentMaxID
	}
}

// ---------------------------------------------------------------------------
// SQL helpers
// ---------------------------------------------------------------------------

func kpiTableExists(db *sql.DB, tableName string) bool {
	var name string
	err := db.QueryRow(
		"SELECT name FROM sqlite_master WHERE type='table' AND name=?",
		tableName,
	).Scan(&name)
	return err == nil
}

func migrateKpiEventsSchema(db *sql.DB) {
	migrations := []string{
		"ALTER TABLE kpi_events ADD COLUMN label TEXT",
		"ALTER TABLE kpi_events ADD COLUMN distinct_id TEXT",
		"ALTER TABLE kpi_events ADD COLUMN value REAL",
	}
	for _, stmt := range migrations {
		_, _ = db.Exec(stmt)
	}
	_, _ = db.Exec("CREATE INDEX IF NOT EXISTS idx_kpi_events_type_date ON kpi_events(event_type, created_at)")
	_, _ = db.Exec("CREATE INDEX IF NOT EXISTS idx_kpi_events_type_distinct ON kpi_events(event_type, distinct_id)")
}

const sqliteDatetimeFmt = "2006-01-02 15:04:05"

func computeKPIValue(db *sql.DB, kpi skill.KPIDefinition, now time.Time, previous bool) (float64, error) {
	start, end := getTimeBounds(kpi.GetComparison(), now, previous)

	switch kpi.Aggregation {
	case "count":
		return queryKPICount(db, kpi.EventType, start, end)
	case "count_unique":
		return queryKPICountUnique(db, kpi.EventType, start, end)
	case "sum":
		return queryKPISum(db, kpi.EventType, start, end)
	case "avg":
		return queryKPIAvg(db, kpi.EventType, start, end)
	case "ratio":
		num, err := queryKPICount(db, kpi.Numerator, start, end)
		if err != nil {
			return 0, err
		}
		den, err := queryKPICount(db, kpi.Denominator, start, end)
		if err != nil {
			return 0, err
		}
		if den == 0 {
			return 0, nil
		}
		return (num / den) * 100, nil
	default:
		return 0, fmt.Errorf("unsupported aggregation: %s", kpi.Aggregation)
	}
}

func queryKPICount(db *sql.DB, eventType string, start, end time.Time) (float64, error) {
	var count float64
	err := db.QueryRow(
		"SELECT COUNT(*) FROM kpi_events WHERE event_type=? AND created_at>=? AND created_at<?",
		eventType, start.UTC().Format(sqliteDatetimeFmt), end.UTC().Format(sqliteDatetimeFmt),
	).Scan(&count)
	return count, err
}

func queryKPICountUnique(db *sql.DB, eventType string, start, end time.Time) (float64, error) {
	var count float64
	err := db.QueryRow(
		"SELECT COUNT(DISTINCT distinct_id) FROM kpi_events WHERE event_type=? AND created_at>=? AND created_at<? AND distinct_id IS NOT NULL",
		eventType, start.UTC().Format(sqliteDatetimeFmt), end.UTC().Format(sqliteDatetimeFmt),
	).Scan(&count)
	return count, err
}

func queryKPISum(db *sql.DB, eventType string, start, end time.Time) (float64, error) {
	var sum float64
	err := db.QueryRow(
		"SELECT COALESCE(SUM(value),0) FROM kpi_events WHERE event_type=? AND created_at>=? AND created_at<?",
		eventType, start.UTC().Format(sqliteDatetimeFmt), end.UTC().Format(sqliteDatetimeFmt),
	).Scan(&sum)
	return sum, err
}

func queryKPIAvg(db *sql.DB, eventType string, start, end time.Time) (float64, error) {
	var avg float64
	err := db.QueryRow(
		"SELECT COALESCE(AVG(value),0) FROM kpi_events WHERE event_type=? AND created_at>=? AND created_at<?",
		eventType, start.UTC().Format(sqliteDatetimeFmt), end.UTC().Format(sqliteDatetimeFmt),
	).Scan(&avg)
	return avg, err
}

func getTimeBounds(comparison string, now time.Time, previous bool) (time.Time, time.Time) {
	switch comparison {
	case "previous_week":
		weekStart := now.Truncate(24*time.Hour).AddDate(0, 0, -int(now.Weekday()))
		if previous {
			return weekStart.AddDate(0, 0, -7), weekStart
		}
		return weekStart, now

	case "previous_month":
		monthStart := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, now.Location())
		if previous {
			prevMonthStart := monthStart.AddDate(0, -1, 0)
			return prevMonthStart, monthStart
		}
		return monthStart, now

	default: // "previous_day" or "previous_period"
		dayStart := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
		if previous {
			return dayStart.AddDate(0, 0, -1), dayStart
		}
		return dayStart, now
	}
}

func computeTrend(current, previous float64, positiveDirection string) (trend string, trendValue string, status string) {
	if previous == 0 {
		if current == 0 {
			return "neutral", "0%", "neutral"
		}
		return "up", "+100%", statusFromDirection("up", positiveDirection)
	}

	change := ((current - previous) / math.Abs(previous)) * 100

	if math.Abs(change) < 0.5 {
		return "neutral", "0%", "neutral"
	}

	if change > 0 {
		trend = "up"
		trendValue = fmt.Sprintf("+%.1f%%", change)
	} else {
		trend = "down"
		trendValue = fmt.Sprintf("%.1f%%", change)
	}

	status = statusFromDirection(trend, positiveDirection)
	return
}

func statusFromDirection(trend, positiveDirection string) string {
	if positiveDirection == "" {
		positiveDirection = "up"
	}
	if trend == positiveDirection {
		return "positive"
	}
	if trend == "neutral" {
		return "neutral"
	}
	return "negative"
}

func formatKPIValue(v float64, valueType string) interface{} {
	switch valueType {
	case "percentage":
		return math.Round(v*10) / 10
	case "currency":
		return math.Round(v*100) / 100
	case "duration":
		return strings.TrimRight(strings.TrimRight(fmt.Sprintf("%.1f", v), "0"), ".")
	default: // "number"
		if v == math.Trunc(v) {
			return int(v)
		}
		return math.Round(v*10) / 10
	}
}
