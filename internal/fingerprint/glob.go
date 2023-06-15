package fingerprint

import (
	"os"
	"sort"

	"github.com/go-task/task/v3/internal/execext"
	"github.com/go-task/task/v3/internal/filepathext"
	"github.com/go-task/task/v3/internal/fingerprint/zglob"
)

func globs(dir string, globs []string, ignoredGlobs []string) ([]string, error) {
	files := make([]string, 0)
	for _, g := range globs {
		f, err := Glob(dir, g, ignoredGlobs)
		if err != nil {
			continue
		}
		files = append(files, f...)
	}
	sort.Strings(files)
	return files, nil
}

func Glob(dir string, glob string, ignoredGlobs []string) ([]string, error) {
	files := make([]string, 0)
	glob = filepathext.SmartJoin(dir, glob)

	glob, err := execext.Expand(glob)
	if err != nil {
		return nil, err
	}

	fs, err := zglob.GlobFollowSymlinks(glob, ignoredGlobs)
	if err != nil {
		return nil, err
	}

	for _, f := range fs {
		info, err := os.Stat(f)
		if err != nil {
			return nil, err
		}
		if info.IsDir() {
			continue
		}
		files = append(files, f)
	}
	return files, nil
}
