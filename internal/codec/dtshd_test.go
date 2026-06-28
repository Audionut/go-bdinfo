package codec

import "testing"

func TestDTSSpeakerActivityMaskChannelLayout(t *testing.T) {
	mask := uint16(0x0001 | 0x0002 | 0x0004 | 0x0008 | 0x0020)

	if got := dtsHDSpeakerActivityMaskChannelLayout(mask); got != "C L R Ls Rs LFE Lh Rh" {
		t.Fatalf("dtsHDSpeakerActivityMaskChannelLayout()=%q", got)
	}
}
