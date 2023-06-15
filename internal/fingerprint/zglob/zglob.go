package zglob

import (
	"bytes"
	"fmt"
	"github.com/mattn/go-zglob/fastwalk"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"sync"
)

var (
	envre = regexp.MustCompile(`^(\$[a-zA-Z][a-zA-Z0-9_]+|\$\([a-zA-Z][a-zA-Z0-9_]+\))$`)
	mu    sync.Mutex
)

type zenv struct {
	dirmask  string
	fre      *regexp.Regexp
	braceDir bool
	pattern  string
	root     string
}

func toSlash(path string) string {
	if filepath.Separator == '/' {
		return path
	}
	var buf bytes.Buffer
	cc := []rune(path)
	for i := 0; i < len(cc); i++ {
		if i < len(cc)-2 && cc[i] == '\\' && (cc[i+1] == '{' || cc[i+1] == '}') {
			buf.WriteRune(cc[i])
			buf.WriteRune(cc[i+1])
			i++
		} else if cc[i] == '\\' {
			buf.WriteRune('/')
		} else {
			buf.WriteRune(cc[i])
		}
	}
	return buf.String()
}

func New(pattern string) (*zenv, error) {
	globmask := ""
	root := ""
	for n, i := range strings.Split(toSlash(pattern), "/") {
		if root == "" && (strings.Index(i, "*") != -1 || strings.Index(i, "{") != -1) {
			if globmask == "" {
				root = "."
			} else {
				root = toSlash(globmask)
			}
		}
		if n == 0 && i == "~" {
			if runtime.GOOS == "windows" {
				i = os.Getenv("USERPROFILE")
			} else {
				i = os.Getenv("HOME")
			}
		}
		if envre.MatchString(i) {
			i = strings.Trim(strings.Trim(os.Getenv(i[1:]), "()"), `"`)
		}

		globmask = path.Join(globmask, i)
		if n == 0 {
			if runtime.GOOS == "windows" && filepath.VolumeName(i) != "" {
				globmask = i + "/"
			} else if len(globmask) == 0 {
				globmask = "/"
			}
		}
	}
	if root == "" {
		return &zenv{
			dirmask: "",
			fre:     nil,
			pattern: pattern,
			root:    "",
		}, nil
	}
	if globmask == "" {
		globmask = "."
	}
	globmask = toSlash(path.Clean(globmask))

	cc := []rune(globmask)
	dirmask := ""
	filemask := ""
	staticDir := true
	for i := 0; i < len(cc); i++ {
		if i < len(cc)-2 && cc[i] == '\\' {
			i++
			filemask += fmt.Sprintf("[\\x%02X]", cc[i])
			if staticDir {
				dirmask += string(cc[i])
			}
		} else if cc[i] == '*' {
			staticDir = false
			if i < len(cc)-2 && cc[i+1] == '*' && cc[i+2] == '/' {
				filemask += "(.*/)?"
				i += 2
			} else {
				filemask += "[^/]*"
			}
		} else {
			if cc[i] == '{' {
				staticDir = false
				pattern := ""
				for j := i + 1; j < len(cc); j++ {
					if cc[j] == ',' {
						pattern += "|"
					} else if cc[j] == '}' {
						i = j
						break
					} else {
						c := cc[j]
						if c == '/' {
							pattern += string(c)
						} else if ('0' <= c && c <= '9') || ('a' <= c && c <= 'z') || ('A' <= c && c <= 'Z') || 255 < c {
							pattern += string(c)
						} else {
							pattern += fmt.Sprintf("[\\x%02X]", c)
						}
					}
				}
				if pattern != "" {
					filemask += "(" + pattern + ")"
					continue
				}
			} else if i < len(cc)-1 && cc[i] == '!' && cc[i+1] == '(' {
				i++
				pattern := ""
				for j := i + 1; j < len(cc); j++ {
					if cc[j] == ')' {
						i = j
						break
					} else {
						c := cc[j]
						pattern += fmt.Sprintf("[^\\x%02X/]*", c)
					}
				}
				if pattern != "" {
					if dirmask == "" {
						dirmask = filemask
						root = filemask
					}
					filemask += pattern
					continue
				}
			}
			c := cc[i]
			if c == '/' || ('0' <= c && c <= '9') || ('a' <= c && c <= 'z') || ('A' <= c && c <= 'Z') || 255 < c {
				filemask += string(c)
			} else {
				filemask += fmt.Sprintf("[\\x%02X]", c)
			}
			if staticDir {
				dirmask += string(c)
			}
		}
	}
	if len(filemask) > 0 && filemask[len(filemask)-1] == '/' {
		if root == "" {
			root = filemask
		}
		filemask += "[^/]*"
	}
	if runtime.GOOS == "windows" || runtime.GOOS == "darwin" {
		filemask = "(?i:" + filemask + ")"
	}
	return &zenv{
		dirmask: path.Dir(dirmask) + "/",
		fre:     regexp.MustCompile("^" + filemask + "$"),
		pattern: pattern,
		root:    filepath.Clean(root),
	}, nil
}

