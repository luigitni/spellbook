package content

import (
	"cloud.google.com/go/storage"
	"context"
	"decodica.com/flamel"
	"decodica.com/spellbook"
	"errors"
	"fmt"
	"github.com/disintegration/imaging"
	"google.golang.org/api/iterator"
	"google.golang.org/appengine/file"
	"google.golang.org/appengine/log"
	"strings"
)

func NewFileController() *spellbook.RestController {
	return NewFileControllerWithKey("")
}

func NewFileControllerWithKey(key string) *spellbook.RestController {
	handler := fileHandler{spellbook.BaseRestHandler{Manager: fileManager{}}}
	c := spellbook.NewRestController(handler)
	c.Key = key
	return c
}

type fileManager struct{}

func (manager fileManager) BucketName(ctx context.Context) (string, error) {
	bucket := spellbook.Application().Options().Bucket
	if bucket == "" {
		b, err := file.DefaultBucketName(ctx)
		if err != nil {
			return "", fmt.Errorf("error retrieving default bucket %s", err.Error())
		}
		bucket = b
	}
	return bucket, nil
}

func (manager fileManager) NewResource(ctx context.Context) (spellbook.Resource, error) {
	return &File{}, nil
}

func (manager fileManager) FromId(ctx context.Context, id string) (spellbook.Resource, error) {
	if current := spellbook.IdentityFromContext(ctx); current == nil || (!current.HasPermission(spellbook.PermissionReadContent) && !current.HasPermission(spellbook.PermissionReadMedia)) {
		var p spellbook.Permission
		p = spellbook.PermissionReadContent
		if !current.HasPermission(spellbook.PermissionReadMedia) {
			p = spellbook.PermissionReadMedia
		}
		return nil, spellbook.NewPermissionError(spellbook.PermissionName(p))
	}

	bucket, err := manager.BucketName(ctx)
	if err != nil {
		return nil, err
	}

	res, _ := manager.NewResource(ctx)
	f := res.(*File)
	f.Name = id
	f.ResourceUrl = fmt.Sprintf(publicURL, bucket, id)

	return f, nil
}

func (manager fileManager) ListOf(ctx context.Context, opts spellbook.ListOptions) ([]spellbook.Resource, error) {

	if current := spellbook.IdentityFromContext(ctx); current == nil || (!current.HasPermission(spellbook.PermissionReadContent) && !current.HasPermission(spellbook.PermissionReadMedia)) {
		var p spellbook.Permission
		p = spellbook.PermissionReadContent
		if !current.HasPermission(spellbook.PermissionReadMedia) {
			p = spellbook.PermissionReadMedia
		}
		return nil, spellbook.NewPermissionError(spellbook.PermissionName(p))
	}

	bucket, err := manager.BucketName(ctx)
	if err != nil {
		return nil, err
	}

	client, err := storage.NewClient(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to create client: %s", err.Error())
	}
	defer client.Close()

	var files []*File

	handle := client.Bucket(bucket)

	q := &storage.Query{}
	q.Versions = false

	it := handle.Objects(ctx, q)
	objs, err := manager.listPagination(ctx, it, opts)
	if err != nil {
		log.Errorf(ctx, "listBucket: unable to list bucket %q: %v", bucket, err)
		return nil, err
	}
	for _, obj := range objs {
		name := obj.Name
		s := strings.Split(obj.Name, "/")
		if len(s) > 0 {
			name = s[len(s)-1]
		}
		// create file resource
		res, _ := manager.NewResource(ctx)
		f := res.(*File)
		f.Name = name
		f.ResourceUrl = obj.MediaLink
		f.ContentType = obj.ContentType
		// append file
		files = append(files, f)
	}

	resources := make([]spellbook.Resource, len(files))
	for i := range files {
		resources[i] = files[i]
	}

	return resources, nil
}

func (manager fileManager) listPagination(ctx context.Context, it *storage.ObjectIterator, opts spellbook.ListOptions) ([]*storage.ObjectAttrs, error) {
	p := iterator.NewPager(it, opts.Size+1, "")
	var objs []*storage.ObjectAttrs
	for i := 0; i < opts.Page+1; i++ {
		objs = make([]*storage.ObjectAttrs, 0, 0)
		nextPageToken, err := p.NextPage(&objs)
		if err != nil {
			return nil, err
		}
		if nextPageToken == "" {
			// end pagination
			if i != opts.Page {
				// page requested is out of bound
				objs = make([]*storage.ObjectAttrs, 0, 0)
			}
			break
		}
	}
	return objs, nil
}

func (manager fileManager) ListOfProperties(ctx context.Context, opts spellbook.ListOptions) ([]string, error) {
	return nil, spellbook.NewUnsupportedError()
}

