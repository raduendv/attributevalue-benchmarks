package main

import (
	"bufio"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

// Matches: <app>_bench_<timestamp>_<arch>.txt
// Examples: v1_bench_2026_05_04_16_47_06_arm64.txt, v2-codegen_bench_..._amd64.txt
var namePattern = regexp.MustCompile(`^(v[^_]+)_bench_(.+)_([[:alnum:]_]+)\.txt$`)

var benchLinePattern = regexp.MustCompile(`^Benchmark_\S+_(MarshalMap|UnmarshalMap)_User(1KB|10KB|100KB|300KB)-\d+\s+\d+\s+([0-9.]+)\s+ns/op\s+([0-9.]+)\s+B/op\s+([0-9.]+)\s+allocs/op`)

var (
	apps       = []string{"v1", "v2", "v2-codegen", "v2-ptr"}
	operations = []string{"MarshalMap", "UnmarshalMap"}
	sizes      = []string{"1KB", "10KB", "100KB", "300KB"}
)

type benchmarkKey struct {
	App       string
	Arch      string
	Operation string
	Size      string
}

type benchmarkAgg struct {
	Count       int
	NsOpSum     float64
	BytesOpSum  float64
	AllocsOpSum float64
}

func (a *benchmarkAgg) add(nsOp, bytesOp, allocsOp float64) {
	a.Count++
	a.NsOpSum += nsOp
	a.BytesOpSum += bytesOp
	a.AllocsOpSum += allocsOp
}

func (a benchmarkAgg) avgNsOp() float64 {
	if a.Count == 0 {
		return 0
	}
	return a.NsOpSum / float64(a.Count)
}

func (a benchmarkAgg) avgBytesOp() float64 {
	if a.Count == 0 {
		return 0
	}
	return a.BytesOpSum / float64(a.Count)
}

func (a benchmarkAgg) avgAllocsOp() float64 {
	if a.Count == 0 {
		return 0
	}
	return a.AllocsOpSum / float64(a.Count)
}

func main() {
	filesByArch := getFilesByArch()
	agg := aggregateBenchmarks(filesByArch)
	fmt.Print(renderMarkdown(agg))
}

func getFilesByArch() map[string]map[string][]string {
	filesByArch := make(map[string]map[string][]string)

	entries, err := os.ReadDir("../results")
	if err != nil {
		return filesByArch
	}

	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}

		name := entry.Name()
		matches := namePattern.FindStringSubmatch(name)
		if len(matches) < 4 {
			continue
		}

		app := matches[1]
		arch := matches[3]

		if _, ok := filesByArch[arch]; !ok {
			filesByArch[arch] = make(map[string][]string)
		}
		filesByArch[arch][app] = append(filesByArch[arch][app], filepath.Join("../results", name))
	}

	for arch := range filesByArch {
		for app := range filesByArch[arch] {
			sort.Strings(filesByArch[arch][app])
		}
	}

	return filesByArch
}

func aggregateBenchmarks(filesByArch map[string]map[string][]string) map[benchmarkKey]benchmarkAgg {
	out := make(map[benchmarkKey]benchmarkAgg)

	for arch, filesByApp := range filesByArch {
		for app, files := range filesByApp {
			for _, file := range files {
				if err := parseBenchmarkFile(file, func(operation, size string, nsOp, bytesOp, allocsOp float64) {
					k := benchmarkKey{App: app, Arch: arch, Operation: operation, Size: size}
					a := out[k]
					a.add(nsOp, bytesOp, allocsOp)
					out[k] = a
				}); err != nil {
					fmt.Fprintf(os.Stderr, "warning: failed to parse %s: %v\n", file, err)
				}
			}
		}
	}

	return out
}

