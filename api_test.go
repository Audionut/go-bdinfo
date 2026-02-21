package bdinfo

import (
	"context"
	"strings"
	"testing"

	"github.com/autobrr/go-bdinfo/internal/bdrom"
	"github.com/autobrr/go-bdinfo/internal/stream"
)

func TestDefaultOptions(t *testing.T) {
	opts := DefaultOptions()
	if !opts.GenerateStreamDiagnostics {
		t.Fatalf("expected GenerateStreamDiagnostics default true")
	}
	if !opts.EnableSSIF {
		t.Fatalf("expected EnableSSIF default true")
	}
	if opts.FilterShortPlaylistsVal != 20 {
		t.Fatalf("expected FilterShortPlaylistsVal=20, got %d", opts.FilterShortPlaylistsVal)
	}
}

func TestRenderReportWithSyntheticAnalysis(t *testing.T) {
	opts := DefaultOptions()

	video := &stream.VideoStream{
		Stream: stream.Stream{
			PID:        0x1011,
			StreamType: stream.StreamTypeAVCVideo,
			BitRate:    8_000_000,
		},
		Height:        1080,
		FrameRateEnum: 24000,
		FrameRateDen:  1001,
		AspectRatio:   stream.Aspect169,
	}

	playlist := &bdrom.PlaylistFile{
		Name:         "00001.MPLS",
		Settings:     opts.toSettings("."),
		Streams:      map[uint16]stream.Info{0x1011: video},
		VideoStreams: []*stream.VideoStream{video},
		SortedStreams: []stream.Info{
			video,
		},
	}

	rom := &bdrom.BDROM{
		VolumeLabel: "TEST_DISC",
		DiscTitle:   "TEST_DISC",
		Size:        123456,
	}

	analysis := &Analysis{
		Path: "fake-disc",
		Disc: DiscInfo{
			VolumeLabel: rom.VolumeLabel,
			DiscTitle:   rom.DiscTitle,
			SizeBytes:   rom.Size,
		},
		raw: &analysisRaw{
			rom:       rom,
			playlists: []*bdrom.PlaylistFile{playlist},
			scan:      bdrom.ScanResult{},
			settings:  opts,
		},
	}

	reportText, err := RenderReport(context.Background(), analysis, nil)
	if err != nil {
		t.Fatalf("RenderReport() error = %v", err)
	}
	if !strings.Contains(reportText, "PLAYLIST: 00001.MPLS") {
		t.Fatalf("expected report to include playlist header")
	}
}

func TestRenderReportNilAnalysis(t *testing.T) {
	_, err := RenderReport(context.Background(), nil, nil)
	if err == nil {
		t.Fatalf("expected error for nil analysis")
	}
}

func TestNormalizePlaylistName(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{name: "empty", input: "", want: ""},
		{name: "no extension", input: "00000", want: "00000.MPLS"},
		{name: "with extension", input: "00000.mpls", want: "00000.MPLS"},
		{name: "path like", input: "PLAYLIST/00000.mpls", want: "00000.MPLS"},
		{name: "whitespace", input: "  00001.mpls  ", want: "00001.MPLS"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := normalizePlaylistName(tt.input)
			if got != tt.want {
				t.Fatalf("normalizePlaylistName(%q)=%q want=%q", tt.input, got, tt.want)
			}
		})
	}
}

func TestFilterROMToPlaylist(t *testing.T) {
	rom := &bdrom.BDROM{
		PlaylistFiles: map[string]*bdrom.PlaylistFile{
			"00000.MPLS": {Name: "00000.MPLS"},
			"00001.MPLS": {Name: "00001.MPLS"},
		},
		PlaylistOrder: []string{"00000.MPLS", "00001.MPLS"},
	}

	if err := filterROMToPlaylist(rom, "playlist/00001.mpls"); err != nil {
		t.Fatalf("filterROMToPlaylist() error = %v", err)
	}

	if len(rom.PlaylistFiles) != 1 {
		t.Fatalf("playlist files len = %d want = 1", len(rom.PlaylistFiles))
	}
	if _, ok := rom.PlaylistFiles["00001.MPLS"]; !ok {
		t.Fatalf("filtered playlist 00001.MPLS not found")
	}
	if len(rom.PlaylistOrder) != 1 || rom.PlaylistOrder[0] != "00001.MPLS" {
		t.Fatalf("playlist order = %q want = [00001.MPLS]", strings.Join(rom.PlaylistOrder, ","))
	}
}

func TestFilterROMToPlaylistMissing(t *testing.T) {
	rom := &bdrom.BDROM{
		PlaylistFiles: map[string]*bdrom.PlaylistFile{
			"00000.MPLS": {Name: "00000.MPLS"},
		},
		PlaylistOrder: []string{"00000.MPLS"},
	}

	err := filterROMToPlaylist(rom, "00077")
	if err == nil {
		t.Fatal("expected error for missing playlist")
	}
	if !strings.Contains(err.Error(), "playlist not found") {
		t.Fatalf("unexpected error: %v", err)
	}
}
