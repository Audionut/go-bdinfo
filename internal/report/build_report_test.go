package report

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/autobrr/go-bdinfo/internal/bdrom"
	"github.com/autobrr/go-bdinfo/internal/settings"
	"github.com/autobrr/go-bdinfo/internal/stream"
)

func TestBuildReportMatchesWriteReport(t *testing.T) {
	tmpDir := t.TempDir()
	outPath := filepath.Join(tmpDir, "out.bdinfo")
	cfg := settings.Default(tmpDir)
	cfg.GenerateTextSummary = false

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
		Settings:     cfg,
		Streams:      map[uint16]stream.Info{0x1011: video},
		VideoStreams: []*stream.VideoStream{video},
		SortedStreams: []stream.Info{
			video,
		},
	}

	bd := &bdrom.BDROM{
		VolumeLabel: "TEST_DISC",
		DiscTitle:   "TEST_DISC",
		Size:        123456,
	}

	built := BuildReport(bd, []*bdrom.PlaylistFile{playlist}, bdrom.ScanResult{}, cfg)
	if _, err := WriteReport(outPath, bd, []*bdrom.PlaylistFile{playlist}, bdrom.ScanResult{}, cfg); err != nil {
		t.Fatalf("WriteReport() error = %v", err)
	}

	written, err := os.ReadFile(outPath)
	if err != nil {
		t.Fatalf("read report: %v", err)
	}
	if string(written) != built {
		t.Fatalf("BuildReport output mismatch with WriteReport output")
	}
}
