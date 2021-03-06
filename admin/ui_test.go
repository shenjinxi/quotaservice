package admin

import (
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
)

func TestGetDevelopment(t *testing.T) {
	a := NewMockAdministrable()
	handler := NewUIHandler(a, "public", true)
	ts := httptest.NewServer(handler)
	defer ts.Close()

	err := ioutil.WriteFile("public/tempfile.html", []byte("hello"), 777)

	if err != nil {
		t.Fatal(err)
	}

	defer os.Remove("public/tempfile.html")

	body := getUrl(ts.URL+"/admin/tempfile.html", t)

	if body != "hello" {
		t.Fatalf("development did not reload and catch /admin/tempfile:\n%s", body)
	}
}

func TestGet(t *testing.T) {
	handler := NewUIHandler(NewMockAdministrable(), "public", false)
	ts := httptest.NewServer(handler)
	defer ts.Close()

	body := getUrl(ts.URL+"/admin/", t)

	if !strings.HasPrefix(body, "<!doctype html>") {
		t.Fatalf("Received invalid html from /admin/:\n%s", body)
	}
}

func TestGetNotFound(t *testing.T) {
	handler := NewUIHandler(NewMockAdministrable(), "public", false)
	ts := httptest.NewServer(handler)
	defer ts.Close()

	body := getUrl(ts.URL+"/admin/thisdoesnotexist.html", t)

	if body != "404 page not found\n" {
		t.Fatalf("Did not receive 404 from /admin/thisdoesnotexist.html:\n\"%s\"", body)
	}
}

func getUrl(url string, t *testing.T) string {
	res, err := http.Get(url)

	if err != nil {
		t.Fatal(err)
	}

	bytes, err := ioutil.ReadAll(res.Body)
	res.Body.Close()

	if err != nil {
		t.Fatal(err)
	}

	return string(bytes)
}
