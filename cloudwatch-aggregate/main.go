package main

import (
	"context"
	"flag"
	"fmt"
	"math"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/cloudwatch"
	cwtypes "github.com/aws/aws-sdk-go-v2/service/cloudwatch/types"
)

var (
	namespaces = []string{
		"Benchmark/GoSDKv1",
		"Benchmark/GoSDKv2",
		"Benchmark/GoSDKv2CodeGen",
		"Benchmark/GoSDKv2Ptr",
	}

	nsLabel = map[string]string{
		"Benchmark/GoSDKv1":        "v1",
		"Benchmark/GoSDKv2":        "v2",
		"Benchmark/GoSDKv2CodeGen": "v2-codegen",
		"Benchmark/GoSDKv2Ptr":     "v2-ptr",
	}

	nsSDK = map[string]string{
		"Benchmark/GoSDKv1":        "aws-sdk-go",
		"Benchmark/GoSDKv2":        "aws-sdk-go-v2",
		"Benchmark/GoSDKv2CodeGen": "aws-sdk-go-v2",
		"Benchmark/GoSDKv2Ptr":     "aws-sdk-go-v2",
	}

	// nsTablePrefix maps each benchmark namespace to the DynamoDB table-name
	// prefix the corresponding app uses. The full table name is
	// "<prefix>_<arch>_<size>" (see apps/<app>/main.go TableNameFormat).
	nsTablePrefix = map[string]string{
		"Benchmark/GoSDKv1":        "test_table_benchmarks_v1",
		"Benchmark/GoSDKv2":        "test_table_benchmarks_v2",
		"Benchmark/GoSDKv2CodeGen": "test_table_benchmarks_v2_codegen",
		"Benchmark/GoSDKv2Ptr":     "test_table_benchmarks_v2_ptr",
	}

	operations = []string{"MarshalMap", "UnmarshalMap"}
	sizes      = []string{"1KB", "10KB", "100KB", "300KB"}
	arches     = []string{"arm64", "amd64"}
	osName     = "linux"

	// API operations paired with each serialization direction.
	marshalOps   = []string{"PutItem", "UpdateItem", "BatchWriteItem", "TransactWriteItems"}
	unmarshalOps = []string{"GetItem", "Query", "Scan", "BatchGetItem", "TransactGetItems"}

	// serToAPIOps maps a serialization operation to the API operations it is used with.
	serToAPIOps = map[string][]string{
		"MarshalMap":   marshalOps,
		"UnmarshalMap": unmarshalOps,
	}
)

type metricKey struct {
	Label     string
	Operation string
	Size      string
	Arch      string
}

type metricStats struct {
	Average float64
	P50     float64
	P90     float64
	P95     float64
	P99     float64
	Min     float64
	Max     float64
}

type apiMetricKey struct {
	Label   string
	APIName string
	Size    string
	Arch    string
}

// apiQueryID returns a unique CloudWatch query ID for an API operation metric.
func apiQueryID(ns, apiOp, size, arch string) string {
	r := strings.NewReplacer("/", "_", " ", "_", ".", "_", "-", "_", "%", "pct", "(", "", ")", "", ",", "_")
	return strings.ToLower(r.Replace(fmt.Sprintf("api_%s_%s_%s_%s", ns, apiOp, size, arch)))
}

func resolveRegion() string {
	for _, env := range []string{"AWS_REGION", "AWS_DEFAULT_REGION"} {
		if v := os.Getenv(env); v != "" {
			return v
		}
	}
	return "eu-west-1"
}

// safeID builds a lowercase, CloudWatch-safe query ID from the given parts.
func safeID(ns, operation, size, arch, stat string) string {
	r := strings.NewReplacer(
		"/", "_", " ", "_", ".", "_", "-", "_",
		"%", "pct", "(", "", ")", "", ",", "_",
	)
	return strings.ToLower(r.Replace(
		fmt.Sprintf("%s_%s_%s_%s_%s", ns, operation, size, arch, stat),
	))
}

func nanDefault(v float64) float64 {
	if math.IsNaN(v) || math.IsInf(v, 0) {
		return 0
	}
	return v
}

func formatMicros(v float64) string {
	if v == 0 {
		return "—"
	}
	return fmt.Sprintf("%.2f µs", v)
}

func formatDelta(delta, baseline float64) string {
	if baseline == 0 {
		return "—"
	}
	sign := "+"
	if delta < 0 {
		sign = ""
	}
	pct := delta / baseline * 100
	return fmt.Sprintf("%s%.2f µs (%s%.2f%%)", sign, delta, sign, pct)
}

func formatDurationPct(duration, baseline float64) string {
	if duration == 0 || baseline == 0 {
		return "—"
	}
	return fmt.Sprintf("%.2f%% (%.2f µs)", duration/baseline*100, baseline)
}

func sizeOrder(s string) int {
	m := map[string]int{"1KB": 0, "10KB": 1, "100KB": 2, "300KB": 3}
	if v, ok := m[s]; ok {
		return v
	}
	return 99
}

func namespaceForLabel(label string) string {
	for ns, l := range nsLabel {
		if l == label {
			return ns
		}
	}
	return label
}

// buildQueries builds one MetricDataQuery per (namespace × operation × size × arch) for
// the given CloudWatch statistic string (e.g. "Average", "p50", "Minimum"…).
func buildQueries(stat string, period int32) ([]cwtypes.MetricDataQuery, map[string]metricKey) {
	var queries []cwtypes.MetricDataQuery
	idToKey := make(map[string]metricKey)

	for _, ns := range namespaces {
		sdk := nsSDK[ns]
		label := nsLabel[ns]
		for _, op := range operations {
			for _, sz := range sizes {
				for _, arch := range arches {
					id := safeID(ns, op, sz, arch, stat)
					idToKey[id] = metricKey{Label: label, Operation: op, Size: sz, Arch: arch}
					queries = append(queries, cwtypes.MetricDataQuery{
						Id:    aws.String(id),
						Label: aws.String(fmt.Sprintf("%s/%s/%s/%s/%s", label, op, sz, arch, stat)),
						MetricStat: &cwtypes.MetricStat{
							Metric: &cwtypes.Metric{
								Namespace:  aws.String(ns),
								MetricName: aws.String(op),
								Dimensions: []cwtypes.Dimension{
									{Name: aws.String("OS"), Value: aws.String(osName)},
									{Name: aws.String("Size"), Value: aws.String(sz)},
									{Name: aws.String("Arch"), Value: aws.String(arch)},
									{Name: aws.String("SDK"), Value: aws.String(sdk)},
								},
							},
							Period: aws.Int32(period),
							Stat:   aws.String(stat),
						},
						ReturnData: aws.Bool(true),
					})
				}
			}
		}
	}

	return queries, idToKey
}

// buildAPIQueries builds one MetricDataQuery per (namespace × apiOp × size × arch) for
// the requested statistic. v1 emits per-operation metrics in ms; v2 uses
// client.call.duration with an rpc.method dimension in µs.
func buildAPIQueries(stat string, period int32) ([]cwtypes.MetricDataQuery, map[string]apiMetricKey) {
	allAPIops := append(append([]string{}, marshalOps...), unmarshalOps...)
	var queries []cwtypes.MetricDataQuery
	idToKey := make(map[string]apiMetricKey)

	for _, ns := range namespaces {
		sdk := nsSDK[ns]
		label := nsLabel[ns]
		for _, apiOp := range allAPIops {
			for _, sz := range sizes {
				for _, arch := range arches {
					id := apiQueryID(ns, apiOp, sz, arch)
					idToKey[id] = apiMetricKey{Label: label, APIName: apiOp, Size: sz, Arch: arch}

					var metricName string
					var dims []cwtypes.Dimension
					if ns == "Benchmark/GoSDKv1" {
						metricName = apiOp
						dims = []cwtypes.Dimension{
							{Name: aws.String("OS"), Value: aws.String(osName)},
							{Name: aws.String("Size"), Value: aws.String(sz)},
							{Name: aws.String("Arch"), Value: aws.String(arch)},
							{Name: aws.String("SDK"), Value: aws.String(sdk)},
						}
					} else {
						metricName = "client.call.duration"
						dims = []cwtypes.Dimension{
							{Name: aws.String("OS"), Value: aws.String(osName)},
							{Name: aws.String("Size"), Value: aws.String(sz)},
							{Name: aws.String("Arch"), Value: aws.String(arch)},
							{Name: aws.String("SDK"), Value: aws.String(sdk)},
							{Name: aws.String("rpc.method"), Value: aws.String(apiOp)},
						}
					}

					queries = append(queries, cwtypes.MetricDataQuery{
						Id:    aws.String(id),
						Label: aws.String(fmt.Sprintf("%s/%s/%s/%s", label, apiOp, sz, arch)),
						MetricStat: &cwtypes.MetricStat{
							Metric: &cwtypes.Metric{
								Namespace:  aws.String(ns),
								MetricName: aws.String(metricName),
								Dimensions: dims,
							},
							Period: aws.Int32(period),
							Stat:   aws.String(stat),
						},
						ReturnData: aws.Bool(true),
					})
				}
			}
		}
	}

	return queries, idToKey
}