func (manager fileManager) Create(ctx context.Context, res spellbook.Resource, bundle []byte) error {

	if current := spellbook.IdentityFromContext(ctx); current == nil || (!current.HasPermission(spellbook.PermissionWriteContent) && !current.HasPermission(spellbook.PermissionWriteMedia)) {
		var p spellbook.Permission
		p = spellbook.PermissionWriteContent
		if !current.HasPermission(spellbook.PermissionWriteMedia) {
			p = spellbook.PermissionWriteMedia
		}
		return spellbook.NewPermissionError(spellbook.PermissionName(p))
	}

	rfile := res.(*File)

	ins := flamel.InputsFromContext(ctx)

	tv := spellbook.NewField("type", true, ins)
	tv.AddValidator(spellbook.FileNameValidator{})
	typ, err := tv.Value()
	if err != nil {
		return spellbook.NewFieldError("type", err)
	}

	// namespace is the sub folder where the file will be loaded
	nsv := spellbook.NewField("namespace", false, ins)
	nsv.AddValidator(spellbook.FileNameValidator{AllowEmpty: true})
	namespace, err := nsv.Value()
	if err != nil {
		return spellbook.NewFieldError("namespace", err)
	}

	// prepend a slash to build the filename
	if namespace != "" {
		namespace = fmt.Sprintf("/%s", namespace)
	}

	nv := spellbook.NewField("name", true, ins)
	nv.AddValidator(spellbook.FileNameValidator{})
	name, err := nv.Value()
	if err != nil {
		return spellbook.NewFieldError("name", err)
	}

	// get the file headers
	fhs := ins["file"].Files()
	if len(fhs) == 0 {
		msg := fmt.Sprintf("error no file: %v", fhs)
		return spellbook.NewFieldError("no file", errors.New(msg))
	}
	// todo: handle multiple files
	fh := fhs[0]
	f, err := fh.Open()
	defer f.Close()

	buffer := make([]byte, fh.Size)
	if err != nil {
		msg := fmt.Sprintf("error buffer: %s", err.Error())
		return spellbook.NewFieldError("buffer", errors.New(msg))
	}

	_, err = f.Read(buffer)
	if err == nil {
		// reset the buffer
		_, err = f.Seek(0, 0)
		if err != nil {
			msg := fmt.Sprintf("error Seek buffer: %s", err.Error())
			return spellbook.NewFieldError("buffer", errors.New(msg))
		}
	} else {
		return spellbook.NewFieldError("read", err)
	}

	// build the filename
	filename := fmt.Sprintf("%s%s/%s", typ, namespace, name)

	// handle the upload to Google Cloud Storage
	bucket, err := manager.BucketName(ctx)
	if err != nil {
		return err
	}

	client, err := storage.NewClient(ctx)
	if err != nil {
		msg := fmt.Sprintf("failed to create client: %s", err.Error())
		return spellbook.NewFieldError("client", errors.New(msg))
	}
	defer client.Close()

	handle := client.Bucket(bucket)
	writer := handle.Object(filename).NewWriter(ctx)
	writer.ContentType = fh.Header.Get("Content-Type")

	if _, err := writer.Write(buffer); err != nil {
		msg := fmt.Sprintf("upload: unable to write file %s to bucket %s: %s", filename, bucket, err.Error())
		return spellbook.NewFieldError("bucket", errors.New(msg))
	}

	if err := writer.Close(); err != nil {
		msg := fmt.Sprintf("upload: unable to close bucket %s: %s", bucket, err.Error())
		return spellbook.NewFieldError("bucket", errors.New(msg))
	}

	uri := fmt.Sprintf(publicURL, bucket, filename)

	rfile.ResourceUrl = uri
	rfile.Name = name

	// -----------------------------thumbnail
	if strings.Contains(writer.ContentType, "image/") {
		// get image
		image, err := imaging.Decode(f)
		if err != nil {
			msg := fmt.Sprintf("error in opening image %s", err)
			return spellbook.NewFieldError("bucket", errors.New(msg))
		}
		// create thumbnail
		fileNameThumbnail := fmt.Sprintf("%s%s/thumb/%s", typ, namespace, name)
		afterImage := imaging.Thumbnail(image, 100, 100, imaging.Linear)
		// Save thumbnail
		wc := handle.Object(fileNameThumbnail).NewWriter(ctx)
		wc.ContentType = fh.Header.Get("Content-Type")
		if imaging.Encode(wc, afterImage, imaging.JPEG); err != nil {
			msg := fmt.Sprintf("%s in saving image thumbnail", err.Error())
			return spellbook.NewFieldError("bucket", errors.New(msg))
		}
		if err = wc.Close(); err != nil {
			msg := fmt.Sprintf("CreateFileThumbnail: unable to close bucket %q, file %q: %v", bucket, fileNameThumbnail, err)
			return spellbook.NewFieldError("bucket", errors.New(msg))
		}

		uriThumb := fmt.Sprintf(publicURL, bucket, fileNameThumbnail)
		rfile.ResourceThumbUrl = uriThumb
	}

	return nil
}

func (manager fileManager) Update(ctx context.Context, res spellbook.Resource, bundle []byte) error {
	return spellbook.NewUnsupportedError()
}

func (manager fileManager) Delete(ctx context.Context, res spellbook.Resource) error {
	return spellbook.NewUnsupportedError()
}
