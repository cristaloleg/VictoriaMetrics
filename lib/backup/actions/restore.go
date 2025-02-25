package actions

import (
	"fmt"
	"io"
	"sync/atomic"
	"time"

	"github.com/VictoriaMetrics/VictoriaMetrics/lib/backup/common"
	"github.com/VictoriaMetrics/VictoriaMetrics/lib/backup/fslocal"
	"github.com/VictoriaMetrics/VictoriaMetrics/lib/logger"
)

// Restore restores data according to the provided settings.
//
// Note that the restore works only for VictoriaMetrics backups made from snapshots.
// It works improperly on mutable files.
type Restore struct {
	// Concurrency is the number of concurrent workers to run during restore.
	// Concurrency=1 is used by default.
	Concurrency int

	// Src is the source containing backed up data.
	Src common.RemoteFS

	// Dst is destination to restore the data.
	//
	// If dst points to existing directory, then incremental restore is performed,
	// i.e. only new data is downloaded from src.
	Dst *fslocal.FS
}

// Run runs r with the provided settings.
func (r *Restore) Run() error {
	startTime := time.Now()

	concurrency := r.Concurrency
	src := r.Src
	dst := r.Dst
	logger.Infof("starting restore from %s to %s", src, dst)

	logger.Infof("obtaining list of parts at %s", src)
	srcParts, err := src.ListParts()
	if err != nil {
		return fmt.Errorf("cannot list src parts: %s", err)
	}
	logger.Infof("obtaining list of parts at %s", dst)
	dstParts, err := dst.ListParts()
	if err != nil {
		return fmt.Errorf("cannot list dst parts: %s", err)
	}

	backupSize := getPartsSize(srcParts)

	// Validate srcParts. They must cover the whole files.
	common.SortParts(srcParts)
	offset := uint64(0)
	var pOld common.Part
	var path string
	for _, p := range srcParts {
		if p.Path != path {
			if offset != pOld.FileSize {
				return fmt.Errorf("invalid size for %q; got %d; want %d", path, offset, pOld.FileSize)
			}
			pOld = p
			path = p.Path
			offset = 0
		}
		if p.Offset < offset {
			return fmt.Errorf("there is an overlap in %d bytes between %s and %s", offset-p.Offset, &pOld, &p)
		}
		if p.Offset > offset {
			if offset == 0 {
				return fmt.Errorf("there is a gap in %d bytes from file start to %s", p.Offset, &p)
			}
			return fmt.Errorf("there is a gap in %d bytes between %s and %s", p.Offset-offset, &pOld, &p)
		}
		if p.Size != p.ActualSize {
			return fmt.Errorf("invalid size for %s; got %d; want %d", &p, p.ActualSize, p.Size)
		}
		offset += p.Size
	}

	partsToDelete := common.PartsDifference(dstParts, srcParts)
	deleteSize := uint64(0)
	if len(partsToDelete) > 0 {
		// Fully remove local file if certain parts from the remote part are missing.
		pathsToDelete := make(map[string]bool)
		for _, p := range partsToDelete {
			pathsToDelete[p.Path] = true
		}
		logger.Infof("deleting %d files from %s", len(pathsToDelete), dst)
		for path := range pathsToDelete {
			logger.Infof("deleting %s from %s", path, dst)
			size, err := dst.DeletePath(path)
			if err != nil {
				return fmt.Errorf("cannot delete %s from %s: %s", path, dst, err)
			}
			deleteSize += size
		}
		if err != nil {
			return err
		}
		if err := dst.RemoveEmptyDirs(); err != nil {
			return fmt.Errorf("cannot remove empty directories at %s: %s", dst, err)
		}
	}

	// Re-read dstParts, since additional parts may be removed on the previous step.
	dstParts, err = dst.ListParts()
	if err != nil {
		return fmt.Errorf("cannot list dst parts after the deletion: %s", err)
	}

	partsToCopy := common.PartsDifference(srcParts, dstParts)
	downloadSize := getPartsSize(partsToCopy)
	if len(partsToCopy) > 0 {
		perPath := make(map[string][]common.Part)
		for _, p := range partsToCopy {
			parts := perPath[p.Path]
			parts = append(parts, p)
			perPath[p.Path] = parts
		}
		logger.Infof("downloading %d parts from %s to %s", len(partsToCopy), src, dst)
		bytesDownloaded := uint64(0)
		err = runParallelPerPath(concurrency, perPath, func(parts []common.Part) error {
			// Sort partsToCopy in order to properly grow file size during downloading.
			common.SortParts(parts)
			for _, p := range parts {
				logger.Infof("downloading %s from %s to %s", &p, src, dst)
				wc, err := dst.NewWriteCloser(p)
				if err != nil {
					return fmt.Errorf("cannot create writer for %q to %s: %s", &p, dst, err)
				}
				sw := &statWriter{
					w:            wc,
					bytesWritten: &bytesDownloaded,
				}
				if err := src.DownloadPart(p, sw); err != nil {
					return fmt.Errorf("cannot download %s to %s: %s", &p, dst, err)
				}
				if err := wc.Close(); err != nil {
					return fmt.Errorf("cannot close reader fro %s from %s: %s", &p, src, err)
				}
			}
			return nil
		}, func(elapsed time.Duration) {
			n := atomic.LoadUint64(&bytesDownloaded)
			logger.Infof("downloaded %d out of %d bytes from %s to %s in %s", n, downloadSize, src, dst, elapsed)
		})
		if err != nil {
			return err
		}
	}

	logger.Infof("restored %d bytes from backup in %s; deleted %d bytes; downloaded %d bytes", backupSize, time.Since(startTime), deleteSize, downloadSize)

	return nil
}

type statWriter struct {
	w            io.Writer
	bytesWritten *uint64
}

func (sw *statWriter) Write(p []byte) (int, error) {
	n, err := sw.w.Write(p)
	atomic.AddUint64(sw.bytesWritten, uint64(n))
	return n, err
}
