package bdinfo

import (
	"context"
	"errors"
	"fmt"
	"os"
	"sort"
	"strings"

	"github.com/autobrr/go-bdinfo/internal/bdrom"
	"github.com/autobrr/go-bdinfo/internal/report"
	"github.com/autobrr/go-bdinfo/internal/stream"
)

type analysisRaw struct {
	rom       *bdrom.BDROM
	playlists []*bdrom.PlaylistFile
	scan      bdrom.ScanResult
	settings  Options
}

// Analyze scans a single Blu-ray disc path (folder root or ISO) and returns structured analysis data.
func Analyze(ctx context.Context, path string, opts *Options) (*Analysis, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if strings.TrimSpace(path) == "" {
		return nil, errors.New("path is required")
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	cfg := DefaultOptions()
	if opts != nil {
		cfg = *opts
	}

	cwd, err := os.Getwd()
	if err != nil {
		cwd = "."
	}
	scanSettings := cfg.toSettings(cwd)

	rom, err := bdrom.New(path, scanSettings)
	if err != nil {
		return nil, err
	}
	defer rom.Close()

	scanResult := rom.Scan()
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	playlists := orderedPlaylists(rom)
	analysis := buildAnalysis(path, rom, playlists, scanResult, cfg)
	return analysis, nil
}

// RenderReport builds report text from analysis data using the provided options.
func RenderReport(ctx context.Context, analysis *Analysis, opts *Options) (string, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return "", err
	}
	if analysis == nil || analysis.raw == nil {
		return "", errors.New("analysis is nil")
	}

	cfg := analysis.raw.settings
	if opts != nil {
		cfg = *opts
	}

	cwd, err := os.Getwd()
	if err != nil {
		cwd = "."
	}
	reportSettings := cfg.toSettings(cwd)
	text := report.BuildReport(analysis.raw.rom, analysis.raw.playlists, analysis.raw.scan, reportSettings)
	if err := ctx.Err(); err != nil {
		return "", err
	}
	return text, nil
}

// WriteReport writes a report from analysis data to the configured destination.
func WriteReport(ctx context.Context, path string, analysis *Analysis, opts *Options) (string, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return "", err
	}
	if analysis == nil || analysis.raw == nil {
		return "", errors.New("analysis is nil")
	}

	cfg := analysis.raw.settings
	if opts != nil {
		cfg = *opts
	}

	cwd, err := os.Getwd()
	if err != nil {
		cwd = "."
	}
	reportSettings := cfg.toSettings(cwd)
	writtenPath, err := report.WriteReport(path, analysis.raw.rom, analysis.raw.playlists, analysis.raw.scan, reportSettings)
	if err != nil {
		return "", err
	}
	return writtenPath, ctx.Err()
}

// AnalyzeAndRender is a convenience function that scans and builds report text in one call.
func AnalyzeAndRender(ctx context.Context, path string, opts *Options) (*Analysis, string, error) {
	analysis, err := Analyze(ctx, path, opts)
	if err != nil {
		return nil, "", err
	}
	text, err := RenderReport(ctx, analysis, opts)
	if err != nil {
		return nil, "", err
	}
	return analysis, text, nil
}

func orderedPlaylists(rom *bdrom.BDROM) []*bdrom.PlaylistFile {
	playlists := make([]*bdrom.PlaylistFile, 0, len(rom.PlaylistFiles))
	if len(rom.PlaylistOrder) > 0 {
		for _, name := range rom.PlaylistOrder {
			if pl, ok := rom.PlaylistFiles[name]; ok {
				playlists = append(playlists, pl)
			}
		}
		return playlists
	}

	for _, pl := range rom.PlaylistFiles {
		playlists = append(playlists, pl)
	}
	sort.Slice(playlists, func(i, j int) bool {
		return playlists[i].Name < playlists[j].Name
	})
	return playlists
}

func buildAnalysis(path string, rom *bdrom.BDROM, playlists []*bdrom.PlaylistFile, scanResult bdrom.ScanResult, cfg Options) *Analysis {
	analysis := &Analysis{
		Path: path,
		Disc: DiscInfo{
			VolumeLabel: rom.VolumeLabel,
			DiscTitle:   rom.DiscTitle,
			SizeBytes:   rom.Size,
			IsBDPlus:    rom.IsBDPlus,
			IsBDJava:    rom.IsBDJava,
			IsDBOX:      rom.IsDBOX,
			IsPSP:       rom.IsPSP,
			Is3D:        rom.Is3D,
			Is50Hz:      rom.Is50Hz,
			IsUHD:       rom.IsUHD,
		},
		Playlists: make([]PlaylistInfo, 0, len(playlists)),
		Scan: ScanInfo{
			FileErrors: map[string]string{},
		},
		raw: &analysisRaw{
			rom:       rom,
			playlists: playlists,
			scan:      scanResult,
			settings:  cfg,
		},
	}

	if scanResult.ScanError != nil {
		analysis.Scan.ScanError = scanResult.ScanError.Error()
	}
	for name, err := range scanResult.FileErrors {
		analysis.Scan.FileErrors[name] = err.Error()
	}

	for _, pl := range playlists {
		if pl == nil {
			continue
		}
		playlistInfo := PlaylistInfo{
			Name:            pl.Name,
			LengthSeconds:   pl.TotalLength(),
			SizeBytes:       pl.TotalSize(),
			TotalBitratebps: pl.TotalBitRate(),
			FileSizeBytes:   pl.FileSize(),
			HasHiddenTracks: pl.HasHiddenTracks,
			HasLoops:        pl.HasLoops,
			IsValid:         pl.IsValid(),
			Streams:         make([]StreamInfo, 0, len(pl.SortedStreams)),
		}
		for _, st := range pl.SortedStreams {
			if st == nil {
				continue
			}
			base := st.Base()
			playlistInfo.Streams = append(playlistInfo.Streams, StreamInfo{
				PID:          base.PID,
				TypeHex:      fmt.Sprintf("0x%02X", uint8(base.StreamType)),
				Kind:         streamKind(base),
				Codec:        stream.CodecNameForInfo(st),
				CodecAlt:     stream.CodecAltNameForInfo(st),
				LanguageCode: base.LanguageCode(),
				LanguageName: base.LanguageName,
				BitRatebps:   base.BitRate,
				Description:  st.Description(),
				IsHidden:     base.IsHidden,
			})
		}
		analysis.Playlists = append(analysis.Playlists, playlistInfo)
	}

	return analysis
}

func streamKind(base *stream.Stream) string {
	switch {
	case base.IsVideoStream():
		return "video"
	case base.IsAudioStream():
		return "audio"
	case base.IsGraphicsStream():
		return "graphics"
	case base.IsTextStream():
		return "text"
	default:
		return "unknown"
	}
}