func buildAPICountQueries(period int32) ([]cwtypes.MetricDataQuery, map[string]apiMetricKey) {
	return buildAPIQueries("SampleCount", period)
}

// buildRuntimeMetricsQueries builds queries for runtime metrics used in the appendix.
func buildRuntimeMetricsQueries(period int32) ([]cwtypes.MetricDataQuery, map[string]runtimeMetricKey) {
	runtimeMetrics := []string{
		"Memory.GCCPUFraction",
		"Client.CPU.Usage",
		"Client.CPU.Idle",
		"Client.CPU.User",
		"Client.CPU.System",
		"Client.CPU.IOWait",
		"gc.heap.allocs.bytes",
		"gc.heap.frees.bytes",
		"Memory.PauseUs",
		"gc.heap.allocs.objects",
		"gc.heap.frees.objects",
	}
	var queries []cwtypes.MetricDataQuery
	idToKey := make(map[string]runtimeMetricKey)

	for _, ns := range namespaces {
		sdk := nsSDK[ns]
		label := nsLabel[ns]
		for _, metricName := range runtimeMetrics {
			for _, sz := range sizes {
				for _, arch := range arches {
					id := safeID(ns, metricName, sz, arch, "Average")
					stat := "Average"
					if metricName == "gc.heap.allocs.objects" || metricName == "gc.heap.frees.objects" {
						stat = "Maximum"
					}
					id = safeID(ns, metricName, sz, arch, stat)
					idToKey[id] = runtimeMetricKey{Label: label, MetricName: metricName, Size: sz, Arch: arch}

					queries = append(queries, cwtypes.MetricDataQuery{
						Id:    aws.String(id),
						Label: aws.String(fmt.Sprintf("%s/%s/%s/%s", label, metricName, sz, arch)),
						MetricStat: &cwtypes.MetricStat{
							Metric: &cwtypes.Metric{
								Namespace:  aws.String(ns),
								MetricName: aws.String(metricName),
								Dimensions: []cwtypes.Dimension{
									{Name: aws.String("OS"), Value: aws.String(osName)},
									{Name: aws.String("Size"), Value: aws.String(sz)},
									{Name: aws.String("Arch"), Value: aws.String(arch)},
									{Name: aws.String("SDK"), Value: aws.String(sdk)},
								},
							},
							Period: aws.Int32(period),
							Stat:   aws.String(stat),
						},
						ReturnData: aws.Bool(true),
					})
				}
			}
		}
	}

	return queries, idToKey
}

// runKey identifies a single benchmark run series: one SDK label, payload size
// and architecture. Each app×size step emitted by `make all-sizes-seq` maps to
// exactly one runKey.
type runKey struct {
	Label string
	Size  string
	Arch  string
}

// timeWindow is the inclusive [start, end] span of a single run, derived from
// the RunMarker datapoints emitted by the CLI around each benchmark step.
type timeWindow struct {
	start, end time.Time
}

// runMarkerKey identifies a RunMarker series (one per runKey × phase).
type runMarkerKey struct {
	Label string
	Size  string
	Arch  string
	Phase string
}

// buildRunMarkerQueries builds queries for the RunMarker metric emitted by
// `make all-sizes-seq` to bracket each run (Phase=start|end). The marker lives
// in each app's own namespace (Benchmark/GoSDKv1, Benchmark/GoSDKv2, …), so the
// namespace identifies the app and only Size/Arch/Phase are needed as dimensions.
func buildRunMarkerQueries(period int32) ([]cwtypes.MetricDataQuery, map[string]runMarkerKey) {
	phases := []string{"start", "end"}
	var queries []cwtypes.MetricDataQuery
	idToKey := make(map[string]runMarkerKey)

	for _, ns := range namespaces {
		label := nsLabel[ns]
		for _, sz := range sizes {
			for _, arch := range arches {
				for _, phase := range phases {
					id := safeID("runmarker", label, sz, arch, phase)
					idToKey[id] = runMarkerKey{Label: label, Size: sz, Arch: arch, Phase: phase}
					queries = append(queries, cwtypes.MetricDataQuery{
						Id:    aws.String(id),
						Label: aws.String(fmt.Sprintf("%s/%s/%s/%s", label, sz, arch, phase)),
						MetricStat: &cwtypes.MetricStat{
							Metric: &cwtypes.Metric{
								Namespace:  aws.String(ns),
								MetricName: aws.String("RunMarker"),
								Dimensions: []cwtypes.Dimension{
									{Name: aws.String("Size"), Value: aws.String(sz)},
									{Name: aws.String("Arch"), Value: aws.String(arch)},
									{Name: aws.String("Phase"), Value: aws.String(phase)},
								},
							},
							Period: aws.Int32(period),
							Stat:   aws.String("Maximum"),
						},
						ReturnData: aws.Bool(true),
					})
				}
			}
		}
	}

	return queries, idToKey
}

func maxTime(ts []time.Time) time.Time {
	m := ts[0]
	for _, t := range ts[1:] {
		if t.After(m) {
			m = t
		}
	}
	return m
}

// deriveGap infers the idle gap that separates two benchmark runs from the raw
// RunMarker timestamps. Runs are sequential per host (Arch), so for each arch it
// finds, for every start marker, the nearest preceding end marker and treats the
// span between them as an inter-run break. The returned gap sits halfway between
// the CloudWatch period (the intra-run datapoint spacing) and the smallest
// observed break, so fallback run detection splits between runs but not within a
// run. Returns 0 when no break can be determined.
func deriveGap(starts, ends map[string][]time.Time, period int32) time.Duration {
	periodDur := time.Duration(period) * time.Second

	minBreak := time.Duration(0)
	found := false
	for arch, ss := range starts {
		es := ends[arch]
		for _, s := range ss {
			var prev time.Time
			for _, e := range es {
				if e.Before(s) && e.After(prev) {
					prev = e
				}
			}
			if prev.IsZero() {
				continue
			}
			b := s.Sub(prev)
			if b <= 0 {
				continue
			}
			if !found || b < minBreak {
				minBreak = b
				found = true
			}
		}
	}

	if !found {
		return 0
	}
	if minBreak <= periodDur {
		return periodDur + periodDur/2
	}
	return (periodDur + minBreak) / 2
}

// fetchRunWindows derives, for each runKey, the [start, end] window of its most
// recent run from the RunMarker datapoints. A small slack of one period is
// applied on each side so datapoints on the boundary buckets are retained. It
// also returns a gap auto-derived from the markers (see deriveGap) for use as
// the fallback run-segmentation threshold. Returns an empty map and a zero gap
// when no markers exist (callers fall back to gap detection).
func fetchRunWindows(
	ctx context.Context,
	client *cloudwatch.Client,
	startTime, endTime time.Time,
	period int32,
) (map[runKey]timeWindow, time.Duration, error) {
	queries, idToKey := buildRunMarkerQueries(period)
	raw, err := fetchRawSeries(ctx, client, queries, startTime, endTime)
	if err != nil {
		return nil, 0, err
	}

	type acc struct {
		starts, ends []time.Time
	}
	accs := make(map[runKey]*acc)
	// Per-arch marker timestamps used to auto-derive the inter-run gap.
	archStarts := make(map[string][]time.Time)
	archEnds := make(map[string][]time.Time)
	for id, pts := range raw {
		k, ok := idToKey[id]
		if !ok {
			continue
		}
		rk := runKey{Label: k.Label, Size: k.Size, Arch: k.Arch}
		a := accs[rk]
		if a == nil {
			a = &acc{}
			accs[rk] = a
		}
		for _, p := range pts {
			if k.Phase == "start" {
				a.starts = append(a.starts, p.ts)
				archStarts[k.Arch] = append(archStarts[k.Arch], p.ts)
			} else {
				a.ends = append(a.ends, p.ts)
				archEnds[k.Arch] = append(archEnds[k.Arch], p.ts)
			}
		}
	}

	gap := deriveGap(archStarts, archEnds, period)

	slack := time.Duration(period) * time.Second
	windows := make(map[runKey]timeWindow)
	for rk, a := range accs {
		if len(a.starts) == 0 {
			continue
		}
		start := maxTime(a.starts)
		// The end of the most recent run is the latest end marker at/after the
		// most recent start. If none exists the run is still in progress.
		end := endTime
		var best time.Time
		for _, e := range a.ends {
			if !e.Before(start) && e.After(best) {
				best = e
			}
		}
		if !best.IsZero() {
			end = best
		}
		windows[rk] = timeWindow{start: start.Add(-slack), end: end.Add(slack)}
	}

	return windows, gap, nil
}

func runKeysFromMetric(m map[string]metricKey) map[string]runKey {
	out := make(map[string]runKey, len(m))
	for id, k := range m {
		out[id] = runKey{Label: k.Label, Size: k.Size, Arch: k.Arch}
	}
	return out
}

func runKeysFromAPI(m map[string]apiMetricKey) map[string]runKey {
	out := make(map[string]runKey, len(m))
	for id, k := range m {
		out[id] = runKey{Label: k.Label, Size: k.Size, Arch: k.Arch}
	}
	return out
}

func runKeysFromRuntime(m map[string]runtimeMetricKey) map[string]runKey {
	out := make(map[string]runKey, len(m))
	for id, k := range m {
		out[id] = runKey{Label: k.Label, Size: k.Size, Arch: k.Arch}
	}
	return out
}

