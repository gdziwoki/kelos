package v1alpha1

import (
	"reflect"
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	v1alpha2 "github.com/kelos-dev/kelos/api/v1alpha2"
)

func TestTaskConvert_IdentityRoundTrip(t *testing.T) {
	orig := &Task{
		ObjectMeta: metav1.ObjectMeta{Name: "t", Namespace: "ns"},
		Spec: TaskSpec{
			Type:   "claude-code",
			Prompt: "do the thing",
			Credentials: Credentials{
				Type:      CredentialTypeAPIKey,
				SecretRef: &SecretReference{Name: "creds"},
			},
			Branch: "kelos-task-1",
			PodOverrides: &PodOverrides{
				Env: []corev1.EnvVar{{Name: "FOO", Value: "bar"}},
			},
		},
		Status: TaskStatus{Phase: TaskPhaseRunning, JobName: "job-1"},
	}

	hub := &v1alpha2.Task{}
	if err := orig.ConvertTo(hub); err != nil {
		t.Fatalf("ConvertTo() error = %v", err)
	}
	back := &Task{}
	if err := back.ConvertFrom(hub); err != nil {
		t.Fatalf("ConvertFrom() error = %v", err)
	}
	if !reflect.DeepEqual(orig.Spec, back.Spec) {
		t.Errorf("spec round-trip mismatch:\n orig=%#v\n back=%#v", orig.Spec, back.Spec)
	}
	if !reflect.DeepEqual(orig.Status, back.Status) {
		t.Errorf("status round-trip mismatch:\n orig=%#v\n back=%#v", orig.Status, back.Status)
	}
}

func TestWorkspaceConvert_IdentityRoundTrip(t *testing.T) {
	orig := &Workspace{
		ObjectMeta: metav1.ObjectMeta{Name: "w", Namespace: "ns"},
		Spec: WorkspaceSpec{
			Repo:      "https://github.com/o/r",
			Ref:       "main",
			SecretRef: &SecretReference{Name: "gh"},
		},
	}
	hub := &v1alpha2.Workspace{}
	if err := orig.ConvertTo(hub); err != nil {
		t.Fatalf("ConvertTo() error = %v", err)
	}
	back := &Workspace{}
	if err := back.ConvertFrom(hub); err != nil {
		t.Fatalf("ConvertFrom() error = %v", err)
	}
	if !reflect.DeepEqual(orig.Spec, back.Spec) {
		t.Errorf("spec round-trip mismatch:\n orig=%#v\n back=%#v", orig.Spec, back.Spec)
	}
}
