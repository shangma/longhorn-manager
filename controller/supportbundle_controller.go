package controller

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/longhorn/longhorn-manager/datastore"
	longhorn "github.com/longhorn/longhorn-manager/k8s/pkg/apis/longhorn/v1beta1"
	lhinformers "github.com/longhorn/longhorn-manager/k8s/pkg/client/informers/externalversions/longhorn/v1beta1"
	"github.com/longhorn/longhorn-manager/manager"
	"github.com/longhorn/longhorn-manager/types"
	"github.com/longhorn/longhorn-manager/util"

	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/apimachinery/pkg/util/wait"
	clientset "k8s.io/client-go/kubernetes"
	v1core "k8s.io/client-go/kubernetes/typed/core/v1"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/tools/record"
	"k8s.io/kubernetes/pkg/controller"
)

type SupportBundleController struct {
	*baseController

	// which namespace controller is running with
	namespace string
	// use as the OwnerID of the controller
	controllerID string

	kubeClient    clientset.Interface
	eventRecorder record.EventRecorder

	ds *datastore.DataStore

	sStoreSynced cache.InformerSynced

	SupportBundleInitialized util.Cond

	ServiceAccount string
}

func NewSupportBundleController(
	logger logrus.FieldLogger,
	ds *datastore.DataStore,
	scheme *runtime.Scheme,
	supportBundleInformer lhinformers.SupportBundleInformer,
	kubeClient clientset.Interface,
	controllerID string,
	namespace string,
	serviceAccount string) *SupportBundleController {
	eventBroadcaster := record.NewBroadcaster()
	eventBroadcaster.StartLogging(logrus.Infof)
	// TODO: remove the wrapper when every clients have moved to use the clientset.
	eventBroadcaster.StartRecordingToSink(&v1core.EventSinkImpl{
		Interface: v1core.New(kubeClient.CoreV1().RESTClient()).Events(""),
	})

	sbc := &SupportBundleController{
		baseController: newBaseController("longhorn-support-bundle", logger),

		namespace:    namespace,
		controllerID: controllerID,

		ds: ds,

		kubeClient:    kubeClient,
		eventRecorder: eventBroadcaster.NewRecorder(scheme, corev1.EventSource{Component: "longhorn-support-bundle-controller"}),

		sStoreSynced:             supportBundleInformer.Informer().HasSynced,
		SupportBundleInitialized: "Initialized",
		ServiceAccount:           serviceAccount,
	}

	supportBundleInformer.Informer().AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc:    sbc.enqueueSupportBundle,
		UpdateFunc: func(old, cur interface{}) { sbc.enqueueSupportBundle(cur) },
		DeleteFunc: sbc.enqueueSupportBundle,
	})

	return sbc
}

func (sbc *SupportBundleController) enqueueSupportBundle(obj interface{}) {
	key, err := controller.KeyFunc(obj)
	if err != nil {
		utilruntime.HandleError(fmt.Errorf("couldn't get key for object %#v: %v", obj, err))
		return
	}

	sbc.queue.AddRateLimited(key)
}

func (sbc *SupportBundleController) Run(workers int, stopCh <-chan struct{}) {
	defer utilruntime.HandleCrash()
	defer sbc.queue.ShutDown()

	sbc.logger.Infof("Start Longhorn Support Bundle controller")
	defer sbc.logger.Infof("Shutting down Longhorn Support Bundle controller")

	if !cache.WaitForNamedCacheSync(sbc.name, stopCh, sbc.sStoreSynced) {
		return
	}
	for i := 0; i < workers; i++ {
		go wait.Until(sbc.worker, time.Second, stopCh)
	}
	<-stopCh
}

func (sbc *SupportBundleController) worker() {
	for sbc.processNextWorkItem() {
	}
}

func (sbc *SupportBundleController) processNextWorkItem() bool {
	key, quit := sbc.queue.Get()
	if quit {
		return false
	}
	defer sbc.queue.Done(key)
	err := sbc.syncHandler(key.(string))
	sbc.handleErr(err, key)
	return true
}

func (sbc *SupportBundleController) handleErr(err error, key interface{}) {
	if err == nil {
		sbc.queue.Forget(key)
		return
	}

	if sbc.queue.NumRequeues(key) < maxRetries {
		sbc.logger.WithError(err).Warnf("Error syncing Longhorn Support Bundle %v", key)
		sbc.queue.AddRateLimited(key)
		return
	}

	utilruntime.HandleError(err)
	sbc.logger.WithError(err).Warnf("Dropping Longhorn Support Bundle %v out of the queue", key)
	sbc.queue.Forget(key)
}

