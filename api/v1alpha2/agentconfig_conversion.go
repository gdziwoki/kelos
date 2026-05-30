package v1alpha2

// Hub marks AgentConfig as the conversion hub (storage version). Other versions
// convert to and from this type.
func (*AgentConfig) Hub() {}
