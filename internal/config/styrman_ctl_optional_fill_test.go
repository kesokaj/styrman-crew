package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// BUG-081: CLOUDFLARE_API_TOKEN left the must-fill keys in v1.39.0, and
// `styrman-ctl install --env-file` fills .env from the must-fill list only —
// so a token the operator put in the answers file was dropped without a word
// and the host issued certificates over HTTP-01 anyway.
// TestStyrmanCtlInstallPlan covers the token being written when given (its
// six planned writes); these are the other two branches.

func optionalFillFixture(t *testing.T, answersBody string) (ctlEnv, string, string) {
	t.Helper()
	e := ctlSetup(t, true)
	envFile := filepath.Join(e.root, "checkout.env")
	if err := os.WriteFile(envFile, []byte("BASE_DOMAIN=\nGOOGLE_CLIENT_ID=\nGOOGLE_CLIENT_SECRET=\nCLOUDFLARE_API_TOKEN=\nADMIN_EMAIL=\nSTYRMAN_ALLOWED_EMAILS=\nSTYRMAN_HOSTNAME=crew\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	answers := filepath.Join(e.root, "secrets", "answers.env")
	if err := os.WriteFile(answers, []byte(answersBody), 0o600); err != nil {
		t.Fatal(err)
	}
	e.env = append(e.env, "STYRMAN_ENV_FILE="+envFile, "SUDO_USER=tester")
	return e, envFile, answers
}

const mustFillAnswers = "BASE_DOMAIN=example.test\nGOOGLE_CLIENT_ID=id.apps\nGOOGLE_CLIENT_SECRET=s3cret\nADMIN_EMAIL=a@example.test\nSTYRMAN_ALLOWED_EMAILS=a@example.test,b@example.test\n"

// An answers file without the optional token plans the five must-fill writes
// and nothing for the token: optional means never required and never asked.
func TestStyrmanCtlInstallOptionalKeyAbsent(t *testing.T) {
	e, _, answers := optionalFillFixture(t, mustFillAnswers)
	out, code := ctl(t, e, "install", "--dry-run", "--yes", "--version", "1.2.3", "--env-file", answers, "--skip-auto-update")
	if code != 0 {
		t.Fatalf("install --dry-run without the optional token: %d\n%s", code, out)
	}
	if n := strings.Count(out, "+ env_set "); n != 5 {
		t.Fatalf("expected 5 planned .env writes (the must-fill keys), got %d:\n%s", n, out)
	}
	if strings.Contains(out, "CLOUDFLARE_API_TOKEN") {
		t.Fatalf("the plan names CLOUDFLARE_API_TOKEN although the answers file does not carry it:\n%s", out)
	}
}

// A token that is given but invalid stops the install; skipping it would be
// the same silent HTTP-01 fallback by another route.
func TestStyrmanCtlInstallOptionalKeyInvalidStops(t *testing.T) {
	e, _, answers := optionalFillFixture(t, mustFillAnswers+"CLOUDFLARE_API_TOKEN=has space\n")
	out, code := ctl(t, e, "install", "--dry-run", "--yes", "--version", "1.2.3", "--env-file", answers, "--skip-auto-update")
	if code == 0 {
		t.Fatalf("install accepted an invalid CLOUDFLARE_API_TOKEN:\n%s", out)
	}
	if !strings.Contains(out, "invalid value for CLOUDFLARE_API_TOKEN") {
		t.Fatalf("the refusal does not name the key:\n%s", out)
	}
	if strings.Contains(out, "docker build") {
		t.Fatalf("the install went on to the image build after an invalid token:\n%s", out)
	}
}
