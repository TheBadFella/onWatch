package store

import "time"

// CostUsage is bounded, month-scoped token telemetry. Cache counts must be
// explicitly supplied; missing cache data never silently becomes zero.
type CostUsage struct {
	Provider     string `json:"provider"`
	Account      string `json:"account"`
	Model        string `json:"model"`
	Requests     int64  `json:"requests"`
	Input        int64  `json:"input_tokens"`
	Output       int64  `json:"output_tokens"`
	Cached       int64  `json:"cached_input_tokens"`
	UnknownCache int64  `json:"unknown_cache_requests"`
}

func (s *Store) QueryCostUsage(start, end time.Time) ([]CostUsage, error) {
	rows, err := s.db.Query(`
 WITH usage AS (
 SELECT *, json_extract(CASE WHEN json_valid(metadata_json) THEN metadata_json ELSE '{}' END,'$.cached_prompt_tokens') AS cached
 FROM api_integration_usage_events WHERE captured_at >= ? AND captured_at < ?
 AND julianday(captured_at) >= julianday(?) AND julianday(captured_at) < julianday(?)
 )
 SELECT provider,account_name,model,COUNT(*),SUM(prompt_tokens),SUM(completion_tokens),
 SUM(CASE WHEN typeof(cached)='integer' AND cached BETWEEN 0 AND prompt_tokens THEN cached ELSE 0 END),
 SUM(CASE WHEN typeof(cached)='integer' AND cached BETWEEN 0 AND prompt_tokens THEN 0 ELSE 1 END)
 FROM usage GROUP BY provider,account_name,model ORDER BY provider,account_name,model LIMIT 501
 `, start.UTC().Format("2006-01-02T15:04:05"), end.UTC().Add(time.Second).Truncate(time.Second).Format("2006-01-02T15:04:05"), start.UTC().Format(time.RFC3339Nano), end.UTC().Format(time.RFC3339Nano))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := []CostUsage{}
	for rows.Next() {
		var row CostUsage
		if err := rows.Scan(&row.Provider, &row.Account, &row.Model, &row.Requests, &row.Input, &row.Output, &row.Cached, &row.UnknownCache); err != nil {
			return nil, err
		}
		result = append(result, row)
	}
	return result, rows.Err()
}
