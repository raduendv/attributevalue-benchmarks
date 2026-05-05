### Appendix #7: Benchmarks

> Aggregation uses arithmetic mean across files for each `(sdk, arch, operation, size)` tuple.
> `MarshalMap` = struct -> `map[string]types.AttributeValue`; `UnmarshalMap` = reverse mapping.

#### Test Entities

- `User` benchmark entity at payload targets: `1KB`, `10KB`, `100KB`, `300KB`.
  - Scalar fields: strings, numbers, booleans.
  - Optional/pointer fields, including optional nested objects.
  - Temporal fields: `time.Time` and optional timestamps.
  - Collections: byte slice, string lists, and maps.
  - Nested structs: `Address` list/object and embedded `Timestampable` metadata.
- This mix is intended to stress realistic `dynamodbav` mapping paths (nested structs, optional values, collections, and timestamps).
- Input source: benchmark rows matching `Benchmark_*_(MarshalMap|UnmarshalMap)_User<size>`.
- Current report scope: `User` entity only; primitive-type entity benchmarks are not included in these tables.

#### MarshalMap

**Arch: arm64 - 1KB**

| SDK | ns/op | B/op | allocs/op | files |
|-----|------:|-----:|----------:|------:|
| v1 | 34337.83 | 21141.17 | 206.50 | 6 |
| v2 | 22984.83 | 8300.83 | 143.67 | 6 |
| v2-codegen | 25306.33 | 8869.17 | 160.67 | 6 |
| v2-ptr | 8165.50 | 3492.00 | 43.50 | 6 |

**Arch: arm64 - 10KB**

| SDK | ns/op | B/op | allocs/op | files |
|-----|------:|-----:|----------:|------:|
| v1 | 32536.67 | 19753.83 | 195.83 | 6 |
| v2 | 22941.00 | 8246.00 | 144.00 | 6 |
| v2-codegen | 22659.50 | 8170.33 | 143.00 | 6 |
| v2-ptr | 8203.17 | 3504.00 | 44.00 | 6 |

**Arch: arm64 - 100KB**

| SDK | ns/op | B/op | allocs/op | files |
|-----|------:|-----:|----------:|------:|
| v1 | 27744.33 | 16791.33 | 167.33 | 6 |
| v2 | 23478.00 | 8394.33 | 147.67 | 6 |
| v2-codegen | 22466.67 | 8189.00 | 139.00 | 6 |
| v2-ptr | 8237.33 | 3504.00 | 44.00 | 6 |

**Arch: arm64 - 300KB**

| SDK | ns/op | B/op | allocs/op | files |
|-----|------:|-----:|----------:|------:|
| v1 | 31668.67 | 19061.50 | 189.83 | 6 |
| v2 | 23886.17 | 8486.50 | 149.67 | 6 |
| v2-codegen | 23639.17 | 8484.50 | 148.83 | 6 |
| v2-ptr | 8284.17 | 3516.00 | 44.50 | 6 |

**Arch: amd64 - 1KB**

| SDK | ns/op | B/op | allocs/op | files |
|-----|------:|-----:|----------:|------:|
| v1 | 17727.00 | 18369.67 | 180.83 | 6 |
| v2 | 12182.67 | 8090.33 | 137.33 | 6 |
| v2-codegen | 12067.17 | 8083.33 | 136.33 | 6 |
| v2-ptr | 4866.00 | 3524.00 | 45.00 | 6 |

**Arch: amd64 - 10KB**

| SDK | ns/op | B/op | allocs/op | files |
|-----|------:|-----:|----------:|------:|
| v1 | 19759.00 | 20536.00 | 204.33 | 6 |
| v2 | 13842.67 | 8859.00 | 159.67 | 6 |
| v2-codegen | 12979.00 | 8492.17 | 151.00 | 6 |
| v2-ptr | 4873.00 | 3528.00 | 45.00 | 6 |

**Arch: amd64 - 100KB**

| SDK | ns/op | B/op | allocs/op | files |
|-----|------:|-----:|----------:|------:|
| v1 | 18360.17 | 19047.33 | 189.50 | 6 |
| v2 | 11828.83 | 7868.33 | 132.67 | 6 |
| v2-codegen | 13021.50 | 8518.83 | 151.67 | 6 |
| v2-ptr | 4856.17 | 3514.67 | 44.50 | 6 |

**Arch: amd64 - 300KB**