func Glob(pattern string, ignoredPatterns []string) ([]string, error) {
	return glob(pattern, ignoredPatterns, false)
}

func GlobFollowSymlinks(pattern string, ignoredPatterns []string) ([]string, error) {
	return glob(pattern, ignoredPatterns, true)
}

func glob(pattern string, ignoredPatterns []string, followSymlinks bool) ([]string, error) {
	zenvP, err := New(pattern)
	if err != nil {
		return nil, err
	}
	if zenvP.root == "" {
		_, err := os.Stat(pattern)
		if err != nil {
			return nil, os.ErrNotExist
		}
		return []string{pattern}, nil
	}
	var zenvIPs []*zenv
	for _, ignoredPattern := range ignoredPatterns {
		zenvIP, err := New(ignoredPattern)
		if err != nil {
			return nil, err
		}
		zenvIPs = append(zenvIPs, zenvIP)
	}

	relative := !filepath.IsAbs(pattern)
	matches := []string{}

	err = fastwalk.FastWalk(zenvP.root, func(path string, info os.FileMode) error {
		if zenvP.root == "." && len(zenvP.root) < len(path) {
			path = path[len(zenvP.root)+1:]
		}
		path = filepath.ToSlash(path)

		println(path) // TODO should not traverse glob excluded if detected

		if followSymlinks && info == os.ModeSymlink {
			followedPath, err := filepath.EvalSymlinks(path)
			if err == nil {
				fi, err := os.Lstat(followedPath)
				if err == nil && fi.IsDir() {
					return fastwalk.TraverseLink
				}
			}
		}

		if info.IsDir() {
			if path == "." || len(path) <= len(zenvP.root) {
				return nil
			}
			if globIgnored(path, zenvIPs) {
				return filepath.SkipDir
			}
			if zenvP.fre.MatchString(path) {
				mu.Lock()
				matches = append(matches, path)
				mu.Unlock()
				return nil
			}
			if len(path) < len(zenvP.dirmask) && !strings.HasPrefix(zenvP.dirmask, path+"/") {
				return filepath.SkipDir
			}
		}

		if zenvP.fre.MatchString(path) {
			if !globIgnored(path, zenvIPs) {
				if relative && filepath.IsAbs(path) {
					path = path[len(zenvP.root)+1:]
				}
				mu.Lock()
				matches = append(matches, path)
				mu.Unlock()
			}
		}
		return nil
	})

	if err != nil {
		return nil, err
	}

	return matches, nil
}

func globIgnored(path string, zenvIPs []*zenv) bool {
	for _, zenvIP := range zenvIPs {
		if zenvIP.fre != nil && zenvIP.fre.MatchString(path) {
			return true
		}
		absPath, _ := filepath.Abs(zenvIP.pattern)
		if absPath == path || strings.HasPrefix(absPath, path+string(os.PathSeparator)) {
			return true
		}
	}
	return false
}

func Match(pattern, name string) (matched bool, err error) {
	zenv, err := New(pattern)
	if err != nil {
		return false, err
	}
	return zenv.Match(name), nil
}

func (z *zenv) Match(name string) bool {
	if z.root == "" {
		return z.pattern == name
	}

	name = filepath.ToSlash(name)

	if name == "." || len(name) <= len(z.root) {
		return false
	}

	if z.fre.MatchString(name) {
		return true
	}
	return false
}
