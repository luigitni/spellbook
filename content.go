package page

import (
	"distudio.com/mage"
	"distudio.com/mage/model"
	"distudio.com/page/content"
	"distudio.com/page/identity"
	"distudio.com/page/validators"
	"encoding/json"
	"errors"
	"fmt"
	"golang.org/x/net/context"
	"google.golang.org/appengine/datastore"
	"google.golang.org/appengine/log"
	"net/http"
	"net/url"
	"reflect"
	"strconv"
	"time"
)

type ContentController struct {
	mage.Controller
}

func (controller *ContentController) OnDestroy(ctx context.Context) {}

func (controller *ContentController) Process(ctx context.Context, out *mage.ResponseOutput) mage.Redirect {
	ins := mage.InputsFromContext(ctx)
	method := ins[mage.KeyRequestMethod].Value()
	switch method {
	case http.MethodPost:
		u := ctx.Value(identity.KeyUser)
		user, ok := u.(identity.User)
		if !ok {
			return mage.Redirect{Status: http.StatusUnauthorized}
		}

		if !user.HasPermission(identity.PermissionCreateContent) {
			return mage.Redirect{Status: http.StatusForbidden}
		}

		// get the p data
		j, ok := ins[mage.KeyRequestJSON]
		if !ok {
			return mage.Redirect{Status: http.StatusBadRequest}
		}

		errs := validators.Errors{}

		thecontent := content.Content{}
		err := json.Unmarshal([]byte(j.Value()), &thecontent)
		if err != nil {
			msg := fmt.Sprintf("bad json: %s", err.Error())
			errs.AddError("", errors.New(msg))
			log.Errorf(ctx, msg)
		}

		// check for client input erros
		if errs.HasErrors() {
			log.Errorf(ctx, "error HasErrors %+v", errs)
			renderer := mage.JSONRenderer{}
			renderer.Data = errs
			out.Renderer = &renderer
			return mage.Redirect{Status: http.StatusBadRequest}
		}

		thecontent.Created = time.Now().UTC()
		thecontent.Revision = 1
		if thecontent.Published != content.ZeroTime {
			thecontent.Published = time.Now().UTC()
		}
		// validate input fields

		if thecontent.Title == "" || thecontent.Name == "" {
			msg := fmt.Sprintf(" title and name can't be empty")
			errs.AddError("", errors.New(msg))
			log.Errorf(ctx, msg)
			renderer := mage.JSONRenderer{}
			renderer.Data = errs
			out.Renderer = &renderer
			return mage.Redirect{Status: http.StatusBadRequest}
		}

		if thecontent.Slug == "" {
			thecontent.Slug = url.PathEscape(thecontent.Title)
		}
		thecontent.Author = user.Username()

		// input is valid, create the resource
		opts := model.CreateOptions{}
		opts.WithStringId(thecontent.Slug)

		// // WARNING: the volatile field Multimedia because Memcache (Gob)
		//	can't ignore field
		tmp := thecontent.Attachments
		thecontent.Attachments = nil

		err = model.CreateWithOptions(ctx, &thecontent, &opts)
		if err != nil {
			log.Errorf(ctx, "error creating post %s: %s", thecontent.Slug, err)
			errs.AddError("", err)
			renderer := mage.JSONRenderer{}
			renderer.Data = errs
			out.Renderer = &renderer
			return mage.Redirect{Status: http.StatusInternalServerError}
		}

		// return the swapped multimedia value
		thecontent.Attachments = tmp
		renderer := mage.JSONRenderer{}
		renderer.Data = &thecontent
		out.Renderer = &renderer
		return mage.Redirect{Status: http.StatusCreated}
	case http.MethodGet:
		// check if current user has permission
		me := ctx.Value(identity.KeyUser)
		current, ok := me.(identity.User)

		if !ok {
			return mage.Redirect{Status: http.StatusUnauthorized}
		}

		if !current.HasPermission(identity.PermissionReadContent) {
			return mage.Redirect{Status: http.StatusForbidden}
		}

		params := mage.RoutingParams(ctx)
		// try to get the username.
		// if there is no param then it is a list request
		param, ok := params["slug"]
		if !ok {

			// handle query params for page data:
			page := 0
			size := 20
			if pin, ok := ins["page"]; ok {
				if num, err := strconv.Atoi(pin.Value()); err == nil {
					page = num
				} else {
					return mage.Redirect{Status: http.StatusBadRequest}
				}
			}

			if sin, ok := ins["results"]; ok {
				if num, err := strconv.Atoi(sin.Value()); err == nil {
					size = num
					// cap the size to 100
					if size > 100 {
						size = 100
					}
				} else {
					return mage.Redirect{Status: http.StatusBadRequest}
				}
			}

			var result interface{}
			l := 0
			// check property
			property, ok := ins["property"]
			if ok {
				// property
				properties, err := controller.HandleResourceProperties(ctx, property.Value(), page, size)
				if err != nil {
					log.Errorf(ctx, "Error retrieving posts %+v", err)
					return mage.Redirect{Status: http.StatusInternalServerError}
				}
				l = len(properties)
				result = properties[:controller.GetCorrectCountForPaging(size, l)]
			} else {
				// list posts
				var conts []*content.Content
				q := model.NewQuery(&content.Content{})
				q = q.OffsetBy(page * size)
				// get one more so we know if we are done
				q = q.Limit(size + 1)
				err := q.GetMulti(ctx, &conts)
				if err != nil {
					log.Errorf(ctx, "Error retrieving posts %+v", err)
					return mage.Redirect{Status: http.StatusInternalServerError}
				}
				l = len(conts)
				result = conts[:controller.GetCorrectCountForPaging(size, l)]
			}

			// todo: generalize list handling and responses
			response := struct {
				Items interface{} `json:"items"`
				More  bool        `json:"more"`
			}{result, l > size}
			renderer := mage.JSONRenderer{}
			renderer.Data = response
			out.Renderer = &renderer
			return mage.Redirect{Status: http.StatusOK}
		}

		slug := param.Value()
		item := content.Content{}
		err := model.FromStringID(ctx, &item, slug, nil)
		if err == datastore.ErrNoSuchEntity {
			return mage.Redirect{Status: http.StatusNotFound}
		}

		if err != nil {
			log.Errorf(ctx, "error retrieving p %s: %s", slug, err.Error())
			return mage.Redirect{Status: http.StatusInternalServerError}
		}

		// get post related multimedia


		q := model.NewQuery(&content.Attachment{})
		q.WithField("Parent =", item.Slug)
		err = q.GetMulti(ctx, &item.Attachments)
		if err != nil {
			log.Errorf(ctx, "error retrieving attachments: %s", err)
			return mage.Redirect{Status: http.StatusInternalServerError}
		}

		renderer := mage.JSONRenderer{}
		renderer.Data = &item
		out.Renderer = &renderer
		return mage.Redirect{Status: http.StatusOK}
	case http.MethodPut:
		me := ctx.Value(identity.KeyUser)
		current, ok := me.(identity.User)
		if !ok {
			return mage.Redirect{Status: http.StatusUnauthorized}
		}

		if !current.HasPermission(identity.PermissionEditContent) {
			return mage.Redirect{Status: http.StatusForbidden}
		}

		// control if the user has been specified
		params := mage.RoutingParams(ctx)
		param, ok := params["slug"]
		if !ok {
			return mage.Redirect{Status: http.StatusBadRequest}
		}

		j, ok := ins[mage.KeyRequestJSON]
		if !ok {
			return mage.Redirect{Status: http.StatusBadRequest}
		}

		// handle the json request
		jdata := j.Value()

		jpost := content.Content{}

		err := json.Unmarshal([]byte(jdata), &jpost)
		if err != nil {
			log.Errorf(ctx, "malformed json: %s", err.Error())
			return mage.Redirect{Status: http.StatusBadRequest}
		}

		// retrieve the user
		slug := param.Value()
		p := content.Content{}
		err = model.FromStringID(ctx, &p, slug, nil)
		if err == datastore.ErrNoSuchEntity {
			return mage.Redirect{Status: http.StatusNotFound}
		}

		if err != nil {
			return mage.Redirect{Status: http.StatusBadRequest}
		}

		p.Name = jpost.Name
		p.Title = jpost.Title
		p.Subtitle = jpost.Subtitle
		p.Category = jpost.Category
		p.Topic = jpost.Topic
		p.Locale = jpost.Locale
		p.Body = jpost.Body
		p.Cover = jpost.Cover
		p.Revision = jpost.Revision
		p.Updated = time.Now().UTC()
		p.Tags = jpost.Tags
		p.Author = current.Username()
		if jpost.Published == content.ZeroTime {
			// not setted
			p.Published = content.ZeroTime
		} else {
			// setted
			// check previous data
			if p.Published == content.ZeroTime {
				p.Published = time.Now().UTC()
			}
		}

		err = model.Update(ctx, &p)
		if err != nil {
			log.Errorf(ctx, "error updating p %s: %s", slug, err.Error())
			return mage.Redirect{Status: http.StatusInternalServerError}
		}

		// WARNING: value the volatile field Multimedia because Memcache (Gob)
		// can't ignore field
		p.Attachments = jpost.Attachments

		renderer := mage.JSONRenderer{}
		renderer.Data = &p
		out.Renderer = &renderer
		return mage.Redirect{Status: http.StatusOK}
	}

	return mage.Redirect{Status: http.StatusNotImplemented}
}

func (controller *ContentController) GetCorrectCountForPaging(size int, l int) int {
	count := size
	if l < size {
		count = l
	}
	return count
}

func (controller *ContentController) HandleResourceProperties(ctx context.Context, property string, page int, size int) ([]interface{}, error) {
	// todo: generalize
	name := ""
	switch property {
	case "category":
		name = "Category"
	case "topic":
		name = "Topic"
	case "name":
		name = "Name"
	default:
		return nil, errors.New("no property found")
	}

	var posts []*content.Content
	q := model.NewQuery(&content.Content{})
	q = q.OffsetBy(page * size)
	q = q.Distinct(name)
	// get one more so we know if we are done
	q = q.Limit(size + 1)
	err := q.GetAll(ctx, &posts)
	if err != nil {
		log.Errorf(ctx, "Error retrieving result: %+v", err)
		return nil, err
	}
	var result []interface{}
	for _, p := range posts {
		value := reflect.ValueOf(p).Elem().FieldByName(name).String()
		result = append(result, &value)
	}
	return result, nil

}