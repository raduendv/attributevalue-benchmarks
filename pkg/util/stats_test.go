package util

import (
	"testing"

	"github.com/aws/aws-sdk-go/service/cloudwatchlogs"
)

func TestGoToCloudwatchMetricNameAndUnit_NoNameCollisionAcrossUnits(t *testing.T) {
	bytesNameUnit := GoToCloudwatchMetricNameAndUnit("/gc/heap/frees:bytes")
	objectsNameUnit := GoToCloudwatchMetricNameAndUnit("/gc/heap/frees:objects")

	if bytesNameUnit[0] == "" || objectsNameUnit[0] == "" {
		t.Fatalf("expected non-empty metric names, got bytes=%q objects=%q", bytesNameUnit[0], objectsNameUnit[0])
	}
	if bytesNameUnit[0] == objectsNameUnit[0] {
		t.Fatalf("metric name collision detected: both mapped to %q", bytesNameUnit[0])
	}

	if bytesNameUnit[0] != "gc.heap.frees.bytes" {
		t.Fatalf("unexpected bytes metric name: got %q want %q", bytesNameUnit[0], "gc.heap.frees.bytes")
	}
	if objectsNameUnit[0] != "gc.heap.frees.objects" {
		t.Fatalf("unexpected objects metric name: got %q want %q", objectsNameUnit[0], "gc.heap.frees.objects")
	}

	if bytesNameUnit[1] != cloudwatchlogs.StandardUnitBytes {
		t.Fatalf("unexpected bytes metric unit: got %q want %q", bytesNameUnit[1], cloudwatchlogs.StandardUnitBytes)
	}
	if objectsNameUnit[1] != cloudwatchlogs.StandardUnitCount {
		t.Fatalf("unexpected objects metric unit: got %q want %q", objectsNameUnit[1], cloudwatchlogs.StandardUnitCount)
	}
}

func TestGoToCloudwatchMetricNameAndUnit_InvalidInput(t *testing.T) {
	got := GoToCloudwatchMetricNameAndUnit("/gc/heap/frees")
	if got != [2]string{} {
		t.Fatalf("expected empty result for missing unit, got %v", got)
	}
}