func parseBenchmarkFile(path string, onRow func(operation, size string, nsOp, bytesOp, allocsOp float64)) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()

	s := bufio.NewScanner(f)
	for s.Scan() {
		line := strings.TrimSpace(s.Text())
		m := benchLinePattern.FindStringSubmatch(line)
		if len(m) == 0 {
			continue
		}

		nsOp, err := strconv.ParseFloat(m[3], 64)
		if err != nil {
			continue
		}
		bytesOp, err := strconv.ParseFloat(m[4], 64)
		if err != nil {
			continue
		}
		allocsOp, err := strconv.ParseFloat(m[5], 64)
		if err != nil {
			continue
		}

		onRow(m[1], m[2], nsOp, bytesOp, allocsOp)
	}

	return s.Err()
}

func renderMarkdown(agg map[benchmarkKey]benchmarkAgg) string {
	var sb strings.Builder
	arches := sortedArches(agg)

	sb.WriteString("### Appendix #7: Benchmarks\n\n")
	sb.WriteString("> Aggregation uses arithmetic mean across files for each `(sdk, arch, operation, size)` tuple.\n")
	sb.WriteString("> `MarshalMap` = struct -> `map[string]types.AttributeValue`; `UnmarshalMap` = reverse mapping.\n\n")

	sb.WriteString("#### Test Entities\n\n")
	sb.WriteString("- `User` benchmark entity at payload targets: `1KB`, `10KB`, `100KB`, `300KB`.\n")
	sb.WriteString("  - Scalar fields: strings, numbers, booleans.\n")
	sb.WriteString("  - Optional/pointer fields, including optional nested objects.\n")
	sb.WriteString("  - Temporal fields: `time.Time` and optional timestamps.\n")
	sb.WriteString("  - Collections: byte slice, string lists, and maps.\n")
	sb.WriteString("  - Nested structs: `Address` list/object and embedded `Timestampable` metadata.\n")
	sb.WriteString("- This mix is intended to stress realistic `dynamodbav` mapping paths (nested structs, optional values, collections, and timestamps).\n")
	sb.WriteString("- Input source: benchmark rows matching `Benchmark_*_(MarshalMap|UnmarshalMap)_User<size>`.\n")
	sb.WriteString("- Current report scope: `User` entity only; primitive-type entity benchmarks are not included in these tables.\n\n")

	for _, op := range operations {
		sb.WriteString(fmt.Sprintf("#### %s\n\n", op))

		for _, arch := range arches {
			for _, size := range sizes {
				sb.WriteString(fmt.Sprintf("**Arch: %s - %s**\n\n", arch, size))
				sb.WriteString("| SDK | ns/op | B/op | allocs/op | files |\n")
				sb.WriteString("|-----|------:|-----:|----------:|------:|\n")

				for _, app := range apps {
					k := benchmarkKey{App: app, Arch: arch, Operation: op, Size: size}
					a, ok := agg[k]
					if !ok {
						continue
					}

					sb.WriteString(fmt.Sprintf("| %s | %.2f | %.2f | %.2f | %d |\n",
						app,
						a.avgNsOp(),
						a.avgBytesOp(),
						a.avgAllocsOp(),
						a.Count,
					))
				}
				sb.WriteString("\n")
			}
		}
	}

	sb.WriteString("#### Overhead vs baseline\n\n")
	sb.WriteString("> Metric used for overhead: **average ns/op**.\n")
	sb.WriteString("> Baseline mapping: `v2` is compared to `v1`; `v2-codegen` and `v2-ptr` are compared to `v2`.\n")
	sb.WriteString("> Delta formula: `delta_ns = sdk_ns - baseline_ns`; negative means faster, positive means slower.\n")
	sb.WriteString("> Percentage formula: `delta_pct = delta_ns / baseline_ns * 100`.\n")
	sb.WriteString("> Values are based on per-tuple means across files, not on run-by-run paired comparisons.\n\n")

	for _, op := range operations {
		sb.WriteString(fmt.Sprintf("##### %s\n\n", op))

		for _, arch := range arches {
			for _, size := range sizes {
				v1, hasV1 := agg[benchmarkKey{App: "v1", Arch: arch, Operation: op, Size: size}]
				v2, hasV2 := agg[benchmarkKey{App: "v2", Arch: arch, Operation: op, Size: size}]
				if !hasV1 && !hasV2 {
					continue
				}

				sb.WriteString(fmt.Sprintf("**Arch: %s - %s**\n\n", arch, size))
				sb.WriteString("| SDK | Baseline | ns/op | Delta vs baseline |\n")
				sb.WriteString("|-----|----------|------:|------------------:|\n")

				if hasV1 {
					sb.WriteString(fmt.Sprintf("| v1 | - | %.2f | - |\n", v1.avgNsOp()))
				}
				if hasV2 {
					if hasV1 {
						sb.WriteString(fmt.Sprintf("| v2 | v1 | %.2f | %s |\n", v2.avgNsOp(), formatDelta(v2.avgNsOp(), v1.avgNsOp())))
					} else {
						sb.WriteString(fmt.Sprintf("| v2 | - | %.2f | - |\n", v2.avgNsOp()))
					}
				}

				for _, variant := range []string{"v2-codegen", "v2-ptr"} {
					v, ok := agg[benchmarkKey{App: variant, Arch: arch, Operation: op, Size: size}]
					if !ok {
						continue
					}
					if hasV2 {
						sb.WriteString(fmt.Sprintf("| %s | v2 | %.2f | %s |\n", variant, v.avgNsOp(), formatDelta(v.avgNsOp(), v2.avgNsOp())))
					} else {
						sb.WriteString(fmt.Sprintf("| %s | - | %.2f | - |\n", variant, v.avgNsOp()))
					}
				}

				sb.WriteString("\n")
			}
		}
	}

	sb.WriteString("#### Conclusions\n\n")
	sb.WriteString(renderConclusions(agg, arches))
	sb.WriteString("\n")

	return sb.String()
}

