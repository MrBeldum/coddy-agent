//go:build http && memory

package httpserver

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"testing"

	"github.com/EvilFreelancer/coddy-agent/internal/acp"
)

func TestMemoryTreeRejectsTraversal(t *testing.T) {
	mgr, srv, _ := testHTTPServerPersist(t)
	ctx := context.Background()
	nr, err := mgr.HandleSessionNew(ctx, acp.SessionNewParams{CWD: "/tmp"})
	if err != nil {
		t.Fatal(err)
	}
	sid := nr.SessionID
	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()

	u := ts.URL + "/coddy/sessions/" + sid + "/memory/tree?root=global&path=" + url.QueryEscape("../etc/passwd")
	r, err := http.Get(u)
	if err != nil {
		t.Fatal(err)
	}
	b, err := ioReadAllClose(r.Body)
	if err != nil {
		t.Fatal(err)
	}
	if r.StatusCode != http.StatusBadRequest {
		t.Fatalf("want 400 got %d: %s", r.StatusCode, b)
	}
}

func TestMemoryFileDeleteRejectsMemoryRoot(t *testing.T) {
	mgr, srv, _ := testHTTPServerPersist(t)
	ctx := context.Background()
	nr, err := mgr.HandleSessionNew(ctx, acp.SessionNewParams{CWD: "/tmp"})
	if err != nil {
		t.Fatal(err)
	}
	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()

	req, err := http.NewRequest(http.MethodDelete,
		ts.URL+"/coddy/sessions/"+nr.SessionID+"/memory/file?root=global&path=", nil)
	if err != nil {
		t.Fatal(err)
	}
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	b, err := ioReadAllClose(res.Body)
	if err != nil {
		t.Fatal(err)
	}
	if res.StatusCode != http.StatusBadRequest {
		t.Fatalf("want 400 got %d: %s", res.StatusCode, b)
	}
	if _, err := os.Stat(srv.coddyPaths().globalMemoryRoot); err != nil {
		t.Fatalf("memory root was removed: %v", err)
	}
}