// tsPoint is a single timestamped CloudWatch datapoint.
type tsPoint struct {
	ts  time.Time
	val float64
}

// fetchRawSeries fetches every datapoint (timestamp + value) for each query id,
// accumulating across pagination and chunking. Run segmentation is applied by
// the callers via mostRecentRunValues.
func fetchRawSeries(
	ctx context.Context,
	client *cloudwatch.Client,
	queries []cwtypes.MetricDataQuery,
	startTime, endTime time.Time,
) (map[string][]tsPoint, error) {
	results := make(map[string][]tsPoint)

	const chunkSize = 500
	for i := 0; i < len(queries); i += chunkSize {
		end := i + chunkSize
		if end > len(queries) {
			end = len(queries)
		}
		chunk := queries[i:end]

		var nextToken *string
		for {
			resp, err := client.GetMetricData(ctx, &cloudwatch.GetMetricDataInput{
				StartTime:         aws.Time(startTime),
				EndTime:           aws.Time(endTime),
				MetricDataQueries: chunk,
				NextToken:         nextToken,
			})
			if err != nil {
				return nil, fmt.Errorf("GetMetricData: %w", err)
			}

			for _, r := range resp.MetricDataResults {
				id := aws.ToString(r.Id)
				n := len(r.Values)
				if len(r.Timestamps) < n {
					n = len(r.Timestamps)
				}
				for j := 0; j < n; j++ {
					results[id] = append(results[id], tsPoint{ts: r.Timestamps[j], val: r.Values[j]})
				}
			}

			if resp.NextToken == nil {
				break
			}
			nextToken = resp.NextToken
		}
	}

	return results, nil
}

// mostRecentRunValues returns the values belonging to the most recent
// gap-separated run. Benchmarks are executed one-by-one with an idle break
// between them, so a stretch of >= gap with no datapoints marks a run boundary.
// Only the newest contiguous cluster of datapoints is kept.
func mostRecentRunValues(points []tsPoint, gap time.Duration) []float64 {
	if len(points) == 0 {
		return nil
	}

	sorted := make([]tsPoint, len(points))
	copy(sorted, points)
	sort.Slice(sorted, func(a, b int) bool {
		return sorted[a].ts.After(sorted[b].ts)
	})

	out := []float64{sorted[0].val}
	for i := 1; i < len(sorted); i++ {
		// Descending order: sorted[i-1] is newer than sorted[i].
		if sorted[i-1].ts.Sub(sorted[i].ts) > gap {
			break
		}
		out = append(out, sorted[i].val)
	}
	return out
}

// selectRunValues returns the values of a single run for one query id. When an
// explicit RunMarker window is known for the id's series it filters datapoints
// to that window; otherwise it falls back to gap-based detection of the most
// recent run.
func selectRunValues(
	id string,
	points []tsPoint,
	keyOf map[string]runKey,
	windows map[runKey]timeWindow,
	gap time.Duration,
) []float64 {
	if rk, ok := keyOf[id]; ok {
		if w, ok := windows[rk]; ok {
			var out []float64
			for _, p := range points {
				if !p.ts.Before(w.start) && !p.ts.After(w.end) {
					out = append(out, p.val)
				}
			}
			return out
		}
	}
	return mostRecentRunValues(points, gap)
}

// fetchStat averages the datapoints of the most recent run for each query id.
func fetchStat(
	ctx context.Context,
	client *cloudwatch.Client,
	queries []cwtypes.MetricDataQuery,
	startTime, endTime time.Time,
	gap time.Duration,
	keyOf map[string]runKey,
	windows map[runKey]timeWindow,
) (map[string]float64, error) {
	raw, err := fetchRawSeries(ctx, client, queries, startTime, endTime)
	if err != nil {
		return nil, err
	}

	results := make(map[string]float64)
	// Preserve prior behaviour: ids with no data report NaN.
	for _, q := range queries {
		if q.Id != nil {
			results[aws.ToString(q.Id)] = math.NaN()
		}
	}

	for id, pts := range raw {
		vals := selectRunValues(id, pts, keyOf, windows, gap)
		if len(vals) == 0 {
			results[id] = math.NaN()
			continue
		}
		results[id] = sumFloat64s(vals) / float64(len(vals))
	}

	return results, nil
}

// fetchSeriesValues returns the raw datapoints of the most recent run per id.
func fetchSeriesValues(
	ctx context.Context,
	client *cloudwatch.Client,
	queries []cwtypes.MetricDataQuery,
	startTime, endTime time.Time,
	gap time.Duration,
	keyOf map[string]runKey,
	windows map[runKey]timeWindow,
) (map[string][]float64, error) {
	raw, err := fetchRawSeries(ctx, client, queries, startTime, endTime)
	if err != nil {
		return nil, err
	}

	results := make(map[string][]float64)
	for id, pts := range raw {
		results[id] = selectRunValues(id, pts, keyOf, windows, gap)
	}

	return results, nil
}

func sumFloat64s(values []float64) float64 {
	sum := 0.0
	for _, v := range values {
		sum += v
	}
	return sum
}

func deltaFloat64s(values []float64) float64 {
	if len(values) == 0 {
		return 0
	}
	minV, maxV := values[0], values[0]
	for _, v := range values[1:] {
		if v < minV {
			minV = v
		}
		if v > maxV {
			maxV = v
		}
	}
	return maxV - minV
}

type runtimeMetricKey struct {
	Label      string
	MetricName string
	Size       string
	Arch       string
}

type allData struct {
	avg, p50, p90, p95, p99, min, max map[string]float64
	apiAvg                            map[string]float64
	apiCountSeries                    map[string][]float64
	runtimeMetrics                    map[string]float64
	runtimeCountSeries                map[string][]float64
	// rps holds the mean successful DynamoDB requests/sec during the run,
	// keyed by the id from buildRPSQueries.
	rps map[string]float64
}

func fetchAll(
	ctx context.Context,
	client *cloudwatch.Client,
	period int32,
	startTime, endTime time.Time,
) (*allData, map[string]metricKey, map[string]apiMetricKey, error) {
	type statDef struct {
		cwStat string
		dest   *map[string]float64
	}

	data := &allData{}
	defs := []statDef{
		{"Average", &data.avg},
		{"p50", &data.p50},
		{"p90", &data.p90},
		{"p95", &data.p95},
		{"p99", &data.p99},
		{"Minimum", &data.min},
		{"Maximum", &data.max},
	}

	// Resolve the exact window of each run from the per-app RunMarker datapoints
	// emitted at the start and end of every run. When markers are absent the
	// windows map is empty and every fetch falls back to gap-based detection of
	// the most recent run, using a gap auto-derived from the marker spacing.
	windows, autoGap, err := fetchRunWindows(ctx, client, startTime, endTime, period)
	if err != nil {
		fmt.Fprintf(os.Stderr, "  ! run markers unavailable, falling back to gap detection: %v\n", err)
		windows = map[runKey]timeWindow{}
		autoGap = 0
	} else {
		fmt.Fprintf(os.Stderr, "  ✓ resolved %d run windows from markers\n", len(windows))
	}

	// The fallback segmentation gap is derived entirely from the RunMarker
	// spacing; a conservative default is used only when no markers exist.
	var gap time.Duration
	if autoGap > 0 {
		gap = autoGap
		fmt.Fprintf(os.Stderr, "  ✓ auto-detected gap %s from RunMarker spacing\n", gap)
	} else {
		gap = 15 * time.Minute
		fmt.Fprintf(os.Stderr, "  ! no RunMarker spacing available, using default gap %s\n", gap)
	}

	var idToKey map[string]metricKey

	for _, d := range defs {
		queries, ik := buildQueries(d.cwStat, period)
		if idToKey == nil {
			idToKey = ik
		}

		vals, err := fetchStat(ctx, client, queries, startTime, endTime, gap, runKeysFromMetric(ik), windows)
		if err != nil {
			return nil, nil, nil, fmt.Errorf("fetching %s: %w", d.cwStat, err)
		}
		*d.dest = vals
		fmt.Fprintf(os.Stderr, "  ✓ fetched %s (%d series)\n", d.cwStat, len(vals))
	}

	apiQueries, apiIDToKey := buildAPIQueries("Average", period)
	apiVals, err := fetchStat(ctx, client, apiQueries, startTime, endTime, gap, runKeysFromAPI(apiIDToKey), windows)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("fetching API durations: %w", err)
	}
	data.apiAvg = apiVals
	fmt.Fprintf(os.Stderr, "  ✓ fetched API durations (%d series)\n", len(apiVals))

	apiCountQueries, apiCountIDToKey := buildAPICountQueries(period)
	apiCountSeries, err := fetchSeriesValues(ctx, client, apiCountQueries, startTime, endTime, gap, runKeysFromAPI(apiCountIDToKey), windows)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("fetching API counts: %w", err)
	}
	data.apiCountSeries = apiCountSeries
	fmt.Fprintf(os.Stderr, "  ✓ fetched API counts (%d series)\n", len(apiCountSeries))

	runtimeQueries, runtimeIDToKey := buildRuntimeMetricsQueries(period)
	runtimeKeyOf := runKeysFromRuntime(runtimeIDToKey)
	runtimeVals, err := fetchStat(ctx, client, runtimeQueries, startTime, endTime, gap, runtimeKeyOf, windows)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("fetching runtime metrics: %w", err)
	}
	data.runtimeMetrics = runtimeVals
	fmt.Fprintf(os.Stderr, "  ✓ fetched runtime metrics (%d series)\n", len(runtimeVals))

	runtimeCountSeries, err := fetchSeriesValues(ctx, client, runtimeQueries, startTime, endTime, gap, runtimeKeyOf, windows)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("fetching runtime metric series: %w", err)
	}
	data.runtimeCountSeries = runtimeCountSeries
	fmt.Fprintf(os.Stderr, "  ✓ fetched runtime metric series (%d series)\n", len(runtimeCountSeries))

	rpsQueries, rpsKeyOf := buildRPSQueries(period)
	rpsAvg, err := fetchStat(ctx, client, rpsQueries, startTime, endTime, gap, rpsKeyOf, windows)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("fetching RPS: %w", err)
	}
	// Each datapoint is the total successful requests in one `period` bucket;
	// dividing the run's mean bucket count by the period yields requests/sec.
	data.rps = make(map[string]float64, len(rpsAvg))
	for id, v := range rpsAvg {
		data.rps[id] = v / float64(period)
	}
	fmt.Fprintf(os.Stderr, "  ✓ fetched RPS (%d series)\n", len(rpsAvg))

	return data, idToKey, apiIDToKey, nil
}