| SDK | ns/op | B/op | allocs/op | files |
|-----|------:|-----:|----------:|------:|
| v1 | 19330.67 | 20061.67 | 200.33 | 6 |
| v2 | 13642.33 | 8731.67 | 158.67 | 6 |
| v2-codegen | 11700.67 | 7937.50 | 133.50 | 6 |
| v2-ptr | 4820.67 | 3504.00 | 44.00 | 6 |

#### UnmarshalMap

**Arch: arm64 - 1KB**

| SDK | ns/op | B/op | allocs/op | files |
|-----|------:|-----:|----------:|------:|
| v1 | 17996.33 | 3003.50 | 32.17 | 6 |
| v2 | 17430.50 | 2824.83 | 36.50 | 6 |
| v2-codegen | 12628.17 | 3397.33 | 49.50 | 6 |
| v2-ptr | 3122.17 | 984.00 | 6.50 | 6 |

**Arch: arm64 - 10KB**

| SDK | ns/op | B/op | allocs/op | files |
|-----|------:|-----:|----------:|------:|
| v1 | 17379.00 | 2958.17 | 35.67 | 6 |
| v2 | 17421.83 | 2830.50 | 37.50 | 6 |
| v2-codegen | 10671.83 | 3237.33 | 47.17 | 6 |
| v2-ptr | 3167.00 | 992.00 | 7.00 | 6 |

**Arch: arm64 - 100KB**

| SDK | ns/op | B/op | allocs/op | files |
|-----|------:|-----:|----------:|------:|
| v1 | 15218.83 | 2764.83 | 33.33 | 6 |
| v2 | 17899.33 | 2854.67 | 37.33 | 6 |
| v2-codegen | 11603.33 | 3197.33 | 47.33 | 6 |
| v2-ptr | 3200.50 | 992.00 | 7.00 | 6 |

**Arch: arm64 - 300KB**

| SDK | ns/op | B/op | allocs/op | files |
|-----|------:|-----:|----------:|------:|
| v1 | 17076.67 | 2920.83 | 35.00 | 6 |
| v2 | 18167.50 | 2889.00 | 39.17 | 6 |
| v2-codegen | 12320.17 | 3309.33 | 46.67 | 6 |
| v2-ptr | 3242.33 | 1000.00 | 7.50 | 6 |

**Arch: amd64 - 1KB**

| SDK | ns/op | B/op | allocs/op | files |
|-----|------:|-----:|----------:|------:|
| v1 | 8461.17 | 2866.17 | 32.83 | 6 |
| v2 | 8929.50 | 2782.00 | 36.17 | 6 |
| v2-codegen | 6003.50 | 3194.67 | 48.17 | 6 |
| v2-ptr | 1875.33 | 1008.00 | 8.00 | 6 |

**Arch: amd64 - 10KB**

| SDK | ns/op | B/op | allocs/op | files |
|-----|------:|-----:|----------:|------:|
| v1 | 9336.50 | 3011.83 | 35.17 | 6 |
| v2 | 10046.50 | 2955.50 | 38.83 | 6 |
| v2-codegen | 6380.50 | 3310.67 | 47.83 | 6 |
| v2-ptr | 1876.00 | 1008.00 | 8.00 | 6 |

**Arch: amd64 - 100KB**

| SDK | ns/op | B/op | allocs/op | files |
|-----|------:|-----:|----------:|------:|
| v1 | 8810.00 | 2923.83 | 35.50 | 6 |
| v2 | 8789.33 | 2744.83 | 37.50 | 6 |
| v2-codegen | 6337.33 | 3316.00 | 47.50 | 6 |
| v2-ptr | 1848.17 | 1000.00 | 7.50 | 6 |

**Arch: amd64 - 300KB**

| SDK | ns/op | B/op | allocs/op | files |
|-----|------:|-----:|----------:|------:|
| v1 | 9171.17 | 2983.67 | 35.00 | 6 |
| v2 | 9899.67 | 2958.33 | 38.67 | 6 |
| v2-codegen | 5963.50 | 3208.00 | 46.00 | 6 |
| v2-ptr | 1818.67 | 992.00 | 7.00 | 6 |

#### Overhead vs baseline

> Metric used for overhead: **average ns/op**.
> Baseline mapping: `v2` is compared to `v1`; `v2-codegen` and `v2-ptr` are compared to `v2`.
> Delta formula: `delta_ns = sdk_ns - baseline_ns`; negative means faster, positive means slower.
> Percentage formula: `delta_pct = delta_ns / baseline_ns * 100`.
> Values are based on per-tuple means across files, not on run-by-run paired comparisons.

