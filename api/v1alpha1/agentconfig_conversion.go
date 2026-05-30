package v1alpha1

import (
	"sort"

	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/conversion"

	v1alpha2 "github.com/kelos-dev/kelos/api/v1alpha2"
)

// ConvertTo converts this v1alpha1 AgentConfig (spoke) to the v1alpha2 hub.
//
// The only shape difference is MCPServerSpec.Env: v1alpha1 stores a
// map[string]string, v1alpha2 a []corev1.EnvVar. The map converts losslessly
// to a list of literal Name/Value entries.
func (src *AgentConfig) ConvertTo(dstRaw conversion.Hub) error {
	dst := dstRaw.(*v1alpha2.AgentConfig)

	dst.ObjectMeta = src.ObjectMeta
	dst.Spec.AgentsMD = src.Spec.AgentsMD
	dst.Spec.Plugins = pluginsToV1alpha2(src.Spec.Plugins)
	dst.Spec.Skills = skillsToV1alpha2(src.Spec.Skills)
	dst.Spec.MCPServers = mcpServersToV1alpha2(src.Spec.MCPServers)
	return nil
}

// ConvertFrom converts the v1alpha2 hub into this v1alpha1 AgentConfig (spoke).
//
// MCPServerSpec.Env is downgraded from []corev1.EnvVar to map[string]string:
// literal Value entries become map keys, while entries that use ValueFrom have
// no representation in a map and are dropped (best-effort). v1alpha2 is the
// storage version and the source of truth, so this loss only affects an
// explicit v1alpha1 read.
func (dst *AgentConfig) ConvertFrom(srcRaw conversion.Hub) error {
	src := srcRaw.(*v1alpha2.AgentConfig)

	dst.ObjectMeta = src.ObjectMeta
	dst.Spec.AgentsMD = src.Spec.AgentsMD
	dst.Spec.Plugins = pluginsFromV1alpha2(src.Spec.Plugins)
	dst.Spec.Skills = skillsFromV1alpha2(src.Spec.Skills)
	dst.Spec.MCPServers = mcpServersFromV1alpha2(src.Spec.MCPServers)
	return nil
}

func mcpServersToV1alpha2(in []MCPServerSpec) []v1alpha2.MCPServerSpec {
	if in == nil {
		return nil
	}
	out := make([]v1alpha2.MCPServerSpec, len(in))
	for i, s := range in {
		out[i] = v1alpha2.MCPServerSpec{
			Name:        s.Name,
			Type:        s.Type,
			Command:     s.Command,
			Args:        copyStrings(s.Args),
			URL:         s.URL,
			Headers:     copyStringMap(s.Headers),
			HeadersFrom: secretValuesSourceToV1alpha2(s.HeadersFrom),
			Env:         envMapToList(s.Env),
			EnvFrom:     secretValuesSourceToV1alpha2(s.EnvFrom),
		}
	}
	return out
}

func mcpServersFromV1alpha2(in []v1alpha2.MCPServerSpec) []MCPServerSpec {
	if in == nil {
		return nil
	}
	out := make([]MCPServerSpec, len(in))
	for i, s := range in {
		out[i] = MCPServerSpec{
			Name:        s.Name,
			Type:        s.Type,
			Command:     s.Command,
			Args:        copyStrings(s.Args),
			URL:         s.URL,
			Headers:     copyStringMap(s.Headers),
			HeadersFrom: secretValuesSourceFromV1alpha2(s.HeadersFrom),
			Env:         envListToMap(s.Env),
			EnvFrom:     secretValuesSourceFromV1alpha2(s.EnvFrom),
		}
	}
	return out
}

// envMapToList converts the v1alpha1 env map into a sorted list of literal
// EnvVar entries so the result is deterministic.
func envMapToList(m map[string]string) []corev1.EnvVar {
	if len(m) == 0 {
		return nil
	}
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	out := make([]corev1.EnvVar, 0, len(keys))
	for _, k := range keys {
		out = append(out, corev1.EnvVar{Name: k, Value: m[k]})
	}
	return out
}

