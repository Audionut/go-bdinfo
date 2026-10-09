// Copyright (c) 2026, s0up and the autobrr contributors.
// SPDX-License-Identifier: GPL-2.0-or-later

package bdinfo

import (
	"errors"
	"fmt"

	"github.com/autobrr/go-bdinfo/internal/bdrom"
	"github.com/autobrr/go-bdinfo/pkg/bdinfo/video"
)

// PlaylistTimeline retains exact ordered MPLS occurrences. All tick fields use
// 45,000 ticks/second; source PES clocks use 90,000. SelectedAngle is zero in this
// API; each item records its selected alternative and retains other angles.
// Complete means parsed timing and selected STC associations are valid, not that
// video/HDR was extracted. Scanned distinguishes Run from metadata-only discovery
// and indicates a scan attempt; consult Result.Collection for transport outcomes.
// Discovery validates file extents using nominal 192-byte M2TS framing. Run can
// invalidate that association when it observes a bare 188-byte TS container.
// Err explains invalid/unsupported timing. Returned slices are owned snapshots.
type PlaylistTimeline struct {
	Name          string
	SelectedAngle int
	Items         []PlayItem
	Duration45    uint64
	Complete      bool
	Scanned       bool
	Err           error
}

// PlayItem is one occurrence, including repeated uses of the same physical clip.
// Index is zero-based. In45/Out45 preserve unsigned MPLS values and define a
// half-open interval. Offset45 and Duration45 are checked integer accumulations,
// counted once per item. ConnectionCondition preserves the raw join condition;
// it alone does not prove decoder/metadata state continuity. SelectedAlternative
// indexes Angles and currently equals zero. Err covers unsupported join semantics.
type PlayItem struct {
	Index               int
	In45                uint32
	Out45               uint32
	Offset45            uint64
	Duration45          uint64
	ConnectionCondition uint8
	SelectedAlternative int
	Angles              []Angle
	Err                 error
}

// Angle identifies an alternate physical/logical clip and its per-item video
// mappings. Index is zero-based. STCID is the original MPLS clock reference;
// STC is in the logical M2TS packet domain, even when Source is SSIF. An absent
// or contradictory association, including unverified SSIF coordinate mapping,
// sets Err. DifferentAudio and Seamless preserve raw multiangle flags and are
// false for ordinary single-angle items. These flags do not validate an HEVC join.
type Angle struct {
	Index          int
	Source         video.Source
	STCID          uint8
	DifferentAudio bool
	Seamless       bool
	Video          []video.Mapping
	STC            *video.STCSequence
	Err            error
}

func buildTimelines(playlists []*bdrom.PlaylistFile, streams map[string]*bdrom.StreamFile, scanned bool) []PlaylistTimeline {
	out := make([]PlaylistTimeline, 0, len(playlists))
	for _, p := range playlists {
		t := PlaylistTimeline{Name: p.Name, Duration45: p.TimelineDuration45, Scanned: scanned, Err: p.TimelineErr, Complete: p.CaptureTimeline && p.TimelineErr == nil}
		for _, item := range p.TimelineItems {
			entry := PlayItem{Index: item.Index, In45: item.In45, Out45: item.Out45, Offset45: item.Offset45, Duration45: item.Duration45, ConnectionCondition: item.Connection}
			if item.Connection != 1 && item.Connection != 5 && item.Connection != 6 {
				entry.Err = fmt.Errorf("%w: connection condition %d", video.ErrUnsupportedMapping, item.Connection)
			}
			for _, a := range item.Angles {
				angle := Angle{Index: a.Index, Source: a.Source, STCID: a.STCID, DifferentAudio: a.DifferentAudio, Seamless: a.Seamless, Video: append([]video.Mapping(nil), a.Video...), Err: a.Err}
				if sf := streams[a.Source.LogicalClip]; scanned && sf != nil && sf.PacketSize == 188 {
					angle.Err = errors.Join(angle.Err, fmt.Errorf("%w: 188-byte TS and CLPI packet domains require mapping", video.ErrUnsupportedMapping))
				}
				if a.STC != nil {
					copy := *a.STC
					angle.STC = &copy
				}
				entry.Angles = append(entry.Angles, angle)
			}
			if len(entry.Angles) > 0 {
				t.Err = errors.Join(t.Err, entry.Err, entry.Angles[0].Err)
			}
			t.Items = append(t.Items, entry)
		}
		t.Complete = t.Complete && t.Err == nil
		out = append(out, t)
	}
	return out
}
