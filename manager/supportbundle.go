package manager

import (
	longhorn "github.com/longhorn/longhorn-manager/k8s/pkg/apis/longhorn/v1beta1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/labels"
)

func (m *VolumeManager) GetSupportBundle(name string) (*longhorn.SupportBundle, error) {
	return m.ds.GetSupportBundle(name)
}

func (m *VolumeManager) DeleteSupportBundle(name string) error {
	return m.ds.DeleteSupportBundle(name)
}

func (m *VolumeManager) ListPods(selector labels.Selector) ([]*corev1.Pod, error) {
	return m.ds.ListPodsBySelector(selector)
}
