# prow-timeline

Create an HTML report from the timeline artifacts of a public Prow job.
The report contains the event data, styles, and scripts.
Open the report in a browser without a server or network connection.

## Requirements

- Go 1.26 or later.
- GNU Make for the commands below.
- Node.js 18 or later for JavaScript tests.

## Build and run

1. Build the program.

   ```sh
   make build
   ```

2. Generate a report.

   ```sh
   ./prow-timeline -output timeline.html -timeout 5m \
     'https://prow.ci.openshift.org/view/gs/test-platform-results-public/pr-logs/pull/31837/pull-ci-openshift-origin-main-e2e-aws-ovn-serial-1of2/2117281483113631744'
   ```

3. Open `timeline.html` in a browser.

Place all flags before the job URL.
The program writes the report after it downloads and checks all artifacts.
The report contains the original artifact data and makes no network requests when opened.
You decide where to store it, who to share it with, and whether it needs any access restrictions.

| Flag | Default | Purpose |
| --- | --- | --- |
| `-output` | `timeline.html` | Set the report path. |
| `-timeout` | `5m` | Set a positive time limit for all requests. |

## Use the report

Open `timeline.html` in your browser, then choose a **Step / run artifact**.
This selects the timeline file whose tests and event chart you want to explore.
Use **Search tests** to find a test by name or outcome, then select an attempt
under **Select test attempt**. The report shows its status, start and end times,
and duration.

### Compare event counts

Selecting a test updates **Events by resource** so you can compare how many events
occurred during that test with how many occurred across the loaded run data.
Each row groups events with the same category/source, namespace, resource,
and event type, such as repeated events for a particular pod.

| Column | Meaning |
| --- | --- |
| **Timeline records** | Number of matching event entries across all artifacts loaded into the report, including duplicates and events outside the selected test. |
| **Records in selected test** | Number of those entries whose time intervals overlap the selected test, including its start and end points. |
| **KubeEvent cumulative (by UID)** | Sum of the maximum `message.annotations.count` for each distinct Kubernetes Event UID among matching records. Duplicate snapshots and changes to the message or `hmsg` do not increase this total. |
| **Cumulative so far (test end)** | Sum of maximum counters per UID observed at or before the selected test's end. Future snapshots and invalid timestamps are excluded. Click **View progression** for the sequence within the test. |
| **Counter observations** | Click **Inspect counters** to view each snapshot's UID, `hmsg`, count, first/last timestamps, interval, message, pathological flag and number of duplicate records in **Event details**. |

For example, a row with **Timeline records = 20** and **Records in selected test = 3** means that
20 entries match that resource and event type across the loaded artifacts,
and 3 overlap the selected test.
The total covers the data included in the report; it may not cover the whole Prow job.
Changing **Step / run artifact** changes the available tests and the chart,
while **Timeline records** continues to cover all loaded artifacts.

Each timeline entry counts once. A cumulative count recorded inside an event
does not increase this number, and entries repeated across artifacts are counted separately.
Event types use the event's reason and cause, or its message when no reason is present.
Overlapping tests can include the same event: overlap tells you when it happened,
without proving that a particular test caused it.

For example, the same Event UID observed with counts 79, 97 and 103 contributes
**103** to the cumulative column, even if each snapshot appears twice and its
message changes. These values describe the Event object's cumulative history;
they do not mean each displayed message occurred that many times. Kubernetes can
combine similar messages into a single Event object.

The cumulative column covers matching snapshots across all loaded artifacts;
it does not count occurrences within the selected test. The Event history can
begin before monitoring or the selected test. Gaps between snapshots hide the
time and message of individual occurrences. Filters can hide later snapshots,
so the cumulative column uses the maximum still visible under those filters.
Missing `count` defaults to 1, matching the monitor. Records without an Event UID
or with an invalid count (anything other than a nonnegative integer) are excluded; the column
marks the result as partial. Other event sources show a dash. The maximum per UID
is an observed high-water mark, not a reconstructed history of counter resets.

The pathological flag is original monitor metadata. This report does not
reproduce the monitor's allowlists, thresholds or test result. Use the snapshots
to compare the counter values with the job's failure output.

**View progression** opens a table showing the latest observation before the
selected test for each UID, followed by its in-test observations. Each row shows
the timestamp, cumulative count, increase since the previous observation for that
UID, message and `hmsg`. Duplicate copies appear once. For example, 97 followed by
103 within the test shows an increase of **+6** bounded by those two observations.
This locates the increase within the test window without proving its cause.

An increase from a pre-test baseline to an in-test snapshot is marked
**Crosses test start**: some occurrences could precede the test. With no previous
observation, the increase is unknown. A counter decrease also makes that increment
unknown; subsequent increases use the new observed value. **So far** retains the
maximum-per-UID rule. Occurrence timestamps, rather than the extra second added
by the monitor for chart display, determine whether a snapshot is inside the test.
Filters apply to the baseline and sequence. The progression has 60 rows per page.

### Filter events

Use the controls below the test selector to narrow down the counts and chart:

| Filter | What it selects | Example |
| --- | --- | --- |
| **Category / source** | The kind of information recorded in an event's `source` field. The available values come from the loaded data. | `E2ETest` for test execution intervals or `PodState` for pod state information. |
| **Namespace** | Events associated with a specific Kubernetes namespace. | Select your application's namespace to focus on its resources. |
| **Event text** | A case-insensitive substring anywhere in an event's stored data, including resource names, messages, reasons, severity levels, and annotations. | Enter a pod name, `Failed`, or a phrase from an event message. |