func (sbc *SupportBundleController) syncHandler(key string) (err error) {
	defer func() {
		err = errors.Wrapf(err, "%v: fail to sync support-bundle %v", sbc.name, key)
	}()

	namespace, name, err := cache.SplitMetaNamespaceKey(key)
	if err != nil {
		return err
	}
	if namespace != sbc.namespace {
		return nil
	}
	return sbc.reconcile(name)
}

func getLoggerForSupportBundle(logger logrus.FieldLogger, supportBundle *longhorn.SupportBundle) *logrus.Entry {
	return logger.WithFields(
		logrus.Fields{
			"support-bundle": supportBundle.Name,
		},
	)
}

func (sbc *SupportBundleController) reconcile(supportBundleName string) (err error) {
	// Get SupportBundle CR
	sb, err := sbc.ds.GetSupportBundle(supportBundleName)
	if err != nil {
		if !apierrors.IsNotFound(err) {
			return err
		}
		return nil
	}

	if sb == nil || sb.DeletionTimestamp != nil {
		return nil
	}

	log := getLoggerForSupportBundle(sbc.logger, sb)
	log.Debugf("owner version %s, owner kind %s", sb.APIVersion, sb.Kind)
	switch sb.Status.State {
	case types.SupportBundleStateNone:
		log.Debugf("[%s] generating a support bundle", sb.Name)

		supportBundleImage, err := sbc.ds.GetSetting(types.SettingNameSupportBundleImage)
		if err != nil {
			return err
		}

		err = sbc.Create(sb, supportBundleImage.Value)
		toUpdate := sb.DeepCopy()
		if err != nil {
			sbc.setError(toUpdate, fmt.Sprintf("fail to create manager for %s: %s", sb.Name, err))
			return err
		}

		sbc.setState(toUpdate, types.SupportBundleStateInProgress)
		_, err = sbc.ds.UpdateSupportBundle(toUpdate)
		return err
	case types.SupportBundleStateInProgress:
		logrus.Debugf("[%s] support bundle is being generated", sb.Name)
		_, err = sbc.checkManagerStatus(sb)
		return err
	default:
		log.Debugf("[%s] noop for state %s", sb.Name, sb.Status.State)
		return nil
	}

}

func (sbc *SupportBundleController) getManagerName(supportBundle *longhorn.SupportBundle) string {
	return fmt.Sprintf("supportbundle-manager-%s", supportBundle.Name)
}

func (sbc *SupportBundleController) getImagePullPolicy() corev1.PullPolicy {

	imagePullPolicy, err := sbc.ds.GetSetting(types.SettingNameSystemManagedPodsImagePullPolicy)
	if err != nil {
		return corev1.PullIfNotPresent
	}

	switch strings.ToLower(imagePullPolicy.Value) {
	case "always":
		return corev1.PullAlways
	case "if-not-present":
		return corev1.PullIfNotPresent
	case "never":
		return corev1.PullNever
	default:
		return corev1.PullIfNotPresent
	}
}

func (sbc *SupportBundleController) checkManagerStatus(sb *longhorn.SupportBundle) (*longhorn.SupportBundle, error) {
	if time.Now().After(sb.CreationTimestamp.Add(types.SupportBundleCreationTimeout)) {
		return sbc.setError(sb, "fail to generate support bundle: timeout")
	}

	managerStatus, err := sbc.GetStatus(sb)
	if err != nil {
		logrus.Debugf("[%s] manager pod is not ready: %s", sb.Name, err)
		sbc.enqueueSupportBundle(sb)
		return sb, nil
	}

	if managerStatus.Error {
		return sbc.setError(sb, managerStatus.ErrorMessage)
	}

	switch managerStatus.Phase {
	case string(types.SupportBundleStateReadyForDownload):
		sb.Spec.ProgressPercentage = types.SupportBundleProgressPercentageTotal
		sb.Status.State = types.SupportBundleStateReadyForDownload
		return sbc.setReady(sb, managerStatus.Filename, managerStatus.Filesize)
	default:
		sb.Spec.ProgressPercentage = types.SupportBundleProgressPercentageYaml
		sb.Status.State = types.SupportBundleStateInProgress
		return sbc.setProgress(sb, int(types.SupportBundleProgressPercentageYaml))
	}
}

