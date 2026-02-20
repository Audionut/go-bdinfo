package main

import (
	"testing"

	"github.com/autobrr/go-bdinfo/internal/settings"
)

func TestNormalizeArgs_BoolValueTokens(t *testing.T) {
	in := []string{"-m", "true", "-q", "false", "--enablessif", "TRUE", "--summaryonly", "false"}
	got := normalizeArgs(in)
	want := []string{
		"--generatetextsummary=true",
		"--includeversionandnotes=false",
		"--enablessif=true",
		"--summaryonly=false",
	}
	if len(got) != len(want) {
		t.Fatalf("len=%d want=%d; got=%q", len(got), len(want), got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("idx=%d got=%q want=%q (all=%q)", i, got[i], want[i], got)
		}
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

func TestToLibrarySettings_PlaylistOnly(t *testing.T) {
	in := settings.Settings{PlaylistOnly: "00001.MPLS"}
	out := toLibrarySettings(in)
	if out.PlaylistOnly != "00001.MPLS" {
		t.Fatalf("PlaylistOnly=%q want=%q", out.PlaylistOnly, "00001.MPLS")
	}
}
