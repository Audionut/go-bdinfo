// Copyright (c) 2026, s0up and the autobrr contributors.
// SPDX-License-Identifier: GPL-2.0-or-later

package fs

import (
	"context"
	"errors"
	"fmt"
	"io"
	stdfs "io/fs"
	"path"
	"slices"
	"strings"
	"sync"
	"time"
)

// IOFileSystem borrows a standard filesystem. The caller owns containment,
// resource limits and concurrency support. Errors survive optional probes that
// legacy BDROM initialization intentionally tolerates.
type IOFileSystem struct {
	ctx    context.Context
	source stdfs.FS
	mu     sync.Mutex
	err    error
}

func NewIOFileSystem(ctx context.Context, source stdfs.FS) *IOFileSystem {
	return &IOFileSystem{ctx: ctx, source: source}
}
func (s *IOFileSystem) IsISO() bool { return false }
func (s *IOFileSystem) Err() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.err == nil {
		s.err = s.ctx.Err()
	}
	return s.err
}
func (s *IOFileSystem) remember(err error) {
	if err != nil {
		s.mu.Lock()
		if s.err == nil {
			s.err = err
		}
		s.mu.Unlock()
	}
}
func (s *IOFileSystem) lookupError(err error) {
	if !absenceOnly(err) {
		s.remember(err)
	}
}
func (s *IOFileSystem) rememberClose(err error) {
	if err == nil {
		return
	}
	s.mu.Lock()
	s.err = errors.Join(s.err, err)
	s.mu.Unlock()
}

// A joined absence and cleanup failure is not an absent optional lookup.
func absenceOnly(err error) bool {
	if err == nil {
		return false
	}
	if multiple, ok := err.(interface{ Unwrap() []error }); ok {
		children := multiple.Unwrap()
		if len(children) == 0 {
			return false
		}
		for _, child := range children {
			if !absenceOnly(child) {
				return false
			}
		}
		return true
	}
	if single, ok := err.(interface{ Unwrap() error }); ok {
		if child := single.Unwrap(); child != nil {
			return absenceOnly(child)
		}
	}
	return errors.Is(err, stdfs.ErrNotExist)
}
func (s *IOFileSystem) valid(name string) error {
	if !stdfs.ValidPath(name) || strings.ContainsAny(name, `\:`) {
		err := &stdfs.PathError{Op: "lookup", Path: name, Err: stdfs.ErrInvalid}
		s.remember(err)
		return err
	}
	return s.Err()
}

func (s *IOFileSystem) stat(name string) (info stdfs.FileInfo, result error) {
	if err := s.valid(name); err != nil {
		return nil, err
	}
	if source, ok := s.source.(stdfs.StatFS); ok {
		info, result = source.Stat(name)
		s.lookupError(result)
		return info, result
	}
	file, err := s.source.Open(name)
	if err != nil {
		s.lookupError(err)
		return nil, err
	}
	defer func() { err := file.Close(); s.rememberClose(err); result = errors.Join(result, err) }()
	if err := s.Err(); err != nil {
		return nil, err
	}
	info, result = file.Stat()
	s.remember(result)
	return info, result
}

func (s *IOFileSystem) readDir(name string) (entries []stdfs.DirEntry, result error) {
	if err := s.valid(name); err != nil {
		return nil, err
	}
	if source, ok := s.source.(stdfs.ReadDirFS); ok {
		entries, result = source.ReadDir(name)
		s.lookupError(result)
		return entries, result
	}
	file, err := s.source.Open(name)
	if err != nil {
		s.lookupError(err)
		return nil, err
	}
	defer func() { err := file.Close(); s.rememberClose(err); result = errors.Join(result, err) }()
	if err := s.Err(); err != nil {
		return nil, err
	}
	dir, ok := file.(stdfs.ReadDirFile)
	if !ok {
		result = fmt.Errorf("filesystem directory %s does not support ReadDir", name)
		s.remember(result)
		return nil, result
	}
	entries, result = dir.ReadDir(-1)
	if result == io.EOF {
		result = nil
	}
	s.remember(result)
	slices.SortFunc(entries, func(a, b stdfs.DirEntry) int { return strings.Compare(a.Name(), b.Name()) })
	return entries, result
}

func (s *IOFileSystem) GetDirectoryInfo(name string) (DirectoryInfo, error) {
	info, err := s.stat(name)
	if err != nil {
		return nil, err
	}
	if !info.IsDir() {
		err := fmt.Errorf("%s is not a directory: %w", name, stdfs.ErrInvalid)
		s.remember(err)
		return nil, err
	}
	return &ioDirectory{system: s, path: name, name: info.Name()}, nil
}
func (s *IOFileSystem) GetFileInfo(name string) (FileInfo, error) {
	info, err := s.stat(name)
	if err != nil {
		return nil, err
	}
	if info.IsDir() {
		err := fmt.Errorf("%s is a directory: %w", name, stdfs.ErrInvalid)
		s.remember(err)
		return nil, err
	}
	return &ioFile{system: s, path: name, info: info}, nil
}

