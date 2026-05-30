package v1alpha1

import (
	"sigs.k8s.io/controller-runtime/pkg/conversion"

	v1alpha2 "github.com/kelos-dev/kelos/api/v1alpha2"
)

// Workspace v1alpha2 is an identity mirror of v1alpha1 (no status), so
// conversion is a structural copy of Spec.

func (src *Workspace) ConvertTo(dstRaw conversion.Hub) error {
	dst := dstRaw.(*v1alpha2.Workspace)
	dst.ObjectMeta = src.ObjectMeta
	return convertViaJSON(&src.Spec, &dst.Spec)
}

func (dst *Workspace) ConvertFrom(srcRaw conversion.Hub) error {
	src := srcRaw.(*v1alpha2.Workspace)
	dst.ObjectMeta = src.ObjectMeta
	return convertViaJSON(&src.Spec, &dst.Spec)
}