// envListToMap downgrades the v1alpha2 env list to a v1alpha1 map. Entries that
// reference a value via ValueFrom cannot be represented in a map and are
// dropped.
func envListToMap(list []corev1.EnvVar) map[string]string {
	if len(list) == 0 {
		return nil
	}
	out := make(map[string]string, len(list))
	for _, e := range list {
		if e.ValueFrom != nil {
			continue
		}
		out[e.Name] = e.Value
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func secretValuesSourceToV1alpha2(s *SecretValuesSource) *v1alpha2.SecretValuesSource {
	if s == nil {
		return nil
	}
	return &v1alpha2.SecretValuesSource{
		SecretRef: v1alpha2.SecretReference{Name: s.SecretRef.Name},
	}
}

func secretValuesSourceFromV1alpha2(s *v1alpha2.SecretValuesSource) *SecretValuesSource {
	if s == nil {
		return nil
	}
	return &SecretValuesSource{
		SecretRef: SecretReference{Name: s.SecretRef.Name},
	}
}

func pluginsToV1alpha2(in []PluginSpec) []v1alpha2.PluginSpec {
	if in == nil {
		return nil
	}
	out := make([]v1alpha2.PluginSpec, len(in))
	for i, p := range in {
		out[i] = v1alpha2.PluginSpec{
			Name:   p.Name,
			Skills: skillDefsToV1alpha2(p.Skills),
			Agents: agentDefsToV1alpha2(p.Agents),
		}
	}
	return out
}

func pluginsFromV1alpha2(in []v1alpha2.PluginSpec) []PluginSpec {
	if in == nil {
		return nil
	}
	out := make([]PluginSpec, len(in))
	for i, p := range in {
		out[i] = PluginSpec{
			Name:   p.Name,
			Skills: skillDefsFromV1alpha2(p.Skills),
			Agents: agentDefsFromV1alpha2(p.Agents),
		}
	}
	return out
}

func skillDefsToV1alpha2(in []SkillDefinition) []v1alpha2.SkillDefinition {
	if in == nil {
		return nil
	}
	out := make([]v1alpha2.SkillDefinition, len(in))
	for i, s := range in {
		out[i] = v1alpha2.SkillDefinition{Name: s.Name, Content: s.Content}
	}
	return out
}

func skillDefsFromV1alpha2(in []v1alpha2.SkillDefinition) []SkillDefinition {
	if in == nil {
		return nil
	}
	out := make([]SkillDefinition, len(in))
	for i, s := range in {
		out[i] = SkillDefinition{Name: s.Name, Content: s.Content}
	}
	return out
}

func agentDefsToV1alpha2(in []AgentDefinition) []v1alpha2.AgentDefinition {
	if in == nil {
		return nil
	}
	out := make([]v1alpha2.AgentDefinition, len(in))
	for i, a := range in {
		out[i] = v1alpha2.AgentDefinition{Name: a.Name, Content: a.Content}
	}
	return out
}

func agentDefsFromV1alpha2(in []v1alpha2.AgentDefinition) []AgentDefinition {
	if in == nil {
		return nil
	}
	out := make([]AgentDefinition, len(in))
	for i, a := range in {
		out[i] = AgentDefinition{Name: a.Name, Content: a.Content}
	}
	return out
}

func skillsToV1alpha2(in []SkillsShSpec) []v1alpha2.SkillsShSpec {
	if in == nil {
		return nil
	}
	out := make([]v1alpha2.SkillsShSpec, len(in))
	for i, s := range in {
		out[i] = v1alpha2.SkillsShSpec{Source: s.Source, Skill: s.Skill}
	}
	return out
}

func skillsFromV1alpha2(in []v1alpha2.SkillsShSpec) []SkillsShSpec {
	if in == nil {
		return nil
	}
	out := make([]SkillsShSpec, len(in))
	for i, s := range in {
		out[i] = SkillsShSpec{Source: s.Source, Skill: s.Skill}
	}
	return out
}

func copyStrings(in []string) []string {
	if in == nil {
		return nil
	}
	out := make([]string, len(in))
	copy(out, in)
	return out
}

func copyStringMap(in map[string]string) map[string]string {
	if in == nil {
		return nil
	}
	out := make(map[string]string, len(in))
	for k, v := range in {
		out[k] = v
	}
	return out
}
