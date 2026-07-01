package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"pkg/util"
	"runtime"
	"strconv"
	"strings"

	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/cloudwatch"
)

const dashboardName = "go-sdks-benchmarks"

type Dashboard struct {
	Variables []DashboardVariable `json:"variables,omitempty"`
	Widgets   []Widget            `json:"widgets"`
}

type DashboardVariable struct {
	Type         string          `json:"type"`
	Pattern      string          `json:"pattern,omitempty"`
	Property     string          `json:"property,omitempty"`
	InputType    string          `json:"inputType"`
	ID           string          `json:"id"`
	Label        string          `json:"label"`
	DefaultValue string          `json:"defaultValue"`
	Visible      bool            `json:"visible"`
	Values       []VariableValue `json:"values"`
}

type VariableValue struct {
	Value string `json:"value"`
	Label string `json:"label"`
}

type Widget struct {
	Type       string `json:"type"`
	X          int    `json:"x"`
	Y          int    `json:"y"`
	Width      int    `json:"width"`
	Height     int    `json:"height"`
	Properties any    `json:"properties"`
}

type MetricWidgetProperties struct {
	LiveData  bool   `json:"liveData,omitempty"`
	Region    string `json:"region"`
	Title     string `json:"title"`
	View      string `json:"view"`
	Stacked   bool   `json:"stacked"`
	Period    int    `json:"period"`
	Stat      string `json:"stat"`
	Metrics   []any  `json:"metrics"`
	Sparkline bool   `json:"sparkline,omitempty"`
	YAxis     *YAxis `json:"yAxis,omitempty"`
}

type TextWidgetProperties struct {
	Markdown string `json:"markdown"`
}

type YAxis struct {
	Left Axis `json:"left"`
}

type Axis struct {
	Min float64 `json:"min"`
	Max float64 `json:"max"`
}

var sdkVersions = map[string]string{
	"Benchmark/GoSDKv1":        "aws-sdk-go",
	"Benchmark/GoSDKv2":        "aws-sdk-go-v2",
	"Benchmark/GoSDKv2CodeGen": "aws-sdk-go-v2",
	"Benchmark/GoSDKv2Ptr":     "aws-sdk-go-v2",
}

var namespaceLabels = map[string]string{
	"Benchmark/GoSDKv1":        "v1",
	"Benchmark/GoSDKv2":        "v2",
	"Benchmark/GoSDKv2CodeGen": "v2-codegen",
	"Benchmark/GoSDKv2Ptr":     "v2-ptr",
}

var tableVersionByNamespace = map[string]string{
	"Benchmark/GoSDKv1":        "v1",
	"Benchmark/GoSDKv2":        "v2",
	"Benchmark/GoSDKv2CodeGen": "v2_codegen",
	"Benchmark/GoSDKv2Ptr":     "v2_ptr",
}

var writeOperations = []string{
	"PutItem",
	"UpdateItem",
	"BatchWriteItem",
	"TransactWriteItems",
}

var readOperations = []string{
	"GetItem",
	"Query",
	"Scan",
	"BatchGetItem",
	"TransactGetItems",
}

var metricsOS = resolveMetricsOS()

