package web

import (
	"encoding/json"
	"testing"

	"github.com/septi0/dockvmap/internal/model"
	"github.com/septi0/dockvmap/internal/service"
)

func TestAuditSummary(t *testing.T) {
	cases := []struct {
		auditType string
		data      string
		want      string
	}{
		{service.AuditTypeImageTagChanged, `{"name":"redis","oldTag":"8.0.2","newTag":"8.0.3"}`, "redis: 8.0.2 → 8.0.3"},
		{service.AuditTypeImageTagChanged, `{"name":"loki","oldTag":"3.2.0","newTag":"3.3.1","group":"Grafana Loki"}`, "loki: 3.2.0 → 3.3.1 (Grafana Loki group)"},
		{service.AuditTypeImageCreated, `{"name":"redis","registry":"docker.io","repository":"library/redis","tag":"8.0.2"}`, "redis: docker.io/library/redis:8.0.2"},
		{service.AuditTypeHostUpdated, `{"name":"nas-2","oldName":"nas","address":"10.0.0.5:9002","oldAddress":"10.0.0.5:9002"}`, "nas → nas-2"},
		{service.AuditTypeHostUpdated, `{"name":"nas","oldName":"nas","address":"10.0.0.6:9002","oldAddress":"10.0.0.5:9002"}`, "nas"},
		{service.AuditTypeProxyTokenCreated, `{"label":"ci-runner","canPush":true}`, "ci-runner (can push)"},
		{service.AuditTypeImagePushed, `{"repository":"team/web","tag":"v3","token":"ci"}`, "team/web:v3"},
		{service.AuditTypeManifestDeleted, `{"repository":"team/web","reference":"sha256:` + "0000000000000000000000000000000000000000000000000000000000000000" + `"}`, "team/web@sha256:0000000000000000000000000000000000000000000000000000000000000000"},
		{service.AuditTypeUserLoginFailed, `{"username":"root"}`, "root"},
		{service.AuditTypeUserPasswordReset, `{"username":"admin"}`, ""},
		{service.AuditTypeUserLoggedIn, ``, ""},
	}

	for _, c := range cases {
		entry := model.AuditLog{Type: c.auditType, Data: json.RawMessage(c.data), Username: "admin"}

		if got := auditSummary(entry); got != c.want {
			t.Errorf("%s %s: summary %q, want %q", c.auditType, c.data, got, c.want)
		}
	}
}
