package content

import (
	"cloud.google.com/go/storage"
	"context"
	"distudio.com/mage"
	"distudio.com/page"
	"google.golang.org/appengine/log"
	"net/http"
)

type fileHandler struct {
	page.BaseRestHandler
}

func (handler fileHandler) HandlePost(ctx context.Context, out *mage.ResponseOutput) mage.Redirect {

	res, err := handler.Manager.NewResource(ctx)
	if err != nil {
		return handler.ErrorToStatus(ctx, err, out)
	}

	if err = handler.Manager.Create(ctx, res, nil); err != nil {
		return handler.ErrorToStatus(ctx, err, out)
	}

	renderer := mage.JSONRenderer{}
	renderer.Data = res
	out.Renderer = &renderer
	return mage.Redirect{Status: http.StatusCreated}
}

// Converts an error to its equivalent HTTP representation
func (handler fileHandler) ErrorToStatus(ctx context.Context, err error, out *mage.ResponseOutput) mage.Redirect {
	if err == storage.ErrObjectNotExist {
		log.Errorf(ctx, "%s", err.Error())
		return mage.Redirect{Status: http.StatusNotFound}
	}
	return handler.BaseRestHandler.ErrorToStatus(ctx, err, out)
}
