package middleware

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"go.uber.org/zap"
)

type staticFileServerOptions struct {
	root          string
	indexFile     string
	allowDotFiles bool
	spa           bool
}

// guardedFileSystem wraps http.Dir to block dotfiles, suppress directory listings,
// and prevent symlink escapes — none of which http.Dir handles.
// from http.fs
// // Note that Dir could expose sensitive files and directories. Dir will follow
// // symlinks pointing out of the directory tree, which can be especially dangerous
// // if serving from a directory in which users are able to create arbitrary symlinks.
// // Dir will also allow access to files and directories starting with a period,
// // which could expose sensitive directories like .git or sensitive files like
// // .htpasswd. To exclude files with a leading period, remove the files/directories
// // from the server or create a custom FileSystem implementation.
type guardedFileSystem struct {
	fs            http.Dir
	root          string
	allowDotFiles bool
	indexFile     string
}

func (g guardedFileSystem) Open(name string) (http.File, error) {
	// blocks dotfiles (.env, .git, .htaccess)
	if !g.allowDotFiles {
		for _, seg := range strings.Split(name, "/") {
			if strings.HasPrefix(seg, ".") && seg != "." && seg != ".." {
				return nil, os.ErrNotExist
			}
		}
	}

	f, err := g.fs.Open(name)
	if err != nil {
		return nil, err
	}

	// symlink containment
	fullPath := filepath.Join(g.root, filepath.FromSlash(name))
	realPath, err := filepath.EvalSymlinks(fullPath)
	if err != nil {
		f.Close()
		return nil, os.ErrNotExist
	}
	if !strings.HasPrefix(realPath, g.root+string(filepath.Separator)) && realPath != g.root {
		f.Close()
		return nil, os.ErrPermission
	}

	fi, err := f.Stat()
	if err != nil {
		f.Close()
		return nil, err
	}

	// suppress directory listings
	if fi.IsDir() {
		indexPath := filepath.Join(fullPath, g.indexFile)
		if _, err := os.Stat(indexPath); os.IsNotExist(err) {
			f.Close()
			return nil, os.ErrNotExist
		}
	}

	return f, nil
}

func StaticFileServerMiddleware(_ context.Context, _ http.HandlerFunc, conf Config) http.HandlerFunc {
	opts, err := parseStaticFileServerConfig(conf)
	if err != nil {
		zap.S().Errorf("StaticFileServerMiddleware: failed to parse config: %v. Failing closed.", err)
		return func(w http.ResponseWriter, r *http.Request) {
			http.Error(w, "StaticFileServerMiddleware misconfigured", http.StatusServiceUnavailable)
		}
	}

	absRoot, err := filepath.Abs(opts.root)
	if err != nil {
		zap.S().Errorf("StaticFileServerMiddleware: cannot resolve root %q: %v. Failing closed.", opts.root, err)
		return func(w http.ResponseWriter, r *http.Request) {
			http.Error(w, "StaticFileServerMiddleware misconfigured", http.StatusServiceUnavailable)
		}
	}

	gfs := guardedFileSystem{
		fs:            http.Dir(absRoot),
		root:          absRoot,
		allowDotFiles: opts.allowDotFiles,
		indexFile:     opts.indexFile,
	}
	fileServer := http.FileServer(gfs)

	if !opts.spa {
		return func(w http.ResponseWriter, r *http.Request) {
			fileServer.ServeHTTP(w, r)
		}
	}

	// SPA mode: if the file doesn't exist, serve the root index instead
	return func(w http.ResponseWriter, r *http.Request) {
		f, err := gfs.Open(r.URL.Path)
		if err != nil {
			r.URL.Path = "/"
		} else {
			f.Close()
		}
		fileServer.ServeHTTP(w, r)
	}
}

func parseStaticFileServerConfig(conf Config) (*staticFileServerOptions, error) {
	if conf.Options == nil {
		return nil, fmt.Errorf("options cannot be nil")
	}

	root, err := ParseStringRequired(conf.Options, "root")
	if err != nil {
		return nil, err
	}

	indexFile, err := ParseStringOpt(conf.Options, "index-file", "index.html")
	if err != nil {
		return nil, err
	}

	return &staticFileServerOptions{
		root:          root,
		indexFile:     indexFile,
		allowDotFiles: ParseBoolOpt(conf.Options, "allow-dot-files", false),
		spa:           ParseBoolOpt(conf.Options, "spa", false),
	}, nil
}
