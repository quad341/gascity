# Coordstore Benchmark Harness

## Performance measurement methodology

Latency results (p99 targets) are only authoritative when the test host is quiesced.
If loadavg/GOMAXPROCS > 0.80, the harness automatically suppresses p99 gating
and marks results as informational. Stop co-located workloads before baselining
latency performance.

Correctness targets (functional requirements, throughput thresholds) run regardless
of host load.
