// Package nihreporter is the library behind the nihreporter command line:
// the HTTP client, request shaping, and the typed data models for the NIH
// Reporter API.
//
// The Client here is the spine every command shares. It sets a real
// User-Agent, paces requests so a busy session stays polite, and retries the
// transient failures (429 and 5xx) that any public API throws under load.
// Build your endpoint calls and JSON decoding on top of it.
package nihreporter

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// Host is the NIH Reporter site, used for Locate / URI resolution.
const Host = "reporter.nih.gov"

// --- Config ---

// Config carries the tunable parameters for the NIH Reporter client.
type Config struct {
	BaseURL   string
	Rate      time.Duration
	Retries   int
	Timeout   time.Duration
	UserAgent string
}

// DefaultConfig returns a Config with sensible defaults for the NIH Reporter API.
func DefaultConfig() Config {
	return Config{
		BaseURL:   "https://api.reporter.nih.gov/v2",
		Rate:      500 * time.Millisecond,
		Retries:   3,
		Timeout:   30 * time.Second,
		UserAgent: "nihreporter-cli/0.1.0 (github.com/tamnd/nihreporter-cli)",
	}
}

// --- Client ---

// Client talks to the NIH Reporter API over HTTP.
type Client struct {
	HTTP *http.Client
	cfg  Config
	last time.Time
}

// NewClient returns a Client with the default config.
func NewClient() *Client {
	return NewClientWithConfig(DefaultConfig())
}

// NewClientWithConfig returns a Client using the given config.
func NewClientWithConfig(cfg Config) *Client {
	if cfg.BaseURL == "" {
		cfg.BaseURL = "https://api.reporter.nih.gov/v2"
	}
	return &Client{
		HTTP: &http.Client{Timeout: cfg.Timeout},
		cfg:  cfg,
	}
}

// post sends a POST request with a JSON body and decodes the JSON response into dst.
func (c *Client) post(ctx context.Context, endpoint string, body any, dst any) error {
	var lastErr error
	for attempt := 0; attempt <= c.cfg.Retries; attempt++ {
		if attempt > 0 {
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(backoff(attempt)):
			}
		}
		retry, err := c.doPost(ctx, endpoint, body, dst)
		if err == nil {
			return nil
		}
		lastErr = err
		if !retry {
			return err
		}
	}
	return fmt.Errorf("post %s: %w", endpoint, lastErr)
}

