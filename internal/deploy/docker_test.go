package deploy_test

import (
	"os"
	"strings"
	"testing"
)

// repoFile returns the absolute path to a file in the repository root.
// Tests run with cwd = package dir, so walk up to find the module root.
func repoFile(t *testing.T, name string) string {
	t.Helper()
	dir, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	// Walk up until we find go.mod
	for {
		if _, err := os.Stat(dir + "/go.mod"); err == nil {
			return dir + "/" + name
		}
		parent := dir[:strings.LastIndex(dir, "/")]
		if parent == dir {
			t.Fatalf("could not find repository root from %s", dir)
		}
		dir = parent
	}
}

func readFile(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return string(b)
}

// --- Dockerfile tests ---

func TestDockerfile_GoBuilderImageTag(t *testing.T) {
	content := readFile(t, repoFile(t, "Dockerfile"))
	// Must use minor-version tag (e.g. golang:1.25-alpine), not a patch tag
	// that may not exist on Docker Hub (e.g. golang:1.25.9-alpine).
	if strings.Contains(content, "golang:1.25.9") {
		t.Error("Dockerfile contains non-existent patch tag golang:1.25.9-alpine; use golang:1.25-alpine")
	}
	if !strings.Contains(content, "FROM golang:1.25-alpine AS go-builder") {
		t.Error("Dockerfile go-builder stage must use FROM golang:1.25-alpine AS go-builder")
	}
}

func TestDockerfile_ThreeStages(t *testing.T) {
	content := readFile(t, repoFile(t, "Dockerfile"))
	stages := []string{"AS web-builder", "AS go-builder"}
	for _, s := range stages {
		if !strings.Contains(content, s) {
			t.Errorf("Dockerfile missing build stage %q", s)
		}
	}
	// Runtime stage: no alias needed, just FROM alpine
	if !strings.Contains(content, "FROM alpine:") {
		t.Error("Dockerfile missing runtime stage (FROM alpine:...)")
	}
}

func TestDockerfile_EntrypointCmdSplit(t *testing.T) {
	content := readFile(t, repoFile(t, "Dockerfile"))
	// ENTRYPOINT sets the binary; CMD provides default subcommand.
	// This split allows `docker compose run ocp-cli status` to override CMD.
	if !strings.Contains(content, `ENTRYPOINT ["ocp"]`) {
		t.Error(`Dockerfile must have ENTRYPOINT ["ocp"]`)
	}
	if !strings.Contains(content, `CMD ["serve"]`) {
		t.Error(`Dockerfile must have CMD ["serve"] so compose can override the subcommand`)
	}
}

func TestDockerfile_ExposesPort8080(t *testing.T) {
	content := readFile(t, repoFile(t, "Dockerfile"))
	if !strings.Contains(content, "EXPOSE 8080") {
		t.Error("Dockerfile must EXPOSE 8080")
	}
}

// --- docker-compose.yaml tests ---

func TestCompose_OcpServicePresent(t *testing.T) {
	content := readFile(t, repoFile(t, "docker-compose.yaml"))
	if !strings.Contains(content, "container_name: ocp-server") {
		t.Error("docker-compose.yaml must define ocp-server container")
	}
}

func TestCompose_DataVolumeMount(t *testing.T) {
	content := readFile(t, repoFile(t, "docker-compose.yaml"))
	// ./data:/data ensures the SQLite database survives container restarts (DOCK-03).
	if !strings.Contains(content, "./data:/data") {
		t.Error("docker-compose.yaml must mount ./data:/data for database persistence (DOCK-03)")
	}
}

func TestCompose_ConfigVolumeMount(t *testing.T) {
	content := readFile(t, repoFile(t, "docker-compose.yaml"))
	if !strings.Contains(content, "config.yaml:/app/config.yaml") {
		t.Error("docker-compose.yaml must mount config.yaml into /app/config.yaml")
	}
}

func TestCompose_CliProfile(t *testing.T) {
	content := readFile(t, repoFile(t, "docker-compose.yaml"))
	// ocp-cli must be gated behind a profile so it doesn't start on `docker compose up` (DOCK-04).
	if !strings.Contains(content, "profiles:") {
		t.Error("docker-compose.yaml must define profiles: so ocp-cli doesn't start by default")
	}
	if !strings.Contains(content, "container_name: ocp-cli") {
		t.Error("docker-compose.yaml must define ocp-cli container")
	}
}

func TestCompose_EncryptionKeyEnv(t *testing.T) {
	content := readFile(t, repoFile(t, "docker-compose.yaml"))
	if !strings.Contains(content, "OCP_ENCRYPTION_KEY") {
		t.Error("docker-compose.yaml must pass OCP_ENCRYPTION_KEY env var to the ocp service")
	}
}

// --- .dockerignore tests ---

func TestDockerignore_ExcludesSecrets(t *testing.T) {
	content := readFile(t, repoFile(t, ".dockerignore"))
	secrets := []string{"config.yaml", ".env"}
	for _, s := range secrets {
		if !strings.Contains(content, s) {
			t.Errorf(".dockerignore must exclude %q to prevent secrets leaking into the image", s)
		}
	}
}

func TestDockerignore_ExcludesGit(t *testing.T) {
	content := readFile(t, repoFile(t, ".dockerignore"))
	if !strings.Contains(content, ".git") {
		t.Error(".dockerignore must exclude .git to keep image size down")
	}
}
