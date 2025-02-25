package gcsremote

import (
	"context"
	"fmt"
	"io"
	"strings"

	"cloud.google.com/go/storage"
	"github.com/VictoriaMetrics/VictoriaMetrics/lib/backup/common"
	"github.com/VictoriaMetrics/VictoriaMetrics/lib/logger"
	"google.golang.org/api/iterator"
	"google.golang.org/api/option"
)

// FS represents filesystem for backups in GCS.
//
// Init must be called before calling other FS methods.
type FS struct {
	// Path to GCP credentials file.
	//
	// Default credentials are used if empty.
	CredsFilePath string

	// GCS bucket to use.
	Bucket string

	// Directory in the bucket to write to.
	Dir string

	bkt *storage.BucketHandle
}

// Init initializes fs.
func (fs *FS) Init() error {
	if fs.bkt != nil {
		logger.Panicf("BUG: fs.Init has been already called")
	}
	for strings.HasPrefix(fs.Dir, "/") {
		fs.Dir = fs.Dir[1:]
	}
	if !strings.HasSuffix(fs.Dir, "/") {
		fs.Dir += "/"
	}
	ctx := context.Background()
	var client *storage.Client
	if len(fs.CredsFilePath) > 0 {
		creds := option.WithCredentialsFile(fs.CredsFilePath)
		c, err := storage.NewClient(ctx, creds)
		if err != nil {
			return fmt.Errorf("cannot create gcs client with credsFile %q: %s", fs.CredsFilePath, err)
		}
		client = c
	} else {
		c, err := storage.NewClient(ctx)
		if err != nil {
			return fmt.Errorf("cannot create default gcs client: %q", err)
		}
		client = c
	}
	fs.bkt = client.Bucket(fs.Bucket)
	return nil
}

// String returns human-readable description for fs.
func (fs *FS) String() string {
	return fmt.Sprintf("GCS{bucket: %q, dir: %q}", fs.Bucket, fs.Dir)
}

// ListParts returns all the parts for fs.
func (fs *FS) ListParts() ([]common.Part, error) {
	dir := fs.Dir
	ctx := context.Background()
	q := &storage.Query{
		Prefix: dir,
	}
	it := fs.bkt.Objects(ctx, q)
	var parts []common.Part
	for {
		attr, err := it.Next()
		if err == iterator.Done {
			return parts, nil
		}
		if err != nil {
			return nil, fmt.Errorf("error when iterating objects at %q: %s", dir, err)
		}
		file := attr.Name
		if !strings.HasPrefix(file, dir) {
			return nil, fmt.Errorf("unexpected prefix for gcs key %q; want %q", file, dir)
		}
		var p common.Part
		if !p.ParseFromRemotePath(file[len(dir):]) {
			logger.Infof("skipping unknown object %q", file)
			continue
		}
		p.ActualSize = uint64(attr.Size)
		parts = append(parts, p)
	}
}

// DeletePart deletes part p from fs.
func (fs *FS) DeletePart(p common.Part) error {
	o := fs.object(p)
	ctx := context.Background()
	if err := o.Delete(ctx); err != nil {
		return fmt.Errorf("cannot delete %q at %s (remote path %q): %s", p.Path, fs, o.ObjectName(), err)
	}
	return nil
}

// RemoveEmptyDirs recursively removes empty dirs in fs.
func (fs *FS) RemoveEmptyDirs() error {
	// GCS has no directories, so nothing to remove.
	return nil
}

// CopyPart copies p from srcFS to fs.
func (fs *FS) CopyPart(srcFS common.OriginFS, p common.Part) error {
	src, ok := srcFS.(*FS)
	if !ok {
		return fmt.Errorf("cannot perform server-side copying from %s to %s: both of them must be GCS", srcFS, fs)
	}
	srcObj := src.object(p)
	dstObj := fs.object(p)

	copier := dstObj.CopierFrom(srcObj)
	ctx := context.Background()
	attr, err := copier.Run(ctx)
	if err != nil {
		return fmt.Errorf("cannot copy %q from %s to %s: %s", p.Path, src, fs, err)
	}
	if uint64(attr.Size) != p.Size {
		return fmt.Errorf("unexpected %q size after copying from %s to %s; got %d bytes; want %d bytes", p.Path, src, fs, attr.Size, p.Size)
	}
	return nil
}

// DownloadPart downloads part p from fs to w.
func (fs *FS) DownloadPart(p common.Part, w io.Writer) error {
	o := fs.object(p)
	ctx := context.Background()
	r, err := o.NewReader(ctx)
	if err != nil {
		return fmt.Errorf("cannot open reader for %q at %s (remote path %q): %s", p.Path, fs, o.ObjectName(), err)
	}
	n, err := io.Copy(w, r)
	if err1 := r.Close(); err1 != nil && err == nil {
		err = err1
	}
	if err != nil {
		return fmt.Errorf("cannot download %q from at %s (remote path %q): %s", p.Path, fs, o.ObjectName(), err)
	}
	if uint64(n) != p.Size {
		return fmt.Errorf("wrong data size downloaded from %q at %s; got %d bytes; want %d bytes", p.Path, fs, n, p.Size)
	}
	return nil
}

// UploadPart uploads part p from r to fs.
func (fs *FS) UploadPart(p common.Part, r io.Reader) error {
	o := fs.object(p)
	ctx := context.Background()
	w := o.NewWriter(ctx)
	n, err := io.Copy(w, r)
	if err1 := w.Close(); err1 != nil && err == nil {
		err = err1
	}
	if err != nil {
		return fmt.Errorf("cannot upload data to %q at %s (remote path %q): %s", p.Path, fs, o.ObjectName(), err)
	}
	if uint64(n) != p.Size {
		return fmt.Errorf("wrong data size uploaded to %q at %s; got %d bytes; want %d bytes", p.Path, fs, n, p.Size)
	}
	return nil
}

func (fs *FS) object(p common.Part) *storage.ObjectHandle {
	path := p.RemotePath(fs.Dir)
	return fs.bkt.Object(path)
}