func main() {
	region := resolveRegion()
	variables := dashboardVariables()
	selectedStat := variableDefault(variables, "stat", "Average")
	selectedPeriod := variableDefaultInt(variables, "period", 300)

	namespaces := []string{
		"Benchmark/GoSDKv1",
		"Benchmark/GoSDKv2",
		"Benchmark/GoSDKv2CodeGen",
		"Benchmark/GoSDKv2Ptr",
	}

	dashboard := Dashboard{
		Variables: variables,
		Widgets:   []Widget{},
	}

	arm64Widgets, headerHeight := newArchHeaderWidgets(0, "arm64", namespaces, selectedPeriod, selectedStat, region)
	amd64Widgets, _ := newArchHeaderWidgets(12, "amd64", namespaces, selectedPeriod, selectedStat, region)
	dashboard.Widgets = append(dashboard.Widgets, arm64Widgets...)
	dashboard.Widgets = append(dashboard.Widgets, amd64Widgets...)

	y := headerHeight
	for _, api := range writeOperations {
		md := fmt.Sprintf(
			"### MarshalMap — %s\n"+
				"_How long application-level `attributevalue.MarshalMap` (struct -> AttributeValue map) takes compared to total `%s` call duration. "+
				"This is **not** SDK internal API-call serialization (for example `client.call.serialization_duration`). "+
				"Each SDK widget shows: API call duration (ms for v1, µs for v2), MarshalMap duration (µs), and AttributeValue mapping overhead %%._",
			api, api,
		)
		titleWidget, titleH := newSectionTitleWidget(0, y, md)
		dashboard.Widgets = append(dashboard.Widgets, titleWidget)
		y += titleH

		overheadWidgets, overheadH := newOperationOverheadWidgets(0, y, "arm64", "MarshalMap", api, namespaces, selectedPeriod, selectedStat, region)
		dashboard.Widgets = append(dashboard.Widgets, overheadWidgets...)
		amd64OverheadWidgets, _ := newOperationOverheadWidgets(12, y, "amd64", "MarshalMap", api, namespaces, selectedPeriod, selectedStat, region)
		dashboard.Widgets = append(dashboard.Widgets, amd64OverheadWidgets...)
		y += overheadH
	}
	for _, api := range readOperations {
		md := fmt.Sprintf(
			"### UnmarshalMap — %s\n"+
				"_How long application-level `attributevalue.UnmarshalMap` (AttributeValue map -> struct) takes compared to total `%s` call duration. "+
				"This is **not** SDK internal API-call deserialization (for example `client.call.deserialization_duration`). "+
				"Each SDK widget shows: API call duration (ms for v1, µs for v2), UnmarshalMap duration (µs), and AttributeValue mapping overhead %%._",
			api, api,
		)
		titleWidget, titleH := newSectionTitleWidget(0, y, md)
		dashboard.Widgets = append(dashboard.Widgets, titleWidget)
		y += titleH

		overheadWidgets, overheadH := newOperationOverheadWidgets(0, y, "arm64", "UnmarshalMap", api, namespaces, selectedPeriod, selectedStat, region)
		dashboard.Widgets = append(dashboard.Widgets, overheadWidgets...)
		amd64OverheadWidgets, _ := newOperationOverheadWidgets(12, y, "amd64", "UnmarshalMap", api, namespaces, selectedPeriod, selectedStat, region)
		dashboard.Widgets = append(dashboard.Widgets, amd64OverheadWidgets...)
		y += overheadH
	}

	b, err := json.MarshalIndent(dashboard, "", "  ")
	if err != nil {
		panic(err)
	}

	if err := putDashboard(context.Background(), dashboardName, string(b), region); err != nil {
		panic(err)
	}

	fmt.Println("Dashboard updated successfully")
}

func dashboardVariables() []DashboardVariable {
	return []DashboardVariable{
		{
			Type:         "property",
			Property:     "stat",
			InputType:    "select",
			ID:           "stat",
			Label:        "Statistic",
			DefaultValue: "Average",
			Visible:      true,
			Values: []VariableValue{
				{Value: "Average", Label: "Average"},
				{Value: "p50", Label: "p50"},
				{Value: "p90", Label: "p90"},
				{Value: "p95", Label: "p95"},
				{Value: "p99", Label: "p99"},
				{Value: "Minimum", Label: "Minimum"},
				{Value: "Maximum", Label: "Maximum"},
			},
		},
		{
			Type:         "property",
			Property:     "period",
			InputType:    "select",
			ID:           "period",
			Label:        "Period",
			DefaultValue: "300",
			Visible:      true,
			Values: []VariableValue{
				{Value: "60", Label: "1 minute"},
				{Value: "300", Label: "5 minutes"},
				{Value: "900", Label: "15 minutes"},
			},
		},
		{
			Type:         "pattern",
			Pattern:      "\\$\\{size\\}",
			InputType:    "select",
			ID:           "size",
			Label:        "Size",
			DefaultValue: "1KB",
			Visible:      true,
			Values: []VariableValue{
				{Value: "1KB", Label: "1KB"},
				{Value: "10KB", Label: "10KB"},
				{Value: "100KB", Label: "100KB"},
				{Value: "300KB", Label: "300KB"},
			},
		},
	}
}

