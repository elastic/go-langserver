// Copyright 2019 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package lsp

import (
	"context"

	"golang.org/x/tools/internal/lsp/protocol"
	"golang.org/x/tools/internal/span"
	"golang.org/x/tools/internal/telemetry/log"
	errors "golang.org/x/xerrors"
	"os/exec"
)

func (s *Server) changeFolders(ctx context.Context, event protocol.WorkspaceFoldersChangeEvent) error {
	for _, folder := range event.Removed {
		view := s.session.View(folder.Name)
		if view != nil {
			view.Shutdown(ctx)
		} else {
			return errors.Errorf("view %s for %v not found", folder.Name, folder.URI)
		}
	}

	for _, folder := range event.Added {
		if err := s.addView(ctx, folder.Name, span.NewURI(folder.URI)); err != nil {
			return err
		}
	}
	return nil
}

func (s *Server) addView(ctx context.Context, name string, uri span.URI) error {
	if s.installGoDependency {
		cmd := exec.Command("go", "mod", "download")
		cmd.Dir = uri.Filename()
		if err := cmd.Run(); err != nil {
			log.Error(ctx, "failed to download the dependencies", err)
		}
	} else {
		// If we disable the go dependency download, trying to find the deps from the vendor folder.
		ctx = context.WithValue(ctx, "ENABLEVENDOR", true)
	}
	view := s.session.NewView(ctx, name, uri)
	s.stateMu.Lock()
	state := s.state
	s.stateMu.Unlock()
	if state >= serverInitialized {
		s.fetchConfig(ctx, view)
	}
	return nil
}