func renderConclusions(agg map[benchmarkKey]benchmarkAgg, arches []string) string {
	var sb strings.Builder

	sb.WriteString("- **Execution Speed**\n")
	for _, op := range operations {
		comparisons := 0
		sumV2VsV1Pct := 0.0

		codegenComparisons := 0
		codegenWins := 0
		codegenWinMarginSum := 0.0

		ptrComparisons := 0
		ptrWins := 0
		ptrWinMarginSum := 0.0

		for _, arch := range arches {
			for _, size := range sizes {
				v1, hasV1 := agg[benchmarkKey{App: "v1", Arch: arch, Operation: op, Size: size}]
				v2, hasV2 := agg[benchmarkKey{App: "v2", Arch: arch, Operation: op, Size: size}]

				if hasV1 && hasV2 && v1.avgNsOp() > 0 {
					sumV2VsV1Pct += (v2.avgNsOp() - v1.avgNsOp()) / v1.avgNsOp() * 100
					comparisons++
				}

				v2Codegen, hasCodegen := agg[benchmarkKey{App: "v2-codegen", Arch: arch, Operation: op, Size: size}]
				if hasV2 && hasCodegen {
					codegenComparisons++
					if v2Codegen.avgNsOp() < v2.avgNsOp() {
						codegenWins++
						if v2.avgNsOp() > 0 {
							codegenWinMarginSum += (v2.avgNsOp() - v2Codegen.avgNsOp()) / v2.avgNsOp() * 100
						}
					}
				}

				v2Ptr, hasPtr := agg[benchmarkKey{App: "v2-ptr", Arch: arch, Operation: op, Size: size}]
				if hasV2 && hasPtr {
					ptrComparisons++
					if v2Ptr.avgNsOp() < v2.avgNsOp() {
						ptrWins++
						if v2.avgNsOp() > 0 {
							ptrWinMarginSum += (v2.avgNsOp() - v2Ptr.avgNsOp()) / v2.avgNsOp() * 100
						}
					}
				}
			}
		}

		sb.WriteString(fmt.Sprintf("  - **%s**\n", op))

		if comparisons == 0 {
			sb.WriteString("    - v2 vs v1: n/a (insufficient comparable datapoints).\n")
		} else {
			avgPct := sumV2VsV1Pct / float64(comparisons)
			trend := "faster"
			if avgPct > 0 {
				trend = "slower"
			}
			sb.WriteString(fmt.Sprintf("    - v2 vs v1: %.2f%% %s on average across %d arch/size combinations.\n", math.Abs(avgPct), trend, comparisons))
		}

		if codegenComparisons == 0 {
			sb.WriteString("    - v2-codegen vs v2: n/a.\n")
		} else {
			winRate := float64(codegenWins) / float64(codegenComparisons) * 100
			winMargin := 0.0
			if codegenWins > 0 {
				winMargin = codegenWinMarginSum / float64(codegenWins)
			}
			sb.WriteString(fmt.Sprintf("    - v2-codegen vs v2: wins %d/%d (%.2f%%), by %.2f%% on average when it wins.\n", codegenWins, codegenComparisons, winRate, winMargin))
		}

		if ptrComparisons == 0 {
			sb.WriteString("    - v2-ptr vs v2: n/a.\n")
		} else {
			winRate := float64(ptrWins) / float64(ptrComparisons) * 100
			winMargin := 0.0
			if ptrWins > 0 {
				winMargin = ptrWinMarginSum / float64(ptrWins)
			}
			sb.WriteString(fmt.Sprintf("    - v2-ptr vs v2: wins %d/%d (%.2f%%), by %.2f%% on average when it wins.\n", ptrWins, ptrComparisons, winRate, winMargin))
		}
	}

	sb.WriteString("- **Allocations**\n")
	for _, op := range operations {
		comparisons := 0
		sumV2VsV1Pct := 0.0

		codegenComparisons := 0
		codegenWins := 0
		codegenWinMarginSum := 0.0
		codegenLosses := 0
		codegenLossMarginSum := 0.0

		ptrComparisons := 0
		ptrWins := 0
		ptrWinMarginSum := 0.0
		ptrLosses := 0
		ptrLossMarginSum := 0.0

		for _, arch := range arches {
			for _, size := range sizes {
				v1, hasV1 := agg[benchmarkKey{App: "v1", Arch: arch, Operation: op, Size: size}]
				v2, hasV2 := agg[benchmarkKey{App: "v2", Arch: arch, Operation: op, Size: size}]

				if hasV1 && hasV2 && v1.avgAllocsOp() > 0 {
					sumV2VsV1Pct += (v2.avgAllocsOp() - v1.avgAllocsOp()) / v1.avgAllocsOp() * 100
					comparisons++
				}

				v2Codegen, hasCodegen := agg[benchmarkKey{App: "v2-codegen", Arch: arch, Operation: op, Size: size}]
				if hasV2 && hasCodegen {
					codegenComparisons++
					if v2Codegen.avgAllocsOp() < v2.avgAllocsOp() {
						codegenWins++
						if v2.avgAllocsOp() > 0 {
							codegenWinMarginSum += (v2.avgAllocsOp() - v2Codegen.avgAllocsOp()) / v2.avgAllocsOp() * 100
						}
					} else if v2Codegen.avgAllocsOp() > v2.avgAllocsOp() {
						codegenLosses++
						if v2.avgAllocsOp() > 0 {
							codegenLossMarginSum += (v2Codegen.avgAllocsOp() - v2.avgAllocsOp()) / v2.avgAllocsOp() * 100
						}
					}
				}

				v2Ptr, hasPtr := agg[benchmarkKey{App: "v2-ptr", Arch: arch, Operation: op, Size: size}]
				if hasV2 && hasPtr {
					ptrComparisons++
					if v2Ptr.avgAllocsOp() < v2.avgAllocsOp() {
						ptrWins++
						if v2.avgAllocsOp() > 0 {
							ptrWinMarginSum += (v2.avgAllocsOp() - v2Ptr.avgAllocsOp()) / v2.avgAllocsOp() * 100
						}
					} else if v2Ptr.avgAllocsOp() > v2.avgAllocsOp() {
						ptrLosses++
						if v2.avgAllocsOp() > 0 {
							ptrLossMarginSum += (v2Ptr.avgAllocsOp() - v2.avgAllocsOp()) / v2.avgAllocsOp() * 100
						}
					}
				}
			}
		}

		sb.WriteString(fmt.Sprintf("  - **%s**\n", op))

		if comparisons == 0 {
			sb.WriteString("    - v2 vs v1: n/a (insufficient comparable datapoints).\n")
		} else {
			avgPct := sumV2VsV1Pct / float64(comparisons)
			trend := "fewer allocations"
			if avgPct > 0 {
				trend = "more allocations"
			}
			sb.WriteString(fmt.Sprintf("    - v2 vs v1: %.2f%% %s on average across %d arch/size combinations.\n", math.Abs(avgPct), trend, comparisons))
		}

		if codegenComparisons == 0 {
			sb.WriteString("    - v2-codegen vs v2: n/a.\n")
		} else if codegenWins > 0 {
			winRate := float64(codegenWins) / float64(codegenComparisons) * 100
			winMargin := codegenWinMarginSum / float64(codegenWins)
			sb.WriteString(fmt.Sprintf("    - v2-codegen vs v2: lower allocs in %d/%d (%.2f%%), by %.2f%% on average when it wins.\n", codegenWins, codegenComparisons, winRate, winMargin))
		} else if codegenLosses > 0 {
			lossRate := float64(codegenLosses) / float64(codegenComparisons) * 100
			lossMargin := codegenLossMarginSum / float64(codegenLosses)
			sb.WriteString(fmt.Sprintf("    - v2-codegen vs v2: lower allocs in 0/%d (0.00%%); higher allocs in %d/%d (%.2f%%), by %.2f%% on average when it loses.\n", codegenComparisons, codegenLosses, codegenComparisons, lossRate, lossMargin))
		} else {
			sb.WriteString(fmt.Sprintf("    - v2-codegen vs v2: tied on allocations in all %d comparable combinations.\n", codegenComparisons))
		}

		if ptrComparisons == 0 {
			sb.WriteString("    - v2-ptr vs v2: n/a.\n")
		} else if ptrWins > 0 {
			winRate := float64(ptrWins) / float64(ptrComparisons) * 100
			winMargin := ptrWinMarginSum / float64(ptrWins)
			sb.WriteString(fmt.Sprintf("    - v2-ptr vs v2: lower allocs in %d/%d (%.2f%%), by %.2f%% on average when it wins.\n", ptrWins, ptrComparisons, winRate, winMargin))
		} else if ptrLosses > 0 {
			lossRate := float64(ptrLosses) / float64(ptrComparisons) * 100
			lossMargin := ptrLossMarginSum / float64(ptrLosses)
			sb.WriteString(fmt.Sprintf("    - v2-ptr vs v2: lower allocs in 0/%d (0.00%%); higher allocs in %d/%d (%.2f%%), by %.2f%% on average when it loses.\n", ptrComparisons, ptrLosses, ptrComparisons, lossRate, lossMargin))
		} else {
			sb.WriteString(fmt.Sprintf("    - v2-ptr vs v2: tied on allocations in all %d comparable combinations.\n", ptrComparisons))
		}
	}

	return sb.String()
}

func sortedArches(agg map[benchmarkKey]benchmarkAgg) []string {
	set := make(map[string]struct{})
	for k := range agg {
		set[k.Arch] = struct{}{}
	}

	ordered := []string{"arm64", "amd64"}
	var out []string
	seen := make(map[string]struct{})

	for _, arch := range ordered {
		if _, ok := set[arch]; ok {
			out = append(out, arch)
			seen[arch] = struct{}{}
		}
	}

	var extras []string
	for arch := range set {
		if _, ok := seen[arch]; ok {
			continue
		}
		extras = append(extras, arch)
	}
	sort.Strings(extras)
	out = append(out, extras...)

	return out
}

func formatDelta(value, baseline float64) string {
	if baseline == 0 {
		return "-"
	}
	delta := value - baseline
	pct := delta / baseline * 100
	return fmt.Sprintf("%+.2f ns/op (%+.2f%%)", delta, pct)
}
