// Copyright (c) ClaceIO, LLC
// SPDX-License-Identifier: Apache-2.0

package plugins

import (
	"context"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"sync"

	"github.com/claceio/clace/internal/app"
	"github.com/claceio/clace/internal/plugin"
	"github.com/claceio/clace/internal/types"
	"go.starlark.net/starlark"
	"golang.org/x/sync/errgroup"
)

const (
	DEFAULT_FILE_LIMIT = 10_000
	MAX_FILE_LIMIT     = 100_000
)

func init() {
	h := &fsPlugin{}
	pluginFuncs := []plugin.PluginFunc{
		app.CreatePluginApi(h.Abs, app.READ),
		app.CreatePluginApi(h.List, app.READ),
		app.CreatePluginApi(h.Find, app.READ),
	}
	app.RegisterPlugin("fs", NewFSPlugin, pluginFuncs)
}

type fsPlugin struct {
}

func NewFSPlugin(pluginContext *types.PluginContext) (any, error) {
	return &fsPlugin{}, nil
}

func (h *fsPlugin) Abs(thread *starlark.Thread, builtin *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	var path starlark.String
	if err := starlark.UnpackArgs("abs", args, kwargs, "path", &path); err != nil {
		return nil, err
	}

	pathStr := string(path)
	ret, err := filepath.Abs(pathStr)
	if err != nil {
		return nil, err
	}

	return app.NewResponse(ret), nil
}

func (h *fsPlugin) List(thread *starlark.Thread, builtin *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	var path starlark.String
	var recursiveSize starlark.Bool
	var ignoreError starlark.Bool
	if err := starlark.UnpackArgs("list", args, kwargs, "path", &path, "recursive_size?", &recursiveSize, "ignore_errors", &ignoreError); err != nil {
		return nil, err
	}

	pathStr := string(path)
	ctx := app.GetContext(thread)
	ret, err := listDir(ctx, pathStr, bool(recursiveSize), bool(ignoreError))
	if err != nil {
		return nil, err
	}
	return app.NewResponse(ret), nil
}

func (h *fsPlugin) Find(thread *starlark.Thread, builtin *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	var path, nameGlob starlark.String
	var minSize, limit starlark.Int
	var ignoreError starlark.Bool

	if err := starlark.UnpackArgs("find", args, kwargs, "path", &path, "name?", nameGlob, "limit?", &limit, "min_size?", &minSize, "ignore_errors", &ignoreError); err != nil {
		return nil, err
	}

	minSizeInt, ok := minSize.Int64()
	if !ok {
		return nil, fmt.Errorf("min_size must be an integer")
	}

	limitInt, ok := limit.Int64()
	if !ok {
		return nil, fmt.Errorf("limit must be an integer")
	}

	if limitInt > MAX_FILE_LIMIT {
		return nil, fmt.Errorf("file limit %d exceeds max limit %d", limitInt, MAX_FILE_LIMIT)
	}
	if limitInt <= 0 {
		limitInt = DEFAULT_FILE_LIMIT
	}

	ctx := app.GetContext(thread)
	ret, err := find(ctx, string(path), string(nameGlob), limitInt, minSizeInt, bool(ignoreError))
	if err != nil {
		return nil, err
	}

	return app.NewResponse(ret), nil
}

type FileInfo struct {
	Name  string
	Size  int64
	IsDir bool
	Mode  int
}

func listDir(ctx context.Context, path string, recursiveSize, ignoreError bool) ([]map[string]any, error) {
	entries, err := os.ReadDir(path)
	if err != nil {
		return nil, err
	}

	blockSize := int64(4 * 1024) // syscall.Statfs is not available on Windows, using 4K as block size
	fileInfo := map[string]*FileInfo{}
	for _, entry := range entries {
		info, err := entry.Info()
		if err != nil {
			return nil, err
		}

		fileInfo[entry.Name()] = &FileInfo{
			Name:  entry.Name(),
			Size:  convertToBlockSize(info.Size(), blockSize),
			IsDir: info.IsDir(),
			Mode:  int(info.Mode()),
		}
	}

	if recursiveSize {
		errs, ctx := errgroup.WithContext(ctx)
		for name, info := range fileInfo {
			name := name
			info := info
			if info.IsDir {
				errs.Go(func() error {
					size, err := dirSize(ctx, filepath.Join(path, name), blockSize, ignoreError)
					if err != nil {
						return err
					}
					fileInfo[name].Size = size
					return nil
				})
			}
		}

		if err := errs.Wait(); err != nil {
			if !ignoreError {
				return nil, err
			}
		}
	}

	var totalSize int64
	ret := make([]map[string]any, 0, len(fileInfo))
	for _, info := range fileInfo {
		fi := map[string]any{
			"name":   filepath.Join(path, info.Name),
			"size":   info.Size,
			"is_dir": info.IsDir,
			"mode":   info.Mode,
		}
		totalSize += info.Size
		ret = append(ret, fi)
	}

	topLevel := map[string]any{
		"name":   path,
		"size":   totalSize,
		"is_dir": true,
		"mode":   0,
	}

	ret = append(ret, topLevel)
	return ret, nil
}

