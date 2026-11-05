// Copyright (c) 2026, s0up and the autobrr contributors.
// SPDX-License-Identifier: GPL-2.0-or-later

package bdinfo_test

import (
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/autobrr/go-bdinfo/pkg/bdinfo"
)

type discoveryFS struct {
	source fs.FS
	open   func(string, fs.File) (fs.File, error)
	lookup func(string) error
}

// Virtual links exercise the FS contract without Windows symlink privileges.
type linkDiscoveryFS struct {
	fs.FS
	name, target string
}
type linkEntry struct{ name string }

func (e linkEntry) Name() string      { return e.name }
func (e linkEntry) IsDir() bool       { return false }
func (e linkEntry) Type() fs.FileMode { return fs.ModeSymlink }
func (e linkEntry) Info() (fs.FileInfo, error) {
	return nil, errors.New("link Info used instead of resolved stat")
}

type namedFileInfo struct {
	fs.FileInfo
	name string
}

func (f namedFileInfo) Name() string { return f.name }
func (f linkDiscoveryFS) Open(name string) (fs.File, error) {
	if name == f.name {
		name = f.target
	}
	return f.FS.Open(name)
}
func (f linkDiscoveryFS) Stat(name string) (fs.FileInfo, error) {
	if name != f.name {
		return fs.Stat(f.FS, name)
	}
	info, err := fs.Stat(f.FS, f.target)
	if err != nil {
		return nil, err
	}
	return namedFileInfo{info, path.Base(name)}, nil
}
func (f linkDiscoveryFS) ReadDir(name string) ([]fs.DirEntry, error) {
	entries, err := fs.ReadDir(f.FS, name)
	if err != nil {
		return nil, err
	}
	if name == path.Dir(f.name) {
		entries = append(entries, linkEntry{path.Base(f.name)})
		slices.SortFunc(entries, func(a, b fs.DirEntry) int { return strings.Compare(a.Name(), b.Name()) })
	}
	return entries, nil
}

func TestDiscoverPlaylistsFSLinks(t *testing.T) {
	root := t.TempDir()
	writeDiscFixture(t, root)
	filesystem := linkDiscoveryFS{FS: os.DirFS(root), name: "BDMV/PLAYLIST/00002.mpls", target: "BDMV/PLAYLIST/00000.mpls"}
	result, err := bdinfo.DiscoverPlaylistsFS(t.Context(), filesystem, bdinfo.Options{IncludeTimeline: true})
	if err != nil || len(result.Playlists) != 3 || len(result.Timelines) != 3 || result.Timelines[2].Name != "00002.MPLS" || !result.Timelines[2].Complete {
		t.Fatalf("valid link lost: %+v %v", result, err)
	}
	filesystem.name = "BDMV/PLAYLIST/unrelated.link"
	filesystem.target = "BDMV/PLAYLIST/missing"
	result, err = bdinfo.DiscoverPlaylistsFS(t.Context(), filesystem, bdinfo.Options{IncludeTimeline: true})
	if !errors.Is(err, fs.ErrNotExist) || !reflect.DeepEqual(result, bdinfo.Result{}) {
		t.Fatalf("dangling link silently discarded playlists: %+v %v", result, err)
	}
}

type canceledOpenFile struct {
	fs.File
	next, closed *int
}

func (f canceledOpenFile) Stat() (fs.FileInfo, error) { *f.next++; return f.File.Stat() }
func (f canceledOpenFile) ReadDir(n int) ([]fs.DirEntry, error) {
	*f.next++
	return f.File.(fs.ReadDirFile).ReadDir(n)
}
func (f canceledOpenFile) Close() error { *f.closed++; return f.File.Close() }
func TestDiscoverPlaylistsFSStopsAfterOpenCancellation(t *testing.T) {
	root := t.TempDir()
	writeDiscFixture(t, root)
	for _, statFallback := range []bool{false, true} {
		t.Run(fmt.Sprint("stat=", statFallback), func(t *testing.T) {
			ctx, cancel := context.WithCancel(t.Context())
			defer cancel()
			next, closed := 0, 0
			open := func(_ string, file fs.File) (fs.File, error) {
				cancel()
				return canceledOpenFile{file, &next, &closed}, nil
			}
			var filesystem fs.FS = discoveryFS{source: os.DirFS(root), open: open}
			if statFallback {
				filesystem = openDiscoveryFS{source: os.DirFS(root), open: open}
			}
			result, err := bdinfo.DiscoverPlaylistsFS(ctx, filesystem, bdinfo.Options{})
			if !errors.Is(err, context.Canceled) || !reflect.DeepEqual(result, bdinfo.Result{}) || next != 0 || closed != 1 {
				t.Fatalf("continued after cancellation: next%d closed%d error%v", next, closed, err)
			}
		})
	}
}

