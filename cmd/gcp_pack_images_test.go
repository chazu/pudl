package cmd

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// fakeImagesGcloud answers the gcloud commands of gcp-container-images for
// the projects proj-a and proj-b, logging every invocation to $GCLOUD_LOG.
// Each project has a gcr.io repository in us and an apps repository in
// us-central1; apps holds a nested image (web/frontend, listed escaped) and
// an image with no tagged version. The version list ignores the filter, sort
// and limit flags, which the test reads from the log instead.
const fakeImagesGcloud = `#!/bin/sh
echo "$*" >> "$GCLOUD_LOG"
for a in "$@"; do
	case $a in
	--project=*) p=${a#--project=} ;;
	--repository=*) repo=${a#--repository=} ;;
	esac
done
digest=sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa
case "$1 $2 $3" in
"projects list --format=json") echo '[{"projectId":"proj-a"},{"projectId":"proj-b"}]' ;;
"artifacts repositories list") printf 'us\tgcr.io\nus-central1\tapps\n' ;;
"artifacts packages list")
	if [ "$repo" = gcr.io ]; then echo api; else printf 'web%%2Ffrontend\nscratch\n'; fi ;;
"artifacts docker images")
	case $4 in
	*/scratch) echo '[]' ;;
	*) printf '[{"package":"%s","version":"%s","tags":["v1","latest"],"createTime":"2026-11-01T00:00:00Z","updateTime":"2026-11-02T00:00:00Z","metadata":{"imageSizeBytes":"1024","mediaType":"application/vnd.oci.image.index.v1+json"}}]\n' "$4" "$digest" ;;
	esac ;;
*) echo "unexpected gcloud $*" >&2; exit 1 ;;
esac
`

// The bundled fixture stores one record per image: the newest tagged
// digest, and per version its tags, push time, size and media type.
func TestGCPPackContainerImagesFixture(t *testing.T) {
	cliWorkspace(t)
	r := runCLI(t, "pack", "install", "gcp")
	require.NoError(t, r.Err, r.Stdout+r.Stderr)

	r = runCLI(t, "check", "gcp-fixture-container-images", "--json")
	require.NoError(t, r.Err, r.Stdout+r.Stderr)
	pr := checkedModel(t, r.Stdout).Populate
	require.Equal(t, 3, pr.Records, "one per image, the untagged-only image included")

	r = runCLI(t, "query", "gcp_container_image_tag")
	require.NoError(t, r.Err, r.Stdout+r.Stderr)
	require.Equal(t, 4, strings.Count(r.Stdout, `"tag":`), r.Stdout)
	for _, tag := range []string{"latest", "3f2a9c1", "8be04d7", "v1.4.0"} {
		require.Contains(t, r.Stdout, `"`+tag+`"`)
	}

	r = runCLI(t, "list", "--json", "--all", "--schema", "gcp.#ContainerImage", "--select", "name", "--select", "digest")
	require.NoError(t, r.Err, r.Stdout+r.Stderr)
	require.Contains(t, r.Stdout, "us-central1-docker.pkg.dev/example-project/apps/web/frontend")
	require.Contains(t, r.Stdout, "sha256:1111111111111111111111111111111111111111111111111111111111111111")
}

// The live collector runs the helper per project: every Docker repository,
// every image in it (nested names unescaped), then each image's newest
// _gcpImageHistory tagged versions, newest push first.
func TestGCPPackContainerImagesThroughGcloud(t *testing.T) {
	pudlDir := cliWorkspace(t)
	bin := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(bin, "gcloud"), []byte(fakeImagesGcloud), 0o755))
	log := filepath.Join(t.TempDir(), "gcloud.log")
	t.Setenv("GCLOUD_LOG", log)
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))

	r := runCLI(t, "pack", "install", "gcp")
	require.NoError(t, r.Err, r.Stdout+r.Stderr)
	writeFile(t, filepath.Join(pudlDir, "schema", "models", "gcp_settings.cue"), "package models\n\n_gcpImageHistory: 3\n")

	r = runCLI(t, "check", "gcp-container-images", "--json")
	require.NoError(t, r.Err, r.Stdout+r.Stderr)
	pr := checkedModel(t, r.Stdout).Populate
	require.Nil(t, pr.Fallback, "gcloud only")
	require.Empty(t, pr.FailedInputs)
	require.Equal(t, 6, pr.Records, "three images per project")
	require.True(t, snapshotComplete(t, pr.SnapshotID))

	calls := readLog(t, log)
	require.NotContains(t, calls, "asset ")
	require.Contains(t, calls, "artifacts repositories list --project=proj-b --filter=format=DOCKER AND mode=STANDARD_REPOSITORY")
	require.Contains(t, calls, "artifacts packages list --project=proj-a --location=us-central1 --repository=apps --page-size=1000 --format=value(name)")
	require.Contains(t, calls, `artifacts docker images list us-central1-docker.pkg.dev/proj-a/apps/web/frontend --include-tags --filter=tags:* AND package="us-central1-docker.pkg.dev/proj-a/apps/web/frontend" --sort-by=~CREATE_TIME --limit=3 --page-size=1000 --format=json`)
	require.Contains(t, calls, "artifacts docker images list us-docker.pkg.dev/proj-b/gcr.io/api ")

	r = runCLI(t, "query", "gcp_container_image")
	require.NoError(t, r.Err, r.Stdout+r.Stderr)
	require.Contains(t, r.Stdout, "projects/proj-b/locations/us-central1/repositories/apps")
}
