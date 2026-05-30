package integration

import (
	"encoding/base64"
	"fmt"
	"net"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"

	kelosv1alpha1 "github.com/kelos-dev/kelos/api/v1alpha1"
	kelosv1alpha2 "github.com/kelos-dev/kelos/api/v1alpha2"
)

// pointConversionToEnvtest rewrites the agentconfigs CRD conversion to the local
// webhook envtest serves, so conversion works regardless of any service-based
// config left by the install tests.
func pointConversionToEnvtest() {
	crdGVK := schema.GroupVersionKind{Group: "apiextensions.k8s.io", Version: "v1", Kind: "CustomResourceDefinition"}
	crd := &unstructured.Unstructured{}
	crd.SetGroupVersionKind(crdGVK)
	Expect(k8sClient.Get(ctx, types.NamespacedName{Name: "agentconfigs.kelos.dev"}, crd)).To(Succeed())

	url := fmt.Sprintf("https://%s/convert", net.JoinHostPort(webhookHost, fmt.Sprintf("%d", webhookPort)))
	conversion := map[string]interface{}{
		"strategy": "Webhook",
		"webhook": map[string]interface{}{
			"conversionReviewVersions": []interface{}{"v1"},
			"clientConfig": map[string]interface{}{
				"url":      url,
				"caBundle": base64.StdEncoding.EncodeToString(webhookCA),
			},
		},
	}
	Expect(unstructured.SetNestedMap(crd.Object, conversion, "spec", "conversion")).To(Succeed())
	Expect(k8sClient.Update(ctx, crd)).To(Succeed())
}

var _ = Describe("AgentConfig conversion webhook", Ordered, func() {
	BeforeAll(func() {
		pointConversionToEnvtest()
	})

	AfterAll(func() {
		// Remove the AgentConfig objects created here so later install/uninstall
		// specs that delete the agentconfigs CRD are not left with instances.
		for _, ns := range []string{"test-conv-up", "test-conv-down"} {
			_ = k8sClient.Delete(ctx, &kelosv1alpha2.AgentConfig{
				ObjectMeta: metav1.ObjectMeta{Name: "cfg", Namespace: ns},
			})
		}
	})

	It("converts a v1alpha1 map env up to a v1alpha2 list", func() {
		ns := &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "test-conv-up"}}
		Expect(k8sClient.Create(ctx, ns)).To(Succeed())

		By("Creating the AgentConfig in v1alpha1 with a map env")
		v1 := &kelosv1alpha1.AgentConfig{
			ObjectMeta: metav1.ObjectMeta{Name: "cfg", Namespace: ns.Name},
			Spec: kelosv1alpha1.AgentConfigSpec{
				MCPServers: []kelosv1alpha1.MCPServerSpec{
					{
						Name: "local",
						Type: "stdio",
						Env:  map[string]string{"B": "2", "A": "1"},
					},
				},
			},
		}
		Expect(k8sClient.Create(ctx, v1)).To(Succeed())

		By("Reading it back as v1alpha2 and asserting the env is a sorted list")
		got := &kelosv1alpha2.AgentConfig{}
		Expect(k8sClient.Get(ctx, types.NamespacedName{Name: "cfg", Namespace: ns.Name}, got)).To(Succeed())
		Expect(got.Spec.MCPServers).To(HaveLen(1))
		Expect(got.Spec.MCPServers[0].Env).To(Equal([]corev1.EnvVar{
			{Name: "A", Value: "1"},
			{Name: "B", Value: "2"},
		}))
	})

	It("drops valueFrom when a v1alpha2 list is read down as a v1alpha1 map", func() {
		ns := &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "test-conv-down"}}
		Expect(k8sClient.Create(ctx, ns)).To(Succeed())

		By("Creating the AgentConfig in v1alpha2 with a literal and a valueFrom env")
		v2 := &kelosv1alpha2.AgentConfig{
			ObjectMeta: metav1.ObjectMeta{Name: "cfg", Namespace: ns.Name},
			Spec: kelosv1alpha2.AgentConfigSpec{
				MCPServers: []kelosv1alpha2.MCPServerSpec{
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
		Expect(k8sClient.Create(ctx, v2)).To(Succeed())

		By("Reading it back as v1alpha1 and asserting valueFrom was dropped")
		got := &kelosv1alpha1.AgentConfig{}
		Expect(k8sClient.Get(ctx, types.NamespacedName{Name: "cfg", Namespace: ns.Name}, got)).To(Succeed())
		Expect(got.Spec.MCPServers).To(HaveLen(1))
		Expect(got.Spec.MCPServers[0].Env).To(Equal(map[string]string{"LITERAL": "x"}))
	})
})
