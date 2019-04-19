package lsp

import (
	"context"
	"go/ast"
	"go/parser"
	"go/token"
	"golang.org/x/tools/internal/lsp/xlog"
	"golang.org/x/tools/internal/span"
	"strings"
	"testing"

	"golang.org/x/tools/go/packages/packagestest"
	"golang.org/x/tools/internal/lsp/cache"
	"golang.org/x/tools/internal/lsp/protocol"
)

func TestLSPExt(t *testing.T) {
	packagestest.TestAll(t, testLSPExt)
}

func testLSPExt(t *testing.T, exporter packagestest.Exporter) {
	ctx := context.Background()
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
	s := &Server{
		views: []*cache.View{cache.NewView(ctx, log, "elasticext_test", span.FileURI(cfg.Dir), &cfg)},
	}
	es := &ElasticServer{*s}

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
}

type QNameKindResult struct {
	Qname string
	Kind  int64
}

type PkgResultTuple struct {
	PkgName string
	RepoURI string
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
