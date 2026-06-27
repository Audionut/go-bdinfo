package codec

import (
	"github.com/autobrr/go-bdinfo/internal/buffer"
	"github.com/autobrr/go-bdinfo/internal/stream"
)

var ac3BitrateKbps = []int{32, 40, 48, 56, 64, 80, 96, 112, 128, 160, 192, 224, 256, 320, 384, 448, 512, 576, 640}
var ac3Channels = []int{2, 1, 2, 3, 3, 4, 4, 5}
var ac3FrameSize44K = []int{138, 174, 208, 242, 278, 348, 416, 486, 556, 696, 834, 974, 1114, 1392, 1670, 1950, 2228, 2506, 2786}

func ac3ChanMap(chanMap uint16) int {
	channels := 0
	for i := range 16 {
		if (chanMap & uint16(1<<uint(15-i))) != 0 {
			switch i {
			case 5, 6, 9, 10, 11:
				channels += 2
			}
		}
	}
	return channels
}

// ScanAC3 updates audio metadata from the first usable AC-3 or E-AC-3 frames in data.
//
// For E-AC-3, the stream can require both an independent core frame and a later
// dependent frame to expose Atmos/JOC metadata, expanded channel count, and embedded
// AC3 core details. The scan stops once the stream reaches initialized state.
func ScanAC3(a *stream.AudioStream, data []byte) {
	if a.IsInitialized {
		return
	}
	for offset := findAC3Sync(data); offset >= 0 && offset+7 <= len(data); {
		frameSize, ok := scanAC3Frame(a, data[offset:])
		if ok && a.IsInitialized {
			return
		}

		next := offset + 2
		if ok && frameSize > 0 {
			next = offset + frameSize
		}
		if next <= offset || next >= len(data) {
			return
		}
		rel := findAC3Sync(data[next:])
		if rel < 0 {
			return
		}
		offset = next + rel
	}
}

// scanAC3Frame parses one sync-aligned AC-3 or E-AC-3 frame and returns its byte size.
// It mutates a with any metadata found in the frame; ok is false when the frame header
// is absent or unsupported.
func scanAC3Frame(a *stream.AudioStream, data []byte) (int, bool) {
	if len(data) < 7 {
		return 0, false
	}
	if data[0] != 0x0b || data[1] != 0x77 {
		return 0, false
	}
	frameSizeBytes, ok := ac3FrameSize(data)
	if !ok {
		return 0, false
	}

	secondFrame := a.ChannelCount > 0
	bsidPeek := (data[5] & 0xF8) >> 3

	br := buffer.NewBitReader(data)
	read := func(bits int) uint64 {
		v, _ := br.ReadBits(bits)
		return v
	}
	readBool := func() bool {
		v, _ := br.ReadBit()
		return v == 1
	}

	var (
		srCode        uint64
		frameSize     uint64
		frameSizeCode uint64
		channelMode   uint64
		lfeOn         uint64
		dialNorm      uint64
		dialNormExt   uint64
		numBlocks     uint64
		bsid          uint64
	)

	_ = read(16) // sync
	if bsidPeek <= 10 {
		_ = read(16) // crc1
		srCode = read(2)
		frameSizeCode = read(6)
		bsid = read(5)
		_ = read(3) // bsmod
		channelMode = read(3)
		if (channelMode&0x1) > 0 && channelMode != 0x1 {
			_ = read(2)
		}
		if (channelMode & 0x4) > 0 {
			_ = read(2)
		}
		if channelMode == 0x2 {
			dsurmod := read(2)
			if dsurmod == 0x2 {
				a.AudioMode = stream.AudioModeSurround
			}
		}
		lfeOn = read(1)
		dialNorm = read(5)
		if readBool() {
			_ = read(8)
		}
		if readBool() {
			_ = read(8)
		}
		if readBool() {
			_ = read(7)
		}
		if channelMode == 0 {
			_ = read(5)
			if readBool() {
				_ = read(8)
			}
			if readBool() {
				_ = read(8)
			}
			if readBool() {
				_ = read(7)
			}
		}
		_ = read(2)
		if bsid == 6 {
			if readBool() {
				_ = read(14)
			}
			if readBool() {
				dsurexmod := read(2)
				_ = read(2) // dheadphonmod
				_ = read(10)
				if dsurexmod == 2 {
					a.AudioMode = stream.AudioModeExtended
				}
			}
		}
	} else {
		frameType := read(2)
		_ = read(3) // substreamid
		frameSize = (read(11) + 1) << 1
		srCode = read(2)
		if srCode == 3 {
			srCode = read(2)
			numBlocks = 3
		} else {
			numBlocks = read(2)
		}
		channelMode = read(3)
		lfeOn = read(1)
		bsid = read(5)
		dialNormExt = read(5)

		if readBool() {
			_ = read(8)
		}
		if channelMode == 0 {
			_ = read(5)
			if readBool() {
				_ = read(8)
			}
		}
		if frameType == 1 {
			a.CoreStream = a.Clone().(*stream.AudioStream)
			a.CoreStream.StreamType = stream.StreamTypeAC3Audio
			if readBool() {
				chanmap := read(16)
				a.ChannelCount = a.CoreStream.ChannelCount
				a.ChannelCount += ac3ChanMap(uint16(chanmap))
				lfeOn = uint64(a.CoreStream.LFE)
			}
		}

		if emdfBitPos, ok := findEmdfSync(data, br.BitPosition()); ok {
			br.SetBitPosition(emdfBitPos + 16)
			emdfContainerSize := read(16)
			remainAfterEmdf := br.BitsRemaining() - int(emdfContainerSize)*8
			emdfVersion := read(2)
			if emdfVersion == 3 {
				emdfVersion += read(2)
			}
			if emdfVersion > 0 {
				skip := br.BitsRemaining() - remainAfterEmdf
				if skip > 0 {
					br.SkipBits(skip)
				}
			} else {
				temp := read(3)
				if temp == 0x7 {
					_ = read(2)
				}
				emdfPayloadID := read(5)
				if emdfPayloadID > 0 && emdfPayloadID < 16 {
					if emdfPayloadID == 0x1F {
						_ = read(5)
					}
					emdfPayloadConfig(br)
					emdfPayloadSize := read(8) * 8
					_ = br.SkipBits(int(emdfPayloadSize + 1))
				}

				for emdfPayloadID != 14 && br.BitPosition() < br.Length()*8 {
					emdfPayloadID = read(5)
					if emdfPayloadID == 0x1F {
						_ = read(5)
					}
					emdfPayloadConfig(br)
					emdfPayloadSize := read(8) * 8
					_ = br.SkipBits(int(emdfPayloadSize + 1))
				}

				if br.BitPosition() < br.Length()*8 && emdfPayloadID == 14 {
					emdfPayloadConfig(br)
					_ = read(12)
					jocNumObjectsBits := read(6)
					if jocNumObjectsBits > 0 {
						a.HasExtensions = true
					}
				}
			}
		}
	}

	if channelMode < uint64(len(ac3Channels)) && a.ChannelCount == 0 {
		a.ChannelCount = ac3Channels[int(channelMode)]
	}
	if a.AudioMode == stream.AudioModeUnknown {
		switch channelMode {
		case 0:
			a.AudioMode = stream.AudioModeDualMono
		case 2:
			a.AudioMode = stream.AudioModeStereo
		}
	}

	switch srCode {
	case 0:
		a.SampleRate = 48000
	case 1:
		a.SampleRate = 44100
	case 2:
		a.SampleRate = 32000
	default:
		a.SampleRate = 0
	}

	if bsid <= 10 {
		fSize := frameSizeCode >> 1
		if int(fSize) < len(ac3BitrateKbps) {
			a.BitRate = int64(ac3BitrateKbps[fSize] * 1000)
		}
	} else if a.SampleRate > 0 && numBlocks > 0 {
		a.BitRate = int64(4.0 * float64(frameSize) * float64(a.SampleRate) / (float64(numBlocks) * 256))
		if a.CoreStream != nil {
			a.BitRate += a.CoreStream.BitRate
		}
	}

	a.LFE = int(lfeOn)
	if a.StreamType != stream.StreamTypeAC3PlusSecondaryAudio {
		switch {
		case a.StreamType == stream.StreamTypeAC3PlusAudio && bsid == 6:
			a.DialNorm = -int(dialNorm)
		case a.StreamType == stream.StreamTypeAC3Audio:
			a.DialNorm = -int(dialNorm)
		case a.StreamType == stream.StreamTypeAC3PlusAudio && secondFrame:
			a.DialNorm = -int(dialNormExt)
		}
	}

	a.IsVBR = false
	if a.StreamType == stream.StreamTypeAC3PlusAudio && bsid == 6 && !secondFrame {
		a.IsInitialized = false
	} else {
		a.IsInitialized = true
	}
	return frameSizeBytes, true
}

