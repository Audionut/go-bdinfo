package bdinfo

// Analysis contains structured scan data and supports report rendering.
type Analysis struct {
	Path      string
	Disc      DiscInfo
	Playlists []PlaylistInfo
	Scan      ScanInfo

	raw *analysisRaw
}

// DiscInfo summarizes disc-level metadata.
type DiscInfo struct {
	VolumeLabel string
	DiscTitle   string
	SizeBytes   uint64
	IsBDPlus    bool
	IsBDJava    bool
	IsDBOX      bool
	IsPSP       bool
	Is3D        bool
	Is50Hz      bool
	IsUHD       bool
}

// PlaylistInfo summarizes playlist-level metadata.
type PlaylistInfo struct {
	Name            string
	LengthSeconds   float64
	SizeBytes       uint64
	TotalBitratebps uint64
	FileSizeBytes   uint64
	HasHiddenTracks bool
	HasLoops        bool
	IsValid         bool
	Streams         []StreamInfo
}

// StreamInfo summarizes stream-level metadata.
type StreamInfo struct {
	PID          uint16
	TypeHex      string
	Kind         string
	Codec        string
	CodecAlt     string
	LanguageCode string
	LanguageName string
	BitRatebps   int64
	Description  string
	IsHidden     bool
}

// ScanInfo summarizes non-fatal scan issues.
type ScanInfo struct {
	ScanError  string
	FileErrors map[string]string
}
