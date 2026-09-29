package web

// A route serving the files of a file system.

import (
	"io/fs"
	"net/http"
	"strings"

	"github.com/mbauer83/effect-golang-schema/schema"
	"github.com/mbauer83/effect-golang/effect"
)

// Files serves the files of a file system beneath a path: GET prefix/{file...}.
//
// What net/http already does for a file is kept -- the content type from the
// name, ranges, and a conditional request answered 304 -- because
// http.ServeFileFS does the serving. What it would also do is refused: a
// directory is not listed, and a name that is not a file in the file system
// is 404, whatever is on the disk beside it. An embed.FS is the usual file
// system, and it is why a program's assets can ship inside its binary.
func Files[R, E any](prefix string, files fs.FS) Route[R, E] {
	prefix = strings.TrimSuffix(prefix, "/")
	return Handle(
		GET(prefix+"/{file...}", PathParam("file", schema.Text()).WithDescription("the file's path beneath "+prefix),
			ReturnsResponse(http.StatusOK, "application/octet-stream", fileResponse(files))).
			WithSummary("A file served from "+prefix).
			WithFailure(http.StatusNotFound, "no such file"),
		func(file string) effect.Effect[R, E, string] {
			return effect.Succeed[R, E](file)
		},
	)
}

// fileResponse is a file as net/http serves one, or 404 for anything that is
// not a file in the file system.
func fileResponse(files fs.FS) func(string) (Response, error) {
	return func(name string) (Response, error) {
		info, err := fs.Stat(files, name)
		if err != nil || info.IsDir() {
			return Empty(http.StatusNotFound), nil
		}
		return Delegate(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
			http.ServeFileFS(writer, request, files, name)
		})), nil
	}
}