// buildRPSQueries builds one metric-math query per (namespace × size × arch)
// that sums the AWS/DynamoDB SuccessfulRequestLatency SampleCount across all
// operations for the benchmark's table, per `period`-second bucket.
func buildRPSQueries(period int32) ([]cwtypes.MetricDataQuery, map[string]runKey) {
	var queries []cwtypes.MetricDataQuery
	keyOf := make(map[string]runKey)

	for _, ns := range namespaces {
		label := nsLabel[ns]
		prefix := nsTablePrefix[ns]
		for _, sz := range sizes {
			for _, arch := range arches {
				table := fmt.Sprintf("%s_%s_%s", prefix, arch, sz)
				id := safeID("rps", label, sz, arch, "sum")
				keyOf[id] = runKey{Label: label, Size: sz, Arch: arch}
				expr := fmt.Sprintf(
					`SUM(SEARCH('{AWS/DynamoDB,Operation,TableName} MetricName="SuccessfulRequestLatency" TableName="%s"', 'SampleCount', %d))`,
					table, period,
				)
				queries = append(queries, cwtypes.MetricDataQuery{
					Id:         aws.String(id),
					Label:      aws.String(fmt.Sprintf("%s/%s/%s/rps", label, sz, arch)),
					Expression: aws.String(expr),
					ReturnData: aws.Bool(true),
				})
			}
		}
	}

	return queries, keyOf
}

func buildAPIResultTable(data *allData, idToKey map[string]apiMetricKey) map[apiMetricKey]float64 {
	table := make(map[apiMetricKey]float64, len(idToKey))
	for id, k := range idToKey {
		v := nanDefault(data.apiAvg[id])
		// v1 API durations are emitted in milliseconds; normalise to µs.
		if k.Label == "v1" {
			v *= 1000
		}
		table[k] = v
	}
	return table
}

// buildResultTable converts raw per-id maps into a metricKey → metricStats map.
func buildResultTable(data *allData, idToKey map[string]metricKey) map[metricKey]metricStats {
	table := make(map[metricKey]metricStats, len(idToKey))

	for avgID, k := range idToKey {
		ns := namespaceForLabel(k.Label)
		table[k] = metricStats{
			Average: nanDefault(data.avg[avgID]),
			P50:     nanDefault(data.p50[safeID(ns, k.Operation, k.Size, k.Arch, "p50")]),
			P90:     nanDefault(data.p90[safeID(ns, k.Operation, k.Size, k.Arch, "p90")]),
			P95:     nanDefault(data.p95[safeID(ns, k.Operation, k.Size, k.Arch, "p95")]),
			P99:     nanDefault(data.p99[safeID(ns, k.Operation, k.Size, k.Arch, "p99")]),
			Min:     nanDefault(data.min[safeID(ns, k.Operation, k.Size, k.Arch, "Minimum")]),
			Max:     nanDefault(data.max[safeID(ns, k.Operation, k.Size, k.Arch, "Maximum")]),
		}
	}

	return table
}

func renderAppendixSummary(table map[metricKey]metricStats, apiTable map[apiMetricKey]float64) string {
	var sb strings.Builder

	buildSerLine := func(op string) string {
		comparisons := 0
		sumV2VsV1Pct := 0.0
		codegenComparisons := 0
		codegenWins := 0
		codegenWinMarginSum := 0.0
		ptrComparisons := 0
		ptrWins := 0
		ptrWinMarginSum := 0.0

		for _, arch := range arches {
			for _, sz := range sizes {
				v1, hasV1 := table[metricKey{Label: "v1", Operation: op, Size: sz, Arch: arch}]
				v2, hasV2 := table[metricKey{Label: "v2", Operation: op, Size: sz, Arch: arch}]
				if hasV1 && hasV2 && v1.Average > 0 {
					sumV2VsV1Pct += (v2.Average - v1.Average) / v1.Average * 100
					comparisons++
				}

				v2ptr, hasV2Ptr := table[metricKey{Label: "v2-ptr", Operation: op, Size: sz, Arch: arch}]
				v2codegen, hasV2Codegen := table[metricKey{Label: "v2-codegen", Operation: op, Size: sz, Arch: arch}]
				if hasV2 && hasV2Codegen {
					codegenComparisons++
					if v2codegen.Average < v2.Average {
						codegenWins++
						if v2.Average > 0 {
							codegenWinMarginSum += (v2.Average - v2codegen.Average) / v2.Average * 100
						}
					}
				}
				if hasV2 && hasV2Ptr {
					ptrComparisons++
					if v2ptr.Average < v2.Average {
						ptrWins++
						if v2.Average > 0 {
							ptrWinMarginSum += (v2.Average - v2ptr.Average) / v2.Average * 100
						}
					}
				}
			}
		}

		if comparisons == 0 {
			return "  - insufficient comparable v1/v2 datapoints."
		}

		avgPct := sumV2VsV1Pct / float64(comparisons)
		trend := "faster"
		if avgPct > 0 {
			trend = "slower"
		}

		lines := []string{fmt.Sprintf("  - v2 vs v1: %.2f%% %s on average", math.Abs(avgPct), trend)}

		if codegenComparisons > 0 {
			codegenWinMarginAvg := 0.0
			if codegenWins > 0 {
				codegenWinMarginAvg = codegenWinMarginSum / float64(codegenWins)
			}
			lines = append(lines, fmt.Sprintf("  - v2-codegen vs v2: wins, by %.2f%% on average when it wins",
				codegenWinMarginAvg,
			))
		}

		if ptrComparisons > 0 {
			ptrWinMarginAvg := 0.0
			if ptrWins > 0 {
				ptrWinMarginAvg = ptrWinMarginSum / float64(ptrWins)
			}
			lines = append(lines, fmt.Sprintf("  - v2-ptr vs v2: wins, by %.2f%% on average when it wins",
				ptrWinMarginAvg,
			))
		}

		return strings.Join(lines, "\n")
	}

	avgAPIDelta := func(target, baseline string, ops []string) (float64, int) {
		sum := 0.0
		count := 0
		for _, apiOp := range ops {
			for _, arch := range arches {
				for _, sz := range sizes {
					b := apiTable[apiMetricKey{Label: baseline, APIName: apiOp, Size: sz, Arch: arch}]
					t := apiTable[apiMetricKey{Label: target, APIName: apiOp, Size: sz, Arch: arch}]
					if b <= 0 || t <= 0 {
						continue
					}
					sum += (t - b) / b * 100
					count++
				}
			}
		}
		if count == 0 {
			return 0, 0
		}
		return sum / float64(count), count
	}

	formatAPIComparison := func(target, baseline string, ops []string) string {
		avg, n := avgAPIDelta(target, baseline, ops)
		if n == 0 {
			return fmt.Sprintf("%s vs %s: n/a", target, baseline)
		}
		trend := "faster"
		if avg > 0 {
			trend = "slower"
		}
		return fmt.Sprintf("%s vs %s: %.2f%% %s", target, baseline, math.Abs(avg), trend)
	}

	sb.WriteString("- **Serialization (MarshalMap)**\n")
	sb.WriteString(buildSerLine("MarshalMap"))
	sb.WriteString("\n")
	sb.WriteString("- **Serialization (UnmarshalMap)**\n")
	sb.WriteString(buildSerLine("UnmarshalMap"))
	sb.WriteString("\n")
	sb.WriteString("- **Item operations**\n")
	sb.WriteString("  - **write APIs**\n")
	sb.WriteString("    - ")
	sb.WriteString(formatAPIComparison("v2", "v1", marshalOps))
	sb.WriteString("\n")
	sb.WriteString("    - ")
	sb.WriteString(formatAPIComparison("v2-codegen", "v2", marshalOps))
	sb.WriteString("\n")
	sb.WriteString("    - ")
	sb.WriteString(formatAPIComparison("v2-ptr", "v2", marshalOps))
	sb.WriteString("\n")
	sb.WriteString("  - **read APIs**\n")
	sb.WriteString("    - ")
	sb.WriteString(formatAPIComparison("v2", "v1", unmarshalOps))
	sb.WriteString("\n")
	sb.WriteString("    - ")
	sb.WriteString(formatAPIComparison("v2-codegen", "v2", unmarshalOps))
	sb.WriteString("\n")
	sb.WriteString("    - ")
	sb.WriteString(formatAPIComparison("v2-ptr", "v2", unmarshalOps))

	return sb.String()
}