// ac3FrameSize returns the byte length of a sync-aligned AC-3 or E-AC-3 frame.
func ac3FrameSize(data []byte) (int, bool) {
	if len(data) < 6 || data[0] != 0x0b || data[1] != 0x77 {
		return 0, false
	}
	bsid := (data[5] & 0xF8) >> 3
	if bsid <= 10 {
		srCode := (data[4] & 0xC0) >> 6
		frameSizeCode := data[4] & 0x3F
		if srCode == 3 || int(frameSizeCode) >= len(ac3FrameSize44K)*2 {
			return 0, false
		}
		bitrateIndex := int(frameSizeCode >> 1)
		switch srCode {
		case 0:
			return ac3BitrateKbps[bitrateIndex] * 4, true
		case 1:
			size := ac3FrameSize44K[bitrateIndex]
			if frameSizeCode&1 != 0 {
				size += 2
			}
			return size, true
		case 2:
			return ac3BitrateKbps[bitrateIndex] * 6, true
		default:
			return 0, false
		}
	}
	if bsid > 16 {
		return 0, false
	}
	frameSize := ((int(data[2]&0x07) << 8) | int(data[3])) + 1
	return frameSize << 1, true
}

func findEmdfSync(data []byte, startBit int) (int, bool) {
	totalBits := len(data) * 8
	for bitPos := startBit; bitPos+16 <= totalBits; bitPos++ {
		var val uint16
		for i := range 16 {
			bytePos := (bitPos + i) / 8
			bitOffset := 7 - ((bitPos + i) % 8)
			if bytePos >= len(data) {
				return 0, false
			}
			val = (val << 1) | uint16((data[bytePos]>>bitOffset)&0x01)
		}
		if val == 0x5838 {
			return bitPos, true
		}
	}
	return 0, false
}

func emdfPayloadConfig(br *buffer.BitReader) {
	readBool := func() bool {
		v, _ := br.ReadBit()
		return v == 1
	}
	sampleOffsetE := readBool()
	if sampleOffsetE {
		_ = br.SkipBits(12)
	}
	if readBool() { // duratione
		_ = br.SkipBits(11)
	}
	if readBool() { // groupide
		_ = br.SkipBits(2)
	}
	if readBool() {
		_ = br.SkipBits(8)
	}
	if readBool() { // discard_unknown_payload
		return
	}
	_ = br.SkipBits(1)
	if sampleOffsetE {
		return
	}
	if readBool() { // payload_frame_aligned
		_ = br.SkipBits(9)
	}
}
