// The steps: the repo's numbered tasks, in their order, each with its check (TestWalk walks them).

package e2e

import (
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"testing"
	"time"
)

// step is one numbered task of the repo's own mise file (its .mise/tasks/mise-cloudflare.toml, which is the
// template's, templates/.mise/tasks/mise-cloudflare.toml): cloudflare:<task>, the first stage that runs it, and how the
// walk runs it and checks it did what it says. A step the walk can't show working says why instead, in skip.
type step struct {
	task  string
	stage stage
	skip  string
	check func(w *walk)
}

// steps are the repo's numbered tasks, in their order, each checked by what a person would see it do. walkSteps fails
// unless they're exactly the numbered tasks of the repo it walks.
var steps = []step{
	{task: "03-init", stage: serve, check: func(w *walk) {
		before := w.snapshot()
		w.mustRunMiseTask(nil, "cloudflare:03-init")
		if changed := diff(before, w.snapshot()); len(changed) > 0 {
			w.t.Fatalf("cloudflare:03-init changed %s: the repo isn't what init writes", strings.Join(changed, ", "))
		}
		w.pass("cloudflare:03-init changed nothing: the repo is what init writes")
	}},
	{task: "04-check", stage: serve, check: func(w *walk) {
		if out := w.mustRunMiseTask(nil, "cloudflare:04-check"); !strings.Contains(out, "rules: pass") {
			w.t.Fatalf("cloudflare:04-check: %s", out)
		}
		w.pass("cloudflare:04-check: the rules pass")
		// As on GitHub, from a fresh checkout: nothing installed, so verify's own setup has to install wrangler.
		if err := os.RemoveAll(filepath.Join(w.dir, "node_modules")); err != nil {
			w.t.Fatal(err)
		}
		w.mustRunMiseTask(nil, "cloudflare:verify")
		w.pass("cloudflare:verify: what the repo's GitHub workflow runs passes, from nothing installed, as a fresh checkout is (its setup and test tasks, then 04-check's)")
	}},
	{task: "05-show", stage: serve, check: func(w *walk) {
		if len(w.secrets) == 0 {
			w.miseShowSaysNext("mise run cloudflare:08-deploy")
		} else {
			w.miseShowSaysNext("mise run cloudflare:07-set-secrets")
		}
	}},
	{task: "06-serve", stage: serve, check: func(w *walk) {
		body, stopped := w.miseServeLocally(false)
		if remoteExtras() {
			if body != nil || !strings.Contains(stopped, "AI: wrangler runs it on Cloudflare even locally, which needs the repo's login") {
				w.t.Fatalf("cloudflare:06-serve with no Cloudflare login and an extra wrangler runs on Cloudflare even locally: %s%s", body, stopped)
			}
			w.pass("cloudflare:06-serve with no Cloudflare login, in CI, stops before wrangler dev at the extra wrangler runs on Cloudflare even locally, and says it needs the repo's login")
			return
		}
		if w.adopted {
			if body == nil {
				w.t.Fatalf("the local Worker never answered: %s", stopped)
			}
			w.pass("the local Worker answers, with no Cloudflare login: %.80q", body)
			return
		}
		if !allWork(body) {
			w.t.Fatalf("the local Worker: %s%s", body, stopped)
		}
		w.pass("the local Worker's D1, KV, R2 and secret work, with no Cloudflare login")
	}},
	{task: "07-set-secrets", stage: deploy, check: func(w *walk) {
		if len(w.secrets) == 0 {
			w.pass("cloudflare:07-set-secrets: the Worker requires no secrets")
			return
		}
		if _, err := w.runMiseTask(nil, strings.Repeat("set-by-deploy\n", len(w.secrets)), "cloudflare:07-set-secrets"); err != nil {
			w.t.Fatalf("cloudflare:07-set-secrets: %v", err)
		}
		w.pass("cloudflare:07-set-secrets set %s", strings.Join(w.secrets, ", "))
		w.miseShowSaysNext("mise run cloudflare:08-deploy")
	}},
	{task: "08-deploy", stage: deploy, check: func(w *walk) {
		w.mustRunMiseTask(nil, "cloudflare:08-deploy")
		s := w.miseShowJSON()
		if !s.LoggedIn || !s.Deployed || s.Profile != w.login {
			w.t.Fatalf("not deployed on the login %s: %+v", w.login, s)
		}
		w.url = s.URL
		if w.adopted {
			answers(w.t, w.url, 2*time.Minute)
			w.pass("%s: deployed on the repo's own login, %s; it answers", w.url, s.Profile)
		} else {
			waitTillItWorks(w.t, w.url, 2*time.Minute)
			w.pass("%s: deployed on the repo's own login, %s; its D1, KV, R2 and secret work", w.url, s.Profile)
		}
		if withExtras() {
			token, account := w.api()
			if on := w.extrasSettle(token, account, w.worker, true); len(on) != made() {
				w.t.Fatalf("wrangler deploy made %d of the %d extras it makes: %q", len(on), made(), on)
			} else {
				w.pass("wrangler deploy made the extras too: %s", strings.Join(on, ", "))
			}
		}
		if env := withEnv(); env != "" {
			top := w.repo
			top.worker = w.login
			top.nothingOnCloudflare()
			w.pass("deployed env.%s alone, as %s, on the repo's one login: nothing of the top level's Worker, %s", env, w.worker, w.login)
		}
		if changed := diff(w.before, w.snapshot()); len(changed) > 0 {
			w.t.Fatalf("deploying changed %s", strings.Join(changed, ", "))
		}
		w.pass("deploying wrote nothing into the repo: no IDs")
		if remoteExtras() {
			if body, stopped := w.miseServeLocally(true); !allWork(body) {
				w.t.Fatalf("cloudflare:06-serve on the repo's login, the extras made: %s%s", body, stopped)
			}
			w.pass("cloudflare:06-serve on the repo's login runs the extra wrangler runs on Cloudflare even locally")
		}
		if out, err := w.runMiseTask([]string{"CLOUDFLARE_API_TOKEN=dummy-not-a-real-token"}, "", "cloudflare:08-deploy"); err == nil || !strings.Contains(out, "refusing:") {
			w.t.Fatal("deployed with a token in the shell")
		}
		w.pass("a stray CLOUDFLARE_API_TOKEN in the shell is refused")
		env := filepath.Join(w.dir, ".env")
		os.WriteFile(env, []byte("CLOUDFLARE_API_TOKEN=dummy-not-a-real-token\n"), 0o600)
		out, err := w.runMiseTask(nil, "", "cloudflare:08-deploy")
		os.Remove(env)
		if err == nil || !strings.Contains(out, "refusing:") {
			w.t.Fatal("deployed with a token in .env")
		}
		w.pass("a stray CLOUDFLARE_API_TOKEN in .env is refused")
		w.miseShowSaysNext("mise run cloudflare:09-connect-github")
	}},
	{task: "09-connect-github", stage: connectGithub, check: func(w *walk) {
		w.mustRunMiseTask(nil, "cloudflare:09-connect-github", "--branch", w.prod)
		w.pass("Workers Builds deploys every push, production from %s", w.prod)
		// Again, with no flags, as the docs say a person can: it keeps what's set (an update, not a new connection).
		if out := w.mustRunMiseTask(nil, "cloudflare:09-connect-github"); !strings.Contains(out, "a push to "+w.prod+" runs") {
			w.t.Fatalf("cloudflare:09-connect-github again didn't keep production on %s:\n%s", w.prod, out)
		}
		w.pass("cloudflare:09-connect-github again keeps what's set: production from %s", w.prod)
		w.miseShowSaysNext("mise run cloudflare:10-set-preview-secrets")
	}},
	{task: "10-set-preview-secrets", stage: connectGithub, check: func(w *walk) {
		if _, err := w.runMiseTask(nil, "set-by-deploy\n", "cloudflare:10-set-preview-secrets"); err != nil {
			w.t.Fatalf("cloudflare:10-set-preview-secrets: %v", err)
		}
		w.miseShowSaysNext("push a branch")

		w.heading("A push to %s: Cloudflare builds and deploys production", w.prod)
		w.mustIn(w.top, "git", "push", "-q", "origin", "HEAD:refs/heads/"+w.prod)
		if outcome := w.buildOutcome(w.prod, 15*time.Minute); outcome != "success" {
			w.t.Fatalf("Cloudflare's build of %s: %s", w.prod, outcome)
		}
		waitTillItWorks(w.t, w.url, 2*time.Minute)
		w.pass("Cloudflare built and deployed production from %s; it works", w.prod)

		w.heading("A pull request's branch %s: its preview, with its own D1, KV, R2 and secret", w.prev)
		wt := filepath.Join(w.t.TempDir(), "wt")
		w.mustIn(w.top, "git", "worktree", "add", "-q", "--detach", wt, "HEAD")
		w.t.Cleanup(func() { w.runIn(w.top, nil, "", "git", "worktree", "remove", "--force", wt) })
		rel, _ := filepath.Rel(w.top, w.dir)
		os.WriteFile(filepath.Join(wt, rel, "preview.txt"), []byte("a pull request's change\n"), 0o644)
		w.mustIn(wt, "git", "add", "-A")
		w.mustIn(wt, "git", "commit", "-qm", "prove: a pull request's change")
		w.mustIn(wt, "git", "push", "-q", "origin", "HEAD:refs/heads/"+w.prev)
		preview := strings.Replace(w.url, "https://", "https://"+previewAlias(w.prev)+"-", 1)
		waitTillItWorks(w.t, preview, 15*time.Minute)
		w.pass("%s: built by Cloudflare from %s; its D1, KV, R2 and secret work", preview, w.prev)

		w.heading("Deleting %s, as merging a pull request does: its preview goes with it", w.prev)
		w.mustIn(w.top, "git", "push", "-q", "origin", "--delete", w.prev)
		if !gone(preview, 10*time.Minute) {
			w.t.Fatalf("%s still answers, 10 minutes after its branch went", preview)
		}
		w.pass("%s is gone", preview)
	}},
	{task: "11-preview", stage: deploy, check: func(w *walk) {
		if w.adopted {
			w.pass("cloudflare:11-preview: an adopted Worker's previews aren't walked: they need a previews block and secrets of its developer's")
			return
		}
		if w.upTo < connectGithub && len(w.secrets) > 0 {
			// Without connect-github's steps, its previews have no secrets yet: 10-set-preview-secrets first, as the docs say.
			if _, err := w.runMiseTask(nil, strings.Repeat("set-by-deploy\n", len(w.secrets)), "cloudflare:10-set-preview-secrets"); err != nil {
				w.t.Fatalf("cloudflare:10-set-preview-secrets: %v", err)
			}
			w.pass("cloudflare:10-set-preview-secrets set the previews' %s, which a preview from here needs too", strings.Join(w.secrets, ", "))
		}
		w.mustRunMiseTask(nil, "cloudflare:11-preview", "--name", "e2e")
		token, account := w.api()
		var env map[string]any
		w.preview, env = w.previewOn(token, account, "e2e")
		waitTillItWorks(w.t, w.preview, 5*time.Minute)
		w.pass("%s: a preview from here, on the repo's login; its D1, KV, R2 and secret work", w.preview)
		if withExtras() {
			if on := w.extrasSettle(token, account, w.worker+"-previews", true); len(on) != made() {
				w.t.Fatalf("cloudflare:11-preview made %d of the previews' own %d extras: %q", len(on), made(), on)
			} else if wrong := w.previewBindsOwn(token, account, env); len(wrong) > 0 {
				w.t.Fatalf("the preview doesn't bind the previews' own: %s", strings.Join(wrong, "; "))
			} else {
				w.pass("the previews' own extras, made and bound in the preview: %s", strings.Join(on, ", "))
			}
		}
	}},
	{task: "12-login", stage: serve, check: func(w *walk) {
		if out, err := w.runMiseTask([]string{"CI=true"}, "", "cloudflare:12-login"); err == nil || !strings.Contains(out, "in CI there's no one to log in") {
			w.t.Fatalf("in CI, a login didn't stop at once: %s", out)
		}
		w.pass("in CI, cloudflare:12-login stops at once: no one is there to click Allow")
		if w.upTo >= deploy {
			w.mustRunMiseTask(nil, "cloudflare:12-login")
			w.pass("cloudflare:12-login: the repo's own login, %s", w.login)
		}
	}},
	{task: "13-delete", stage: deploy, check: func(w *walk) {
		if w.upTo >= connectGithub {
			w.mustRunMiseTask(nil, "cloudflare:09-connect-github", "--off")
			w.pass("Workers Builds is off")
			w.mustRunMiseTask(nil, "cloudflare:09-connect-github", "--off")
			w.pass("Workers Builds is off again: cf's 404 for a connection that's gone is taken as gone (cf.NotFound)")
			w.mustIn(w.top, "git", "push", "-q", "origin", "--delete", w.prod)
			w.pass("the branch %s is deleted from GitHub", w.prod)
		}
		w.mustRunMiseTask(nil, "cloudflare:13-delete", "--yes")
		urls := []string{w.url}
		if w.preview != "" {
			urls = append(urls, w.preview)
		}
		for _, u := range urls {
			if !gone(u, time.Minute) {
				w.t.Fatalf("%s still answers", u)
			}
		}
		w.pass("no longer answering: %s", strings.Join(urls, ", "))
		if left := w.leftOnCloudflare(); len(left) > 0 {
			w.t.Fatalf("left on the account: %s", strings.Join(left, ", "))
		}
		if withExtras() {
			w.pass("no D1, KV, R2 or Durable Object left, nor the extras' Queue or Flagship app, nor the previews' own")
		} else {
			w.pass("no D1, KV, R2 or Durable Object left")
		}
	}},
	{task: "14-logout", stage: deploy, check: func(w *walk) {
		if os.Getenv("DEPLOY_CLEAN") != "true" {
			w.pass("the login kept, so the next walk needs no Allow (DEPLOY_CLEAN=true logs out)")
			return
		}
		w.mustRunMiseTask(nil, "cloudflare:14-logout")
		if w.miseShowJSON().LoggedIn {
			w.t.Fatal("the login is still there")
		}
		w.pass("the login is gone")
	}},
	{task: "15-clean", stage: serve, check: func(w *walk) {
		w.mustRunMiseTask(nil, "cloudflare:15-clean", "--yes")
		for _, p := range []string{".wrangler", ".dev.vars"} {
			if _, err := os.Stat(filepath.Join(w.dir, p)); err == nil {
				w.t.Fatalf("%s is still there", p)
			}
		}
		w.pass("no .wrangler/ or .dev.vars left")
	}},
}

// TestSteps checks that the steps are the template's numbered tasks (templates/.mise/tasks/mise-cloudflare.toml's),
// in their order, so the walk is that file's walk.
func TestSteps(t *testing.T) {
	b, err := os.ReadFile("templates/.mise/tasks/mise-cloudflare.toml")
	if err != nil {
		t.Fatal(err)
	}
	var tasks, ours []string
	for _, m := range regexp.MustCompile(`(?m)^\["cloudflare:(\d\d-[a-z-]+)"\]`).FindAllStringSubmatch(string(b), -1) {
		tasks = append(tasks, m[1])
	}
	for _, s := range steps {
		ours = append(ours, s.task)
	}
	if !slices.Equal(ours, tasks) {
		t.Errorf("the steps %v aren't the template's numbered tasks, in their order: %v", ours, tasks)
	}
}