func variableDefault(vars []DashboardVariable, id, fallback string) string {
	for _, v := range vars {
		if v.ID == id && v.DefaultValue != "" {
			return v.DefaultValue
		}
	}

	return fallback
}

func variableDefaultInt(vars []DashboardVariable, id string, fallback int) int {
	v := variableDefault(vars, id, "")
	if v == "" {
		return fallback
	}

	out, err := strconv.Atoi(v)
	if err != nil {
		return fallback
	}

	return out
}

func putDashboard(ctx context.Context, name, body, region string) error {
	cfg, err := config.LoadDefaultConfig(ctx, config.WithRegion(region))
	if err != nil {
		return fmt.Errorf("load aws config: %w", err)
	}

	client := cloudwatch.NewFromConfig(cfg)
	_, err = client.PutDashboard(ctx, &cloudwatch.PutDashboardInput{
		DashboardName: &name,
		DashboardBody: &body,
	})
	if err != nil {
		return fmt.Errorf("put-dashboard failed: %w", err)
	}

	return nil
}

func resolveRegion() string {
	region := os.Getenv("AWS_REGION")
	if region == "" {
		region = os.Getenv("AWS_DEFAULT_REGION")
	}
	if region == "" {
		region = "eu-west-1"
	}

	return region
}

func resolveMetricsOS() string {
	if v := os.Getenv("METRICS_OS"); v != "" {
		return v
	}

	return runtime.GOOS
}

// newArchHeaderWidgets builds the per-arch header section starting at y=0 and
// returns the widgets plus the total height consumed so the caller knows where
// the next row should start.
func newArchHeaderWidgets(x int, arch string, namespaces []string, period int, stat string, region string) ([]Widget, int) {
	var widgets []Widget
	y := 0

	titleWidget, titleH := newTitleWidget(x, y, arch)
	widgets = append(widgets, titleWidget)
	y += titleH

	// Add spacing before RPS widget (1 row)
	y += 3

	chartWidget, chartH := newArchChartWidget(x, y, arch, namespaces, period, stat, region)
	widgets = append(widgets, chartWidget)
	y += chartH

	rpsWidget, rpsH := newRPSWidget(x, y, arch, namespaces, period, region)
	widgets = append(widgets, rpsWidget)
	y += rpsH

	cpuWidget, cpuH := newCPUUsageWidget(x, y, arch, namespaces, period, region)
	widgets = append(widgets, cpuWidget)
	y += cpuH

	appPerfWidget, appPerfH := newAppPerformanceWidget(x, y, arch, namespaces, period, region)
	widgets = append(widgets, appPerfWidget)
	y += appPerfH

	marshalWidget, marshalH := newStatsNumberWidget(x, y, arch, "MarshalMap", namespaces, period, stat, region)
	widgets = append(widgets, marshalWidget)
	y += marshalH

	unmarshalWidget, unmarshalH := newStatsNumberWidget(x, y, arch, "UnmarshalMap", namespaces, period, stat, region)
	widgets = append(widgets, unmarshalWidget)
	y += unmarshalH

	// Diagnostics widget is intentionally placed far below normal content.
	diagWidget, _ := newDiagnosticsWidget(x, 390, arch, namespaces, period, region)
	widgets = append(widgets, diagWidget)

	return widgets, y
}