// renderRPS renders a per-arch SDK×size table of mean requests/sec sustained
// during the run, derived from AWS/DynamoDB SuccessfulRequestLatency SampleCount.
func renderRPS(data *allData) string {
	var sb strings.Builder

	sb.WriteString("##### Requests Per Second (RPS)\n\n")
	sb.WriteString("Mean successful DynamoDB requests per second during the run, summed across all operations on the benchmark's table (`AWS/DynamoDB` `SuccessfulRequestLatency` SampleCount ÷ period). **Higher is better** — it reflects the throughput the host sustained for the same workload.\n\n")

	for _, arch := range arches {
		for _, sz := range sizes {
			sb.WriteString(fmt.Sprintf("**Arch: %s — Size: %s**\n\n", arch, sz))

			sb.WriteString("| SDK | RPS |\n")
			sb.WriteString("|:---|---:|\n")

			for _, ns := range namespaces {
				label := nsLabel[ns]
				id := safeID("rps", label, sz, arch, "sum")
				v, ok := data.rps[id]
				if ok && !math.IsNaN(v) && v > 0 {
					sb.WriteString(fmt.Sprintf("| %-12s | %.1f |\n", label, v))
				} else {
					sb.WriteString(fmt.Sprintf("| %-12s | — |\n", label))
				}
			}
			sb.WriteString("\n")
		}
	}

	return sb.String()
}

func renderRuntimeMetrics(data *allData) string {
	var sb strings.Builder

	sb.WriteString("#### App Runtime Performance\n\n")
	sb.WriteString("> These metrics measure the efficiency characteristics that directly impact host throughput (RPS). Lower values indicate better efficiency and the ability to achieve higher requests per second on the same hardware.\n")
	sb.WriteString("> `—` indicates no data was available for that combination.\n\n")

	sb.WriteString(renderRPS(data))

	// Focus on efficiency metrics that impact host throughput: GC overhead, retained heap pressure, pause duration, and allocation activity.
	runtimeMetrics := []string{
		"Memory.GCCPUFraction",
		"Client.CPU.Usage",
		"Client.CPU.Idle",
		"Client.CPU.User",
		"Client.CPU.System",
		"Client.CPU.IOWait",
		"gc.heap.net_bytes",
		"Memory.PauseUs",
	}
	metricLabels := map[string]string{
		"Memory.GCCPUFraction": "GC Overhead (% of CPU)",
		"Client.CPU.Usage":     "Host CPU Usage (%)",
		"Client.CPU.Idle":      "Host CPU Idle (%)",
		"Client.CPU.User":      "Host CPU User (%)",
		"Client.CPU.System":    "Host CPU System (%)",
		"Client.CPU.IOWait":    "Host CPU IOWait (%)",
		"gc.heap.net_bytes":    "Net Heap Bytes (allocs - frees)",
		"Memory.PauseUs":       "GC Pause Duration (µs)",
	}

	metricDescriptions := map[string]string{
		"Memory.GCCPUFraction": "Percentage of CPU time spent in garbage collection. **Lower percentages are better** — they indicate less CPU time wasted on GC pauses. Differences of even 10-15% are significant and compound during sustained load. Less GC overhead means more CPU available for useful work, directly translating to higher achievable RPS on a fixed-capacity host.",
		"Client.CPU.Usage":     "Overall host CPU busy percentage sampled over the benchmark interval. **Lower is generally better** if throughput is unchanged because it indicates less host CPU consumed per unit of work.",
		"Client.CPU.Idle":      "Overall host CPU idle percentage. **Higher values are better** when request throughput is comparable, indicating additional available CPU headroom.",
		"Client.CPU.User":      "Percentage of host CPU spent in user-space code. Useful to compare SDK/application execution cost across implementations.",
		"Client.CPU.System":    "Percentage of host CPU spent in kernel/system work. Helps identify syscall/kernel overhead differences.",
		"Client.CPU.IOWait":    "Percentage of host CPU time waiting on I/O. Higher values often indicate I/O stalls rather than pure compute pressure.",
		"gc.heap.net_bytes":    "Derived as `/gc/heap/allocs:bytes - /gc/heap/frees:bytes`. This approximates bytes retained in the heap from cumulative allocation activity. **Lower values are better** because they indicate less retained memory pressure and typically less GC work needed to keep the heap healthy under sustained load.",
		"Memory.PauseUs":       "Average duration of individual GC pause events in microseconds. **Lower values indicate shorter pause times**, which means less latency jitter and more consistent application responsiveness. Even modest reductions in pause time (e.g., 10-20µs) improve tail latency under sustained load.",
	}

	// One table per metric
	for _, metricName := range runtimeMetrics {
		sb.WriteString(fmt.Sprintf("##### %s\n\n", metricLabels[metricName]))
		sb.WriteString(fmt.Sprintf("%s\n\n", metricDescriptions[metricName]))

		// One subtable per architecture
		for _, arch := range arches {
			sb.WriteString(fmt.Sprintf("**Arch: %s**\n\n", arch))

			// Build header: SDK | Size1 | Size2 | ...
			sb.WriteString("| SDK |")
			for _, sz := range sizes {
				sb.WriteString(fmt.Sprintf(" %s |", sz))
			}
			sb.WriteString("\n")

			// Build separator
			sb.WriteString("|:---|")
			for range sizes {
				sb.WriteString("---:|")
			}
			sb.WriteString("\n")

			// Build rows for each SDK
			for _, ns := range namespaces {
				label := nsLabel[ns]
				sb.WriteString(fmt.Sprintf("| %-12s |", label))

				for _, sz := range sizes {
					val, ok := getRuntimeMetricValue(data.runtimeMetrics, ns, metricName, sz, arch)
					if ok {
						sb.WriteString(fmt.Sprintf(" %s |", formatMetricValue(metricName, val)))
					} else {
						sb.WriteString(" — |")
					}
				}
				sb.WriteString("\n")
			}
			sb.WriteString("\n")
		}
	}

	sb.WriteString("##### Allocation activity (Allocs/op and Frees/op)\n\n")
	sb.WriteString("Per-request heap-object allocation and free counts derived from the `runtime/metrics` cumulative counters `/gc/heap/allocs:objects` and `/gc/heap/frees:objects` — the same figures Go test benchmarks aggregate to report `allocs/op`. Normalized by the total API operation count in the same benchmark window for the same SDK/size/arch.\n\n")

	for _, arch := range arches {
		sb.WriteString(fmt.Sprintf("**Arch: %s**\n\n", arch))
		allAPIops := append(append([]string{}, marshalOps...), unmarshalOps...)

		sb.WriteString("| SDK |")
		for _, sz := range sizes {
			sb.WriteString(fmt.Sprintf(" %s Allocs/op | %s Frees/op |", sz, sz))
		}
		sb.WriteString("\n")

		sb.WriteString("|:---|")
		for range sizes {
			sb.WriteString("---:|---:|")
		}
		sb.WriteString("\n")

		for _, ns := range namespaces {
			label := nsLabel[ns]
			sb.WriteString(fmt.Sprintf("| %-12s |", label))

			for _, sz := range sizes {
				apiTotal := 0.0
				for _, apiOp := range allAPIops {
					id := apiQueryID(ns, apiOp, sz, arch)
					apiTotal += sumFloat64s(data.apiCountSeries[id])
				}

				allocsDelta := deltaFloat64s(data.runtimeCountSeries[safeID(ns, "gc.heap.allocs.objects", sz, arch, "Maximum")])
				freesDelta := deltaFloat64s(data.runtimeCountSeries[safeID(ns, "gc.heap.frees.objects", sz, arch, "Maximum")])

				if apiTotal == 0 {
					sb.WriteString(" — | — |")
					continue
				}

				sb.WriteString(fmt.Sprintf(" %.3f |", allocsDelta/apiTotal))
				sb.WriteString(fmt.Sprintf(" %.3f |", freesDelta/apiTotal))
			}
			sb.WriteString("\n")
		}
		sb.WriteString("\n")
	}

	return sb.String()
}

