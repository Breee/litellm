package litellm

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
)

func newMCPServerTestData(t *testing.T) *schema.ResourceData {
	t.Helper()
	return schema.TestResourceDataRaw(t, resourceLiteLLMMCPServer().Schema, map[string]interface{}{
		"server_name": "gh",
		"transport":   "http",
		"url":         "https://mcp.example.com/mcp",
	})
}

func newMCPServerTestClient(t *testing.T, status int, body string) *Client {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(status)
		_, _ = w.Write([]byte(body))
	}))
	t.Cleanup(srv.Close)
	return NewClient(srv.URL, "test-key", true)
}

const mcpServerTestBody = `{"server_id":"srv-1","server_name":"gh","transport":"http","url":"https://mcp.example.com/mcp"}`

func TestMCPServerCRUDStatusCodes(t *testing.T) {
	tests := []struct {
		name    string
		op      func(*schema.ResourceData, interface{}) error
		status  int
		body    string
		wantErr bool
		wantID  string
	}{
		{name: "create 201", op: resourceLiteLLMMCPServerCreate, status: http.StatusCreated, body: mcpServerTestBody, wantID: "srv-1"},
		{name: "create 500", op: resourceLiteLLMMCPServerCreate, status: http.StatusInternalServerError, body: `{}`, wantErr: true, wantID: "existing"},
		{name: "update 202", op: resourceLiteLLMMCPServerUpdate, status: http.StatusAccepted, body: mcpServerTestBody, wantID: "existing"},
		{name: "update 400", op: resourceLiteLLMMCPServerUpdate, status: http.StatusBadRequest, body: `{}`, wantErr: true, wantID: "existing"},
		{name: "delete 202", op: resourceLiteLLMMCPServerDelete, status: http.StatusAccepted, body: "", wantID: ""},
		{name: "delete 500", op: resourceLiteLLMMCPServerDelete, status: http.StatusInternalServerError, body: "", wantErr: true, wantID: "existing"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			d := newMCPServerTestData(t)
			d.SetId("existing")

			err := tt.op(d, newMCPServerTestClient(t, tt.status, tt.body))

			if tt.wantErr != (err != nil) {
				t.Fatalf("wantErr=%v, got err=%v", tt.wantErr, err)
			}
			if d.Id() != tt.wantID {
				t.Fatalf("expected ID %q, got %q", tt.wantID, d.Id())
			}
		})
	}
}

func TestMCPServerReadDoesNotPersistServerEnv(t *testing.T) {
	d := schema.TestResourceDataRaw(t, resourceLiteLLMMCPServer().Schema, map[string]interface{}{
		"server_name": "gh",
		"transport":   "stdio",
		"command":     "npx",
		"env": map[string]interface{}{
			"GITHUB_TOKEN": "from-config",
		},
	})
	d.SetId("srv-1")

	resp := &MCPServerResponse{
		ServerID:   "srv-1",
		ServerName: "gh",
		Transport:  "stdio",
		Command:    "npx",
		Env: map[string]string{
			"GITHUB_TOKEN": "raw-from-server",
			"DB_PASSWORD":  "leaked-secret",
		},
	}
	if err := updateSchemaFromResponse(d, resp); err != nil {
		t.Fatalf("updateSchemaFromResponse failed: %v", err)
	}

	got := d.Get("env").(map[string]interface{})
	if got["GITHUB_TOKEN"] != "from-config" {
		t.Fatalf("config env overwritten by server response: %v", got)
	}
	if _, leaked := got["DB_PASSWORD"]; leaked {
		t.Fatalf("server-returned env var persisted into state: %v", got)
	}
	if d.Get("server_name").(string) != "gh" {
		t.Fatalf("read did not populate non-sensitive fields")
	}
}