func newDiagnosticsWidget(x, y int, arch string, namespaces []string, period int, region string) (Widget, int) {
	const h = 20
	metrics := []any{}
	allOps := append(append([]string{}, writeOperations...), readOperations...)
	tableFormat := fmt.Sprintf("test_table_benchmarks_%%s_%s_${size}", arch)
	versions := []string{"v1", "v2", "v2_codegen", "v2_ptr"}

	for _, op := range allOps {
		for _, version := range versions {
			metrics = append(metrics, []any{
				"AWS/DynamoDB",
				"SuccessfulRequestLatency",
				"TableName",
				fmt.Sprintf(tableFormat, version),
				"Operation",
				op,
				map[string]any{"label": fmt.Sprintf("%s %s", version, op)},
			})
		}
	}

	return Widget{
		Type:   "metric",
		X:      x,
		Y:      y,
		Width:  12,
		Height: h,
		Properties: MetricWidgetProperties{
			LiveData: true,
			Region:   region,
			Title:    fmt.Sprintf("API Throughput (RPS) (%s, size=${size})", arch),
			View:     "table",
			Stacked:  false,
			Period:   period,
			Stat:     "SampleCount",
			Metrics:  metrics,
		},
	}, h
}

func newRPSWidget(x, y int, arch string, namespaces []string, period int, region string) (Widget, int) {
	const h = 30
	metrics := []any{}
	allOps := append(append([]string{}, writeOperations...), readOperations...)

	idCounter := 1
	for _, namespace := range namespaces {
		sdk := sdkVersions[namespace]
		sdkLabel := namespaceLabel(namespace)
		apiRows := apiMetricRowsWithVar(namespace, "${size}", arch, sdk)

		for _, op := range allOps {
			apiMetric, ok := apiRows[op]
			if !ok {
				continue
			}

			hiddenID := fmt.Sprintf("m%d", idCounter)
			idCounter++
			metrics = append(metrics, append(apiMetric, map[string]any{
				"id":      hiddenID,
				"stat":    "SampleCount",
				"visible": false,
			}))

			// Throughput per second computed from SDK-emitted API SampleCount.
			exprID := fmt.Sprintf("r%d", idCounter)
			idCounter++
			metrics = append(metrics, []any{map[string]any{
				"id":         exprID,
				"expression": fmt.Sprintf("%[1]s/PERIOD(%[1]s)", hiddenID),
				"label":      fmt.Sprintf("%s %s req/s", sdkLabel, op),
			}})
		}
	}

	return Widget{
		Type:   "metric",
		X:      x,
		Y:      y,
		Width:  12,
		Height: h,
		Properties: MetricWidgetProperties{
			LiveData: true,
			Region:   region,
			Title:    fmt.Sprintf("API Throughput (req/s) (%s, size=${size})", arch),
			View:     "table",
			Stacked:  false,
			Period:   period,
			Stat:     "Average",
			Metrics:  metrics,
		},
	}, h
}