func renderMarkdown(table map[metricKey]metricStats, apiTable map[apiMetricKey]float64, data *allData) string {
	var sb strings.Builder

	sb.WriteString("### Appendix #7: Benchmarks\n\n")
	sb.WriteString("> Data collected from CloudWatch. All durations are in **µs** (microseconds).\n")
	sb.WriteString("> `MarshalMap` = struct → `map[string]types.AttributeValue`; ")
	sb.WriteString("`UnmarshalMap` = `map[string]types.AttributeValue` → struct.\n")
	sb.WriteString("> `—` indicates no data was available for that combination.\n\n")

	for _, op := range operations {
		sb.WriteString(fmt.Sprintf("#### %s\n\n", op))

		for _, arch := range arches {
			for _, sz := range sizes {
				sb.WriteString(fmt.Sprintf("**Arch: %s — %s**\n\n", arch, sz))
				sb.WriteString("| SDK | Average | p50 | p90 | p95 | p99 | Min | Max |\n")
				sb.WriteString("|:---|--------:|----:|----:|----:|----:|----:|----:|\n")

				for _, ns := range namespaces {
					label := nsLabel[ns]
					k := metricKey{Label: label, Operation: op, Size: sz, Arch: arch}
					if s, ok := table[k]; ok {
						sb.WriteString(fmt.Sprintf("| %-12s | %s | %s | %s | %s | %s | %s | %s |\n",
							label,
							formatMicros(s.Average),
							formatMicros(s.P50),
							formatMicros(s.P90),
							formatMicros(s.P95),
							formatMicros(s.P99),
							formatMicros(s.Min),
							formatMicros(s.Max),
						))
					}
				}
				sb.WriteString("\n")
			}
		}
	}

	sb.WriteString("#### Overhead vs baseline\n\n")
	sb.WriteString("> `v2` is compared against `v1`; `v2-codegen` and `v2-ptr` are compared against `v2`.\n")
	sb.WriteString("> Positive values mean slower than the baseline; negative values mean faster.\n\n")
	sb.WriteString("> Metric basis: `Δ avg` uses `Average` latency; `Δ p50` uses `p50` latency for `MarshalMap` / `UnmarshalMap`.\n\n")

	for _, op := range operations {
		sb.WriteString(fmt.Sprintf("##### %s\n\n", op))

		for _, arch := range arches {
			for _, sz := range sizes {
				v1Key := metricKey{Label: "v1", Operation: op, Size: sz, Arch: arch}
				v1, hasV1 := table[v1Key]
				v2Key := metricKey{Label: "v2", Operation: op, Size: sz, Arch: arch}
				v2, hasV2 := table[v2Key]
				if !hasV1 && !hasV2 {
					continue
				}

				sb.WriteString(fmt.Sprintf("**Arch: %s — %s**\n\n", arch, sz))
				sb.WriteString("| SDK | Baseline | Average | Δ avg | p50 | Δ p50 |\n")
				sb.WriteString("|:---|:----------|--------:|------:|----:|------:|\n")

				if hasV1 {
					sb.WriteString(fmt.Sprintf("| %-12s | — | %s | — | %s | — |\n",
						"v1", formatMicros(v1.Average), formatMicros(v1.P50)))
				}
				if hasV2 {
					if hasV1 {
						sb.WriteString(fmt.Sprintf("| %-12s | v1 | %s | %s | %s | %s |\n",
							"v2",
							formatMicros(v2.Average),
							formatDelta(v2.Average-v1.Average, v1.Average),
							formatMicros(v2.P50),
							formatDelta(v2.P50-v1.P50, v1.P50),
						))
					} else {
						sb.WriteString(fmt.Sprintf("| %-12s | — | %s | — | %s | — |\n",
							"v2", formatMicros(v2.Average), formatMicros(v2.P50)))
					}
				}

				for _, variant := range []string{"v2-codegen", "v2-ptr"} {
					k := metricKey{Label: variant, Operation: op, Size: sz, Arch: arch}
					s, ok := table[k]
					if !ok {
						continue
					}
					if hasV2 {
						sb.WriteString(fmt.Sprintf("| %-12s | v2 | %s | %s | %s | %s |\n",
							variant,
							formatMicros(s.Average),
							formatDelta(s.Average-v2.Average, v2.Average),
							formatMicros(s.P50),
							formatDelta(s.P50-v2.P50, v2.P50),
						))
					} else {
						sb.WriteString(fmt.Sprintf("| %-12s | — | %s | — | %s | — |\n",
							variant, formatMicros(s.Average), formatMicros(s.P50)))
					}
				}
				sb.WriteString("\n")
			}
		}
	}

	sb.WriteString("#### Overhead vs API operations\n\n")
	sb.WriteString("> Shows what percentage of the API call duration is spent in `MarshalMap` / `UnmarshalMap`.\n")
	sb.WriteString("> `v1` API durations are in ms and are normalised to µs before computing the ratio.\n\n")
	sb.WriteString("> Metric basis: ratio uses `Average(MarshalMap|UnmarshalMap) / Average(API duration)`.\n\n")

	for _, op := range operations {
		apiOps := serToAPIOps[op]
		sb.WriteString(fmt.Sprintf("##### %s\n\n", op))

		for _, arch := range arches {
			for _, sz := range sizes {
				sb.WriteString(fmt.Sprintf("**Arch: %s — %s**\n\n", arch, sz))

				// Header: SDK | ser (µs) | one column per API op
				header := "| SDK | " + op + " (µs) |"
				sep := "|:---|----------:|"
				for _, apiOp := range apiOps {
					header += fmt.Sprintf(" %s overhead |", apiOp)
					sep += "----------:|"
				}
				sb.WriteString(header + "\n")
				sb.WriteString(sep + "\n")

				for _, ns := range namespaces {
					label := nsLabel[ns]
					sk := metricKey{Label: label, Operation: op, Size: sz, Arch: arch}
					serStats, ok := table[sk]
					if !ok {
						continue
					}
					row := fmt.Sprintf("| %-12s | %s |", label, formatMicros(serStats.Average))
					for _, apiOp := range apiOps {
						ak := apiMetricKey{Label: label, APIName: apiOp, Size: sz, Arch: arch}
						apiDur := apiTable[ak]
						row += fmt.Sprintf(" %s |", formatDurationPct(serStats.Average, apiDur))
					}
					sb.WriteString(row + "\n")
				}
				sb.WriteString("\n")
			}
		}
	}

	sb.WriteString("#### Summary\n\n")
	sb.WriteString(renderAppendixSummary(table, apiTable))
	sb.WriteString("\n")
	sb.WriteString("\n#### Caveats\n\n")
	sb.WriteString("- Item-operation timings are based on API duration metrics (`client.call.duration` for v2 and operation metrics for v1), which represent end-to-end call time, not just wire/network time.\n")
	sb.WriteString("- Even with the same SDK, differences in request payload shape/size, retries, connection reuse, and normal service variability can shift API duration numbers between benchmark variants.\n")
	sb.WriteString("- Treat API-operation deltas as observational results for this dataset; they are not by themselves strict causal proof that the proposed application-level `MarshalMap`/`UnmarshalMap` struct<->`map[string]types.AttributeValue` mapping changes improved full API latency. These are not changes to SDK internal request/response wire serialization or deserialization.\n")

	sb.WriteString("\n\n### Appendix #8: App Runtime Performance\n\n")
	sb.WriteString(renderRuntimeMetrics(data))

	sb.WriteString(renderConclusions(table, apiTable, data))

	return sb.String()
}

// avgSerLatency averages the `Average` latency for the given (label, op) across all (size × arch).
func avgSerLatency(table map[metricKey]metricStats, label, op string) float64 {
	sum, n := 0.0, 0
	for _, arch := range arches {
		for _, sz := range sizes {
			s, ok := table[metricKey{Label: label, Operation: op, Size: sz, Arch: arch}]
			if !ok || s.Average <= 0 {
				continue
			}
			sum += s.Average
			n++
		}
	}
	if n == 0 {
		return 0
	}
	return sum / float64(n)
}

// avgAPILatency averages API call duration (µs) for the given label across the given API ops × sizes × arches.
func avgAPILatency(apiTable map[apiMetricKey]float64, label string, ops []string) float64 {
	sum, n := 0.0, 0
	for _, op := range ops {
		for _, arch := range arches {
			for _, sz := range sizes {
				v := apiTable[apiMetricKey{Label: label, APIName: op, Size: sz, Arch: arch}]
				if v <= 0 {
					continue
				}
				sum += v
				n++
			}
		}
	}
	if n == 0 {
		return 0
	}
	return sum / float64(n)
}

// avgRuntime averages a runtime metric (Average stat) across all sizes × arches for the given namespace.
func avgRuntime(data *allData, ns, metricName string) float64 {
	sum, n := 0.0, 0
	for _, arch := range arches {
		for _, sz := range sizes {
			v, ok := getRuntimeMetricValue(data.runtimeMetrics, ns, metricName, sz, arch)
			if !ok || v == 0 {
				continue
			}
			sum += v
			n++
		}
	}
	if n == 0 {
		return 0
	}
	return sum / float64(n)
}

// avgPerOp averages a "per API operation" derived value (e.g. allocs/op) across all sizes × arches.
// For each (size, arch): value = (MAX(metric) - MIN(metric)) / Σ SampleCount(API ops).
func avgPerOp(data *allData, ns, metricName string) float64 {
	allAPIops := append(append([]string{}, marshalOps...), unmarshalOps...)
	sum, n := 0.0, 0
	for _, arch := range arches {
		for _, sz := range sizes {
			apiTotal := 0.0
			for _, apiOp := range allAPIops {
				apiTotal += sumFloat64s(data.apiCountSeries[apiQueryID(ns, apiOp, sz, arch)])
			}
			if apiTotal == 0 {
				continue
			}
			d := deltaFloat64s(data.runtimeCountSeries[safeID(ns, metricName, sz, arch, "Maximum")])
			if d <= 0 {
				continue
			}
			sum += d / apiTotal
			n++
		}
	}
	if n == 0 {
		return 0
	}
	return sum / float64(n)
}

