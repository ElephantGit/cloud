package server_test

import "testing"

func TestSpacesRoute(t *testing.T) {
	f := newSquadServer(t)
	c := newContractClient(t, f.srv.URL)
	c.login()
	tenant := c.tenant()

	status, body := c.do("GET", "/api/v1/tenants/"+tenant+"/spaces", nil)
	if status != 200 {
		t.Fatalf("spaces = %d %v", status, body)
	}
	items, _ := body["items"].([]any)
	if len(items) != 1 {
		t.Fatalf("spaces = %v", body)
	}
	space := items[0].(map[string]any)
	if space["tenantId"] != tenant || space["slug"] != "demo" || space["role"] != "owner" {
		t.Fatalf("space = %v", space)
	}
	spaceID, _ := space["id"].(string)

	// A caller outside the workspace is refused rather than told the list is empty.
	if status, body := c.do("GET", "/api/v1/tenants/00000000-0000-0000-0000-000000000001/spaces", nil); status != 403 || body["code"] != "forbidden" {
		t.Fatalf("foreign spaces = %d %v", status, body)
	}
	if status, body := c.do("GET", "/api/v1/tenants/not-a-uuid/spaces", nil); status != 404 || body["code"] != "not_found" {
		t.Fatalf("malformed spaces = %d %v", status, body)
	}

	status, body = c.do("GET", "/api/v1/tenants/"+tenant+"/spaces/"+spaceID+"/plugins", nil)
	if status != 200 {
		t.Fatalf("plugins = %d %v", status, body)
	}
	items, _ = body["items"].([]any)
	if len(items) != 2 {
		t.Fatalf("plugins = %v", body)
	}
	for i, want := range []struct{ identifier, version string }{{"ora-space.claude", "0.4.5"}, {"ora-space.opencode", "0.6.11"}} {
		plugin := items[i].(map[string]any)
		if plugin["spaceId"] != spaceID || plugin["tenantId"] != tenant || plugin["kind"] != "agent" {
			t.Fatalf("plugin scope = %v", plugin)
		}
		if plugin["sourceNamespace"] != "official" || plugin["identifier"] != want.identifier {
			t.Fatalf("plugin identity = %v", plugin)
		}
		if plugin["observedState"] != "installed" || plugin["observedVersion"] != want.version {
			t.Fatalf("plugin state = %v", plugin)
		}
	}

	// A space outside the caller's workspace answers forbidden, like an unknown one.
	if status, body := c.do("GET", "/api/v1/tenants/"+tenant+"/spaces/00000000-0000-0000-0000-000000000001/plugins", nil); status != 403 || body["code"] != "forbidden" {
		t.Fatalf("foreign plugins = %d %v", status, body)
	}
	if status, body := c.do("GET", "/api/v1/tenants/"+tenant+"/spaces/not-a-uuid/plugins", nil); status != 404 || body["code"] != "not_found" {
		t.Fatalf("malformed plugins = %d %v", status, body)
	}
}
