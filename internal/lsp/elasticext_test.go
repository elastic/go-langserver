package lsp

import (
	"context"
	"go/ast"
	"go/parser"
	"go/token"
	"golang.org/x/tools/internal/lsp/source"
	"golang.org/x/tools/internal/lsp/xlog"
	"golang.org/x/tools/internal/span"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"golang.org/x/tools/go/packages/packagestest"
	"golang.org/x/tools/internal/lsp/cache"
	"golang.org/x/tools/internal/lsp/protocol"
)

func TestLSPExt(t *testing.T) {
	packagestest.TestAll(t, testLSPExt)
}

const extViewName = "lspext_test"

func testLSPExt(t *testing.T, exporter packagestest.Exporter) {
	const dir = "testdata"

	// We hardcode the expected number of test cases to ensure that all tests
	// are being executed. If a test is added, this number must be changed.
	const expectedQNameKindCount = 57
	const expectedPkgLocatorCount = 2

	files := packagestest.MustCopyFileTree(dir)
	for fragment, operation := range files {
		if trimmed := strings.TrimSuffix(fragment, ".in"); trimmed != fragment {
			delete(files, fragment)
			files[trimmed] = operation
		}
	}
	modules := []packagestest.Module{
		{
			Name:  "golang.org/x/tools/internal/lsp",
			Files: files,
		},
	}
	exported := packagestest.Export(t, exporter, modules)
	defer exported.Cleanup()

	// Merge the exported.Config with the view.Config.
	cfg := *exported.Config
	cfg.Fset = token.NewFileSet()
	cfg.Context = context.Background()
	cfg.ParseFile = func(fset *token.FileSet, filename string, src []byte) (*ast.File, error) {
		return parser.ParseFile(fset, filename, src, parser.AllErrors|parser.ParseComments)
	}

	log := xlog.New(xlog.StdSink{})
	cache := cache.New()
	session := cache.NewSession(log)
	session.NewView(extViewName, span.FileURI(cfg.Dir), &cfg)
	s := &Server{
		session:     session,
		undelivered: make(map[span.URI][]source.Diagnostic),
	}
	goPath := ""
	goRoot := ""
	for _, v := range os.Environ() {
		if strings.HasPrefix(v, "GOPATH=") {
			goPath = strings.TrimPrefix(v, "GOPATH=")
		}
		if strings.HasPrefix(v, "GOROOT=") {
			goRoot = strings.TrimPrefix(v, "GOROOT=")
		}
	}
	depsPath := filepath.Join(filepath.Join(goPath, "pkg"), "mod")
	es := &ElasticServer{*s, depsPath, goRoot}

	expectedQNameKinds := make(qnamekinds)
	expectedPkgLocators := make(pkgs)

	// Collect any data that needs to be used by subsequent tests.
	if err := exported.Expect(map[string]interface{}{
		"packagelocator": expectedPkgLocators.collect,
		"qnamekind":      expectedQNameKinds.collect,
	}); err != nil {
		t.Fatal(err)
	}

	t.Run("QNameKind", func(t *testing.T) {
		t.Helper()
		if len(expectedQNameKinds) != expectedQNameKindCount {
			t.Errorf("got %v qnamekinds expected %v", len(expectedQNameKinds), expectedQNameKindCount)
		}
		expectedQNameKinds.test(t, es)
	})

	t.Run("PKG", func(t *testing.T) {
		t.Helper()
		if len(expectedPkgLocators) != expectedPkgLocatorCount {
			t.Errorf("got %v pkgs expected %v", len(expectedPkgLocators), expectedPkgLocatorCount)
		}
		expectedPkgLocators.test(t, es)
	})
	t.Run("NormalizePath", func(t *testing.T) {
		t.Helper()
		testdata := []NormalizeTuple{
			{
				RepoURI:     "github.com/json-iter@ator",
				Path:        "github.com/json-iter@ator/@/@/@23423afasdf/124/wrew@.go",
				PathWant:    "@/@/@23423afasdf/124/wrew@.go",
				LocatedDeps: true,
			},
			{
				RepoURI:     "github.com/json-iter@ator/@",
				Path:        "github.com/json-iter@ator/@/@v0.0.1/@23423afasdf/124/wrew@.go",
				PathWant:    "@23423afasdf/124/wrew@.go",
				LocatedDeps: true,
			},
			{
				RepoURI:     "github.com/json-iter@ator/@",
				Path:        "github.com/json-iter@ator/@/vv0.0.0-20190519123345-abcdefabcdef/@v0.0.1/@23423afasdf/124/wrew@.go",
				PathWant:    "vv0.0.0-20190519123345-abcdefabcdef/@23423afasdf/124/wrew@.go",
				LocatedDeps: true,
			},
			{
				RepoURI:     "github.com/json-iter@ator/@",
				Path:        "github.com/json-iter@ator/@/@v0.0.0-20190519123345-abcdefabcdef/@v0.0.1/@23423afasdf/124/wrew@.go",
				PathWant:    "@v0.0.0-20190519123345-abcdefabcdef/@v0.0.1/@23423afasdf/124/wrew@.go",
				LocatedDeps: true,
			},
			{
				RepoURI:     "github.com/json-iter@ator/@",
				Path:        "github.com/json-iter@ator/@/@@v0.0.0-20190519123345-abcdefabcdef/@23423afasdf/124/wrew@.go",
				PathWant:    "@/@23423afasdf/124/wrew@.go",
				LocatedDeps: true,
			},
			{
				RepoURI:     "",
				Path:        "lsp/elasticserver.go",
				PathWant:    "lsp/elasticserver.go",
				LocatedDeps: false,
			},
		}

		for _, v := range testdata {
			var path string
			if v.LocatedDeps {
				path = filepath.Join(depsPath, v.Path)
			} else {
				path = filepath.Join(cfg.Dir, v.Path)
			}
			pkgLoc := protocol.PackageLocator{RepoURI: v.RepoURI}
			pathGot := normalizePath(path, cfg, &pkgLoc, es.DepsPath)
			if pathGot != v.PathWant {
				t.Errorf("got %v expected %v", pathGot, v.PathWant)
			}
		}
	})
}