type optionalErrorFS struct {
	fs.FS
	failure   error
	directory bool
}

func (f optionalErrorFS) Stat(name string) (fs.FileInfo, error) {
	if !f.directory && name == "BDMV/META" {
		return nil, f.failure
	}
	return fs.Stat(f.FS, name)
}
func (f optionalErrorFS) ReadDir(name string) ([]fs.DirEntry, error) {
	if f.directory && name == "BDMV/BDJO" {
		return nil, f.failure
	}
	return fs.ReadDir(f.FS, name)
}
func TestDiscoverPlaylistsFSMixedAbsence(t *testing.T) {
	root := t.TempDir()
	writeDiscFixture(t, root)
	if err := os.Mkdir(filepath.Join(root, "BDMV", "BDJO"), 0755); err != nil {
		t.Fatal(err)
	}
	for _, directory := range []bool{false, true} {
		for _, mixed := range []bool{false, true} {
			t.Run(fmt.Sprint("directory=", directory, "/mixed=", mixed), func(t *testing.T) {
				sentinel := errors.New("optional cleanup failed")
				failure := fmt.Errorf("wrapped absence: %w", fs.ErrNotExist)
				if mixed {
					failure = errors.Join(failure, sentinel)
				}
				result, err := bdinfo.DiscoverPlaylistsFS(t.Context(), optionalErrorFS{os.DirFS(root), failure, directory}, bdinfo.Options{IncludeTimeline: true})
				if mixed {
					if !errors.Is(err, sentinel) || !errors.Is(err, fs.ErrNotExist) || !reflect.DeepEqual(result, bdinfo.Result{}) {
						t.Fatalf("mixed error lost: %+v %v", result, err)
					}
				} else if err != nil || len(result.Playlists) != 2 {
					t.Fatalf("ordinary absence failed: %+v %v", result, err)
				}
			})
		}
	}
}

func (f discoveryFS) Open(name string) (fs.File, error) {
	file, err := f.source.Open(name)
	if err != nil {
		return nil, err
	}
	if f.open != nil {
		return f.open(name, file)
	}
	return file, nil
}

// Optimized stat allows tests to forbid video Open while retaining actual sizes.
func (f discoveryFS) Stat(name string) (fs.FileInfo, error) {
	if f.lookup != nil {
		if err := f.lookup(name); err != nil {
			return nil, err
		}
	}
	return fs.Stat(f.source, name)
}

type openDiscoveryFS struct {
	source fs.FS
	open   func(string, fs.File) (fs.File, error)
}

func (f openDiscoveryFS) Open(name string) (fs.File, error) {
	file, err := f.source.Open(name)
	if err != nil {
		return nil, err
	}
	if f.open != nil {
		return f.open(name, file)
	}
	return file, nil
}

type discoveryFile struct {
	fs.File
	readError, closeError     error
	statError, directoryError error
	dataEOF                   bool
	reverseDir                bool
	cancel                    context.CancelFunc
}

func (f *discoveryFile) Read(data []byte) (int, error) {
	if f.cancel != nil {
		f.cancel()
	}
	if f.readError != nil {
		if len(data) > 0 {
			data[0] = 0
			return 1, f.readError
		}
		return 0, f.readError
	}
	n, err := f.File.Read(data)
	if f.dataEOF && n > 0 {
		return n, io.EOF
	}
	return n, err
}
func (f *discoveryFile) Close() error { return errors.Join(f.File.Close(), f.closeError) }
func (f *discoveryFile) Stat() (fs.FileInfo, error) {
	if f.statError != nil {
		return nil, f.statError
	}
	return f.File.Stat()
}
func (f *discoveryFile) ReadDir(n int) ([]fs.DirEntry, error) {
	if f.cancel != nil {
		f.cancel()
	}
	if f.directoryError != nil {
		return nil, f.directoryError
	}
	entries, err := f.File.(fs.ReadDirFile).ReadDir(n)
	if f.reverseDir {
		slices.Reverse(entries)
	}
	return entries, err
}