##### MarshalMap

**Arch: arm64 - 1KB**

| SDK | Baseline | ns/op | Delta vs baseline |
|-----|----------|------:|------------------:|
| v1 | - | 34337.83 | - |
| v2 | v1 | 22984.83 | -11353.00 ns/op (-33.06%) |
| v2-codegen | v2 | 25306.33 | +2321.50 ns/op (+10.10%) |
| v2-ptr | v2 | 8165.50 | -14819.33 ns/op (-64.47%) |

**Arch: arm64 - 10KB**

| SDK | Baseline | ns/op | Delta vs baseline |
|-----|----------|------:|------------------:|
| v1 | - | 32536.67 | - |
| v2 | v1 | 22941.00 | -9595.67 ns/op (-29.49%) |
| v2-codegen | v2 | 22659.50 | -281.50 ns/op (-1.23%) |
| v2-ptr | v2 | 8203.17 | -14737.83 ns/op (-64.24%) |

**Arch: arm64 - 100KB**

| SDK | Baseline | ns/op | Delta vs baseline |
|-----|----------|------:|------------------:|
| v1 | - | 27744.33 | - |
| v2 | v1 | 23478.00 | -4266.33 ns/op (-15.38%) |
| v2-codegen | v2 | 22466.67 | -1011.33 ns/op (-4.31%) |
| v2-ptr | v2 | 8237.33 | -15240.67 ns/op (-64.91%) |

**Arch: arm64 - 300KB**

| SDK | Baseline | ns/op | Delta vs baseline |
|-----|----------|------:|------------------:|
| v1 | - | 31668.67 | - |
| v2 | v1 | 23886.17 | -7782.50 ns/op (-24.57%) |
| v2-codegen | v2 | 23639.17 | -247.00 ns/op (-1.03%) |
| v2-ptr | v2 | 8284.17 | -15602.00 ns/op (-65.32%) |

**Arch: amd64 - 1KB**

| SDK | Baseline | ns/op | Delta vs baseline |
|-----|----------|------:|------------------:|
| v1 | - | 17727.00 | - |
| v2 | v1 | 12182.67 | -5544.33 ns/op (-31.28%) |
| v2-codegen | v2 | 12067.17 | -115.50 ns/op (-0.95%) |
| v2-ptr | v2 | 4866.00 | -7316.67 ns/op (-60.06%) |

**Arch: amd64 - 10KB**

| SDK | Baseline | ns/op | Delta vs baseline |
|-----|----------|------:|------------------:|
| v1 | - | 19759.00 | - |
| v2 | v1 | 13842.67 | -5916.33 ns/op (-29.94%) |
| v2-codegen | v2 | 12979.00 | -863.67 ns/op (-6.24%) |
| v2-ptr | v2 | 4873.00 | -8969.67 ns/op (-64.80%) |

**Arch: amd64 - 100KB**

| SDK | Baseline | ns/op | Delta vs baseline |
|-----|----------|------:|------------------:|
| v1 | - | 18360.17 | - |
| v2 | v1 | 11828.83 | -6531.33 ns/op (-35.57%) |
| v2-codegen | v2 | 13021.50 | +1192.67 ns/op (+10.08%) |
| v2-ptr | v2 | 4856.17 | -6972.67 ns/op (-58.95%) |

**Arch: amd64 - 300KB**

| SDK | Baseline | ns/op | Delta vs baseline |
|-----|----------|------:|------------------:|
| v1 | - | 19330.67 | - |
| v2 | v1 | 13642.33 | -5688.33 ns/op (-29.43%) |
| v2-codegen | v2 | 11700.67 | -1941.67 ns/op (-14.23%) |
| v2-ptr | v2 | 4820.67 | -8821.67 ns/op (-64.66%) |

##### UnmarshalMap

**Arch: arm64 - 1KB**

| SDK | Baseline | ns/op | Delta vs baseline |
|-----|----------|------:|------------------:|
| v1 | - | 17996.33 | - |
| v2 | v1 | 17430.50 | -565.83 ns/op (-3.14%) |
| v2-codegen | v2 | 12628.17 | -4802.33 ns/op (-27.55%) |
| v2-ptr | v2 | 3122.17 | -14308.33 ns/op (-82.09%) |

**Arch: arm64 - 10KB**

