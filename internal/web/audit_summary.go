package web

import (
	"encoding/json"

	"github.com/septi0/dockvmap/internal/model"
	"github.com/septi0/dockvmap/internal/oci"
	"github.com/septi0/dockvmap/internal/service"
)

type auditLogResponse struct {
	model.AuditLog
	Summary string `json:"summary,omitempty"`
}

type auditFields struct {
	Name       string `json:"name"`
	OldName    string `json:"oldName"`
	NewName    string `json:"newName"`
	Tag        string `json:"tag"`
	OldTag     string `json:"oldTag"`
	NewTag     string `json:"newTag"`
	Group      string `json:"group"`
	Pinned     bool   `json:"pinned"`
	Registry   string `json:"registry"`
	Repository string `json:"repository"`
	Reference  string `json:"reference"`
	Label      string `json:"label"`
	CanPush    bool   `json:"canPush"`
	Username   string `json:"username"`
}

func newAuditLogResponses(logs []model.AuditLog) []auditLogResponse {
	responses := make([]auditLogResponse, 0, len(logs))

	for _, entry := range logs {
		responses = append(responses, auditLogResponse{AuditLog: entry, Summary: auditSummary(entry)})
	}

	return responses
}

func auditSummary(entry model.AuditLog) string {
	var f auditFields

	if len(entry.Data) == 0 || json.Unmarshal(entry.Data, &f) != nil {
		return ""
	}

	switch entry.Type {
	case service.AuditTypeImageCreated:
		return f.Name + ": " + f.Registry + "/" + f.Repository + ":" + f.Tag

	case service.AuditTypeImageTagChanged:
		return inGroup(f.Name+": "+f.OldTag+" → "+f.NewTag, f.Group)

	case service.AuditTypeImageRenamed:
		return f.OldName + " → " + f.NewName

	case service.AuditTypeImagePinChanged:
		if f.Pinned {
			return inGroup(f.Name+" pinned", f.Group)
		}

		return inGroup(f.Name+" unpinned", f.Group)

	case service.AuditTypeImageGroupUpdated, service.AuditTypeHostUpdated:
		if f.OldName != "" && f.OldName != f.Name {
			return f.OldName + " → " + f.Name
		}

		return f.Name

	case service.AuditTypeImageDeleted, service.AuditTypeImageGroupCreated, service.AuditTypeImageGroupDeleted,
		service.AuditTypeHostCreated, service.AuditTypeHostAgentTrusted, service.AuditTypeHostDeleted:
		return f.Name

	case service.AuditTypeRegistryCreated, service.AuditTypeRegistryUpdated, service.AuditTypeRegistryDeleted:
		return f.Registry

	case service.AuditTypeReplicationTokenCreated, service.AuditTypeReplicationTokenDeleted:
		return f.Label

	case service.AuditTypeProxyTokenCreated, service.AuditTypeProxyTokenDeleted:
		if f.CanPush {
			return f.Label + " (can push)"
		}

		return f.Label

	case service.AuditTypeImagePushed:
		return f.Repository + ":" + f.Tag

	case service.AuditTypeManifestDeleted:
		if oci.IsDigest(f.Reference) {
			return f.Repository + "@" + f.Reference
		}

		return f.Repository + ":" + f.Reference

	default:
		if f.Username == entry.Username {
			return ""
		}

		return f.Username
	}
}

func inGroup(summary, group string) string {
	if group == "" {
		return summary
	}

	return summary + " (" + group + " group)"
}