You can combine all three filters. All count columns update to use the same filters,
so you can compare the selected test with the wider run for just the events of interest.
The chart and **Invalid events** list use those filters within the selected artifact.
These filters do not change which test is selected; **Search tests** searches the test list separately.

Select **Global overview (no selected test)** to browse totals without a test comparison.
In this mode, **Records in selected test** shows a dash and the chart is empty.
Select a test again to restore the comparison and chart.

### Read the timeline

The chart places events from the selected artifact on a horizontal time axis.
Each row represents one event, with a bar showing when it started and how long it lasted.
The axis shows UTC timestamps and seconds relative to the selected test's start.

By default, **Window → Selected test + margin** shows events that overlap the test,
with some space around its start and end. Choose **Entire step / run (outside events)**
to see events before and after the test in the same artifact.
This changes the chart's time window; the count table still compares all loaded artifacts
with the exact selected test interval.

- Purple bars represent test intervals; blue bars represent other events.
- Each row label shows the source, `message.reason` (for example,
  `ScalingReplicaSet` or `ProbeError`), and resource. A missing reason shows a dash.
- The orange and pink vertical lines mark the selected test's start and end.
  The shaded band between them shows its time interval.
- Arrows and dashed bar edges indicate that an event continues beyond the visible
  window or has an unknown end.

Click an event row to open **Event details**. You can inspect its resource,
message, original timestamps, and the tests whose intervals overlap it.
The chart combines records that are identical in every stored field within the
selected artifact, regardless of JSON object key order. A combined row shows
**2 copies**, for example, and its details include `identicalRecords: 2`.
Different timestamps, counters, messages, UIDs or other fields keep rows separate.
The summary shows both displayed intervals and the original number of matching
records. Original embedded records, count-table totals and test attempts are
preserved.
For an event with an unknown end (`to: null`), the chart extends the bar to the
latest valid timestamp in that artifact. This is the last observed time,
rather than a confirmed end; the details retain the original null value.

Use **Previous** and **Next** to move through the chart, which shows 60 events per page.
The counts and invalid-event lists have separate page controls.
The test search shows at most 300 results; narrow the search if your test is not listed.

## Supported input

Use an HTTPS job URL from either host:

- `prow.ci.openshift.org/view/gs/BUCKET/PATH`
- `gcsweb-ci.apps.ci.l2s4.p1.openshiftapps.com/gcs/BUCKET/PATH`

The job path must use one of these forms:

- `logs/JOB/BUILD_ID`
- `pr-logs/pull/PR/JOB/BUILD_ID`
- `pr-logs/pull/ORG/REPO/PR/JOB/BUILD_ID`

Use a numeric build ID.
Do not use an artifact URL, a query string, or a fragment.
Private artifacts and HTTP redirects are not supported.

The program reads files under `artifacts/` with these names:

- `e2e-timelines_everything_YYYYMMDD-HHMMSS.json`
- `e2e-timelines_everything_YYYYMMDD-HHMMSS.json.gz`

Each file must contain a nonempty `items` array.
The dataset must contain an `E2ETest` entry with a `locator.keys["e2e-test"]` name.
The program rejects JSON that cannot be decoded and missing or empty interval arrays.
Each interval has `valid` and `reason` properties computed by the parser.
Missing, invalid, or reversed timestamps set `valid: false` and an explanatory `reason`.
Valid intervals have `valid: true` and an empty `reason`.
The report retains invalid intervals in total counts and lists them under **Invalid events**
with their reason and original data. The list uses the selected artifact and event filters.
Invalid intervals do not affect the chart, open-interval horizon, or overlap counts,
and invalid test intervals cannot be selected as a time window.
An explicit `to: null` is valid.
The program includes entries with `display: false`.

The report compares UTC timestamps with nanosecond precision.
The chart uses floating point milliseconds to draw intervals.
The report covers the loaded artifacts, which can omit parts of the job.

## Build and test commands

Run `make help` to list all targets.

| Target | Action |
| --- | --- |
| `make build` | Build `./prow-timeline`. |
| `make test` | Run the Go and JavaScript tests. |
| `make test-go` | Run the Go tests. |
| `make test-js` | Run the JavaScript tests. |
| `make test-race` | Run the Go tests with the race detector. |
| `make vet` | Check the Go code with `go vet`. |
| `make fmt` | Format the Go code. |
| `make fmt-check` | Check Go formatting without changing files. |
| `make check` | Check formatting, run vet, and run all tests. |
| `make all` | Run all checks, run the race detector, and build the program. |
| `make coverage` | Write the Go coverage data to `coverage.out`. |
| `make coverage-html` | Create `coverage.html` from the Go coverage data. |
| `make clean` | Remove the binary and coverage files. |

The tests use data and HTTP responses in memory.
The tests do not require GCS access or local network sockets.
The clean target preserves `timeline.html`.

Override a tool or output path when necessary:

```sh
make build GO=go BINARY=/tmp/prow-timeline
make test-js NODE=node
```

## License

This project is licensed under the [MIT License](LICENSE).