func pctDelta(target, baseline float64) (float64, bool) {
	if baseline <= 0 || target <= 0 {
		return 0, false
	}
	return (target - baseline) / baseline * 100, true
}

func formatTrend(target, baseline float64) string {
	d, ok := pctDelta(target, baseline)
	if !ok {
		return "n/a"
	}
	trend := "faster"
	if d > 0 {
		trend = "slower"
	}
	return fmt.Sprintf("%.2f%% %s", math.Abs(d), trend)
}

func formatCountTrend(target, baseline float64) string {
	d, ok := pctDelta(target, baseline)
	if !ok {
		return "n/a"
	}
	trend := "fewer"
	if d > 0 {
		trend = "more"
	}
	return fmt.Sprintf("%.2f%% %s", math.Abs(d), trend)
}

// renderConclusions builds a final "Conclusions" section that summarises every
// metric category by averaging across all (arch × size) combinations and then
// highlights the most actionable comparisons between SDK variants.
func renderConclusions(table map[metricKey]metricStats, apiTable map[apiMetricKey]float64, data *allData) string {
	var sb strings.Builder

	sb.WriteString("\n\n### Conclusions\n\n")
	sb.WriteString("> All values are averaged across every (arch × size) combination measured above. ")
	sb.WriteString("Use these as the headline numbers; consult the per-(arch × size) tables for variability.\n\n")

	// 1) AttributeValue serialization latency
	sb.WriteString("#### AttributeValue serialization latency\n\n")
	sb.WriteString("| SDK | MarshalMap (µs) | UnmarshalMap (µs) |\n")
	sb.WriteString("|:---|---:|---:|\n")
	for _, ns := range namespaces {
		label := nsLabel[ns]
		sb.WriteString(fmt.Sprintf("| %-12s | %.2f | %.2f |\n",
			label,
			avgSerLatency(table, label, "MarshalMap"),
			avgSerLatency(table, label, "UnmarshalMap"),
		))
	}

	// 2) API call duration (per operation)
	sb.WriteString("\n#### API call duration (`client.call.duration` for v2; per-op metric for v1)\n\n")
	// Build a per-operation table instead of grouping write/read operations.
	allAPIops := append(append([]string{}, marshalOps...), unmarshalOps...)
	// Header: SDK | Op1 (µs) | Op2 (µs) | ...
	header := "| SDK |"
	for _, apiOp := range allAPIops {
		header += fmt.Sprintf(" %s (µs) |", apiOp)
	}
	sb.WriteString(header + "\n")
	// Separator
	sep := "|:---|"
	for range allAPIops {
		sep += "---:|"
	}
	sb.WriteString(sep + "\n")
	// Rows: one SDK per row, one column per API operation
	for _, ns := range namespaces {
		label := nsLabel[ns]
		row := fmt.Sprintf("| %-12s |", label)
		for _, apiOp := range allAPIops {
			// avgAPILatency accepts a slice of ops; use a single-op slice to get per-op avg
			v := avgAPILatency(apiTable, label, []string{apiOp})
			row += fmt.Sprintf(" %.2f |", v)
		}
		sb.WriteString(row + "\n")
	}

	// 3) Runtime efficiency
	sb.WriteString("\n#### Runtime efficiency\n\n")
	sb.WriteString("| SDK | GC Overhead | Host CPU Usage | Host CPU Idle | GC Pause (µs) | Allocs/op | Frees/op |\n")
	sb.WriteString("|:---|---:|---:|---:|---:|---:|---:|\n")
	for _, ns := range namespaces {
		label := nsLabel[ns]
		gc := avgRuntime(data, ns, "Memory.GCCPUFraction") * 100
		cpuUsage := avgRuntime(data, ns, "Client.CPU.Usage")
		cpuIdle := avgRuntime(data, ns, "Client.CPU.Idle")
		pause := avgRuntime(data, ns, "Memory.PauseUs")
		allocs := avgPerOp(data, ns, "gc.heap.allocs.objects")
		frees := avgPerOp(data, ns, "gc.heap.frees.objects")
		sb.WriteString(fmt.Sprintf("| %-12s | %.3f%% | %.2f%% | %.2f%% | %.1f | %.2f | %.2f |\n",
			label, gc, cpuUsage, cpuIdle, pause, allocs, frees))
	}

	// 4) Highlights
	sb.WriteString("\n#### Highlights\n\n")

	// Latency comparisons (averaged Average latency across all arch × size).
	type sdkSnap struct {
		marshal, unmarshal float64
		writeAPI, readAPI  float64
		gcPct              float64
		cpuUsagePct        float64
		cpuIdlePct         float64
		pauseUs            float64
		allocsPerOp        float64
		freesPerOp         float64
	}
	snap := map[string]sdkSnap{}
	for _, ns := range namespaces {
		label := nsLabel[ns]
		snap[label] = sdkSnap{
			marshal:     avgSerLatency(table, label, "MarshalMap"),
			unmarshal:   avgSerLatency(table, label, "UnmarshalMap"),
			writeAPI:    avgAPILatency(apiTable, label, marshalOps),
			readAPI:     avgAPILatency(apiTable, label, unmarshalOps),
			gcPct:       avgRuntime(data, ns, "Memory.GCCPUFraction") * 100,
			cpuUsagePct: avgRuntime(data, ns, "Client.CPU.Usage"),
			cpuIdlePct:  avgRuntime(data, ns, "Client.CPU.Idle"),
			pauseUs:     avgRuntime(data, ns, "Memory.PauseUs"),
			allocsPerOp: avgPerOp(data, ns, "gc.heap.allocs.objects"),
			freesPerOp:  avgPerOp(data, ns, "gc.heap.frees.objects"),
		}
	}

	v1, v2, codegen, ptr := snap["v1"], snap["v2"], snap["v2-codegen"], snap["v2-ptr"]

	sb.WriteString("- **AttributeValue serialization (`MarshalMap`)**\n")
	sb.WriteString(fmt.Sprintf("  - v2 vs v1: %s\n", formatTrend(v2.marshal, v1.marshal)))
	sb.WriteString(fmt.Sprintf("  - v2-codegen vs v2: %s\n", formatTrend(codegen.marshal, v2.marshal)))
	sb.WriteString(fmt.Sprintf("  - v2-ptr vs v2: %s\n", formatTrend(ptr.marshal, v2.marshal)))
	sb.WriteString("- **AttributeValue serialization (`UnmarshalMap`)**\n")
	sb.WriteString(fmt.Sprintf("  - v2 vs v1: %s\n", formatTrend(v2.unmarshal, v1.unmarshal)))
	sb.WriteString(fmt.Sprintf("  - v2-codegen vs v2: %s\n", formatTrend(codegen.unmarshal, v2.unmarshal)))
	sb.WriteString(fmt.Sprintf("  - v2-ptr vs v2: %s\n", formatTrend(ptr.unmarshal, v2.unmarshal)))
	sb.WriteString("- **API call duration (write APIs)**\n")
	sb.WriteString(fmt.Sprintf("  - v2 vs v1: %s\n", formatTrend(v2.writeAPI, v1.writeAPI)))
	sb.WriteString(fmt.Sprintf("  - v2-codegen vs v2: %s\n", formatTrend(codegen.writeAPI, v2.writeAPI)))
	sb.WriteString(fmt.Sprintf("  - v2-ptr vs v2: %s\n", formatTrend(ptr.writeAPI, v2.writeAPI)))
	sb.WriteString("- **API call duration (read APIs)**\n")
	sb.WriteString(fmt.Sprintf("  - v2 vs v1: %s\n", formatTrend(v2.readAPI, v1.readAPI)))
	sb.WriteString(fmt.Sprintf("  - v2-codegen vs v2: %s\n", formatTrend(codegen.readAPI, v2.readAPI)))
	sb.WriteString(fmt.Sprintf("  - v2-ptr vs v2: %s\n", formatTrend(ptr.readAPI, v2.readAPI)))

	sb.WriteString("- **GC overhead (% of CPU spent in GC)**\n")
	sb.WriteString(fmt.Sprintf("  - v2 vs v1: %s (v1=%.3f%%, v2=%.3f%%)\n", formatTrend(v2.gcPct, v1.gcPct), v1.gcPct, v2.gcPct))
	sb.WriteString(fmt.Sprintf("  - v2-codegen vs v2: %s (v2-codegen=%.3f%%)\n", formatTrend(codegen.gcPct, v2.gcPct), codegen.gcPct))
	sb.WriteString(fmt.Sprintf("  - v2-ptr vs v2: %s (v2-ptr=%.3f%%)\n", formatTrend(ptr.gcPct, v2.gcPct), ptr.gcPct))
	sb.WriteString("- **Host CPU usage (% busy CPU)**\n")
	sb.WriteString(fmt.Sprintf("  - v2 vs v1: %s (v1=%.2f%%, v2=%.2f%%)\n", formatTrend(v2.cpuUsagePct, v1.cpuUsagePct), v1.cpuUsagePct, v2.cpuUsagePct))
	sb.WriteString(fmt.Sprintf("  - v2-codegen vs v2: %s (v2-codegen=%.2f%%)\n", formatTrend(codegen.cpuUsagePct, v2.cpuUsagePct), codegen.cpuUsagePct))
	sb.WriteString(fmt.Sprintf("  - v2-ptr vs v2: %s (v2-ptr=%.2f%%)\n", formatTrend(ptr.cpuUsagePct, v2.cpuUsagePct), ptr.cpuUsagePct))
	sb.WriteString("- **Host CPU idle (% available headroom)**\n")
	sb.WriteString(fmt.Sprintf("  - v2 vs v1: %s (v1=%.2f%%, v2=%.2f%%)\n", formatTrend(v2.cpuIdlePct, v1.cpuIdlePct), v1.cpuIdlePct, v2.cpuIdlePct))
	sb.WriteString(fmt.Sprintf("  - v2-codegen vs v2: %s (v2-codegen=%.2f%%)\n", formatTrend(codegen.cpuIdlePct, v2.cpuIdlePct), codegen.cpuIdlePct))
	sb.WriteString(fmt.Sprintf("  - v2-ptr vs v2: %s (v2-ptr=%.2f%%)\n", formatTrend(ptr.cpuIdlePct, v2.cpuIdlePct), ptr.cpuIdlePct))
	sb.WriteString("- **GC pause duration (µs)**\n")
	sb.WriteString(fmt.Sprintf("  - v2 vs v1: %s (v1=%.1fµs, v2=%.1fµs)\n", formatTrend(v2.pauseUs, v1.pauseUs), v1.pauseUs, v2.pauseUs))
	sb.WriteString(fmt.Sprintf("  - v2-codegen vs v2: %s (v2-codegen=%.1fµs)\n", formatTrend(codegen.pauseUs, v2.pauseUs), codegen.pauseUs))
	sb.WriteString(fmt.Sprintf("  - v2-ptr vs v2: %s (v2-ptr=%.1fµs)\n", formatTrend(ptr.pauseUs, v2.pauseUs), ptr.pauseUs))
	sb.WriteString("- **Heap allocations per API op (`/gc/heap/allocs:objects`, Go-benchmark `allocs/op`)**\n")
	sb.WriteString(fmt.Sprintf("  - v1=%.0f, v2=%.0f, v2-codegen=%.0f, v2-ptr=%.0f\n",
		v1.allocsPerOp, v2.allocsPerOp, codegen.allocsPerOp, ptr.allocsPerOp))
	sb.WriteString(fmt.Sprintf("  - v2-ptr vs v2: %s allocations per op\n", formatCountTrend(ptr.allocsPerOp, v2.allocsPerOp)))
	sb.WriteString(fmt.Sprintf("  - v2-codegen vs v2: %s allocations per op\n", formatCountTrend(codegen.allocsPerOp, v2.allocsPerOp)))
	sb.WriteString("- **Heap frees per API op (`/gc/heap/frees:objects`)**\n")
	sb.WriteString(fmt.Sprintf("  - v1=%.0f, v2=%.0f, v2-codegen=%.0f, v2-ptr=%.0f\n",
		v1.freesPerOp, v2.freesPerOp, codegen.freesPerOp, ptr.freesPerOp))
	sb.WriteString(fmt.Sprintf("  - v2-ptr vs v2: %s frees per op\n", formatCountTrend(ptr.freesPerOp, v2.freesPerOp)))
	sb.WriteString(fmt.Sprintf("  - v2-codegen vs v2: %s frees per op\n", formatCountTrend(codegen.freesPerOp, v2.freesPerOp)))

	return sb.String()
}