func newCPUUsageWidget(x, y int, arch string, namespaces []string, period int, region string) (Widget, int) {
	const h = 4
	metrics := make([]any, 0, len(namespaces)*6)

	for i, namespace := range namespaces {
		sdk := sdkVersions[namespace]
		sdkLabel := namespaceLabel(namespace)
		usageID := fmt.Sprintf("cpuu%d", i)
		idleID := fmt.Sprintf("cpui%d", i)
		userID := fmt.Sprintf("cpus%d", i)
		systemID := fmt.Sprintf("cpusy%d", i)
		iowaitID := fmt.Sprintf("cpuw%d", i)

		metrics = append(metrics,
			[]any{namespace, "Client.CPU.Usage", "OS", metricsOS, "Size", "${size}", "Arch", arch, "SDK", sdk, map[string]any{
				"id":      usageID,
				"stat":    "Average",
				"visible": false,
			}},
			[]any{namespace, "Client.CPU.Idle", "OS", metricsOS, "Size", "${size}", "Arch", arch, "SDK", sdk, map[string]any{
				"id":      idleID,
				"stat":    "Average",
				"visible": false,
			}},
			[]any{namespace, "Client.CPU.User", "OS", metricsOS, "Size", "${size}", "Arch", arch, "SDK", sdk, map[string]any{
				"id":      userID,
				"stat":    "Average",
				"visible": false,
			}},
			[]any{namespace, "Client.CPU.System", "OS", metricsOS, "Size", "${size}", "Arch", arch, "SDK", sdk, map[string]any{
				"id":      systemID,
				"stat":    "Average",
				"visible": false,
			}},
			[]any{namespace, "Client.CPU.IOWait", "OS", metricsOS, "Size", "${size}", "Arch", arch, "SDK", sdk, map[string]any{
				"id":      iowaitID,
				"stat":    "Average",
				"visible": false,
			}},
		)
		metrics = append(metrics, []any{map[string]any{
			"id":         "cpup" + strconv.Itoa(i),
			"expression": usageID,
			"label":      fmt.Sprintf("%s CPU Usage %%", sdkLabel),
		}})
		metrics = append(metrics, []any{map[string]any{
			"id":         "cpuidlep" + strconv.Itoa(i),
			"expression": idleID,
			"label":      fmt.Sprintf("%s CPU Idle %%", sdkLabel),
		}})
		metrics = append(metrics, []any{map[string]any{
			"id":         "cpuuserp" + strconv.Itoa(i),
			"expression": userID,
			"label":      fmt.Sprintf("%s CPU User %%", sdkLabel),
		}})
		metrics = append(metrics, []any{map[string]any{
			"id":         "cpusystemp" + strconv.Itoa(i),
			"expression": systemID,
			"label":      fmt.Sprintf("%s CPU System %%", sdkLabel),
		}})
		metrics = append(metrics, []any{map[string]any{
			"id":         "cpuiowaitp" + strconv.Itoa(i),
			"expression": iowaitID,
			"label":      fmt.Sprintf("%s CPU IOWait %%", sdkLabel),
		}})
	}

	return Widget{
		Type:   "metric",
		X:      x,
		Y:      y,
		Width:  12,
		Height: h,
		Properties: MetricWidgetProperties{
			LiveData: true,
			Region:   region,
			Title:    fmt.Sprintf("Host CPU Breakdown (%%) (%s, size=${size})", arch),
			View:     "singleValue",
			Stacked:  false,
			Period:   period,
			Stat:     "Average",
			Metrics:  metrics,
		},
	}, h
}

