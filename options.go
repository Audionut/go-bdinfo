package bdinfo

import (
	"os"
	"path/filepath"

	"github.com/autobrr/go-bdinfo/internal/settings"
)

// Options controls scanning and report rendering behavior.
type Options struct {
	GenerateStreamDiagnostics bool
	ExtendedStreamDiagnostics bool
	EnableSSIF                bool
	BigPlaylistOnly           bool
	FilterLoopingPlaylists    bool
	FilterShortPlaylists      bool
	FilterShortPlaylistsVal   int
	KeepStreamOrder           bool
	GenerateTextSummary       bool
	ReportFileName            string
	IncludeVersionAndNotes    bool
	GroupByTime               bool
	ForumsOnly                bool
	MainPlaylistOnly          bool
	SummaryOnly               bool
}

// DefaultOptions returns library defaults aligned with the CLI defaults.
func DefaultOptions() Options {
	cwd, err := os.Getwd()
	if err != nil {
		cwd = "."
	}
	return optionsFromSettings(settings.Default(cwd))
}

func optionsFromSettings(s settings.Settings) Options {
	return Options{
		GenerateStreamDiagnostics: s.GenerateStreamDiagnostics,
		ExtendedStreamDiagnostics: s.ExtendedStreamDiagnostics,
		EnableSSIF:                s.EnableSSIF,
		BigPlaylistOnly:           s.BigPlaylistOnly,
		FilterLoopingPlaylists:    s.FilterLoopingPlaylists,
		FilterShortPlaylists:      s.FilterShortPlaylists,
		FilterShortPlaylistsVal:   s.FilterShortPlaylistsVal,
		KeepStreamOrder:           s.KeepStreamOrder,
		GenerateTextSummary:       s.GenerateTextSummary,
		ReportFileName:            s.ReportFileName,
		IncludeVersionAndNotes:    s.IncludeVersionAndNotes,
		GroupByTime:               s.GroupByTime,
		ForumsOnly:                s.ForumsOnly,
		MainPlaylistOnly:          s.MainPlaylistOnly,
		SummaryOnly:               s.SummaryOnly,
	}
}

func (o Options) toSettings(reportBaseDir string) settings.Settings {
	if o.FilterShortPlaylistsVal == 0 {
		o.FilterShortPlaylistsVal = 20
	}
	if o.ReportFileName == "" {
		o.ReportFileName = filepath.Join(reportBaseDir, "BDInfo_{0}.bdinfo")
	}
	reportFileName := o.ReportFileName
	if !filepath.IsAbs(reportFileName) && reportFileName != "-" {
		reportFileName = filepath.Clean(reportFileName)
	}
	return settings.Settings{
		GenerateStreamDiagnostics: o.GenerateStreamDiagnostics,
		ExtendedStreamDiagnostics: o.ExtendedStreamDiagnostics,
		EnableSSIF:                o.EnableSSIF,
		BigPlaylistOnly:           o.BigPlaylistOnly,
		FilterLoopingPlaylists:    o.FilterLoopingPlaylists,
		FilterShortPlaylists:      o.FilterShortPlaylists,
		FilterShortPlaylistsVal:   o.FilterShortPlaylistsVal,
		KeepStreamOrder:           o.KeepStreamOrder,
		GenerateTextSummary:       o.GenerateTextSummary,
		ReportFileName:            reportFileName,
		IncludeVersionAndNotes:    o.IncludeVersionAndNotes,
		GroupByTime:               o.GroupByTime,
		ForumsOnly:                o.ForumsOnly,
		MainPlaylistOnly:          o.MainPlaylistOnly,
		SummaryOnly:               o.SummaryOnly,
	}
}