func TestDiscoverPlaylistsFSPrimaryAndCloseFailures(t *testing.T) {
	root := t.TempDir()
	writeDiscFixture(t, root)
	for _, operation := range []string{"stat", "directory", "read", "cancel"} {
		t.Run(operation, func(t *testing.T) {
			ctx, cancel := context.WithCancel(t.Context())
			defer cancel()
			primary := errors.New("primary " + operation)
			cleanup := errors.New("cleanup " + operation)
			target := "."
			if operation == "read" {
				target = "BDMV/CLIPINF/00001.clpi"
			}
			wrap := func(name string, file fs.File) (fs.File, error) {
				if name != target {
					return file, nil
				}
				wrapped := &discoveryFile{File: file, closeError: cleanup}
				switch operation {
				case "stat":
					wrapped.statError = primary
				case "directory":
					wrapped.directoryError = primary
				case "read":
					wrapped.readError = primary
				case "cancel":
					cancel()
				}
				return wrapped, nil
			}
			var filesystem fs.FS = discoveryFS{source: os.DirFS(root), open: wrap}
			if operation == "stat" || operation == "cancel" {
				filesystem = openDiscoveryFS{source: os.DirFS(root), open: wrap}
			}
			result, err := bdinfo.DiscoverPlaylistsFS(ctx, filesystem, bdinfo.Options{IncludeTimeline: true})
			if operation == "cancel" {
				primary = context.Canceled
			}
			if !errors.Is(err, primary) || !errors.Is(err, cleanup) || !reflect.DeepEqual(result, bdinfo.Result{}) {
				t.Fatalf("lost primary/cleanup: %+v %v", result, err)
			}
		})
	}
}

func TestDiscoverPlaylistsFSParity(t *testing.T) {
	root := t.TempDir()
	writeDiscFixture(t, root)
	opts := bdinfo.Options{Path: root, Settings: bdinfo.DefaultSettings(root), IncludeTimeline: true}
	want, err := bdinfo.DiscoverPlaylists(t.Context(), opts)
	if err != nil {
		t.Fatal(err)
	}
	contained, err := os.OpenRoot(root)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { contained.Close() })
	for _, logicalRoot := range []string{"", "BDMV"} {
		t.Run("root="+logicalRoot, func(t *testing.T) {
			filesystem := discoveryFS{source: contained.FS(), open: func(name string, file fs.File) (fs.File, error) {
				if strings.HasSuffix(name, ".m2ts") {
					file.Close()
					return nil, errors.New("metadata discovery opened video")
				}
				return &discoveryFile{File: file, reverseDir: true}, nil
			}}
			opts.Path = logicalRoot
			got, err := bdinfo.DiscoverPlaylistsFS(t.Context(), filesystem, opts)
			if err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(got.Playlists, want.Playlists) || !reflect.DeepEqual(got.Timelines, want.Timelines) || !reflect.DeepEqual(got.Scan, want.Scan) {
				t.Fatalf("FS discovery differs: %+v versus %+v", got, want)
			}
			if got.Report != "" || got.QuickSummary != "" || got.ForumsBlock != "" || got.ReportPath != "" {
				t.Fatal("discovery produced a report")
			}
		})
	}
}