func newAppPerformanceWidget(x, y int, arch string, namespaces []string, period int, region string) (Widget, int) {
	const h = 12
	metrics := []any{}
	allOps := append(append([]string{}, writeOperations...), readOperations...)

	// Per-SDK hidden metric IDs, captured so we can emit visible expressions
	// in metric-major order (all GC Overhead rows, then all GC Pause rows, etc.)
	// matching the row ordering of the RPS widget.
	type sdkRefs struct {
		ns       string
		label    string
		gcID     string
		paID     string
		allocsID string
		freesID  string
		apiTotal string
	}
	refs := make([]sdkRefs, 0, len(namespaces))

	for i, ns := range namespaces {
		sdk := sdkVersions[ns]
		sdkLabel := namespaceLabel(ns)
		sfx := strconv.Itoa(i)

		gcID := "gc" + sfx
		paID := "pa" + sfx
		// runtime/metrics counters: /gc/heap/allocs:objects and /gc/heap/frees:objects
		// These are cumulative heap allocation/free counts (in objects) — the same
		// figure Go test benchmarks aggregate to report `allocs/op`.
		allocsID := "alc" + sfx
		freesID := "fre" + sfx

		// Hidden raw runtime metrics (per SDK).
		metrics = append(metrics,
			[]any{ns, "Memory.GCCPUFraction", "OS", metricsOS, "Size", "${size}", "Arch", arch, "SDK", sdk,
				map[string]any{"id": gcID, "stat": "Average", "visible": false}},
			[]any{ns, "Memory.PauseUs", "OS", metricsOS, "Size", "${size}", "Arch", arch, "SDK", sdk,
				map[string]any{"id": paID, "stat": "Average", "visible": false}},
			[]any{ns, "gc.heap.allocs.objects", "OS", metricsOS, "Size", "${size}", "Arch", arch, "SDK", sdk,
				map[string]any{"id": allocsID, "stat": "Maximum", "visible": false}},
			[]any{ns, "gc.heap.frees.objects", "OS", metricsOS, "Size", "${size}", "Arch", arch, "SDK", sdk,
				map[string]any{"id": freesID, "stat": "Maximum", "visible": false}},
		)

		// Hidden API SampleCount metrics — used to normalize allocs/frees by total API
		// operations performed in the window, giving a Go-benchmark-style allocs/op figure.
		// v1 emits per-operation metrics (metric name == operation); v2 uses
		// client.call.duration with the rpc.method dimension.
		var apiSumTerms []string
		for j, op := range allOps {
			apiID := fmt.Sprintf("api%s_%d", sfx, j)
			var row []any
			if ns == "Benchmark/GoSDKv1" {
				row = []any{ns, op, "OS", metricsOS, "Size", "${size}", "Arch", arch, "SDK", sdk,
					map[string]any{"id": apiID, "stat": "SampleCount", "visible": false}}
			} else {
				row = []any{ns, "client.call.duration", "OS", metricsOS, "Size", "${size}", "Arch", arch, "SDK", sdk, "rpc.method", op,
					map[string]any{"id": apiID, "stat": "SampleCount", "visible": false}}
			}
			metrics = append(metrics, row)
			apiSumTerms = append(apiSumTerms, fmt.Sprintf("SUM(%s)", apiID))
		}

		refs = append(refs, sdkRefs{
			ns:       ns,
			label:    sdkLabel,
			gcID:     gcID,
			paID:     paID,
			allocsID: allocsID,
			freesID:  freesID,
			apiTotal: strings.Join(apiSumTerms, "+"),
		})
	}

	// Visible derived expressions, grouped by metric (then by SDK), so the table
	// reads: all GC Overhead rows, all GC Pause rows, all Allocs/op rows, all Frees/op rows.
	for i, r := range refs {
		metrics = append(metrics, []any{map[string]any{
			"id":         "gcpct" + strconv.Itoa(i),
			"expression": fmt.Sprintf("%s*100", r.gcID),
			"label":      fmt.Sprintf("GC Overhead %% %s", r.label),
		}})
	}
	for i, r := range refs {
		metrics = append(metrics, []any{map[string]any{
			"id":         "pauseus" + strconv.Itoa(i),
			"expression": fmt.Sprintf("%s*1", r.paID),
			"label":      fmt.Sprintf("GC Pause Duration (µs) %s", r.label),
		}})
	}
	for i, r := range refs {
		metrics = append(metrics, []any{map[string]any{
			"id":         "alcop" + strconv.Itoa(i),
			"expression": fmt.Sprintf("TIME_SERIES((MAX(%s)-MIN(%s))/(%s))", r.allocsID, r.allocsID, r.apiTotal),
			"label":      fmt.Sprintf("Allocs/op %s", r.label),
		}})
	}
	for i, r := range refs {
		metrics = append(metrics, []any{map[string]any{
			"id":         "freop" + strconv.Itoa(i),
			"expression": fmt.Sprintf("TIME_SERIES((MAX(%s)-MIN(%s))/(%s))", r.freesID, r.freesID, r.apiTotal),
			"label":      fmt.Sprintf("Frees/op %s", r.label),
		}})
	}

	return Widget{
		Type:   "metric",
		X:      x,
		Y:      y,
		Width:  12,
		Height: h,
		Properties: MetricWidgetProperties{
			LiveData: true,
			Region:   region,
			Title:    fmt.Sprintf("App Runtime Performance (%s, size=${size})", arch),
			View:     "table",
			Stacked:  false,
			Period:   period,
			Stat:     "Average",
			Metrics:  metrics,
		},
	}, h
}

