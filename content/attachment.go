package content

import (
	"distudio.com/mage/model"
	"distudio.com/page"
	"encoding/json"
	"time"
)

const (
	// global name for attachments without parents
	AttachmentGlobalParent = "GLOBAL"

	// supported attachments
	AttachmentTypeGallery    = "gallery"
	AttachmentTypeAttachment = "attachments"
	AttachmentTypeVideo      = "video"
)

type Attachment struct {
	model.Model `json:"-"`
	Name        string    `json:"name"`
	Description string    `json:"description";model:"noindex"`
	ResourceUrl string    `json:"resourceUrl";model:"noindex"`
	Group       string    `json:"group"`
	Type        string    `json:"type"`
	Parent      string    `json:"parent"`
	Created     time.Time `json:"created"`
	Updated     time.Time `json:"updated"`
	Uploader    string    `json:"uploader"`
	AltText     string    `json:"altText"`
	Seo         int64     `json:"seo"`
}

func (attachment *Attachment) UnmarshalJSON(data []byte) error {

	alias := struct {
		Name        string    `json:"name"`
		Description string    `json:"description"`
		ResourceUrl string    `json:"resourceUrl"`
		Group       string    `json:"group"`
		Type        string    `json:"type"`
		Parent      string    `json:"parent"`
		Created     time.Time `json:"created"`
		Updated     time.Time `json:"updated"`
		Uploader    string    `json:"uploader"`
		AltText     string    `json:"altText"`
		Seo         int64     `json:"seo"`
	}{}

	err := json.Unmarshal(data, &alias)
	if err != nil {
		return err
	}

	attachment.Name = alias.Name
	attachment.Description = alias.Description
	attachment.ResourceUrl = alias.ResourceUrl
	attachment.Group = alias.Group
	attachment.Type = alias.Type
	attachment.Parent = alias.Parent
	attachment.Created = alias.Created
	attachment.Updated = alias.Updated
	attachment.Uploader = alias.Uploader
	attachment.AltText = alias.AltText
	attachment.Seo = alias.Seo

	return nil
}

func (attachment *Attachment) MarshalJSON() ([]byte, error) {
	type Alias struct {
		Name        string    `json:"name"`
		Description string    `json:"description"`
		ResourceUrl string    `json:"resourceUrl"`
		Group       string    `json:"group"`
		Type        string    `json:"type"`
		Parent      string    `json:"parent"`
		Created     time.Time `json:"created"`
		Updated     time.Time `json:"updated"`
		Uploader    string    `json:"uploader"`
		AltText     string    `json:"altText"`
		Id          int64     `json:"id"`
		Seo         int64     `json:"seo"`
	}

	return json.Marshal(&struct {
		Alias
	}{
		Alias{
			Name:        attachment.Name,
			Description: attachment.Description,
			ResourceUrl: attachment.ResourceUrl,
			Group:       attachment.Group,
			Type:        attachment.Type,
			Parent:      attachment.Parent,
			Created:     attachment.Created,
			Updated:     attachment.Updated,
			Uploader:    attachment.Uploader,
			AltText:     attachment.AltText,
			Id:          attachment.IntID(),
			Seo:         attachment.Seo,
		},
	})
}

func (attachment *Attachment) Id() string {
	return attachment.StringID()
}

func (attachment *Attachment) FromRepresentation(rtype page.RepresentationType, data []byte) error {
	switch rtype {
	case page.RepresentationTypeJSON:
		return json.Unmarshal(data, attachment)
	}
	return page.NewUnsupportedError()
}

func (attachment *Attachment) ToRepresentation(rtype page.RepresentationType) ([]byte, error) {
	switch rtype {
	case page.RepresentationTypeJSON:
		return json.Marshal(attachment)
	}
	return nil, page.NewUnsupportedError()
}
