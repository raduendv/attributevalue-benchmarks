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
	r := strings.NewReplacer("/", "_", " ", "_", ".", "_", "%", "pct", "(", "", ")", "", ",", "_")
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
		"/", "_", " ", "_", ".", "_",
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
// the Average API call duration. v1 emits per-operation metrics in ms; v2 uses
// client.call.duration with an rpc.method dimension in µs.
func buildAPIQueries(period int32) ([]cwtypes.MetricDataQuery, map[string]apiMetricKey) {
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
							Stat:   aws.String("Average"),
						},
						ReturnData: aws.Bool(true),
					})
				}
			}
		}
	}

	return queries, idToKey
}
func fetchStat(
	ctx context.Context,
	client *cloudwatch.Client,
	queries []cwtypes.MetricDataQuery,
	startTime, endTime time.Time,
) (map[string]float64, error) {
	results := make(map[string]float64)

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
				if len(r.Values) == 0 {
					results[id] = math.NaN()
					continue
				}
				sum := 0.0
				for _, v := range r.Values {
					sum += v
				}
				results[id] = sum / float64(len(r.Values))
			}

			if resp.NextToken == nil {
				break
			}
			nextToken = resp.NextToken
		}
	}

	return results, nil
}

type allData struct {
	avg, p50, p90, p95, p99, min, max map[string]float64
	apiAvg                            map[string]float64
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

	var idToKey map[string]metricKey

	for _, d := range defs {
		queries, ik := buildQueries(d.cwStat, period)
		if idToKey == nil {
			idToKey = ik
		}

		vals, err := fetchStat(ctx, client, queries, startTime, endTime)
		if err != nil {
			return nil, nil, nil, fmt.Errorf("fetching %s: %w", d.cwStat, err)
		}
		*d.dest = vals
		fmt.Fprintf(os.Stderr, "  ✓ fetched %s (%d series)\n", d.cwStat, len(vals))
	}

	apiQueries, apiIDToKey := buildAPIQueries(period)
	apiVals, err := fetchStat(ctx, client, apiQueries, startTime, endTime)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("fetching API durations: %w", err)
	}
	data.apiAvg = apiVals
	fmt.Fprintf(os.Stderr, "  ✓ fetched API durations (%d series)\n", len(apiVals))

	return data, idToKey, apiIDToKey, nil
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

func renderMarkdown(table map[metricKey]metricStats, apiTable map[apiMetricKey]float64) string {
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
				sb.WriteString("|-----|--------:|----:|----:|----:|----:|----:|----:|\n")

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
				sb.WriteString("|-----|----------|--------:|------:|----:|------:|\n")

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
				sep := "|-----|----------:|"
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

	return sb.String()
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
		fmt.Print(renderMarkdown(table, apiTable))
	}
}