func runtimeMetricRow(namespace, metricName, arch, sdk string) []any {
	return []any{namespace, metricName, "SDK", sdk, "Size", "${size}", "OS", metricsOS, "Arch", arch}
}

func newTitleWidget(x, y int, arch string) (Widget, int) {
	const h = 2
	return Widget{
		Type:   "text",
		X:      x,
		Y:      y,
		Width:  12,
		Height: h,
		Properties: TextWidgetProperties{
			Markdown: fmt.Sprintf("## %s", arch),
		},
	}, h
}

func newSectionTitleWidget(x, y int, markdown string) (Widget, int) {
	const h = 3
	return Widget{
		Type:   "text",
		X:      x,
		Y:      y,
		Width:  24,
		Height: h,
		Properties: TextWidgetProperties{
			Markdown: markdown,
		},
	}, h
}

func newArchChartWidget(x, y int, arch string, namespaces []string, period int, stat string, region string) (Widget, int) {
	const h = 8
	metrics := make([]any, 0, len(namespaces)*2)

	for _, namespace := range namespaces {
		sdk := sdkVersions[namespace]
		metrics = append(metrics, []any{namespace, "MarshalMap", "OS", metricsOS, "Size", "${size}", "Arch", arch, "SDK", sdk})
		metrics = append(metrics, []any{".", "UnmarshalMap", ".", ".", ".", ".", ".", ".", ".", "."})
	}

	return Widget{
		Type:   "metric",
		X:      x,
		Y:      y,
		Width:  12,
		Height: h,
		Properties: MetricWidgetProperties{
			LiveData: true,
			Region:   region,
			Title:    fmt.Sprintf("AttributeValue (%s, size=${size})", arch),
			View:     "timeSeries",
			Stacked:  false,
			Period:   period,
			Stat:     stat,
			Metrics:  metrics,
		},
	}, h
}

func newStatsNumberWidget(x, y int, arch, operation string, namespaces []string, period int, stat string, region string) (Widget, int) {
	const h = 3
	metrics := make([]any, 0, len(namespaces))

	for _, namespace := range namespaces {
		sdk := sdkVersions[namespace]
		base := []any{namespace, operation, "OS", metricsOS, "Size", "${size}", "Arch", arch, "SDK", sdk}
		metrics = append(metrics, append(append([]any{}, base...), map[string]any{
			"label": namespaceLabel(namespace),
		}))
	}

	return Widget{
		Type:   "metric",
		X:      x,
		Y:      y,
		Width:  12,
		Height: h,
		Properties: MetricWidgetProperties{
			LiveData:  true,
			Region:    region,
			Title:     fmt.Sprintf("%s stats (%s, size=${size})", operation, arch),
			View:      "singleValue",
			Stacked:   false,
			Period:    period,
			Stat:      stat,
			Metrics:   metrics,
			Sparkline: true,
		},
	}, h
}