| SDK | Baseline | ns/op | Delta vs baseline |
|-----|----------|------:|------------------:|
| v1 | - | 17379.00 | - |
| v2 | v1 | 17421.83 | +42.83 ns/op (+0.25%) |
| v2-codegen | v2 | 10671.83 | -6750.00 ns/op (-38.74%) |
| v2-ptr | v2 | 3167.00 | -14254.83 ns/op (-81.82%) |

**Arch: arm64 - 100KB**

| SDK | Baseline | ns/op | Delta vs baseline |
|-----|----------|------:|------------------:|
| v1 | - | 15218.83 | - |
| v2 | v1 | 17899.33 | +2680.50 ns/op (+17.61%) |
| v2-codegen | v2 | 11603.33 | -6296.00 ns/op (-35.17%) |
| v2-ptr | v2 | 3200.50 | -14698.83 ns/op (-82.12%) |

**Arch: arm64 - 300KB**

| SDK | Baseline | ns/op | Delta vs baseline |
|-----|----------|------:|------------------:|
| v1 | - | 17076.67 | - |
| v2 | v1 | 18167.50 | +1090.83 ns/op (+6.39%) |
| v2-codegen | v2 | 12320.17 | -5847.33 ns/op (-32.19%) |
| v2-ptr | v2 | 3242.33 | -14925.17 ns/op (-82.15%) |

**Arch: amd64 - 1KB**

| SDK | Baseline | ns/op | Delta vs baseline |
|-----|----------|------:|------------------:|
| v1 | - | 8461.17 | - |
| v2 | v1 | 8929.50 | +468.33 ns/op (+5.54%) |
| v2-codegen | v2 | 6003.50 | -2926.00 ns/op (-32.77%) |
| v2-ptr | v2 | 1875.33 | -7054.17 ns/op (-79.00%) |

**Arch: amd64 - 10KB**

| SDK | Baseline | ns/op | Delta vs baseline |
|-----|----------|------:|------------------:|
| v1 | - | 9336.50 | - |
| v2 | v1 | 10046.50 | +710.00 ns/op (+7.60%) |
| v2-codegen | v2 | 6380.50 | -3666.00 ns/op (-36.49%) |
| v2-ptr | v2 | 1876.00 | -8170.50 ns/op (-81.33%) |

**Arch: amd64 - 100KB**

| SDK | Baseline | ns/op | Delta vs baseline |
|-----|----------|------:|------------------:|
| v1 | - | 8810.00 | - |
| v2 | v1 | 8789.33 | -20.67 ns/op (-0.23%) |
| v2-codegen | v2 | 6337.33 | -2452.00 ns/op (-27.90%) |
| v2-ptr | v2 | 1848.17 | -6941.17 ns/op (-78.97%) |

**Arch: amd64 - 300KB**

| SDK | Baseline | ns/op | Delta vs baseline |
|-----|----------|------:|------------------:|
| v1 | - | 9171.17 | - |
| v2 | v1 | 9899.67 | +728.50 ns/op (+7.94%) |
| v2-codegen | v2 | 5963.50 | -3936.17 ns/op (-39.76%) |
| v2-ptr | v2 | 1818.67 | -8081.00 ns/op (-81.63%) |

#### Conclusions

- **Speed**
  - **MarshalMap**
    - v2 vs v1: 28.59% faster on average across 8 arch/size combinations.
    - v2-codegen vs v2: wins 6/8 (75.00%), by 4.66% on average when it wins.
    - v2-ptr vs v2: wins 8/8 (100.00%), by 63.43% on average when it wins.
  - **UnmarshalMap**
    - v2 vs v1: 5.24% slower on average across 8 arch/size combinations.
    - v2-codegen vs v2: wins 8/8 (100.00%), by 33.82% on average when it wins.
    - v2-ptr vs v2: wins 8/8 (100.00%), by 81.14% on average when it wins.
- **Allocations**
  - **MarshalMap**
    - v2 vs v1: 23.31% fewer allocations on average across 8 arch/size combinations.
    - v2-codegen vs v2: lower allocs in 6/8 (75.00%), by 4.86% on average when it wins.
    - v2-ptr vs v2: lower allocs in 8/8 (100.00%), by 69.68% on average when it wins.
  - **UnmarshalMap**
    - v2 vs v1: 9.90% more allocations on average across 8 arch/size combinations.
    - v2-codegen vs v2: lower allocs in 0/8 (0.00%); higher allocs in 8/8 (100.00%), by 26.16% on average when it loses.
    - v2-ptr vs v2: lower allocs in 8/8 (100.00%), by 80.60% on average when it wins.

