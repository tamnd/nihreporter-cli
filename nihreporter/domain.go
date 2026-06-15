package nihreporter

import (
	"context"
	"strings"

	"github.com/tamnd/any-cli/kit"
	"github.com/tamnd/any-cli/kit/errs"
)

// domain.go exposes nihreporter as a kit Domain: a driver that a multi-domain
// host (ant) enables with a single blank import,
//
//	import _ "github.com/tamnd/nihreporter-cli/nihreporter"
//
// exactly as a database/sql program enables a driver with `import _
// "github.com/lib/pq"`. The init below registers it; the host then dereferences
// nihreporter:// URIs by routing to the operations Register installs. The same
// Domain also builds the standalone nihreporter binary (see cli.NewApp), so the
// binary and a host share one source of truth.
func init() { kit.Register(Domain{}) }

// Domain is the nihreporter driver. It carries no state; the per-run client is
// built by the factory Register hands kit.
type Domain struct{}

// Info describes the scheme, the hostnames a pasted link is matched against, and
// the identity reused for the binary's help and version.
func (Domain) Info() kit.DomainInfo {
	return kit.DomainInfo{
		Scheme: "nihreporter",
		Hosts:  []string{Host},
		Identity: kit.Identity{
			Binary: "nihreporter",
			Short:  "Search NIH Reporter grants and research projects.",
			Long: `Search NIH Reporter grants and research projects.

nihreporter reads public NIH Reporter data over plain HTTPS, shapes it into
clean records, and prints output that pipes into the rest of your tools. No API
key required — the NIH Reporter API is open to all.`,
			Site: Host,
			Repo: "https://github.com/tamnd/nihreporter-cli",
		},
	}
}

// Register installs the client factory and every operation onto app.
func (Domain) Register(app *kit.App) {
	app.SetClient(newClient)

	// search: full-text search across all grant records.
	kit.Handle(app, kit.OpMeta{Name: "search", Group: "read", List: true,
		Summary: "Search NIH grants by keyword",
		Args:    []kit.Arg{{Name: "query", Help: "search terms", Variadic: true}}}, searchProjects)

	// project: fetch a single grant by project number.
	kit.Handle(app, kit.OpMeta{Name: "project", Group: "read", Single: true,
		Summary: "Fetch a grant by project number", URIType: "project", Resolver: true,
		Args: []kit.Arg{{Name: "project_num", Help: "project number (e.g. R01CA123456)"}}}, getProject)

	// pi: search grants by principal investigator last name.
	kit.Handle(app, kit.OpMeta{Name: "pi", Group: "read", List: true,
		Summary: "Search grants by PI last name",
		Args:    []kit.Arg{{Name: "last_name", Help: "PI last name"}}}, searchByPI)

	// org: search grants by organization name.
	kit.Handle(app, kit.OpMeta{Name: "org", Group: "read", List: true,
		Summary: "Search grants by organization name",
		Args:    []kit.Arg{{Name: "name", Help: "organization name"}}}, searchByOrg)
}

// newClient builds the client from the host-resolved config.
func newClient(_ context.Context, cfg kit.Config) (any, error) {
	dcfg := DefaultConfig()
	if cfg.UserAgent != "" {
		dcfg.UserAgent = cfg.UserAgent
	}
	if cfg.Rate > 0 {
		dcfg.Rate = cfg.Rate
	}
	if cfg.Retries > 0 {
		dcfg.Retries = cfg.Retries
	}
	if cfg.Timeout > 0 {
		dcfg.Timeout = cfg.Timeout
	}
	return NewClientWithConfig(dcfg), nil
}

// --- inputs ---

type searchInput struct {
	Query  []string `kit:"arg,variadic" help:"search terms"`
	Limit  int      `kit:"flag,inherit" help:"max results"`
	Offset int      `kit:"flag,inherit" help:"results offset"`
	Client *Client  `kit:"inject"`
}

type projectInput struct {
	ProjectNum string  `kit:"arg" help:"project number (e.g. R01CA123456)"`
	Client     *Client `kit:"inject"`
}

type piInput struct {
	LastName string  `kit:"arg" help:"PI last name"`
	Limit    int     `kit:"flag,inherit" help:"max results"`
	Offset   int     `kit:"flag,inherit" help:"results offset"`
	Client   *Client `kit:"inject"`
}

type orgInput struct {
	Name   string  `kit:"arg" help:"organization name"`
	Limit  int     `kit:"flag,inherit" help:"max results"`
	Offset int     `kit:"flag,inherit" help:"results offset"`
	Client *Client `kit:"inject"`
}

// --- handlers ---

func searchProjects(ctx context.Context, in searchInput, emit func(*Project) error) error {
	limit := in.Limit
	if limit <= 0 {
		limit = 10
	}
	projects, _, err := in.Client.SearchProjects(ctx, strings.Join(in.Query, " "), limit, in.Offset)
	if err != nil {
		return mapErr(err)
	}
	for _, p := range projects {
		if err := emit(p); err != nil {
			return err
		}
	}
	return nil
}

func getProject(ctx context.Context, in projectInput, emit func(*Project) error) error {
	p, err := in.Client.GetProject(ctx, in.ProjectNum)
	if err != nil {
		return mapErr(err)
	}
	return emit(p)
}

func searchByPI(ctx context.Context, in piInput, emit func(*Project) error) error {
	limit := in.Limit
	if limit <= 0 {
		limit = 10
	}
	projects, _, err := in.Client.SearchByPI(ctx, in.LastName, limit, in.Offset)
	if err != nil {
		return mapErr(err)
	}
	for _, p := range projects {
		if err := emit(p); err != nil {
			return err
		}
	}
	return nil
}

func searchByOrg(ctx context.Context, in orgInput, emit func(*Project) error) error {
	limit := in.Limit
	if limit <= 0 {
		limit = 10
	}
	projects, _, err := in.Client.SearchByOrg(ctx, in.Name, limit, in.Offset)
	if err != nil {
		return mapErr(err)
	}
	for _, p := range projects {
		if err := emit(p); err != nil {
			return err
		}
	}
	return nil
}

// --- Resolver: the URI-native string functions, pure and network-free ---

// Classify turns any accepted input into the canonical (type, id).
// Any non-empty string is treated as a project number.
func (Domain) Classify(input string) (uriType, id string, err error) {
	input = strings.TrimSpace(input)
	if input == "" {
		return "", "", errs.Usage("unrecognized nihreporter reference: empty input")
	}
	return "project", input, nil
}

// Locate is the inverse: the live https URL for a (type, id).
func (Domain) Locate(uriType, id string) (string, error) {
	if uriType != "project" {
		return "", errs.Usage("nihreporter has no resource type %q", uriType)
	}
	return "https://" + Host + "/search-results/" + id, nil
}

// --- helpers ---

// mapErr converts a library error into the kit error kind that carries the right
// exit code.
func mapErr(err error) error {
	return err
}
