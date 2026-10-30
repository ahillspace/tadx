//go:build live

package flow_test

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"hash"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	coreauth "github.com/ahillspace/tadx/internal/auth"
	"github.com/ahillspace/tadx/internal/config"
	"github.com/ahillspace/tadx/internal/tableau"
	tableauauth "github.com/ahillspace/tadx/internal/tableau/auth"
	tableauflow "github.com/ahillspace/tadx/internal/tableau/flow"
)

func TestLiveFlowLifecycleContract(t *testing.T) {
	sourceFlowLUID := requiredLiveFlowValue(t, "TADX_LIVE_FLOW_LUID")
	sourceProjectLUID := requiredLiveFlowValue(t, "TADX_LIVE_FLOW_PROJECT_LUID")
	destinationProjectLUID := strings.TrimSpace(os.Getenv("TADX_LIVE_FLOW_DEST_PROJECT_LUID"))
	if destinationProjectLUID == sourceProjectLUID {
		t.Fatal("TADX_LIVE_FLOW_DEST_PROJECT_LUID must differ from TADX_LIVE_FLOW_PROJECT_LUID")
	}

	environment, session, transport := liveFlowConnection(t)
	client := tableauflow.NewClient(transport, session, environment.URL)
	ctx := context.Background()

	source, err := client.Get(ctx, sourceFlowLUID)
	if err != nil {
		fatalLiveFlow(t, "read source flow", err)
	}
	if source.LUID != sourceFlowLUID || source.ProjectLUID != sourceProjectLUID {
		t.Fatal("source flow identity did not match the configured flow and project LUIDs")
	}

	payload, err := os.CreateTemp(t.TempDir(), "payload")
	if err != nil {
		fatalLiveFlow(t, "stage downloaded flow", err)
	}
	sourceDigest := sha256.New()
	download, err := client.Download(ctx, sourceFlowLUID, io.MultiWriter(payload, sourceDigest))
	if closeErr := payload.Close(); err == nil && closeErr != nil {
		err = closeErr
	}
	if err != nil {
		fatalLiveFlow(t, "download source flow", err)
	}
	extension := strings.ToLower(filepath.Ext(download.Filename))
	if extension != ".tfl" && extension != ".tflx" {
		t.Fatalf("source flow returned unsupported extension %q", extension)
	}
	sourceHash := flowFingerprint(sourceDigest)
	payloadPath := payload.Name() + extension
	if err := os.Rename(payload.Name(), payloadPath); err != nil {
		fatalLiveFlow(t, "stage downloaded flow", err)
	}
	payloadInfo, err := os.Stat(payloadPath)
	if err != nil {
		fatalLiveFlow(t, "stage downloaded flow", err)
	}

	disposableName := uniqueLiveFlowName(t)
	disposableLUID := ""
	deleteAttempted := false
	t.Cleanup(func() {
		if deleteAttempted {
			return
		}
		deleteAttempted = true
		luid := disposableLUID
		if luid == "" {
			luid = findDisposableLiveFlow(t, client, disposableName, sourceProjectLUID, sourceFlowLUID)
		}
		if luid == "" || luid == sourceFlowLUID {
			return
		}
		if _, err := client.Delete(context.Background(), luid); err != nil {
			t.Errorf("cleanup disposable flow failed: %T", err)
		}
	})

	prepared, err := client.Prepare(ctx, tableauflow.PublishRequest{
		Name:                disposableName,
		ProjectLUID:         sourceProjectLUID,
		Filename:            "payload" + extension,
		ContentPath:         payloadPath,
		ContentSize:         payloadInfo.Size(),
		ExpectedFingerprint: sourceHash,
	})
	if err != nil {
		fatalLiveFlow(t, "prepare disposable flow publish", err)
	}
	published, err := prepared.Commit(ctx)
	disposableLUID = published.FlowLUID
	if err != nil {
		fatalLiveFlow(t, "publish disposable flow", err)
	}
	if published.Status != "succeeded" || disposableLUID == "" || disposableLUID == sourceFlowLUID || published.FlowName != disposableName || published.ProjectLUID != sourceProjectLUID {
		t.Fatal("published flow response omitted or changed the disposable flow identity")
	}

	publishedFlow, err := client.Get(ctx, disposableLUID)
	if err != nil {
		fatalLiveFlow(t, "read published flow", err)
	}
	if publishedFlow.LUID != disposableLUID || publishedFlow.Name != disposableName || publishedFlow.ProjectLUID != sourceProjectLUID {
		t.Fatal("published flow read did not preserve the exact flow identity")
	}
	publishedDigest := sha256.New()
	reDownloaded, err := client.Download(ctx, disposableLUID, publishedDigest)
	if err != nil {
		fatalLiveFlow(t, "download published flow", err)
	}
	if got := strings.ToLower(filepath.Ext(reDownloaded.Filename)); got != extension {
		t.Fatalf("published flow extension = %q, want %q", got, extension)
	}
	publishedHash := flowFingerprint(publishedDigest)
	if publishedHash != sourceHash {
		t.Fatal("published flow content hash did not match the unchanged source payload")
	}

	finalProjectLUID := sourceProjectLUID
	moveCount := 0
	if destinationProjectLUID != "" {
		moved, err := client.Move(ctx, disposableLUID, destinationProjectLUID)
		if err != nil {
			fatalLiveFlow(t, "move disposable flow", err)
		}
		if moved.Status != "succeeded" || moved.FlowLUID != disposableLUID || moved.ProjectLUID != destinationProjectLUID {
			t.Fatal("flow move response omitted or changed the exact identities")
		}
		movedFlow, err := client.Get(ctx, disposableLUID)
		if err != nil {
			fatalLiveFlow(t, "read moved flow", err)
		}
		if movedFlow.LUID != disposableLUID || movedFlow.ProjectLUID != destinationProjectLUID {
			t.Fatal("moved flow read did not preserve the exact destination identity")
		}
		finalProjectLUID = destinationProjectLUID
		moveCount = 1
	}

	deleteAttempted = true
	deleted, err := client.Delete(ctx, disposableLUID)
	if err != nil {
		fatalLiveFlow(t, "delete disposable flow", err)
	}
	if deleted.Status != "succeeded" || deleted.FlowLUID != disposableLUID {
		t.Fatal("flow delete response omitted or changed the exact disposable identity")
	}

	t.Logf(
		"sanitized flow lifecycle evidence: source_flow=%s published_flow=%s source_project=%s final_project=%s extension=%s bytes=%d payload_sha256=%s publishes=1 moves=%d deletes=1",
		hashFlowID(sourceFlowLUID),
		hashFlowID(disposableLUID),
		hashFlowID(sourceProjectLUID),
		hashFlowID(finalProjectLUID),
		extension,
		payloadInfo.Size(),
		sourceHash,
		moveCount,
	)
}

