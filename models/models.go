// models/models.go
package models

type Video struct {
	Name     string
	Path     string
	FilePath string
}

type Folder struct {
	Name string
	Path string
}

type VideoURLListResponse struct {
	Folders []PublicVideoFolder `json:"folders"`
}

type PublicVideoFolder struct {
	Name   string        `json:"name"`
	Videos []PublicVideo `json:"videos"`
}

type PublicVideo struct {
	Name string `json:"name"`
	Path string `json:"path"`
}
