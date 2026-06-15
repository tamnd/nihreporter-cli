package nihreporter

import (
	"strings"
	"testing"

	"github.com/tamnd/any-cli/kit"
)

// These tests are offline: they exercise the URI driver's pure string functions
// and the host wiring (mint, body, resolve), which need no network. The client's
// HTTP behaviour is covered in nihreporter_test.go.

func TestDomainInfo(t *testing.T) {
	info := Domain{}.Info()
	if info.Scheme != "nihreporter" {
		t.Errorf("Scheme = %q, want nihreporter", info.Scheme)
	}
	if len(info.Hosts) == 0 || info.Hosts[0] != Host {
		t.Errorf("Hosts = %v, want [%s]", info.Hosts, Host)
	}
	if info.Identity.Binary != "nihreporter" {
		t.Errorf("Identity.Binary = %q, want nihreporter", info.Identity.Binary)
	}
}

func TestClassify(t *testing.T) {
	cases := []struct {
		in  string
		typ string
		id  string
	}{
		{"R01CA123456", "project", "R01CA123456"},
		{"75N94023D00001", "project", "75N94023D00001"},
		{"some-grant-number", "project", "some-grant-number"},
	}
	for _, tc := range cases {
		typ, id, err := Domain{}.Classify(tc.in)
		if err != nil || typ != tc.typ || id != tc.id {
			t.Errorf("Classify(%q) = (%q, %q, %v), want (%q, %q, nil)",
				tc.in, typ, id, err, tc.typ, tc.id)
		}
	}
}

func TestClassifyEmpty(t *testing.T) {
	_, _, err := Domain{}.Classify("")
	if err == nil {
		t.Error("Classify(\"\") should return an error")
	}
}

func TestLocate(t *testing.T) {
	got, err := Domain{}.Locate("project", "R01CA123456")
	want := "https://" + Host + "/search-results/R01CA123456"
	if err != nil || got != want {
		t.Errorf("Locate = (%q, %v), want (%q, nil)", got, err, want)
	}
}

func TestLocateUnknownType(t *testing.T) {
	_, err := Domain{}.Locate("unknown", "R01CA123456")
	if err == nil {
		t.Error("Locate with unknown type should return an error")
	}
}

func TestHostWiring(t *testing.T) {
	h, err := kit.Open()
	if err != nil {
		t.Fatal(err)
	}

	p := &Project{
		ID:         12345,
		ProjectNum: "R01CA123456",
		Title:      "Test Project",
		Abstract:   "A test abstract for wiring check.",
	}
	u, err := h.Mint(p)
	if err != nil {
		t.Fatalf("Mint: %v", err)
	}
	if !strings.HasPrefix(u.String(), "nihreporter://project/") {
		t.Errorf("Mint = %q, want nihreporter://project/... prefix", u.String())
	}

	got, err := h.ResolveOn("nihreporter", "R01CA123456")
	if err != nil || got.String() != "nihreporter://project/R01CA123456" {
		t.Errorf("ResolveOn = (%q, %v), want nihreporter://project/R01CA123456", got.String(), err)
	}
}
