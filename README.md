```
██████╗ ██████╗ ██╗███╗   ██╗███████╗ ██████╗
██╔══██╗██╔══██╗██║████╗  ██║██╔════╝██╔═══██╗
██████╔╝██║  ██║██║██╔██╗ ██║█████╗  ██║   ██║
██╔══██╗██║  ██║██║██║╚██╗██║██╔══╝  ██║   ██║
██████╔╝██████╔╝██║██║ ╚████║██║     ╚██████╔╝
╚═════╝ ╚═════╝ ╚═╝╚═╝  ╚═══╝╚═╝      ╚═════╝
```

Go rewrite of BDInfo.

## Installation

- Homebrew (macOS):

```sh
brew tap autobrr/go-bdinfo https://github.com/autobrr/go-bdinfo
brew install --cask autobrr/go-bdinfo/bdinfo
```

- Go install (requires Go toolchain):

```sh
go install github.com/autobrr/go-bdinfo/cmd/bdinfo@latest
```

- Latest release (one-liner, Linux x86_64):
  - Replace `linux_amd64` with `linux_arm64`, `darwin_amd64`, or `darwin_arm64` as needed.

```sh
curl -sL "$(curl -s https://api.github.com/repos/autobrr/go-bdinfo/releases/latest | grep browser_download_url | grep linux_amd64 | cut -d\" -f4)" | tar -xz -C /usr/local/bin
```

## Usage

Recommended (likely what you want):

```sh
bdinfo /path/to/bluray --summaryonly --main
```

```sh
bdinfo /path/to/bluray --summaryonly --main --stdout
bdinfo /path/to/bluray --forumsonly --main
bdinfo /path/to/bluray --main
bdinfo /path/to/bluray --summaryonly
bdinfo update
bdinfo version
```

Path is required (ISO file or Blu-ray folder).

Report default: `BDInfo_{0}.bdinfo` (disc label substituted).

## Performance

A full-disc scan runs at the speed of the disk. go-bdinfo matches the official BDInfo 2.0.5 CLI on wall time and report content, with 2.4 to 4.1 times less CPU and 2 to 15 times less memory. See [docs/benchmarks.md](docs/benchmarks.md) for discs, method, raw numbers and the parity diff.

## Library Usage

Use the exported API package instead of importing `internal/*`:

```go
package main

import (
  "context"
  "fmt"
  "os"

  "github.com/autobrr/go-bdinfo/pkg/bdinfo"
)

func main() {
  settings := bdinfo.DefaultSettings(".")
  result, err := bdinfo.Run(context.Background(), bdinfo.Options{
    Path:     "/path/to/disc/or.iso",
    Settings: settings,
  })
  if err != nil {
    panic(err)
  }
  if result.ReportPath == "-" {
    fmt.Print(result.Report)
    return
  }
  _ = os.WriteFile(result.ReportPath, []byte(result.Report), 0o644)
}
```

Notes:
- `Run` processes a single disc path per call.
- The API returns structured metadata (`Result.Disc`, `Result.Playlists`, `Result.Scan`) and rendered report content (`Result.Report`).
- `Result.QuickSummary` and `Result.ForumsBlock` hold the `--summaryonly` and `--forumsonly` text from the same scan. They are always filled and do not depend on `SummaryOnly` or `ForumsOnly`.
- File writing is caller-owned.

### Optional video collection and exact timelines

Set `Options.IncludeTimeline` to obtain `Result.Timelines` with original unsigned
MPLS IN/OUT boundaries, integer playlist offsets in **45 kHz ticks**, repeated
occurrences, alternate angles, per-item video mappings, connection conditions,
and CLPI clock sequences. The selected angle is currently zero. `Run` timelines
follow the playlists contributing to the selected report; summary and full-report
selection can differ. `DiscoverPlaylists` returns metadata-only timelines for
every discovered playlist, or the explicit `Settings.PlaylistOnly` selection.
Main/biggest, summary, and other report filters do not narrow discovery, which
never invokes a video collector.

