// Copyright (C) 2014 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

package model

import (
	"fmt"

	"github.com/syncthing/syncthing/lib/config"
	"github.com/syncthing/syncthing/lib/fs"
	"github.com/syncthing/syncthing/lib/fswatcher"
	"github.com/syncthing/syncthing/lib/versioner"
)

func init() {
	folderFactories[config.FolderTypeSendOnly] = newSendOnlyFolder
}

type sendOnlyFolder struct {
	folder
}

func newSendOnlyFolder(model *Model, cfg config.FolderConfiguration, _ versioner.Versioner,
	_ *fs.MtimeFS, fsWatchChan <-chan fswatcher.FsEventsBatch) service {
	return &sendOnlyFolder{
		folder: *newFolder(model, cfg, fsWatchChan),
	}
}

func (f *sendOnlyFolder) Serve() {
	l.Debugln(f, "starting")
	defer l.Debugln(f, "exiting")

	defer func() {
		f.scan.timer.Stop()
	}()

	for {
		select {
		case <-f.stop:
			return

		case <-f.scan.timer.C:
			if err := f.model.CheckFolderHealth(f.folderID); err != nil {
				l.Infoln("Skipping scan of", f.Description(), "due to folder error:", err)
				f.scan.Reschedule()
				continue
			}

			l.Debugln(f, "rescan")

			if err := f.model.internalScanFolderSubdirs(f.folderID, nil); err != nil {
				// Potentially sets the error twice, once in the scanner just
				// by doing a check, and once here, if the error returned is
				// the same one as returned by CheckFolderHealth, though
				// duplicate set is handled by setError.
				f.setError(err)
				f.scan.Reschedule()
				continue
			}

			select {
			case <-f.initialScanCompleted:
			default:
				l.Infoln("Completed initial scan (ro) of", f.Description())
				close(f.initialScanCompleted)
			}

			if f.scan.HasNoInterval() {
				continue
			}

			f.scan.Reschedule()

		case req := <-f.scan.now:
			req.err <- f.scanSubdirsIfHealthy(req.subdirs)

		case next := <-f.scan.delay:
			f.scan.timer.Reset(next)

		case fsEvents := <-f.fsWatchChan:
			l.Debugln(f, "filesystem notification rescan")
			f.scanSubdirsIfHealthy(fsEvents.GetPaths())
		}
	}
}

func (f *sendOnlyFolder) String() string {
	return fmt.Sprintf("sendOnlyFolder/%s@%p", f.folderID, f)
}
