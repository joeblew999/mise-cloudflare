package e2e

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"
)

// TestBasin proves Basin works on a repo's own login, with no API token: in example/ (whose cloudflare.toml has
// pipelines:write) an R2 bucket, Basin Catalog on it, a Basin Pipeline (stream, catalog sink, SQL), events sent with the
// login's token, read back with Basin SQL; then all of it removed.
func TestBasin(t *testing.T) {
	needAccount(t)
	dir, _ := filepath.Abs("../example")
	r := repo{t: t, dir: dir, worker: "cloudflare-zero-example"}
	const b, s, k, p, ns, table = "cloudflare-zero-basin", "cf_zero_stream", "cf_zero_sink", "cf_zero_pipeline", "default", "events"

	r.step("The repo's own login, with its scopes")
	var who struct {
		Accounts []struct{ ID string } `json:"accounts"`
	}
	json.Unmarshal([]byte(jsonPart(r.cf(nil, "-json", "login"))), &who)
	if len(who.Accounts) != 1 {
		t.Fatalf("the login can use %d accounts", len(who.Accounts))
	}
	acct := who.Accounts[0].ID
	token := func() string {
		var tok struct{ Type, Token string }
		json.Unmarshal([]byte(jsonPart(r.wrangler("auth", "token", "--json"))), &tok)
		if tok.Type != "oauth" || tok.Token == "" {
			t.Fatalf("not the login's OAuth token: %q", tok.Type)
		}
		return tok.Token
	}
	token()
	r.pass("authenticated by the login (OAuth), no API token")
	t.Cleanup(func() {
		r.step("Removing everything")
		r.run(nil, "go", "tool", "cloudflare", "wrangler", "basin", "pipelines", "delete", p, "-y")
		r.run(nil, "go", "tool", "cloudflare", "wrangler", "basin", "pipelines", "sinks", "delete", k, "-y")
		r.run(nil, "go", "tool", "cloudflare", "wrangler", "basin", "pipelines", "streams", "delete", s, "-y")
		r.run(nil, "go", "tool", "cloudflare", "wrangler", "basin", "catalog", "disable", b)
		emptyBucket(t, acct, token(), b)
		r.run(nil, "go", "tool", "cloudflare", "wrangler", "r2", "bucket", "delete", b)
		if strings.Contains(r.wrangler("r2", "bucket", "list"), "name: "+b+"\n") {
			t.Errorf("R2 %s is still there", b)
		} else {
			r.pass("nothing left")
		}
	})

	r.step("R2 bucket and Basin Catalog")
	r.run(nil, "go", "tool", "cloudflare", "wrangler", "r2", "bucket", "create", b)
	r.run(nil, "go", "tool", "cloudflare", "wrangler", "basin", "catalog", "enable", b)

	r.step("Basin Pipeline: stream, catalog sink (its catalog token is the login's own), SQL")
	schema := filepath.Join(t.TempDir(), "schema.json")
	os.WriteFile(schema, []byte(`{"fields":[{"name":"path","type":"string","required":true},{"name":"n","type":"int64","required":true}]}`), 0o644)
	out := r.wrangler("basin", "pipelines", "streams", "create", s, "--schema-file", schema)
	ingest := regexp.MustCompile(`https://[a-z0-9]+\.ingest\.cloudflare\.com`).FindString(out)
	if ingest == "" {
		t.Fatal("no ingest endpoint")
	}
	r.pass("stream %s, endpoint %s", s, ingest)
	r.wrangler("basin", "pipelines", "sinks", "create", k, "--type", "r2-data-catalog", "--bucket", b, "--namespace", ns,
		"--table", table, "--catalog-token", token(), "--roll-interval", "60")
	r.pass("sink %s into %s.%s", k, ns, table)
	r.wrangler("basin", "pipelines", "create", p, "--sql", "INSERT INTO "+k+" SELECT * FROM "+s)
	r.pass("pipeline %s", p)

	r.step("Send 3 events, authenticated with the login's own token")
	if code, body := post(ingest, token(), `[{"path":"/a","n":1},{"path":"/b","n":2},{"path":"/a","n":3}]`); code != 200 {
		t.Fatalf("ingest: HTTP %d %s", code, body)
	}
	r.pass("events accepted")

	r.step("Read them back with Basin SQL, with the login's own token (the first files take a few minutes)")
	q := fmt.Sprintf(`{"query":"SELECT path, n FROM %s.%s ORDER BY n"}`, ns, table)
	for i := 1; ; i++ {
		_, body := post("https://api.sql.cloudflarestorage.com/api/v1/accounts/"+acct+"/basin-sql/query/"+b, token(), q)
		var res struct {
			Result struct{ Rows []json.RawMessage } `json:"result"`
		}
		if json.Unmarshal(body, &res) == nil && len(res.Result.Rows) >= 3 {
			r.pass("Basin SQL returned the 3 events")
			break
		}
		if list := r.wrangler("basin", "pipelines", "list"); regexp.MustCompile(`(?m)\b` + p + `\b.*\n?.*failed`).MatchString(list) {
			t.Fatalf("Cloudflare marked the pipeline failed:\n%s", list)
		}
		if i == 40 {
			t.Fatal("no rows after 10 minutes")
		}
		fmt.Printf("waiting (%d): %.160s\n", i, body)
		time.Sleep(15 * time.Second)
	}
}

func post(url, token, body string) (int, []byte) {
	req, _ := http.NewRequest(http.MethodPost, url, strings.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	resp, err := (&http.Client{Timeout: 30 * time.Second}).Do(req)
	if err != nil {
		return 0, []byte(err.Error())
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
	return resp.StatusCode, b
}

// emptyBucket deletes every object in an R2 bucket, page by page, so it can be deleted: wrangler can't list objects,
// Cloudflare's API can, with the login's own token.
func emptyBucket(t *testing.T, acct, token, bucket string) {
	base := "https://api.cloudflare.com/client/v4/accounts/" + acct + "/r2/buckets/" + bucket + "/objects"
	for range 1000 {
		req, _ := http.NewRequest(http.MethodGet, base+"?per_page=1000", nil)
		req.Header.Set("Authorization", "Bearer "+token)
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Logf("listing R2 %s: %v", bucket, err)
			return
		}
		var page struct {
			Result []struct{ Key string } `json:"result"`
		}
		json.NewDecoder(resp.Body).Decode(&page)
		resp.Body.Close()
		if len(page.Result) == 0 {
			return
		}
		keys := make([]string, len(page.Result))
		for i, o := range page.Result {
			keys[i] = o.Key
		}
		body, _ := json.Marshal(keys)
		req, _ = http.NewRequest(http.MethodDelete, base, bytes.NewReader(body))
		req.Header.Set("Authorization", "Bearer "+token)
		req.Header.Set("Content-Type", "application/json")
		if resp, err := http.DefaultClient.Do(req); err == nil {
			resp.Body.Close()
		}
	}
}