func (c *Client) doPost(ctx context.Context, endpoint string, body any, dst any) (retry bool, err error) {
	c.pace()

	encoded, err := json.Marshal(body)
	if err != nil {
		return false, fmt.Errorf("marshal request: %w", err)
	}

	url := strings.TrimRight(c.cfg.BaseURL, "/") + "/" + strings.TrimLeft(endpoint, "/")
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(encoded))
	if err != nil {
		return false, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", c.cfg.UserAgent)

	resp, err := c.HTTP.Do(req)
	if err != nil {
		return true, err
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode == http.StatusTooManyRequests || resp.StatusCode >= 500 {
		return true, fmt.Errorf("http %d", resp.StatusCode)
	}
	if resp.StatusCode != http.StatusOK {
		return false, fmt.Errorf("http %d", resp.StatusCode)
	}

	b, err := io.ReadAll(resp.Body)
	if err != nil {
		return true, err
	}
	if err := json.Unmarshal(b, dst); err != nil {
		return false, fmt.Errorf("decode response: %w", err)
	}
	return false, nil
}

// pace blocks until at least Rate has passed since the previous request.
func (c *Client) pace() {
	if c.cfg.Rate <= 0 {
		return
	}
	if wait := c.cfg.Rate - time.Since(c.last); wait > 0 {
		time.Sleep(wait)
	}
	c.last = time.Now()
}

func backoff(attempt int) time.Duration {
	d := time.Duration(attempt) * 500 * time.Millisecond
	if d > 5*time.Second {
		d = 5 * time.Second
	}
	return d
}

// --- wire types (unexported) ---

type wireMeta struct {
	Total  int `json:"total"`
	Offset int `json:"offset"`
	Limit  int `json:"limit"`
}

type wireProject struct {
	ApplID       int    `json:"appl_id"`
	ProjectNum   string `json:"project_num"`
	ProjectTitle string `json:"project_title"`
	AbstractText string `json:"abstract_text"`
	FiscalYear   int    `json:"fiscal_year"`
	AwardAmount  int64  `json:"award_amount"`
	ProjectStart string `json:"project_start_date"`
	ProjectEnd   string `json:"project_end_date"`
	PINames      []struct {
		FirstName string `json:"first_name"`
		LastName  string `json:"last_name"`
		IsContact bool   `json:"is_contact_pi"`
	} `json:"pi_names"`
	OrgName      string  `json:"org_name"`
	OrgState     string  `json:"org_state"`
	OrgCity      string  `json:"org_city"`
	ActivityCode string  `json:"activity_code"`
	Terms        string  `json:"terms"`
	AgencyCode   string  `json:"agency_code"`
	SubprojectID *string `json:"subproject_id"`
}

type wireSearchResponse struct {
	Meta    wireMeta      `json:"meta"`
	Results []wireProject `json:"results"`
}

// --- public output types ---

// Project is an NIH research grant record.
type Project struct {
	ID           int      `json:"id"            kit:"id"` // appl_id
	ProjectNum   string   `json:"project_num"`
	Title        string   `json:"title"`
	Abstract     string   `json:"abstract,omitempty"`
	FiscalYear   int      `json:"fiscal_year,omitempty"`
	AwardAmount  int64    `json:"award_amount,omitempty"`
	StartDate    string   `json:"start_date,omitempty"`
	EndDate      string   `json:"end_date,omitempty"`
	PINames      []string `json:"pi_names,omitempty"`
	OrgName      string   `json:"org_name,omitempty"`
	OrgState     string   `json:"org_state,omitempty"`
	ActivityCode string   `json:"activity_code,omitempty"`
	Agency       string   `json:"agency,omitempty"`
	Terms        []string `json:"terms,omitempty"`
}

// projectFromWire converts a wire project into the public Project type.
func projectFromWire(wp wireProject) *Project {
	var piNames []string
	for _, pi := range wp.PINames {
		name := strings.TrimSpace(pi.FirstName + " " + pi.LastName)
		if name == "" {
			name = pi.LastName
		}
		if name != "" {
			piNames = append(piNames, name)
		}
	}

	var terms []string
	for _, t := range strings.Split(wp.Terms, ";") {
		t = strings.TrimSpace(t)
		if t != "" {
			terms = append(terms, t)
		}
	}

	return &Project{
		ID:           wp.ApplID,
		ProjectNum:   wp.ProjectNum,
		Title:        wp.ProjectTitle,
		Abstract:     wp.AbstractText,
		FiscalYear:   wp.FiscalYear,
		AwardAmount:  wp.AwardAmount,
		StartDate:    wp.ProjectStart,
		EndDate:      wp.ProjectEnd,
		PINames:      piNames,
		OrgName:      wp.OrgName,
		OrgState:     wp.OrgState,
		ActivityCode: wp.ActivityCode,
		Agency:       wp.AgencyCode,
		Terms:        terms,
	}
}

// --- search request helpers ---

type searchRequest struct {
	Criteria any `json:"criteria"`
	Offset   int `json:"offset"`
	Limit    int `json:"limit"`
}

type textSearchCriteria struct {
	TextSearch struct {
		SearchText  string `json:"search_text"`
		SearchField string `json:"search_field"`
	} `json:"text_search"`
}

type piSearchCriteria struct {
	PINames []struct {
		LastName string `json:"last_name"`
	} `json:"pi_names"`
}

type orgSearchCriteria struct {
	OrgNames []string `json:"org_names"`
}

type projectNumCriteria struct {
	ProjectNums []string `json:"project_nums"`
}

// --- client methods ---

// SearchProjects searches grants by text query.
func (c *Client) SearchProjects(ctx context.Context, query string, limit, offset int) ([]*Project, int, error) {
	var crit textSearchCriteria
	crit.TextSearch.SearchText = query
	crit.TextSearch.SearchField = "all"

	req := searchRequest{Criteria: crit, Offset: offset, Limit: limit}
	var resp wireSearchResponse
	if err := c.post(ctx, "projects/search", req, &resp); err != nil {
		return nil, 0, err
	}
	return projectsFromWire(resp.Results), resp.Meta.Total, nil
}

// SearchByPI searches grants by PI last name.
func (c *Client) SearchByPI(ctx context.Context, lastName string, limit, offset int) ([]*Project, int, error) {
	crit := piSearchCriteria{
		PINames: []struct {
			LastName string `json:"last_name"`
		}{{LastName: lastName}},
	}

	req := searchRequest{Criteria: crit, Offset: offset, Limit: limit}
	var resp wireSearchResponse
	if err := c.post(ctx, "projects/search", req, &resp); err != nil {
		return nil, 0, err
	}
	return projectsFromWire(resp.Results), resp.Meta.Total, nil
}

// SearchByOrg searches grants by organization name.
func (c *Client) SearchByOrg(ctx context.Context, orgName string, limit, offset int) ([]*Project, int, error) {
	crit := orgSearchCriteria{OrgNames: []string{orgName}}

	req := searchRequest{Criteria: crit, Offset: offset, Limit: limit}
	var resp wireSearchResponse
	if err := c.post(ctx, "projects/search", req, &resp); err != nil {
		return nil, 0, err
	}
	return projectsFromWire(resp.Results), resp.Meta.Total, nil
}

// GetProject fetches a single project by project number.
func (c *Client) GetProject(ctx context.Context, projectNum string) (*Project, error) {
	crit := projectNumCriteria{ProjectNums: []string{projectNum}}

	req := searchRequest{Criteria: crit, Offset: 0, Limit: 1}
	var resp wireSearchResponse
	if err := c.post(ctx, "projects/search", req, &resp); err != nil {
		return nil, err
	}
	if len(resp.Results) == 0 {
		return nil, fmt.Errorf("project %s: not found", projectNum)
	}
	return projectFromWire(resp.Results[0]), nil
}

// projectsFromWire converts a slice of wire projects to public Project records.
func projectsFromWire(wps []wireProject) []*Project {
	out := make([]*Project, 0, len(wps))
	for _, wp := range wps {
		out = append(out, projectFromWire(wp))
	}
	return out
}
