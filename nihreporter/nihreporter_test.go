package nihreporter

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

// mockProjectResponse builds a wireSearchResponse JSON body with the given projects.
func mockProjectResponse(total int, projects []wireProject) []byte {
	resp := wireSearchResponse{
		Meta:    wireMeta{Total: total, Offset: 0, Limit: len(projects)},
		Results: projects,
	}
	b, _ := json.Marshal(resp)
	return b
}

// testProject returns a sample wire project for use in tests.
func testProject(applID int, num, title, org, agency string) wireProject {
	return wireProject{
		ApplID:       applID,
		ProjectNum:   num,
		ProjectTitle: title,
		OrgName:      org,
		AgencyCode:   agency,
		FiscalYear:   2024,
		AwardAmount:  500000,
		ProjectStart: "2023-01-01",
		ProjectEnd:   "2026-12-31",
		ActivityCode: "R01",
		Terms:        "cancer;therapy;",
		PINames: []struct {
			FirstName string `json:"first_name"`
			LastName  string `json:"last_name"`
			IsContact bool   `json:"is_contact_pi"`
		}{
			{FirstName: "John", LastName: "Smith", IsContact: true},
		},
	}
}

// newTestClient returns a Client pointed at the given httptest server with rate=0.
func newTestClient(srv *httptest.Server) *Client {
	cfg := DefaultConfig()
	cfg.BaseURL = srv.URL
	cfg.Rate = 0
	return NewClientWithConfig(cfg)
}

func TestSearchProjects(t *testing.T) {
	projects := []wireProject{
		testProject(1001, "R01CA111111", "Cancer Study One", "MIT", "NCI"),
		testProject(1002, "R01CA222222", "Cancer Study Two", "HARVARD", "NCI"),
	}
	body := mockProjectResponse(2, projects)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("method = %s, want POST", r.Method)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(body)
	}))
	defer srv.Close()

	c := newTestClient(srv)
	got, total, err := c.SearchProjects(context.Background(), "cancer", 10, 0)
	if err != nil {
		t.Fatalf("SearchProjects: %v", err)
	}
	if total != 2 {
		t.Errorf("total = %d, want 2", total)
	}
	if len(got) != 2 {
		t.Fatalf("len(got) = %d, want 2", len(got))
	}
	if got[0].ID != 1001 {
		t.Errorf("got[0].ID = %d, want 1001", got[0].ID)
	}
	if got[0].ProjectNum != "R01CA111111" {
		t.Errorf("got[0].ProjectNum = %q, want R01CA111111", got[0].ProjectNum)
	}
	if got[0].Title != "Cancer Study One" {
		t.Errorf("got[0].Title = %q, want Cancer Study One", got[0].Title)
	}
	if got[0].OrgName != "MIT" {
		t.Errorf("got[0].OrgName = %q, want MIT", got[0].OrgName)
	}
	if len(got[0].PINames) != 1 || got[0].PINames[0] != "John Smith" {
		t.Errorf("got[0].PINames = %v, want [John Smith]", got[0].PINames)
	}
	if len(got[0].Terms) == 0 {
		t.Error("got[0].Terms is empty, want non-empty")
	}
}

func TestSearchByPI(t *testing.T) {
	projects := []wireProject{
		testProject(2001, "R01CA333333", "Smith Lab Research", "STANFORD", "NCI"),
	}
	body := mockProjectResponse(1, projects)

	var gotBody map[string]json.RawMessage
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("method = %s, want POST", r.Method)
		}
		_ = json.NewDecoder(r.Body).Decode(&gotBody)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(body)
	}))
	defer srv.Close()

	c := newTestClient(srv)
	got, total, err := c.SearchByPI(context.Background(), "Smith", 10, 0)
	if err != nil {
		t.Fatalf("SearchByPI: %v", err)
	}
	if total != 1 {
		t.Errorf("total = %d, want 1", total)
	}
	if len(got) != 1 {
		t.Fatalf("len(got) = %d, want 1", len(got))
	}
	if got[0].ID != 2001 {
		t.Errorf("got[0].ID = %d, want 2001", got[0].ID)
	}
	if got[0].OrgName != "STANFORD" {
		t.Errorf("got[0].OrgName = %q, want STANFORD", got[0].OrgName)
	}
}

func TestSearchByOrg(t *testing.T) {
	projects := []wireProject{
		testProject(3001, "R01CA444444", "Harvard Cancer Research", "HARVARD", "NCI"),
	}
	body := mockProjectResponse(1, projects)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("method = %s, want POST", r.Method)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(body)
	}))
	defer srv.Close()

	c := newTestClient(srv)
	got, total, err := c.SearchByOrg(context.Background(), "Harvard", 10, 0)
	if err != nil {
		t.Fatalf("SearchByOrg: %v", err)
	}
	if total != 1 {
		t.Errorf("total = %d, want 1", total)
	}
	if len(got) != 1 {
		t.Fatalf("len(got) = %d, want 1", len(got))
	}
	if got[0].ProjectNum != "R01CA444444" {
		t.Errorf("got[0].ProjectNum = %q, want R01CA444444", got[0].ProjectNum)
	}
	if got[0].OrgName != "HARVARD" {
		t.Errorf("got[0].OrgName = %q, want HARVARD", got[0].OrgName)
	}
}

func TestGetProject(t *testing.T) {
	projects := []wireProject{
		testProject(4001, "R01CA123456", "Specific Project", "JOHNS HOPKINS", "NCI"),
	}
	body := mockProjectResponse(1, projects)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("method = %s, want POST", r.Method)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(body)
	}))
	defer srv.Close()

	c := newTestClient(srv)
	p, err := c.GetProject(context.Background(), "R01CA123456")
	if err != nil {
		t.Fatalf("GetProject: %v", err)
	}
	if p.ID != 4001 {
		t.Errorf("p.ID = %d, want 4001", p.ID)
	}
	if p.ProjectNum != "R01CA123456" {
		t.Errorf("p.ProjectNum = %q, want R01CA123456", p.ProjectNum)
	}
	if p.Title != "Specific Project" {
		t.Errorf("p.Title = %q, want Specific Project", p.Title)
	}
	if p.OrgName != "JOHNS HOPKINS" {
		t.Errorf("p.OrgName = %q, want JOHNS HOPKINS", p.OrgName)
	}
	if p.AwardAmount != 500000 {
		t.Errorf("p.AwardAmount = %d, want 500000", p.AwardAmount)
	}
}

func TestGetProjectNotFound(t *testing.T) {
	body := mockProjectResponse(0, nil)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(body)
	}))
	defer srv.Close()

	c := newTestClient(srv)
	_, err := c.GetProject(context.Background(), "R01CA999999")
	if err == nil {
		t.Fatal("GetProject: expected error for not found, got nil")
	}
}
