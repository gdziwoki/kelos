package v1alpha1

import (
	"regexp"

	"sigs.k8s.io/controller-runtime/pkg/conversion"

	v1alpha2 "github.com/kelos-dev/kelos/api/v1alpha2"
)

// TaskSpawner v1alpha2 drops three deprecated v1alpha1 field groups:
//   - root spec.pollInterval (per-source pollInterval is retained)
//   - legacy triggerComment/excludeComments on githubIssues/githubPullRequests
//     (commentPolicy is the only supported shape)
//   - bodyContains on the githubWebhook filter (bodyPattern only)
//
// The bulk conversion is a structural JSON copy; the dropped fields are folded
// into their modern equivalents on the way to v1alpha2.

func (src *TaskSpawner) ConvertTo(dstRaw conversion.Hub) error {
	dst := dstRaw.(*v1alpha2.TaskSpawner)
	dst.ObjectMeta = src.ObjectMeta
	if err := convertViaJSON(&src.Spec, &dst.Spec); err != nil {
		return err
	}
	if err := convertViaJSON(&src.Status, &dst.Status); err != nil {
		return err
	}
	foldTaskSpawnerForward(&src.Spec, &dst.Spec)
	return nil
}

func (dst *TaskSpawner) ConvertFrom(srcRaw conversion.Hub) error {
	src := srcRaw.(*v1alpha2.TaskSpawner)
	dst.ObjectMeta = src.ObjectMeta
	if err := convertViaJSON(&src.Spec, &dst.Spec); err != nil {
		return err
	}
	return convertViaJSON(&src.Status, &dst.Status)
}

// foldTaskSpawnerForward maps the v1alpha1-only deprecated fields onto their
// v1alpha2 replacements after the structural copy.
func foldTaskSpawnerForward(src *TaskSpawnerSpec, dst *v1alpha2.TaskSpawnerSpec) {
	// Root pollInterval -> the active polling source, unless that source
	// already sets its own.
	if src.PollInterval != "" {
		if gi := dst.When.GitHubIssues; gi != nil && gi.PollInterval == "" {
			gi.PollInterval = src.PollInterval
		}
		if pr := dst.When.GitHubPullRequests; pr != nil && pr.PollInterval == "" {
			pr.PollInterval = src.PollInterval
		}
		if j := dst.When.Jira; j != nil && j.PollInterval == "" {
			j.PollInterval = src.PollInterval
		}
	}

	// Legacy triggerComment/excludeComments -> commentPolicy. v1alpha1
	// validation forbids setting both, so commentPolicy is nil here whenever
	// the legacy fields are used.
	if gi := src.When.GitHubIssues; gi != nil && dst.When.GitHubIssues != nil &&
		dst.When.GitHubIssues.CommentPolicy == nil &&
		(gi.TriggerComment != "" || len(gi.ExcludeComments) > 0) {
		dst.When.GitHubIssues.CommentPolicy = &v1alpha2.GitHubCommentPolicy{
			TriggerComment:  gi.TriggerComment,
			ExcludeComments: gi.ExcludeComments,
		}
	}
	if pr := src.When.GitHubPullRequests; pr != nil && dst.When.GitHubPullRequests != nil &&
		dst.When.GitHubPullRequests.CommentPolicy == nil &&
		(pr.TriggerComment != "" || len(pr.ExcludeComments) > 0) {
		dst.When.GitHubPullRequests.CommentPolicy = &v1alpha2.GitHubCommentPolicy{
			TriggerComment:  pr.TriggerComment,
			ExcludeComments: pr.ExcludeComments,
		}
	}

	// bodyContains (substring) -> bodyPattern (regex), quoting the literal.
	if gw := src.When.GitHubWebhook; gw != nil && dst.When.GitHubWebhook != nil {
		for i := range gw.Filters {
			if i >= len(dst.When.GitHubWebhook.Filters) {
				break
			}
			if gw.Filters[i].BodyContains != "" && dst.When.GitHubWebhook.Filters[i].BodyPattern == "" {
				dst.When.GitHubWebhook.Filters[i].BodyPattern = regexp.QuoteMeta(gw.Filters[i].BodyContains)
			}
		}
	}
}
