package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"pkg/util"
	"strconv"

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

func main() {
	region := resolveRegion()
	variables := dashboardVariables()
	selectedStat := variableDefault(variables, "stat", "Average")
	selectedPeriod := variableDefaultInt(variables, "period", 300)
	selectedSize := variableDefault(variables, "size", "1KB")

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

	dashboard.Widgets = append(dashboard.Widgets, newArchHeaderWidgets(0, "arm64", namespaces, selectedSize, selectedPeriod, selectedStat, region)...)
	dashboard.Widgets = append(dashboard.Widgets, newArchHeaderWidgets(12, "amd64", namespaces, selectedSize, selectedPeriod, selectedStat, region)...)

	// Operation overhead sections span both arch columns so we only render one title per operation.
	y := 16
	for _, api := range writeOperations {
		md := fmt.Sprintf(
			"### MarshalMap — %s\n"+
				"_How long application-level `attributevalue.MarshalMap` (struct -> AttributeValue map) takes compared to total `%s` call duration. "+
				"This is **not** SDK internal API-call serialization (for example `client.call.serialization_duration`). "+
				"Each SDK widget shows: API call duration (ms for v1, µs for v2), MarshalMap duration (µs), and AttributeValue mapping overhead %%._",
			api, api,
		)
		dashboard.Widgets = append(dashboard.Widgets, newSectionTitleWidget(0, y, md))
		y += 3
		dashboard.Widgets = append(dashboard.Widgets, newOperationOverheadWidgets(0, y, "arm64", "MarshalMap", api, namespaces, selectedSize, selectedPeriod, selectedStat, region)...)
		dashboard.Widgets = append(dashboard.Widgets, newOperationOverheadWidgets(12, y, "amd64", "MarshalMap", api, namespaces, selectedSize, selectedPeriod, selectedStat, region)...)
		y += 8
	}
	for _, api := range readOperations {
		md := fmt.Sprintf(
			"### UnmarshalMap — %s\n"+
				"_How long application-level `attributevalue.UnmarshalMap` (AttributeValue map -> struct) takes compared to total `%s` call duration. "+
				"This is **not** SDK internal API-call deserialization (for example `client.call.deserialization_duration`). "+
				"Each SDK widget shows: API call duration (ms for v1, µs for v2), UnmarshalMap duration (µs), and AttributeValue mapping overhead %%._",
			api, api,
		)
		dashboard.Widgets = append(dashboard.Widgets, newSectionTitleWidget(0, y, md))
		y += 3
		dashboard.Widgets = append(dashboard.Widgets, newOperationOverheadWidgets(0, y, "arm64", "UnmarshalMap", api, namespaces, selectedSize, selectedPeriod, selectedStat, region)...)
		dashboard.Widgets = append(dashboard.Widgets, newOperationOverheadWidgets(12, y, "amd64", "UnmarshalMap", api, namespaces, selectedSize, selectedPeriod, selectedStat, region)...)
		y += 10
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
			Type:         "property",
			Property:     "Size",
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

func newArchHeaderWidgets(x int, arch string, namespaces []string, size string, period int, stat string, region string) []Widget {
	widgets := []Widget{
		newTitleWidget(x, 0, arch),
		newArchChartWidget(x, 2, arch, namespaces, size, period, stat, region),
		newStatsNumberWidget(x, 10, arch, "MarshalMap", namespaces, size, period, stat, region),
		newStatsNumberWidget(x, 13, arch, "UnmarshalMap", namespaces, size, period, stat, region),
	}

	return widgets
}

func newTitleWidget(x, y int, arch string) Widget {
	return Widget{
		Type:   "text",
		X:      x,
		Y:      y,
		Width:  12,
		Height: 2,
		Properties: TextWidgetProperties{
			Markdown: fmt.Sprintf("## %s", arch),
		},
	}
}

func newSectionTitleWidget(x, y int, markdown string) Widget {
	return Widget{
		Type:   "text",
		X:      x,
		Y:      y,
		Width:  24,
		Height: 3,
		Properties: TextWidgetProperties{
			Markdown: markdown,
		},
	}
}

func newArchChartWidget(x, y int, arch string, namespaces []string, size string, period int, stat string, region string) Widget {
	metrics := make([]any, 0, len(namespaces)*2)

	for _, namespace := range namespaces {
		sdk := sdkVersions[namespace]
		metrics = append(metrics, []any{namespace, "MarshalMap", "OS", "linux", "Size", size, "Arch", arch, "SDK", sdk})
		metrics = append(metrics, []any{".", "UnmarshalMap", ".", ".", ".", ".", ".", ".", ".", "."})
	}

	return Widget{
		Type:   "metric",
		X:      x,
		Y:      y,
		Width:  12,
		Height: 8,
		Properties: MetricWidgetProperties{
			LiveData: true,
			Region:   region,
			Title:    fmt.Sprintf("AttributeValue (%s)", arch),
			View:     "timeSeries",
			Stacked:  false,
			Period:   period,
			Stat:     stat,
			Metrics:  metrics,
		},
	}
}

func newStatsNumberWidget(x, y int, arch, operation string, namespaces []string, size string, period int, stat string, region string) Widget {
	metrics := make([]any, 0, len(namespaces))

	for _, namespace := range namespaces {
		sdk := sdkVersions[namespace]
		base := []any{namespace, operation, "OS", "linux", "Size", size, "Arch", arch, "SDK", sdk}
		metrics = append(metrics, append(append([]any{}, base...), map[string]any{
			"label": namespaceLabel(namespace),
		}))
	}

	return Widget{
		Type:   "metric",
		X:      x,
		Y:      y,
		Width:  12,
		Height: 3,
		Properties: MetricWidgetProperties{
			LiveData:  true,
			Region:    region,
			Title:     fmt.Sprintf("%s stats (%s)", operation, arch),
			View:      "singleValue",
			Stacked:   false,
			Period:    period,
			Stat:      stat,
			Metrics:   metrics,
			Sparkline: true,
		},
	}
}

// newOperationOverheadWidgets returns one widget per SDK (2×2 grid, each 6 wide × 5 tall)
// so every widget has only 3 metrics and labels are never truncated.
func newOperationOverheadWidgets(x, y int, arch, serializationMetric, api string, namespaces []string, size string, period int, stat string, region string) []Widget {
	const colW = 6
	const rowH = 4

	widgets := []Widget{}

	for i, namespace := range namespaces {
		sdk := sdkVersions[namespace]
		sdkLabel := namespaceLabel(namespace)
		apiDurationScale := apiDurationScaleForNamespace(namespace)
		apiUnitLabel := apiDurationUnitForNamespace(namespace)
		apiRows := apiMetricRows(namespace, size, arch, sdk)

		apiMetric, ok := apiRows[api]
		if !ok {
			continue
		}

		col := i % 2 // 0 or 1
		row := i / 2 // 0 or 1
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
			[]any{namespace, serializationMetric, "OS", "linux", "Size", size, "Arch", arch, "SDK", sdk, map[string]any{
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
				Title:     fmt.Sprintf("%s — AttributeValue %s for %s (%s)", sdkLabel, serializationMetric, api, arch),
				View:      "singleValue",
				Stacked:   false,
				Period:    period,
				Stat:      stat,
				Metrics:   metrics,
				Sparkline: false,
			},
		})
	}

	return widgets
}

func namespaceLabel(namespace string) string {
	if label, ok := namespaceLabels[namespace]; ok {
		return label
	}

	return namespace
}

func apiDurationScaleForNamespace(namespace string) int {
	if namespace == "Benchmark/GoSDKv1" {
		// v1 API latency is in milliseconds while Marshal/Unmarshal are in microseconds.
		return 1000
	}

	// v2 client.call.duration is emitted in microseconds in this project.
	return 1
}

func apiDurationUnitForNamespace(namespace string) string {
	if namespace == "Benchmark/GoSDKv1" {
		return "ms"
	}

	return "µs"
}

func apiMetricRows(namespace, size, arch, sdk string) map[string][]any {
	rows := map[string][]any{}

	var metrics []util.Metric
	if namespace == "Benchmark/GoSDKv1" {
		metrics = util.AllMetricsV1(namespace, sdk, size, "linux", arch, "m", nil)
	} else {
		metrics = util.AllMetricsV2(namespace, sdk, size, "linux", arch, "m", nil)
	}

	for _, m := range metrics {
		row := metricRowNoExtras(m)
		op := m.Name
		if v, ok := m.Dimensions[util.DimensionRpcMethod]; ok {
			op = v
		}
		rows[op] = row
	}

	return rows
}

func metricRowNoExtras(m util.Metric) []any {
	row := []any{m.Namespace, m.Name}

	// Keep a stable and valid dimension order for CloudWatch metric arrays.
	if v, ok := m.Dimensions[util.DimensionNameSDK]; ok {
		row = append(row, util.DimensionNameSDK, v)
	}
	if v, ok := m.Dimensions[util.DimensionNameSize]; ok {
		row = append(row, util.DimensionNameSize, v)
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
