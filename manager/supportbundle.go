package manager

import (
	"fmt"
	"github.com/pkg/errors"
	"github.com/rancher/go-rancher/client"
	"github.com/sirupsen/logrus"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/labels"
	"strings"

	"github.com/longhorn/longhorn-manager/datastore"
	longhorn "github.com/longhorn/longhorn-manager/k8s/pkg/apis/longhorn/v1beta1"
	"github.com/longhorn/longhorn-manager/types"
	"github.com/longhorn/longhorn-manager/util"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type SupportBundle struct {
	client.Resource
	NodeID             string                    `json:"nodeID"`
	State              types.SuppportBundleState `json:"state"`
	Name               string                    `json:"name"`
	ErrorMessage       types.SupportBundleError  `json:"errorMessage"`
	ProgressPercentage int                       `json:"progressPercentage"`
}

type BundleMeta struct {
	LonghornVersion       string `json:"longhornVersion"`
	KubernetesVersion     string `json:"kubernetesVersion"`
	LonghornNamespaceUUID string `json:"longhornNamspaceUUID"`
	BundleCreatedAt       string `json:"bundleCreatedAt"`
	IssueURL              string `json:"issueURL"`
	IssueDescription      string `json:"issueDescription"`
}

func (m *VolumeManager) GetSupportBundle(name string) (*longhorn.SupportBundle, error) {
	return m.ds.GetSupportBundle(name)
}

func (m *VolumeManager) DeleteSupportBundle(name string) error {
	return m.ds.DeleteSupportBundle(name)
}

func (m *VolumeManager) InitSupportBundle(issueURL string, description string) (*longhorn.SupportBundle, error) {
	sb, err := m.GenerateSupportBundle(issueURL, description)
	if err != nil {
		return nil, err
	}
	return sb, nil
}

func (m *VolumeManager) GetManagerPodIP() (string, error) {
	ip, err := GetManagerPodIP(m.ds)
	if err != nil {
		return "", err
	}
	return ip, nil
}

func (m *VolumeManager) GenerateSupportBundle(issueURL string, description string) (*longhorn.SupportBundle, error) {
	namespace, err := m.ds.GetLonghornNamespace()
	if err != nil {
		return nil, errors.Wrap(err, "cannot get longhorn namespace")
	}
	kubeVersion, err := m.ds.GetKubernetesVersion()
	if err != nil {
		return nil, errors.Wrap(err, "cannot get kubernetes version")
	}

	bundleMeta := &BundleMeta{
		LonghornVersion:       VERSION,
		KubernetesVersion:     kubeVersion.GitVersion,
		LonghornNamespaceUUID: string(namespace.UID),
		BundleCreatedAt:       util.Now(),
		IssueURL:              issueURL,
		IssueDescription:      description,
	}

	bundleName := "longhorn-support-bundle_" + bundleMeta.LonghornNamespaceUUID + "_" +
		strings.Replace(bundleMeta.BundleCreatedAt, ":", "-", -1)
	bundleFileName := bundleName + ".zip"

	sb, err := m.CreateSupportBundle(&types.SupportBundleSpec{
		Name:        bundleName,
		FileName:    bundleFileName,
		State:       types.SupportBundleStateNone,
		IssueURL:    issueURL,
		Description: description,
	})
	if err != nil {
		return nil, err
	}
	return sb, nil
}

func (m *VolumeManager) CreateSupportBundle(spec *types.SupportBundleSpec) (*longhorn.SupportBundle, error) {
	name := util.AutoCorrectName(spec.Name, datastore.NameMaximumLength)
	if !util.ValidateName(name) {
		return nil, fmt.Errorf("invalid name %v", name)
	}

	supportBundle := &longhorn.SupportBundle{
		ObjectMeta: metav1.ObjectMeta{
			Name: name,
		},
		Spec: *spec,
	}

	supportBundle, err := m.ds.CreateSupportBundle(supportBundle)
	if err != nil {
		return nil, err
	}
	logrus.Infof("Created support bundle %v", name)
	return supportBundle, nil
}

func ListPods(ds *datastore.DataStore, selector labels.Selector) ([]*corev1.Pod, error) {
	return ds.ListPodsBySelector(selector)
}

func (m *VolumeManager) GetManagerNodeIPMap() (map[string]string, error) {
	podList, err := m.ds.ListManagerPods()
	if err != nil {
		return nil, err
	}

	nodeIPMap := map[string]string{}
	for _, pod := range podList {
		if nodeIPMap[pod.Spec.NodeName] != "" {
			return nil, fmt.Errorf("multiple managers on the node %v", pod.Spec.NodeName)
		}
		nodeIPMap[pod.Spec.NodeName] = pod.Status.PodIP
	}
	return nodeIPMap, nil
}

func GetManagerPodIP(ds *datastore.DataStore) (string, error) {
	sets := labels.Set{
		"app": types.SupportBundleManager,
	}

	pods, err := ListPods(ds, sets.AsSelector())
	if err != nil {
		return "", err
	}

	if len(pods) != 1 {
		return "", errors.New("more than one manager pods are found")
	}
	return pods[0].Status.PodIP, nil
}
