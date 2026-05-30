package v1alpha1

import (
	"reflect"
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	v1alpha2 "github.com/kelos-dev/kelos/api/v1alpha2"
)

func TestConvertTo_EnvMapToSortedList(t *testing.T) {
	src := &AgentConfig{
		ObjectMeta: metav1.ObjectMeta{Name: "cfg", Namespace: "default"},
		Spec: AgentConfigSpec{
			MCPServers: []MCPServerSpec{
				{
					Name: "local",
					Type: "stdio",
					Env:  map[string]string{"B": "2", "A": "1"},
				},
			},
		},
	}

	dst := &v1alpha2.AgentConfig{}
	if err := src.ConvertTo(dst); err != nil {
		t.Fatalf("ConvertTo() error = %v", err)
	}

	want := []corev1.EnvVar{{Name: "A", Value: "1"}, {Name: "B", Value: "2"}}
	if !reflect.DeepEqual(dst.Spec.MCPServers[0].Env, want) {
		t.Errorf("Env = %#v, want %#v (sorted)", dst.Spec.MCPServers[0].Env, want)
	}
	if dst.Name != "cfg" || dst.Namespace != "default" {
		t.Errorf("ObjectMeta not copied: %q/%q", dst.Namespace, dst.Name)
	}
}

func TestConvertFrom_EnvListToMap_DropsValueFrom(t *testing.T) {
	src := &v1alpha2.AgentConfig{
		ObjectMeta: metav1.ObjectMeta{Name: "cfg", Namespace: "default"},
		Spec: v1alpha2.AgentConfigSpec{
			MCPServers: []v1alpha2.MCPServerSpec{
				{
					Name: "local",
					Type: "stdio",
					Env: []corev1.EnvVar{
						{Name: "LITERAL", Value: "x"},
						{Name: "FROM_SECRET", ValueFrom: &corev1.EnvVarSource{
							SecretKeyRef: &corev1.SecretKeySelector{
								LocalObjectReference: corev1.LocalObjectReference{Name: "s"},
								Key:                  "k",
							},
						}},
					},
				},
			},
		},
	}

	dst := &AgentConfig{}
	if err := dst.ConvertFrom(src); err != nil {
		t.Fatalf("ConvertFrom() error = %v", err)
	}

	got := dst.Spec.MCPServers[0].Env
	want := map[string]string{"LITERAL": "x"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("Env = %#v, want %#v (valueFrom entry dropped)", got, want)
	}
}

// TestConvertFrom_EnvAllValueFrom_NilMap ensures an env list consisting only of
// valueFrom entries downgrades to a nil map rather than an empty one.
func TestConvertFrom_EnvAllValueFrom_NilMap(t *testing.T) {
	src := &v1alpha2.AgentConfig{
		Spec: v1alpha2.AgentConfigSpec{
			MCPServers: []v1alpha2.MCPServerSpec{
				{
					Name: "local",
					Type: "stdio",
					Env: []corev1.EnvVar{
						{Name: "FROM_SECRET", ValueFrom: &corev1.EnvVarSource{
							SecretKeyRef: &corev1.SecretKeySelector{
								LocalObjectReference: corev1.LocalObjectReference{Name: "s"},
								Key:                  "k",
							},
						}},
					},
				},
			},
		},
	}

	dst := &AgentConfig{}
	if err := dst.ConvertFrom(src); err != nil {
		t.Fatalf("ConvertFrom() error = %v", err)
	}
	if dst.Spec.MCPServers[0].Env != nil {
		t.Errorf("Env = %#v, want nil", dst.Spec.MCPServers[0].Env)
	}
}

func TestConvertRoundTrip_PreservesV1alpha1(t *testing.T) {
	orig := &AgentConfig{
		ObjectMeta: metav1.ObjectMeta{Name: "cfg", Namespace: "ns"},
		Spec: AgentConfigSpec{
			AgentsMD: "hello",
			Plugins: []PluginSpec{
				{
					Name:   "p1",
					Skills: []SkillDefinition{{Name: "s1", Content: "c1"}},
					Agents: []AgentDefinition{{Name: "a1", Content: "ac1"}},
				},
			},
			Skills: []SkillsShSpec{{Source: "owner/repo", Skill: "thing"}},
			MCPServers: []MCPServerSpec{
				{
					Name:        "local",
					Type:        "stdio",
					Command:     "npx",
					Args:        []string{"-y", "pkg"},
					Env:         map[string]string{"A": "1", "B": "2"},
					EnvFrom:     &SecretValuesSource{SecretRef: SecretReference{Name: "bulk"}},
					Headers:     map[string]string{"X": "y"},
					HeadersFrom: &SecretValuesSource{SecretRef: SecretReference{Name: "hdr"}},
					URL:         "https://example.com",
				},
			},
		},
	}

	hub := &v1alpha2.AgentConfig{}
	if err := orig.ConvertTo(hub); err != nil {
		t.Fatalf("ConvertTo() error = %v", err)
	}
	got := &AgentConfig{}
	if err := got.ConvertFrom(hub); err != nil {
		t.Fatalf("ConvertFrom() error = %v", err)
	}

	if !reflect.DeepEqual(orig.Spec, got.Spec) {
		t.Errorf("round-trip mismatch:\n orig = %#v\n got  = %#v", orig.Spec, got.Spec)
	}
}

// TestConvertTo_OptionalFlagSurvivesViaListOnly is a sanity check that the
// v1alpha2 list carries optional valueFrom intact (it has no v1alpha1
// representation, so it only matters on the v1alpha2 side).
func TestConvertTo_PreservesNonEnvFields(t *testing.T) {
	src := &AgentConfig{
		Spec: AgentConfigSpec{
			MCPServers: []MCPServerSpec{
				{
					Name:        "local",
					Type:        "sse",
					URL:         "https://example.com",
					Headers:     map[string]string{"A": "b"},
					HeadersFrom: &SecretValuesSource{SecretRef: SecretReference{Name: "hdr"}},
					EnvFrom:     &SecretValuesSource{SecretRef: SecretReference{Name: "bulk"}},
				},
			},
		},
	}
	dst := &v1alpha2.AgentConfig{}
	if err := src.ConvertTo(dst); err != nil {
		t.Fatalf("ConvertTo() error = %v", err)
	}
	s := dst.Spec.MCPServers[0]
	if s.URL != "https://example.com" || s.Type != "sse" {
		t.Errorf("scalar fields not copied: %#v", s)
	}
	if s.HeadersFrom == nil || s.HeadersFrom.SecretRef.Name != "hdr" {
		t.Errorf("HeadersFrom not copied: %#v", s.HeadersFrom)
	}
	if s.EnvFrom == nil || s.EnvFrom.SecretRef.Name != "bulk" {
		t.Errorf("EnvFrom not copied: %#v", s.EnvFrom)
	}
}
