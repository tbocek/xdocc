package xdocc

import (
	"bytes"
	"compress/gzip"
	"log"
	"path"
	"strings"
	"time"

	"github.com/andybalholm/brotli"
	"github.com/tdewolff/minify/v2"
	"github.com/tdewolff/minify/v2/css"
	"github.com/tdewolff/minify/v2/html"
	"github.com/tdewolff/minify/v2/js"
	"github.com/tdewolff/minify/v2/json"
	"github.com/tdewolff/minify/v2/svg"
	"github.com/tdewolff/minify/v2/xml"
)

// minifyType maps an extension to the media type its minifier is registered
// under. These are the files xdocc rewrites on the way out, which means it
// writes bytes for them instead of linking to the source.
var minifyType = map[string]string{
	".html": "text/html",
	".htm":  "text/html",
	".css":  "text/css",
	".js":   "application/javascript",
	".mjs":  "application/javascript",
	".svg":  "image/svg+xml",
	".json": "application/json",
	".xml":  "text/xml",
}

// compressExt are the extensions that get a .gz and a .br beside them, which is
// what a web server serving pre-compressed files looks for. Everything with a
// minifier is in here, plus the text formats that have none. Media, archives
// and PDFs are left alone: they are compressed already and would only grow.
var compressExt = map[string]bool{
	".html": true, ".htm": true, ".css": true, ".js": true, ".mjs": true,
	".svg": true, ".json": true, ".xml": true, ".txt": true, ".csv": true,
	".md": true, ".bib": true, ".ics": true, ".map": true, ".webmanifest": true,
	".rss": true, ".atom": true, ".vtt": true, ".srt": true, ".wasm": true,
}

// minCompressSize is the size below which a compressed copy is not worth its
// own inode: the frame alone costs more than the file saves.
const minCompressSize = 256

// minifier is shared: it holds no per-file state, only the table of minifiers.
var minifier = func() *minify.M {
	m := minify.New()
	m.AddFunc("text/html", html.Minify)
	m.AddFunc("text/css", css.Minify)
	m.AddFunc("application/javascript", js.Minify)
	m.AddFunc("image/svg+xml", svg.Minify)
	m.AddFunc("application/json", json.Minify)
	m.AddFunc("text/xml", xml.Minify)
	return m
}()

// minifyBytes shrinks data for the given output path. Minifying is an
// optimisation, never a requirement: a file the minifier chokes on is passed
// through as it is, with a word about it, so a single malformed SVG cannot
// take the build down.
func minifyBytes(rel string, data []byte) []byte {
	mediatype, ok := minifyType[strings.ToLower(path.Ext(rel))]
	if !ok {
		return data
	}
	out, err := minifier.Bytes(mediatype, data)
	if err != nil {
		log.Printf("xdocc: cannot minify %s (%v), writing it unchanged", rel, err)
		return data
	}
	return out
}

// encoder is one pre-compressed copy of an output file.
type encoder struct {
	suffix string
	encode func([]byte) ([]byte, error)
}

// encoders are written at the highest setting each format has: a static site is
// compressed once and served many times, so the time belongs here and not in
// the request.
var encoders = []encoder{
	{".gz", gzipBytes},
	{".br", brotliBytes},
}

func gzipBytes(data []byte) ([]byte, error) {
	var buf bytes.Buffer
	w, err := gzip.NewWriterLevel(&buf, gzip.BestCompression)
	if err != nil {
		return nil, err
	}
	if _, err := w.Write(data); err != nil {
		return nil, err
	}
	if err := w.Close(); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func brotliBytes(data []byte) ([]byte, error) {
	var buf bytes.Buffer
	w := brotli.NewWriterLevel(&buf, brotli.BestCompression)
	if _, err := w.Write(data); err != nil {
		return nil, err
	}
	if err := w.Close(); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// placement is what xdocc last put at an output path. Remembering it is what
// lets a rebuild skip the output tree entirely: without it every run stats
// every symlink and reads back every page it wrote. It follows that xdocc owns
// the output tree - change it from outside and xdocc will not notice until it
// is restarted.
type placement struct {
	link string   // symlink target, when the output is a link
	hash [32]byte // of the bytes written, when it is a file

	// size and mtime of the single source file the output came from, when
	// there is one. An asset that has not been touched is not read again.
	srcSize int64
	srcMod  time.Time

	sidecars bool // a .gz and a .br were written next to it
}
