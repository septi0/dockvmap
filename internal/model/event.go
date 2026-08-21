package model

import "time"

type ImageEvent struct {
	ID          int64         `json:"id"`
	ImageID     int64         `json:"imageId"`
	ImageName   string        `json:"imageName"`
	Type        string        `json:"type"`
	Data        TagsEventData `json:"data"`
	CreatedAt   time.Time     `json:"createdAt"`
	Notify      bool          `json:"notify"`
	NotifSentAt *time.Time    `json:"notifSentAt,omitempty"`
}

type TagsEventData struct {
	Tags []string `json:"tags"`
}