// newOperationOverheadWidgets returns one widget per SDK arranged in a 2×2 grid.
// Returns the widgets and the total height of the grid (2 rows × rowH).
func newOperationOverheadWidgets(x, y int, arch, serializationMetric, api string, namespaces []string, period int, stat string, region string) ([]Widget, int) {
	const colW = 6
	const rowH = 4

	widgets := []Widget{}

	for i, namespace := range namespaces {
		sdk := sdkVersions[namespace]
		sdkLabel := namespaceLabel(namespace)
		apiDurationScale := apiDurationScaleForNamespace(namespace)
		apiUnitLabel := apiDurationUnitForNamespace(namespace)
		apiRows := apiMetricRowsWithVar(namespace, "${size}", arch, sdk)

		apiMetric, ok := apiRows[api]
		if !ok {
			continue
		}

		col := i % 2
		row := i / 2
		wx := x + col*colW
		wy := y + row*rowH

		serID := "s1"
		apiID := "a1"
		exprID := "e1"

		metrics := []any{
			append(apiMetric, map[string]any{
				"id":    apiID,
				"label": fmt.Sprintf("%s duration (%s)", api, apiUnitLabel),
			}),
			[]any{namespace, serializationMetric, "OS", metricsOS, "Size", "${size}", "Arch", arch, "SDK", sdk, map[string]any{
				"id":    serID,
				"label": fmt.Sprintf("%s duration (µs)", serializationMetric),
			}},
			[]any{map[string]any{
				"expression": fmt.Sprintf("IF(%s>0, 100*(%s/(%s*%d)), 0)", apiID, serID, apiID, apiDurationScale),
				"label":      fmt.Sprintf("AttributeValue %s overhead %%", serializationMetric),
				"id":         exprID,
			}},
		}

		widgets = append(widgets, Widget{
			Type:   "metric",
			X:      wx,
			Y:      wy,
			Width:  colW,
			Height: rowH,
			Properties: MetricWidgetProperties{
				LiveData:  true,
				Region:    region,
				Title:     fmt.Sprintf("%s — AttributeValue %s for %s (%s, size=${size})", sdkLabel, serializationMetric, api, arch),
				View:      "singleValue",
				Stacked:   false,
				Period:    period,
				Stat:      stat,
				Metrics:   metrics,
				Sparkline: false,
			},
		})
	}

	// 4 namespaces → 2 rows
	totalRows := (len(namespaces) + 1) / 2
	return widgets, totalRows * rowH
}

func namespaceLabel(namespace string) string {
	if label, ok := namespaceLabels[namespace]; ok {
		return label
	}

	return namespace
}

func apiDurationScaleForNamespace(namespace string) int {
	if namespace == "Benchmark/GoSDKv1" {
		return 1000
	}

	return 1
}

func apiDurationUnitForNamespace(namespace string) string {
	if namespace == "Benchmark/GoSDKv1" {
		return "ms"
	}

	return "µs"
}

func apiMetricRowsWithVar(namespace, sizeVar, arch, sdk string) map[string][]any {
	rows := map[string][]any{}

	var metrics []util.Metric
	if namespace == "Benchmark/GoSDKv1" {
		// Use a dummy size for reflection; actual dimension will use variable
		metrics = util.AllMetricsV1(namespace, sdk, "1KB", metricsOS, arch, "m", nil)
	} else {
		metrics = util.AllMetricsV2(namespace, sdk, "1KB", metricsOS, arch, "m", nil)
	}

	for _, m := range metrics {
		row := metricRowWithVar(m, sizeVar)
		op := m.Name
		if v, ok := m.Dimensions[util.DimensionRpcMethod]; ok {
			op = v
		}
		rows[op] = row
	}

	return rows
}

func metricRowWithVar(m util.Metric, sizeVar string) []any {
	row := []any{m.Namespace, m.Name}

	// Keep a stable and valid dimension order for CloudWatch metric arrays.
	if v, ok := m.Dimensions[util.DimensionNameSDK]; ok {
		row = append(row, util.DimensionNameSDK, v)
	}
	if _, ok := m.Dimensions[util.DimensionNameSize]; ok {
		// Use variable instead of hardcoded size
		row = append(row, util.DimensionNameSize, sizeVar)
	}
	if v, ok := m.Dimensions[util.DimensionNameOS]; ok {
		row = append(row, util.DimensionNameOS, v)
	}
	if v, ok := m.Dimensions[util.DimensionNameArch]; ok {
		row = append(row, util.DimensionNameArch, v)
	}
	if v, ok := m.Dimensions[util.DimensionRpcMethod]; ok {
		row = append(row, util.DimensionRpcMethod, v)
	}

	return row
}