type ioFile struct {
	system *IOFileSystem
	path   string
	info   stdfs.FileInfo
}

func (f *ioFile) Name() string       { return f.info.Name() }
func (f *ioFile) FullName() string   { return f.path }
func (f *ioFile) Length() int64      { return f.info.Size() }
func (f *ioFile) Extension() string  { return strings.ToLower(path.Ext(f.path)) }
func (f *ioFile) IsDirectory() bool  { return f.info.IsDir() }
func (f *ioFile) ModTime() time.Time { return f.info.ModTime() }
func (f *ioFile) OpenRead() (io.ReadCloser, error) {
	if err := f.system.valid(f.path); err != nil {
		return nil, err
	}
	file, err := f.system.source.Open(f.path)
	if err != nil {
		f.system.remember(err)
		return nil, err
	}
	return &ioReader{ReadCloser: file, system: f.system}, nil
}

type ioReader struct {
	io.ReadCloser
	system *IOFileSystem
}

func (r *ioReader) Read(data []byte) (int, error) {
	if err := r.system.Err(); err != nil {
		return 0, err
	}
	n, err := r.ReadCloser.Read(data)
	if err != io.EOF {
		r.system.remember(err)
	}
	return n, err
}
func (r *ioReader) Close() error {
	err := r.ReadCloser.Close()
	r.system.rememberClose(err)
	return err
}

type ioDirectory struct {
	system     *IOFileSystem
	path, name string
}

func (d *ioDirectory) Name() string     { return d.name }
func (d *ioDirectory) FullName() string { return d.path }
func (d *ioDirectory) Exists() bool {
	info, err := d.system.stat(d.path)
	return err == nil && info.IsDir()
}
func (d *ioDirectory) child(name string) (string, error) {
	if err := d.system.valid(name); err != nil {
		return "", err
	}
	return path.Join(d.path, name), nil
}
func (d *ioDirectory) GetDirectory(name string) (DirectoryInfo, error) {
	joined, err := d.child(name)
	if err != nil {
		return nil, err
	}
	return d.system.GetDirectoryInfo(joined)
}
func (d *ioDirectory) GetFile(name string) (FileInfo, error) {
	joined, err := d.child(name)
	if err != nil {
		return nil, err
	}
	return d.system.GetFileInfo(joined)
}
func (d *ioDirectory) entries() ([]stdfs.FileInfo, error) {
	entries, err := d.system.readDir(d.path)
	if err != nil {
		return nil, err
	}
	infos := make([]stdfs.FileInfo, 0, len(entries))
	for _, entry := range entries {
		if strings.Contains(entry.Name(), "/") || entry.Name() == "." {
			err := fmt.Errorf("invalid directory entry %q: %w", entry.Name(), stdfs.ErrInvalid)
			d.system.remember(err)
			return nil, err
		}
		name, err := d.child(entry.Name())
		if err != nil {
			return nil, err
		}
		var info stdfs.FileInfo
		if entry.Type()&stdfs.ModeSymlink != 0 {
			info, err = d.system.stat(name)
		} else {
			info, err = entry.Info()
		}
		if err != nil {
			d.system.remember(err)
			return nil, err
		}
		infos = append(infos, info)
	}
	return infos, nil
}
func (d *ioDirectory) GetFiles() ([]FileInfo, error) { return d.GetFilesPattern("*") }
func (d *ioDirectory) GetFilesPattern(pattern string) ([]FileInfo, error) {
	entries, err := d.entries()
	if err != nil {
		return nil, err
	}
	var files []FileInfo
	for _, info := range entries {
		matched, err := path.Match(pattern, info.Name())
		if err != nil {
			return nil, err
		}
		if !info.IsDir() && matched {
			files = append(files, &ioFile{system: d.system, path: path.Join(d.path, info.Name()), info: info})
		}
	}
	return files, nil
}
func (d *ioDirectory) GetDirectories() ([]DirectoryInfo, error) {
	entries, err := d.entries()
	if err != nil {
		return nil, err
	}
	var directories []DirectoryInfo
	for _, info := range entries {
		if info.IsDir() {
			directories = append(directories, &ioDirectory{system: d.system, path: path.Join(d.path, info.Name()), name: info.Name()})
		}
	}
	return directories, nil
}
