package coordinator

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"log"
	"strings"

	"github.com/gofiber/fiber/v2"
	"github.com/sasiruLK/tinycloud-platform/internal/build/types"
)

// GitHub push webhooks, so a commit to an app repo rebuilds it.
//
// Before this, a rebuild was a button someone had to remember to press: the
// platform would deploy a new image the moment one existed, but nothing made
// one exist. This closes that loop.
//
// A webhook rather than polling, for one reason that matters more than
// latency: the platform holds no credential on the app repository. The repo
// pushes to us with a shared secret, instead of us holding a token that can
// read someone's code.

type pushEvent struct {
	Ref        string `json:"ref"`
	Deleted    bool   `json:"deleted"`
	Repository struct {
		CloneURL string `json:"clone_url"`
		HTMLURL  string `json:"html_url"`
		FullName string `json:"full_name"`
	} `json:"repository"`
	HeadCommit *struct {
		ID string `json:"id"`
	} `json:"head_commit"`
}

func (s *Server) githubWebhook(c *fiber.Ctx) error {
	if s.webhookSecret == "" {
		return c.Status(fiber.StatusServiceUnavailable).JSON(fiber.Map{"error": "webhooks not configured"})
	}

	body := c.Body()
	if !validSignature(s.webhookSecret, body, c.Get("X-Hub-Signature-256")) {
		// Deliberately vague: a caller who cannot sign gets no help distinguishing
		// "wrong secret" from "no such route".
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": "unauthorized"})
	}

	// Anything that is not a push is acknowledged and ignored. Returning an
	// error would make GitHub retry, and mark the hook as failing in the UI, for
	// events we simply do not act on.
	if c.Get("X-GitHub-Event") != "push" {
		return c.JSON(fiber.Map{"status": "ignored", "reason": "not a push event"})
	}

	var ev pushEvent
	if err := json.Unmarshal(body, &ev); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid payload"})
	}
	if ev.Deleted || ev.HeadCommit == nil {
		return c.JSON(fiber.Map{"status": "ignored", "reason": "branch deleted or no head commit"})
	}

	branch := strings.TrimPrefix(ev.Ref, "refs/heads/")
	if branch == ev.Ref {
		return c.JSON(fiber.Map{"status": "ignored", "reason": "not a branch push"})
	}

	ctx := context.Background()
	apps, err := s.store.ListAppRepos(ctx)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "failed to look up apps"})
	}

	pushed := normalizeRepo(ev.Repository.CloneURL)
	if pushed == "" {
		pushed = normalizeRepo(ev.Repository.HTMLURL)
	}

	triggered := []string{}
	for _, a := range apps {
		// Both must match. A push to a branch nobody deploys from is not a
		// deployment, and rebuilding on it would ship code from the wrong branch.
		if normalizeRepo(a.RepoURL) != pushed || a.Ref != branch {
			continue
		}

		previous, err := s.store.GetJobByAppName(ctx, a.AppName)
		if err != nil || previous == nil {
			continue
		}
		// One build at a time per app. A busy repo can push several times in a
		// minute and each would otherwise race the last to commit manifests.
		switch previous.Status {
		case types.StatusQueued, types.StatusRunning:
			log.Printf("webhook: %s already building, skipping", a.AppName)
			continue
		}

		job, err := s.startRebuild(ctx, previous)
		if err != nil {
			log.Printf("webhook: rebuild for %s failed: %v", a.AppName, err)
			continue
		}
		triggered = append(triggered, a.AppName+"="+job.ID)
	}

	if len(triggered) == 0 {
		// Still a 200: the delivery was valid and correctly signed, there was just
		// nothing deployed from that repo and branch. A failure here would show up
		// as a broken hook on a repo that is simply not onboarded.
		return c.JSON(fiber.Map{"status": "ignored", "reason": "no app builds from " + ev.Repository.FullName + "@" + branch})
	}

	log.Printf("webhook: %s@%s triggered %d build(s)", ev.Repository.FullName, branch, len(triggered))
	return c.Status(fiber.StatusAccepted).JSON(fiber.Map{"status": "queued", "builds": triggered})
}

// validSignature checks GitHub's X-Hub-Signature-256 over the raw body.
//
// hmac.Equal, not ==, because a byte-by-byte comparison leaks how much of the
// signature was correct through how long it took to reject.
func validSignature(secret string, body []byte, header string) bool {
	const prefix = "sha256="
	if !strings.HasPrefix(header, prefix) {
		return false
	}
	sent, err := hex.DecodeString(strings.TrimPrefix(header, prefix))
	if err != nil {
		return false
	}
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(body)
	return hmac.Equal(sent, mac.Sum(nil))
}

// normalizeRepo makes the repo URL comparable across the several forms GitHub
// and users write it in: clone_url has .git, html_url does not, and case and
// trailing slashes vary.
func normalizeRepo(u string) string {
	u = strings.TrimSpace(strings.ToLower(u))
	u = strings.TrimSuffix(u, "/")
	u = strings.TrimSuffix(u, ".git")
	// Replaced, not trimmed: trimming drops the host and leaves "owner/repo",
	// which then never equals the normalized https form and the push silently
	// matches nothing.
	u = strings.Replace(u, "git@github.com:", "github.com/", 1)
	u = strings.TrimPrefix(u, "https://")
	u = strings.TrimPrefix(u, "http://")
	u = strings.TrimPrefix(u, "www.")
	return u
}