func TestDiscoverPlaylistsFSErrors(t *testing.T) {
	root := t.TempDir()
	writeDiscFixture(t, root)
	if err := os.MkdirAll(filepath.Join(root, "BDMV", "META", "DL"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "BDMV", "META", "DL", "bdmt_eng.xml"), []byte("<disclib/>"), 0644); err != nil {
		t.Fatal(err)
	}
	opts := bdinfo.Options{IncludeTimeline: true}
	for _, tc := range []struct{ name, target, operation string }{
		{"CLPI-read", "BDMV/CLIPINF/00001.clpi", "read"},
		{"MPLS-read", "BDMV/PLAYLIST/00000.mpls", "read"},
		{"optional-XML-read", "BDMV/META/DL/bdmt_eng.xml", "read"},
		{"metadata-close", "BDMV/CLIPINF/00001.clpi", "close"},
		{"stat-fallback-close", ".", "close"},
		{"ReadDir-fallback-close", "BDMV", "close"},
		{"optional-lookup", "BDMV/META", "lookup"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			sentinel := errors.New("owned filesystem failure: " + tc.name)
			wrap := func(name string, file fs.File) (fs.File, error) {
				if name != tc.target || tc.operation == "lookup" {
					return file, nil
				}
				wrapped := &discoveryFile{File: file}
				if tc.operation == "read" {
					wrapped.readError = sentinel
				} else {
					wrapped.closeError = sentinel
				}
				return wrapped, nil
			}
			var filesystem fs.FS = discoveryFS{source: os.DirFS(root), open: wrap}
			if tc.operation == "lookup" {
				filesystem = discoveryFS{source: os.DirFS(root), lookup: func(name string) error {
					if name == tc.target {
						return sentinel
					}
					return nil
				}}
			}
			if tc.name == "stat-fallback-close" {
				filesystem = openDiscoveryFS{source: os.DirFS(root), open: wrap}
			}
			got, err := bdinfo.DiscoverPlaylistsFS(t.Context(), filesystem, opts)
			if !errors.Is(err, sentinel) || !reflect.DeepEqual(got, bdinfo.Result{}) {
				t.Fatalf("lost filesystem failure: %+v %v", got, err)
			}
		})
	}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if _, err := bdinfo.DiscoverPlaylistsFS(ctx, os.DirFS(root), opts); !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
	for _, target := range []string{"BDMV", "BDMV/CLIPINF/00001.clpi"} {
		t.Run("cancel="+target, func(t *testing.T) {
			ctx, cancel := context.WithCancel(t.Context())
			defer cancel()
			filesystem := discoveryFS{source: os.DirFS(root), open: func(name string, file fs.File) (fs.File, error) {
				if name == target {
					return &discoveryFile{File: file, cancel: cancel}, nil
				}
				return file, nil
			}}
			got, err := bdinfo.DiscoverPlaylistsFS(ctx, filesystem, opts)
			if !errors.Is(err, context.Canceled) || !reflect.DeepEqual(got, bdinfo.Result{}) {
				t.Fatalf("lost cancellation: %+v %v", got, err)
			}
		})
	}
	filesystem := discoveryFS{source: os.DirFS(root), open: func(name string, file fs.File) (fs.File, error) {
		if strings.HasSuffix(name, ".clpi") {
			return &discoveryFile{File: file, dataEOF: true}, nil
		}
		return file, nil
	}}
	if _, err := bdinfo.DiscoverPlaylistsFS(t.Context(), filesystem, opts); err != nil {
		t.Fatalf("normal data+EOF poisoned discovery: %v", err)
	}
	if _, err := bdinfo.DiscoverPlaylistsFS(t.Context(), nil, opts); err == nil {
		t.Fatal("nil filesystem accepted")
	}
	for _, name := range []string{"../disc", "/disc", "C:disc", `C:\disc`} {
		opts.Path = name
		if _, err := bdinfo.DiscoverPlaylistsFS(t.Context(), os.DirFS(root), opts); err == nil {
			t.Fatalf("invalid root %q accepted", name)
		}
	}
}

func TestDiscoverPlaylistsFSParallelError(t *testing.T) {
	t.Setenv("BDINFO_WORKERS", "2")
	root := t.TempDir()
	writeDiscFixture(t, root)
	data, err := os.ReadFile(filepath.Join(root, "BDMV", "CLIPINF", "00001.clpi"))
	if err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(filepath.Join(root, "BDMV", "CLIPINF", "00002.clpi"), data, 0644); err != nil {
		t.Fatal(err)
	}
	sentinel := errors.New("parallel metadata allowance")
	filesystem := discoveryFS{source: os.DirFS(root), open: func(name string, file fs.File) (fs.File, error) {
		if strings.HasSuffix(name, ".clpi") {
			return &discoveryFile{File: file, readError: sentinel}, nil
		}
		return file, nil
	}}
	result, err := bdinfo.DiscoverPlaylistsFS(t.Context(), filesystem, bdinfo.Options{IncludeTimeline: true})
	if !errors.Is(err, sentinel) || !reflect.DeepEqual(result, bdinfo.Result{}) {
		t.Fatalf("parallel error lost: %+v %v", result, err)
	}
}
