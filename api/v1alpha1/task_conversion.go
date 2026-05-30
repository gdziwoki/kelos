package v1alpha1

import (
	"sigs.k8s.io/controller-runtime/pkg/conversion"

	v1alpha2 "github.com/kelos-dev/kelos/api/v1alpha2"
)

// Task v1alpha2 is an identity mirror of v1alpha1, so conversion is a
// structural copy of Spec and Status.

func (src *Task) ConvertTo(dstRaw conversion.Hub) error {
	dst := dstRaw.(*v1alpha2.Task)
	dst.ObjectMeta = src.ObjectMeta
	if err := convertViaJSON(&src.Spec, &dst.Spec); err != nil {
		return err
	}
	return convertViaJSON(&src.Status, &dst.Status)
}

func (dst *Task) ConvertFrom(srcRaw conversion.Hub) error {
	src := srcRaw.(*v1alpha2.Task)
	dst.ObjectMeta = src.ObjectMeta
	if err := convertViaJSON(&src.Spec, &dst.Spec); err != nil {
		return err
	}
	return convertViaJSON(&src.Status, &dst.Status)
}
