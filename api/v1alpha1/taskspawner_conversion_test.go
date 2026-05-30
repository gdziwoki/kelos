package v1alpha1

import (
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	v1alpha2 "github.com/kelos-dev/kelos/api/v1alpha2"
)

func TestTaskSpawnerConvertTo_FoldsLegacyCommentAndPollInterval(t *testing.T) {
	src := &TaskSpawner{
		ObjectMeta: metav1.ObjectMeta{Name: "ts", Namespace: "default"},
		Spec: TaskSpawnerSpec{
			PollInterval: "7m",
			When: When{
				GitHubIssues: &GitHubIssues{
					Repo:            "owner/repo",
					TriggerComment:  "/kelos go",
					ExcludeComments: []string{"/kelos stop"},
				},
			},
		},
	}

	dst := &v1alpha2.TaskSpawner{}
	if err := src.ConvertTo(dst); err != nil {
		t.Fatalf("ConvertTo() error = %v", err)
	}

	gi := dst.Spec.When.GitHubIssues
	if gi == nil {
		t.Fatal("githubIssues nil after conversion")
	}
	if gi.PollInterval != "7m" {
		t.Errorf("githubIssues.pollInterval = %q, want 7m (folded from root)", gi.PollInterval)
	}
	if gi.CommentPolicy == nil {
		t.Fatal("commentPolicy nil; legacy fields were not folded")
	}
	if gi.CommentPolicy.TriggerComment != "/kelos go" {
		t.Errorf("commentPolicy.triggerComment = %q", gi.CommentPolicy.TriggerComment)
	}
	if len(gi.CommentPolicy.ExcludeComments) != 1 || gi.CommentPolicy.ExcludeComments[0] != "/kelos stop" {
		t.Errorf("commentPolicy.excludeComments = %v", gi.CommentPolicy.ExcludeComments)
	}
}

func TestTaskSpawnerConvertTo_DoesNotOverrideSourcePollInterval(t *testing.T) {
	src := &TaskSpawner{
		Spec: TaskSpawnerSpec{
			PollInterval: "7m",
			When: When{
				GitHubPullRequests: &GitHubPullRequests{
					Repo:         "owner/repo",
					PollInterval: "2m",
				},
			},
		},
	}
	dst := &v1alpha2.TaskSpawner{}
	if err := src.ConvertTo(dst); err != nil {
		t.Fatalf("ConvertTo() error = %v", err)
	}
	if got := dst.Spec.When.GitHubPullRequests.PollInterval; got != "2m" {
		t.Errorf("githubPullRequests.pollInterval = %q, want 2m (source value kept)", got)
	}
}

func TestTaskSpawnerConvertTo_BodyContainsToBodyPattern(t *testing.T) {
	src := &TaskSpawner{
		Spec: TaskSpawnerSpec{
			When: When{
				GitHubWebhook: &GitHubWebhook{
					Filters: []GitHubWebhookFilter{
						{BodyContains: "/deploy v1.2+x"},
					},
				},
			},
		},
	}
	dst := &v1alpha2.TaskSpawner{}
	if err := src.ConvertTo(dst); err != nil {
		t.Fatalf("ConvertTo() error = %v", err)
	}
	got := dst.Spec.When.GitHubWebhook.Filters[0].BodyPattern
	// QuoteMeta escapes the regex metacharacters in the literal.
	want := `/deploy v1\.2\+x`
	if got != want {
		t.Errorf("bodyPattern = %q, want %q", got, want)
	}
}

func TestTaskSpawnerConvert_ModernFieldsRoundTrip(t *testing.T) {
	optional := "5m"
	src := &TaskSpawner{
		Spec: TaskSpawnerSpec{
			When: When{
				GitHubIssues: &GitHubIssues{
					Repo:         "owner/repo",
					PollInterval: optional,
					CommentPolicy: &GitHubCommentPolicy{
						TriggerComment:    "/go",
						MinimumPermission: "write",
					},
				},
			},
		},
	}
	hub := &v1alpha2.TaskSpawner{}
	if err := src.ConvertTo(hub); err != nil {
		t.Fatalf("ConvertTo() error = %v", err)
	}
	back := &TaskSpawner{}
	if err := back.ConvertFrom(hub); err != nil {
		t.Fatalf("ConvertFrom() error = %v", err)
	}
	gi := back.Spec.When.GitHubIssues
	if gi.PollInterval != "5m" || gi.CommentPolicy == nil || gi.CommentPolicy.TriggerComment != "/go" || gi.CommentPolicy.MinimumPermission != "write" {
		t.Errorf("modern fields not preserved: %#v", gi)
	}
}