func getRuntimeMetricValue(values map[string]float64, namespace, metricName, size, arch string) (float64, bool) {
	if metricName == "gc.heap.net_bytes" {
		allocID := safeID(namespace, "gc.heap.allocs.bytes", size, arch, "Average")
		freeID := safeID(namespace, "gc.heap.frees.bytes", size, arch, "Average")
		allocV, allocOK := values[allocID]
		freeV, freeOK := values[freeID]
		if !allocOK || !freeOK {
			return 0, false
		}
		return nanDefault(allocV) - nanDefault(freeV), true
	}

	id := safeID(namespace, metricName, size, arch, "Average")
	v, ok := values[id]
	if !ok {
		return 0, false
	}
	return nanDefault(v), true
}

// formatMetricValue formats a runtime metric value based on its type.
func formatMetricValue(metricName string, value float64) string {
	if value == 0 {
		return "—"
	}
	switch metricName {
	case "cpu.classes.total":
		// CPU seconds per second (rate)
		return fmt.Sprintf("%.4f", value)
	case "Memory.GCCPUFraction":
		// Percentage (0-1 range, multiply by 100 for display)
		return fmt.Sprintf("%.2f%%", value*100)
	case "Client.CPU.Usage", "Client.CPU.Idle", "Client.CPU.User", "Client.CPU.System", "Client.CPU.IOWait":
		// Already emitted as percentage values.
		return fmt.Sprintf("%.2f%%", value)
	case "Memory.HeapAlloc":
		// Convert bytes to MiB
		mib := value / 1048576
		return fmt.Sprintf("%.2f MiB", mib)
	case "gc.heap.net_bytes":
		// Derived from alloc bytes - free bytes.
		return fmt.Sprintf("%.2f MiB", value/1048576)
	case "Memory.PauseUs":
		// GC pause duration in microseconds
		return fmt.Sprintf("%.1f µs", value)
	case "Memory.Mallocs", "Memory.Frees", "gc.heap.allocs.objects", "gc.heap.frees.objects":
		// Cumulative allocation/deallocation counts (heap objects).
		return fmt.Sprintf("%.0f", value)
	default:
		return fmt.Sprintf("%.4f", value)
	}
}

func renderCSV(table map[metricKey]metricStats) string {
	var sb strings.Builder
	sb.WriteString("SDK,Operation,Size,Arch,Average_us,p50_us,p90_us,p95_us,p99_us,Min_us,Max_us\n")

	type row struct {
		k metricKey
		s metricStats
	}
	var rows []row
	for k, s := range table {
		rows = append(rows, row{k, s})
	}
	sort.Slice(rows, func(i, j int) bool {
		ki, kj := rows[i].k, rows[j].k
		if ki.Label != kj.Label {
			return ki.Label < kj.Label
		}
		if ki.Operation != kj.Operation {
			return ki.Operation < kj.Operation
		}
		if ki.Arch != kj.Arch {
			return ki.Arch < kj.Arch
		}
		return sizeOrder(ki.Size) < sizeOrder(kj.Size)
	})

	for _, r := range rows {
		sb.WriteString(fmt.Sprintf("%s,%s,%s,%s,%.4f,%.4f,%.4f,%.4f,%.4f,%.4f,%.4f\n",
			r.k.Label, r.k.Operation, r.k.Size, r.k.Arch,
			r.s.Average, r.s.P50, r.s.P90, r.s.P95, r.s.P99,
			r.s.Min, r.s.Max,
		))
	}
	return sb.String()
}

func main() {
	regionFlag := flag.String("region", "", "AWS region (overrides AWS_REGION env var)")
	hoursFlag := flag.Int("hours", 24, "How many hours back to query")
	periodFlag := flag.Int("period", 300, "CloudWatch aggregation period in seconds")
	outputFlag := flag.String("output", "markdown", "Output format: markdown | csv")
	flag.Parse()

	region := resolveRegion()
	if *regionFlag != "" {
		region = *regionFlag
	}

	endTime := time.Now().UTC()
	startTime := endTime.Add(-time.Duration(*hoursFlag) * time.Hour)
	period := int32(*periodFlag)

	fmt.Fprintf(os.Stderr, "Region:  %s\n", region)
	fmt.Fprintf(os.Stderr, "Window:  %s → %s\n",
		startTime.Format(time.RFC3339), endTime.Format(time.RFC3339))
	fmt.Fprintf(os.Stderr, "Period:  %d s\n", period)
	fmt.Fprintf(os.Stderr, "Gap:     auto (from RunMarker start/end)\n")
	fmt.Fprintf(os.Stderr, "Output:  %s\n\n", *outputFlag)

	ctx := context.Background()
	cfg, err := config.LoadDefaultConfig(ctx, config.WithRegion(region))
	if err != nil {
		fmt.Fprintf(os.Stderr, "ERROR loading AWS config: %v\n", err)
		os.Exit(1)
	}

	client := cloudwatch.NewFromConfig(cfg)

	fmt.Fprintln(os.Stderr, "Fetching metrics from CloudWatch…")
	data, idToKey, apiIDToKey, err := fetchAll(ctx, client, period, startTime, endTime)
	if err != nil {
		fmt.Fprintf(os.Stderr, "ERROR: %v\n", err)
		os.Exit(1)
	}

	table := buildResultTable(data, idToKey)
	apiTable := buildAPIResultTable(data, apiIDToKey)

	switch *outputFlag {
	case "csv":
		fmt.Print(renderCSV(table))
	default:
		fmt.Print(renderMarkdown(table, apiTable, data))
	}
}