func requiredLiveFlowValue(t *testing.T, name string) string {
	t.Helper()
	value := strings.TrimSpace(os.Getenv(name))
	if value == "" {
		t.Skipf("set %s to run the live flow lifecycle contract test", name)
	}
	return value
}

func findDisposableLiveFlow(t *testing.T, client *tableauflow.RESTClient, name, projectLUID, sourceFlowLUID string) string {
	t.Helper()
	page, err := client.List(context.Background(), tableauflow.ListRequest{
		PageNumber:  1,
		PageSize:    100,
		Name:        name,
		ProjectLUID: projectLUID,
	})
	if err != nil {
		t.Errorf("locate disposable flow for cleanup failed: %T", err)
		return ""
	}
	var luid string
	for _, candidate := range page.Items {
		if candidate.Name != name || candidate.ProjectLUID != projectLUID || candidate.LUID == "" || candidate.LUID == sourceFlowLUID {
			continue
		}
		if luid != "" && luid != candidate.LUID {
			t.Error("cleanup found multiple disposable flows with the generated exact name")
			return ""
		}
		luid = candidate.LUID
	}
	return luid
}

func uniqueLiveFlowName(t *testing.T) string {
	t.Helper()
	var nonce [8]byte
	if _, err := rand.Read(nonce[:]); err != nil {
		fatalLiveFlow(t, "generate disposable flow name", err)
	}
	return fmt.Sprintf("tadx-live-flow-%s-%x", time.Now().UTC().Format("20060102T150405Z"), nonce[:])
}

func flowFingerprint(digest hash.Hash) string {
	return "sha256:" + hex.EncodeToString(digest.Sum(nil))
}

func fatalLiveFlow(t *testing.T, operation string, err error) {
	t.Helper()
	t.Fatalf("%s failed: %T", operation, err)
}

func liveFlowConnection(t *testing.T) (config.Environment, coreauth.Session, *tableau.Transport) {
	t.Helper()
	path := strings.TrimSpace(os.Getenv("TADX_LIVE_CONFIG"))
	if path == "" {
		var err error
		path, err = config.UserConfigPath()
		if err != nil {
			fatalLiveFlow(t, "resolve live configuration", err)
		}
	}
	configuration, err := config.Load(path)
	if err != nil {
		fatalLiveFlow(t, "load live configuration", err)
	}
	alias := strings.TrimSpace(os.Getenv("TADX_LIVE_ENVIRONMENT"))
	if alias == "" {
		alias = "dev"
	}
	environment, err := configuration.ResolveEnvironment(alias)
	if err != nil {
		fatalLiveFlow(t, "resolve live environment", err)
	}
	transport := tableau.NewTransport(nil, "", func() string { return "flow-live-contract" })
	if err := transport.NegotiateAPIVersion(t.Context(), environment.URL); err != nil {
		t.Fatal(err)
	}
	provider := coreauth.NewPATProvider(coreauth.LookupEnvFunc(os.LookupEnv), tableauauth.NewClient(transport))
	session, err := provider.Authenticate(context.Background(), coreauth.Target{Environment: environment.Alias, ServerURL: environment.URL, SiteContentURL: environment.SiteContentURL, PATNameVariable: environment.Auth.PATNameEnv, PATSecretVariable: environment.Auth.PATSecretEnv})
	if err != nil {
		fatalLiveFlow(t, "authenticate live flow client", err)
	}
	return environment, session, transport
}

func hashFlowID(value string) string {
	sum := sha256.Sum256([]byte(value))
	return fmt.Sprintf("sha256:%x", sum[:8])
}