type QNameKindResult struct {
	Qname string
	Kind  int64
}

type PkgResultTuple struct {
	PkgName string
	RepoURI string
}

type NormalizeTuple struct {
	RepoURI     string
	Path        string
	PathWant    string
	LocatedDeps bool
}

type qnamekinds map[protocol.Location]QNameKindResult
type pkgs map[protocol.Location]PkgResultTuple

func (qk qnamekinds) test(t *testing.T, s *ElasticServer) {
	for src, target := range qk {
		params := &protocol.TextDocumentPositionParams{
			TextDocument: protocol.TextDocumentIdentifier{
				URI: src.URI,
			},
			Position: src.Range.Start,
		}
		var locs []protocol.SymbolLocator
		var err error
		locs, err = s.EDefinition(context.Background(), params)
		if err != nil {
			t.Fatalf("failed for %v: %v", src, err)
		}
		if len(locs) != 1 {
			t.Errorf("got %d locations for qnamekind, expected 1", len(locs))
		}

		if locs[0].Qname != target.Qname {
			t.Errorf("Qname: for %v got %v want %v", src, locs[0].Qname, target.Qname)
		}

		if locs[0].Kind != protocol.SymbolKind(target.Kind) {
			t.Errorf("Kind: for %v got %v want %v", src, locs[0].Kind, target.Kind)
		}
	}
}

func (qk qnamekinds) collect(e *packagestest.Exported, fset *token.FileSet, src packagestest.Range, qname string, kind int64) {
	sSrc, mSrc := testLocation(e, fset, src)
	lSrc, err := mSrc.Location(sSrc)
	if err != nil {
		return
	}

	qk[lSrc] = QNameKindResult{Qname: qname, Kind: kind}
}

func (ps pkgs) test(t *testing.T, s *ElasticServer) {
	for src, target := range ps {
		params := &protocol.TextDocumentPositionParams{
			TextDocument: protocol.TextDocumentIdentifier{
				URI: src.URI,
			},
			Position: src.Range.Start,
		}
		var locs []protocol.SymbolLocator
		var err error
		locs, err = s.EDefinition(context.Background(), params)
		if err != nil {
			t.Fatalf("failed for %v: %v", src, err)
		}
		if len(locs) != 1 {
			t.Errorf("got %d locations for package locators, expected 1", len(locs))
		}

		if locs[0].Package.Name != target.PkgName {
			t.Errorf("PkgName: for %v got %v want %v", src, locs[0].Package.Name, target.PkgName)
		}

		if locs[0].Package.RepoURI != target.RepoURI {
			t.Errorf("PkgRepoURI: for %v got %v want %v", src, locs[0].Package.RepoURI, target.RepoURI)
		}
	}
}

func (ps pkgs) collect(e *packagestest.Exported, fset *token.FileSet, src packagestest.Range, pkgname, repouri string) {
	sSrc, mSrc := testLocation(e, fset, src)
	lSrc, err := mSrc.Location(sSrc)
	if err != nil {
		return
	}

	ps[lSrc] = PkgResultTuple{PkgName: pkgname, RepoURI: repouri}
}

func testLocation(e *packagestest.Exported, fset *token.FileSet, rng packagestest.Range) (span.Span, *protocol.ColumnMapper) {
	spn, err := span.NewRange(fset, rng.Start, rng.End).Span()
	if err != nil {
		return spn, nil
	}
	f := fset.File(rng.Start)
	content, err := e.FileContents(f.Name())
	if err != nil {
		return spn, nil
	}
	m := protocol.NewColumnMapper(spn.URI(), f.Name(), fset, f, content)
	return spn, m
}