func (sbc *SupportBundleController) Create(sb *longhorn.SupportBundle, image string) error {

	log := getLoggerForSupportBundle(sbc.logger, sb)

	deployName := sbc.getManagerName(sb)
	log.Debugf("creating deployment %s with image %s", deployName, image)

	pullPolicy := sbc.getImagePullPolicy()

	deployment := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      deployName,
			Namespace: sb.Namespace,
			Labels: map[string]string{
				"app":                       types.SupportBundleManager,
				types.SupportBundleLabelKey: sb.Name,
			},
			OwnerReferences: []metav1.OwnerReference{
				{
					Name:       sb.Name,
					Kind:       sb.Kind,
					UID:        sb.UID,
					APIVersion: sb.APIVersion,
				},
			},
		},
		Spec: appsv1.DeploymentSpec{
			Selector: &metav1.LabelSelector{
				MatchLabels: map[string]string{"app": types.SupportBundleManager},
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{
						"app":                       types.SupportBundleManager,
						types.SupportBundleLabelKey: sb.Name,
					},
				},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{
							Name:            "manager",
							Image:           image,
							Args:            []string{"/usr/bin/support-bundle-utils", "manager"},
							ImagePullPolicy: pullPolicy,
							Env: []corev1.EnvVar{
								{
									Name:  "LONGHORN_NAMESPACE",
									Value: sb.Namespace,
								},
								{
									Name:  "LONGHORN_VERSION",
									Value: manager.FriendlyVersion(),
								},
								{
									Name:  "LONGHORN_SUPPORT_BUNDLE_NAME",
									Value: sb.Name,
								},
								{
									Name:  "LONGHORN_SUPPORT_BUNDLE_DEBUG",
									Value: "true",
								},
								{
									Name: "LONGHORN_SUPPORT_BUNDLE_MANAGER_POD_IP",
									ValueFrom: &corev1.EnvVarSource{
										FieldRef: &corev1.ObjectFieldSelector{
											FieldPath: "status.podIP",
										},
									},
								},
								{
									Name:  "LONGHORN_SUPPORT_BUNDLE_IMAGE",
									Value: image,
								},
								{
									Name:  "LONGHORN_SUPPORT_BUNDLE_IMAGE_PULL_POLICY",
									Value: string(pullPolicy),
								},
							},
							Ports: []corev1.ContainerPort{
								{
									ContainerPort: 8080,
								},
							},
						},
					},
					ServiceAccountName: sbc.ServiceAccount,
				},
			},
		},
	}

	_, err := sbc.ds.CreateDeployment(deployment)
	return err
}

func (sbc *SupportBundleController) setError(sb *longhorn.SupportBundle, reason string) (*longhorn.SupportBundle, error) {
	log := getLoggerForSupportBundle(sbc.logger, sb)
	log.Errorf(reason)

	toUpdate := sb.DeepCopy()
	sbc.SupportBundleInitialized.False(toUpdate)
	sbc.SupportBundleInitialized.Message(toUpdate, reason)

	toUpdate.Status.State = types.SupportBundleStateError
	return sbc.ds.UpdateSupportBundle(toUpdate)
}

func (sbc *SupportBundleController) setState(sb *longhorn.SupportBundle, state types.SuppportBundleState) (*longhorn.SupportBundle, error) {
	log := getLoggerForSupportBundle(sbc.logger, sb)
	log.Debugf("[%s] set state to %s", sb.Name, state)

	toUpdate := sb.DeepCopy()
	toUpdate.Status.State = state
	return sbc.ds.UpdateSupportBundle(toUpdate)
}

func (sbc *SupportBundleController) setReady(sb *longhorn.SupportBundle, filename string, filesize int64) (*longhorn.SupportBundle, error) {
	logrus.Debugf("[%s] set state to %s", sb.Name, types.SupportBundleStateReadyForDownload)
	toUpdate := sb.DeepCopy()
	sbc.SupportBundleInitialized.True(toUpdate)
	toUpdate.Status.State = types.SupportBundleStateReadyForDownload
	toUpdate.Status.Progress = 100
	toUpdate.Status.FileName = filename
	toUpdate.Status.FileSize = filesize
	return sbc.ds.UpdateSupportBundle(toUpdate)
}

func (sbc *SupportBundleController) setProgress(sb *longhorn.SupportBundle, progress int) (*longhorn.SupportBundle, error) {
	logrus.Debugf("[%s] set progress to %d", sb.Name, progress)
	toUpdate := sb.DeepCopy()
	toUpdate.Status.Progress = progress
	return sbc.ds.UpdateSupportBundle(toUpdate)
}

func (sbc *SupportBundleController) GetStatus(sb *longhorn.SupportBundle) (*ManagerStatus, error) {
	podIP, err := manager.GetManagerPodIP(sbc.ds)
	if err != nil {
		return nil, err
	}

	url := fmt.Sprintf("http://%s:8080/status", podIP)
	httpClient := http.Client{
		Timeout: 10 * time.Second,
	}

	resp, err := httpClient.Get(url)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}

	s := &ManagerStatus{}
	err = json.NewDecoder(resp.Body).Decode(s)
	if err != nil {
		return nil, err
	}

	return s, nil
}

type ManagerStatus struct {
	// phase to collect bundle
	Phase string

	// fail to collect bundle
	Error bool

	// error message
	ErrorMessage string

	// progress of the bundle collecting. 0 - 100.
	Progress int

	// bundle filename
	Filename string

	// bundle filesize
	Filesize int64
}