func dirSize(ctx context.Context, path string, blockSize int64, ignoreError bool) (int64, error) {
	var size int64
	err := filepath.WalkDir(path, func(path string, d fs.DirEntry, err error) error {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if !ignoreError && err != nil {
			return err
		}
		info, err := d.Info()
		if err != nil {
			if ignoreError {
				return nil
			}
			return err
		}

		size += convertToBlockSize(info.Size(), blockSize)
		return nil
	})
	return size, err
}

func convertToBlockSize(size, blockSize int64) int64 {
	if size%blockSize == 0 {
		return size
	}
	return ((size / blockSize) + 1) * blockSize
}

func find(ctx context.Context, path, nameGlob string, limit, minSize int64, ignoreError bool) ([]map[string]any, error) {
	entries, err := os.ReadDir(path)
	if err != nil {
		return nil, err
	}

	blockSize := int64(4 * 1024) // syscall.Statfs is not available on Windows, using 4K as block size
	fileInfo := []*FileInfo{}
	dirs := []string{}
	for _, entry := range entries {
		info, err := entry.Info()
		if err != nil {
			return nil, err
		}

		if !info.IsDir() {
			if matchFile(entry.Name(), nameGlob, info.Size(), minSize) {
				fileInfo = append(fileInfo, &FileInfo{
					Name:  filepath.Join(path, entry.Name()),
					Size:  convertToBlockSize(info.Size(), blockSize),
					IsDir: info.IsDir(),
					Mode:  int(info.Mode()),
				})
			}
		} else {
			dirs = append(dirs, entry.Name())
		}
	}

	fileInfo = truncateList(fileInfo, limit)

	var mu sync.Mutex
	errs, ctx := errgroup.WithContext(ctx)
	for _, dir := range dirs {
		dir := dir
		errs.Go(func() error {
			files, err := matchFiles(ctx, filepath.Join(path, dir), nameGlob, limit, minSize, ignoreError)
			if err != nil {
				return err
			}

			mu.Lock()
			defer mu.Unlock()
			fileInfo = append(fileInfo, files...)
			fileInfo = truncateList(fileInfo, limit)
			return nil
		})
	}

	if err := errs.Wait(); err != nil {
		if !ignoreError {
			return nil, err
		}
	}

	ret := make([]map[string]any, 0, len(fileInfo))
	for _, info := range fileInfo {
		fi := map[string]any{
			"name":   info.Name,
			"size":   info.Size,
			"is_dir": info.IsDir,
			"mode":   info.Mode,
		}
		ret = append(ret, fi)
	}
	return ret, nil
}

func truncateList(entries []*FileInfo, limit int64) []*FileInfo {
	if limit > 0 && int64(len(entries)) >= limit {
		copyInfo := make([]*FileInfo, limit)
		slices.SortFunc(entries, func(i, j *FileInfo) int {
			return int((*j).Size - (*i).Size)
		})

		copy(copyInfo, entries)
		return copyInfo
	}
	return entries
}

func matchFile(name, nameGlob string, size, minSize int64) bool {
	if nameGlob != "" {
		matched, err := filepath.Match(nameGlob, name)
		if err != nil {
			return false
		}
		if !matched {
			return false
		}
	}

	if minSize != 0 && size < minSize {
		return false
	}

	return true
}

func matchFiles(ctx context.Context, path string, nameGlob string, limit, minSize int64, ignoreError bool) ([]*FileInfo, error) {
	files := []*FileInfo{}
	err := filepath.WalkDir(path, func(path string, d fs.DirEntry, err error) error {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if !ignoreError && err != nil {
			return err
		}
		info, err := d.Info()
		if err != nil {
			if ignoreError {
				return nil
			}
			return err
		}

		if !info.IsDir() {
			if matchFile(d.Name(), nameGlob, info.Size(), int64(minSize)) {
				files = append(files, &FileInfo{
					Name:  path,
					Size:  info.Size(),
					IsDir: info.IsDir(),
					Mode:  int(info.Mode()),
				})

				if limit > 0 && int64(len(files)) >= 10*limit {
					files = truncateList(files, limit)
				}
			}
		}

		return nil
	})

	files = truncateList(files, limit)
	return files, err
}