Set `Options.VideoConsumer` to a `video.Factory` from
`github.com/autobrr/go-bdinfo/pkg/bdinfo/video` to collect complete video elementary
streams during the existing scan. This also enables timeline capture. Return
`(nil, nil)` to decline a source/PID. Inspect the per-occurrence mappings to select
primary HEVC; PID order and bitrate do not establish a primary stream. A physical
source/PID is collected once, even when several playlists or occurrences use it.
Collection happens before main/biggest report selection and needs no preselected
playlist. With `Settings.PlaylistOnly`, only that playlist's referenced sources
are scanned; otherwise collection can cover the broader disc scan. Decline
unneeded sources/PIDs in the factory to limit decoder work.

Each accepted consumer receives ordered synchronous `Consume` and
`Discontinuity` calls and exactly one `Finish`. Different physical sources may
run concurrently, so each consumer needs independent parser state and shared
result maps need synchronization. Data and PES-boundary slices are borrowed until
`Consume` returns. Callbacks provide backpressure and should observe cancellation;
BDInfo cannot interrupt arbitrary blocked caller code. `Finish` has no context
and must release resources promptly even after cancellation.

Batches contain ES payload beyond the report's 5 MiB probe, excluding TS/PES
headers and bounded-PES padding. Their boundaries retain absolute ES offsets,
physical source packet positions, and original **90 kHz PTS/DTS modulo 2^33**.
Zero is a valid timestamp. Batches and PES starts are not frame boundaries.
Discontinuities establish new segments; damaged input remains incomplete even
after resynchronization. Only `End.CleanEOF` permits normal decoder finalization.

Check every required `Result.Collection` outcome and `PlaylistTimeline.Complete`
before projecting metadata. Optional collector errors preserve valid reports and
remain available as wrapped errors in collection results. Timeline completeness
describes metadata/clock associations, not extracted HDR10+ frames. Missing or
contradictory STC information and unverified SSIF-to-M2TS packet mappings remain
explicitly unsupported. STC packet ranges must fit the logical M2TS file extent.
Metadata-only discovery assumes 192-byte M2TS framing; a scan that detects bare
188-byte TS invalidates that clock association while still collecting raw ES.
CLPI program association currently requires one complete
program sequence starting at packet zero; later or multiple programs require
packet-aware mapping. Duplicate PID declarations and ambiguous transport program
tables cannot establish complete associations or collection. The source path identifies the actual disc-relative
container; persistent caches need a caller-owned source fingerprint and parser
version as well.

The compiling [`ExampleOptions_VideoConsumer`](pkg/bdinfo/example_collection_test.go)
shows a constant-memory primary-HEVC byte counter and exact occurrence intervals.
BDInfo supplies transport and timeline facts; the caller's reusable HEVC/HDR10+
decoder owns typed frames, metadata inheritance, presentation ordering, and
playlist projection. Full HDR10+ decoder integration is pending that separate
implementation. Nil collection and disabled timeline capture retain the normal
scan without collection buffers, detailed timeline allocations, or new workers.

## Options

- `-o, --reportfilename` (use `-` for stdout)
- `--stdout` (write report to stdout)
- `--main` (only main playlist; likely what you want)
- `-f, --forumsonly` (only forums paste block)
- `-s, --summaryonly` (only quick summary block; likely what you want)
- `-b, --enablessif` (default on; use `--enablessif=false` to disable)
- `-l, --filterloopingplaylists` (default on; use `--filterloopingplaylists=false` to disable)
- `-y, --filtershortplaylist` (default on; use `--filtershortplaylist=false` to disable)
- `-v, --filtershortplaylistvalue` (seconds)
- `-k, --keepstreamorder`
- `-m, --generatetextsummary` (default on; use `--generatetextsummary=false` to disable)
- `-q, --includeversionandnotes` (default on; use `--includeversionandnotes=false` to disable)
- `-j, --groupbytime`
- `-g, --generatestreamdiagnostics`
- `-e, --extendedstreamdiagnostics` (extended HEVC video diagnostics)
- `--progress` (print scan progress to stderr)
- `--self-update` (update to latest release; release builds only)
- `BDINFO_WORKERS` env var overrides scan worker count (default: 2)

## Commands

- `update` (same as `--self-update`)
- `version`

## License

GPL-2.0-or-later. See [LICENSE](LICENSE).

go-bdinfo is a Go port of [BDInfo](https://github.com/UniqProject/BDInfo), which is licensed under LGPL-2.1. Section 3 of that license permits this port to be distributed under the GPL. See [NOTICE](NOTICE) for upstream attribution.
